// Command ridgelined is the Ridgeline daemon: it ingests MeshCore packets
// from MQTT, decodes them into SQLite, and serves the REST API, WebSocket
// live feed, and the built web UI.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jjkroell/ridgeline/internal/analytics"
	"github.com/jjkroell/ridgeline/internal/api"
	"github.com/jjkroell/ridgeline/internal/config"
	"github.com/jjkroell/ridgeline/internal/ingest"
	"github.com/jjkroell/ridgeline/internal/mail"
	"github.com/jjkroell/ridgeline/internal/store"
)

var version = "dev" // set via -ldflags at build time

func main() {
	configPath := flag.String("config", "config.json", "path to config file")
	showVersion := flag.Bool("version", false, "print version and exit")
	healthcheck := flag.Bool("healthcheck", false, "probe the local /api/health endpoint and exit 0 if healthy (for container HEALTHCHECK)")
	flag.Parse()

	if *showVersion {
		fmt.Println("ridgelined", version)
		return
	}
	if *healthcheck {
		os.Exit(healthProbe(*configPath))
	}

	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if err := run(log, *configPath); err != nil {
		log.Error("fatal", "err", err)
		os.Exit(1)
	}
}

// healthProbe hits the daemon's own /api/health and returns a process exit code
// (0 = healthy, 1 = not). It's what the container HEALTHCHECK runs: the distroless
// runtime image has no shell or curl, so the binary probes itself. The port comes
// from the config's listenAddr (defaulting to 8080), dialed on loopback.
func healthProbe(configPath string) int {
	port := "8080"
	if cfg, err := config.Load(configPath); err == nil && cfg.ListenAddr != "" {
		if _, p, err := net.SplitHostPort(cfg.ListenAddr); err == nil && p != "" {
			port = p
		}
	}
	client := http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get("http://127.0.0.1:" + port + "/api/health")
	if err != nil {
		fmt.Fprintln(os.Stderr, "healthcheck:", err)
		return 1
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		fmt.Fprintln(os.Stderr, "healthcheck: unhealthy status", resp.StatusCode)
		return 1
	}
	return 0
}

func run(log *slog.Logger, configPath string) error {
	cfg, err := config.Load(configPath)
	if err != nil {
		// Config file is optional in dev; fall back to defaults.
		log.Warn("using default config", "reason", err)
		cfg = config.Default()
	}
	log.Info("ridgelined starting", "version", version, "db", cfg.DBPath, "broker", cfg.MQTT.Broker)

	st, err := store.Open(cfg.DBPath)
	if err != nil {
		return err
	}
	defer st.Close()

	// One-time seed of advert_tx_count (actual transmissions) from history when
	// the column was just added, before ingest starts touching it.
	if st.NeedsAdvertTxBackfill() {
		if n, txs, err := analytics.BackfillAdvertTx(st); err != nil {
			log.Warn("advert-tx backfill", "err", err)
		} else {
			log.Info("advert-tx backfill complete", "nodes", n, "transmissions", txs)
		}
	}

	apiServer := api.New(st, log, version, cfg.WebDir)
	apiServer.SetEnvironment(cfg.Environment)

	// Outbound transactional email (verification + note notifications). Disabled
	// gracefully when no relay is configured.
	apiServer.SetMailer(mail.New(cfg.Email, log))

	// Per-node analytics snapshot, recomputed periodically over a rolling window.
	engine := analytics.New(6)
	apiServer.SetAnalytics(engine)

	in := ingest.New(cfg.MQTT, st, log)
	in.OnObservation = apiServer.Broadcast
	if err := in.Start(); err != nil {
		// Don't abort the whole daemon if the broker is briefly unavailable;
		// paho retries the connection in the background.
		log.Warn("mqtt connect pending", "err", err)
	}
	defer in.Stop()

	srv := &http.Server{Addr: cfg.ListenAddr, Handler: apiServer.Handler()}

	go func() {
		log.Info("listening", "addr", cfg.ListenAddr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("http server exited", "err", err)
		}
	}()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go runAnalytics(ctx, engine, st, log)
	go runHashSizeConsensus(ctx, st, log)
	go runRetention(ctx, st, log)
	go runSessionPrune(ctx, st, log)
	go runClaimPrune(ctx, st, log)
	if cfg.NodeRetentionDays > 0 {
		go runNodeRetention(ctx, st, engine, cfg.NodeRetentionDays, log)
	}
	if cfg.ObserverRetentionMinutes > 0 {
		go runObserverRetention(ctx, st, cfg.ObserverRetentionMinutes, log)
	}
	if cfg.ScrubArtifacts {
		go runArtifactScrub(ctx, st, log)
	}

	<-ctx.Done()

	log.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}

// runAnalytics recomputes the per-node analytics snapshot immediately, then
// every 90s, until ctx is cancelled.
func runAnalytics(ctx context.Context, engine *analytics.Engine, st *store.Store, log *slog.Logger) {
	recompute := func() {
		nodes, err := st.ListNodes()
		if err != nil {
			log.Warn("analytics: list nodes", "err", err)
			return
		}
		if err := engine.Recompute(st, nodes); err != nil {
			log.Warn("analytics: recompute", "err", err)
		}
	}
	recompute()
	t := time.NewTicker(90 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			recompute()
		}
	}
}

// hashSizeConsensusWindow is how far back the hash-size vote looks. The vote is
// per-transmission, and nodes advert only about once a day (~30h observed), so
// the window must span several cadences to gather enough independent broadcasts
// for a confident majority — one corrupt transmission must be a clear minority.
// A week gives ~5 transmissions for a typical node while a quiet node simply
// stays untouched until it has spoken enough.
const hashSizeConsensusWindow = 7 * 24 * time.Hour

// hashSizeConsensusInterval is how often the vote re-runs. Frequent enough to
// repair a misread size promptly, cheap enough to scan the window each time.
const hashSizeConsensusInterval = 6 * time.Hour

// runHashSizeConsensus periodically repairs each node's stored hash size by
// majority vote over its recent adverts. Ingest sets hash_size only while it's
// unknown (a corrupt path-length byte can no longer flip an established size),
// but the very first advert seen for a node could itself be corrupt — so this
// pass owns corrections, voting over the window and overriding the stored value
// when a clear winner disagrees with it. Runs shortly after startup, then on a
// fixed interval.
func runHashSizeConsensus(ctx context.Context, st *store.Store, log *slog.Logger) {
	reconcile := func() {
		cutoff := time.Now().Add(-hashSizeConsensusWindow).UTC().Format(time.RFC3339Nano)
		consensus, err := analytics.ConsensusHashSizes(st, cutoff)
		if err != nil {
			log.Warn("hash-size consensus: compute", "err", err)
			return
		}
		nodes, err := st.ListNodes()
		if err != nil {
			log.Warn("hash-size consensus: list nodes", "err", err)
			return
		}
		current := make(map[string]int, len(nodes))
		for _, n := range nodes {
			current[n.PublicKey] = n.HashSize
		}
		// Only write where the verdict differs from what's stored (also fills an
		// unknown 0). Keeps the update small and the log meaningful.
		corrections := map[string]int{}
		for pk, size := range consensus {
			if cur, ok := current[pk]; ok && cur != size {
				corrections[pk] = size
			}
		}
		if len(corrections) == 0 {
			return
		}
		if err := st.SetHashSizes(corrections); err != nil {
			log.Warn("hash-size consensus: update", "err", err)
			return
		}
		log.Info("hash-size consensus: corrected node hash sizes", "nodes", len(corrections))
	}

	// Let ingest settle so the window holds recent adverts before the first vote.
	select {
	case <-ctx.Done():
		return
	case <-time.After(2 * time.Minute):
	}
	reconcile()
	t := time.NewTicker(hashSizeConsensusInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			reconcile()
		}
	}
}

// runSessionPrune deletes expired login sessions immediately, then daily, until
// ctx is cancelled. Sessions are also validated (and expired ones dropped) on
// use; this just keeps the table from accumulating stale rows.
func runSessionPrune(ctx context.Context, st *store.Store, log *slog.Logger) {
	prune := func() {
		now := time.Now().UTC().Format(time.RFC3339Nano)
		if n, err := st.PruneSessions(now); err != nil {
			log.Warn("session prune", "err", err)
		} else if n > 0 {
			log.Info("session prune: removed expired sessions", "rows", n)
		}
	}
	prune()
	t := time.NewTicker(24 * time.Hour)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			prune()
		}
	}
}

// runClaimPrune deletes expired pending node-ownership claims immediately, then
// hourly, until ctx is cancelled. Verified claims are kept; only unfulfilled
// codes past their expiry are cleared.
func runClaimPrune(ctx context.Context, st *store.Store, log *slog.Logger) {
	prune := func() {
		if n, err := st.PruneExpiredClaims(); err != nil {
			log.Warn("claim prune", "err", err)
		} else if n > 0 {
			log.Info("claim prune: removed expired pending claims", "rows", n)
		}
		if n, err := st.PruneExpiredEmailVerifications(); err != nil {
			log.Warn("email-verification prune", "err", err)
		} else if n > 0 {
			log.Info("email-verification prune: removed expired tokens", "rows", n)
		}
		if n, err := st.PruneExpiredPasswordResets(); err != nil {
			log.Warn("password-reset prune", "err", err)
		} else if n > 0 {
			log.Info("password-reset prune: removed expired tokens", "rows", n)
		}
	}
	prune()
	t := time.NewTicker(time.Hour)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			prune()
		}
	}
}

// telemetryRetention is how long observer telemetry samples are kept. Two weeks
// leaves room for week-over-week comparison while bounding table growth (~12
// observers × one sample / 5 min ≈ a few thousand rows/day).
const telemetryRetention = 14 * 24 * time.Hour

// runRetention prunes aged observer-telemetry samples immediately, then daily,
// until ctx is cancelled.
func runRetention(ctx context.Context, st *store.Store, log *slog.Logger) {
	prune := func() {
		before := time.Now().Add(-telemetryRetention).UTC().Format(time.RFC3339Nano)
		n, err := st.PruneTelemetry(before)
		if err != nil {
			log.Warn("retention: prune telemetry", "err", err)
			return
		}
		if n > 0 {
			log.Info("retention: pruned telemetry", "rows", n)
		}
	}
	prune()
	t := time.NewTicker(24 * time.Hour)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			prune()
		}
	}
}

// runNodeRetention removes nodes that have gone silent past the retention
// threshold — no advert for retentionDays, and not currently relaying (the
// analytics liveness snapshot guards still-active relays whose advert is stale).
// A pruned node's row and stored adverts go; it reappears the moment it
// transmits again, so this only clears genuinely-departed nodes. Like the
// artifact sweep it waits for ingest/analytics to settle, then runs daily.
func runNodeRetention(ctx context.Context, st *store.Store, engine *analytics.Engine, retentionDays int, log *slog.Logger) {
	prune := func() {
		nodes, err := st.ListNodes()
		if err != nil {
			log.Warn("node retention: list nodes", "err", err)
			return
		}
		cutoff := time.Now().Add(-time.Duration(retentionDays) * 24 * time.Hour).UTC().Format(time.RFC3339Nano)
		keys := analytics.StaleNodeKeys(nodes, engine.Liveness(), cutoff)
		if len(keys) == 0 {
			return
		}
		// A node with no advert in the window may still have relayed traffic. The
		// liveness guard above only reaches the short analytics window, so scan the
		// full retention window for relay hops and drop any candidate that relayed.
		relayHops, err := st.RelayHopPrefixesSince(cutoff)
		if err != nil {
			log.Warn("node retention: relay scan", "err", err)
			return
		}
		keys = analytics.FilterRelayedWithin(keys, nodes, relayHops)
		if len(keys) == 0 {
			return
		}
		res, err := st.PurgeTargets(nil, nil, keys)
		if err != nil {
			log.Warn("node retention: purge", "err", err)
			return
		}
		log.Info("node retention: removed silent nodes",
			"thresholdDays", retentionDays, "candidates", len(keys),
			"nodesDeleted", res.Nodes, "observationsDeleted", res.Observations)
	}

	// Let ingest/analytics settle so liveness reflects current relays before the
	// first sweep (a cold snapshot would under-protect active relays).
	select {
	case <-ctx.Done():
		return
	case <-time.After(3 * time.Minute):
	}
	prune()
	t := time.NewTicker(24 * time.Hour)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			prune()
		}
	}
}

// observerSweepInterval is how often the stale-observer sweep runs. It's checked
// frequently relative to the (minutes-scale) retention threshold so a silent
// observer is cleared shortly after crossing it.
const observerSweepInterval = 5 * time.Minute

// runObserverRetention periodically removes observer rows that have gone silent
// for longer than retentionMinutes. Only the observers row is deleted; the
// packets it reported are kept, and the observer reappears if it publishes again.
func runObserverRetention(ctx context.Context, st *store.Store, retentionMinutes int, log *slog.Logger) {
	sweep := func() {
		cutoff := time.Now().Add(-time.Duration(retentionMinutes) * time.Minute).UTC().Format(time.RFC3339Nano)
		ids, err := st.DeleteStaleObservers(cutoff)
		if err != nil {
			log.Warn("observer retention: sweep", "err", err)
			return
		}
		if len(ids) > 0 {
			log.Info("observer retention: removed silent observers",
				"thresholdMinutes", retentionMinutes, "count", len(ids), "observers", ids)
		}
	}

	// A short initial delay lets a fresh start ingest current traffic (and any
	// retained status messages) before the first sweep, so live observers aren't
	// briefly seen as stale.
	select {
	case <-ctx.Done():
		return
	case <-time.After(2 * time.Minute):
	}
	sweep()
	t := time.NewTicker(observerSweepInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			sweep()
		}
	}
}

// artifactScrubInterval is how often the corruption-artifact sweep runs. Daily is
// plenty — artifacts are harmless until removed, and only high-confidence
// (provably corrupt) records are ever deleted.
const artifactScrubInterval = 24 * time.Hour

// runArtifactScrub periodically removes high-confidence packet-corruption
// artifacts — phantom node records whose public key arrived with bytes flipped,
// the same records surfaced in the Hash-IDs UI. It deletes the node row + its
// observations with NO blocklist entry: the exact corrupt key is random and
// unlikely to recur, and if it does the next sweep catches it. Runs a couple of
// minutes after startup, then every 24h.
func runArtifactScrub(ctx context.Context, st *store.Store, log *slog.Logger) {
	scrub := func() {
		nodes, err := st.ListNodes()
		if err != nil {
			log.Warn("artifact scrub: list nodes", "err", err)
			return
		}
		keys := analytics.HighConfidenceArtifactKeys(nodes)
		if len(keys) == 0 {
			return
		}
		// A claimed node is not a corruption artifact, whatever the heuristic
		// scores it — see store.PartitionClaimed.
		keys, skipped, err := st.PartitionClaimed(keys)
		if err != nil {
			log.Warn("artifact scrub: claim check", "err", err)
			return
		}
		if len(skipped) > 0 {
			log.Info("artifact scrub: skipped claimed nodes", "count", len(skipped), "nodes", skipped)
		}
		if len(keys) == 0 {
			return
		}
		res, err := st.PurgeTargets(nil, nil, keys)
		if err != nil {
			log.Warn("artifact scrub: purge", "err", err)
			return
		}
		log.Info("artifact scrub: removed corruption artifacts",
			"candidates", len(keys), "nodesDeleted", res.Nodes, "observationsDeleted", res.Observations)
	}

	// Let ingest/analytics settle before the first sweep.
	select {
	case <-ctx.Done():
		return
	case <-time.After(2 * time.Minute):
	}
	scrub()
	t := time.NewTicker(artifactScrubInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			scrub()
		}
	}
}

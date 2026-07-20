// Package ingest connects to a MeshCore observer MQTT broker, decodes incoming
// packets, and persists them via the store.
package ingest

import (
	"encoding/json"
	"log/slog"
	"strconv"
	"strings"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"

	"github.com/jjkroell/ridgeline/internal/config"
	"github.com/jjkroell/ridgeline/internal/meshcore"
	"github.com/jjkroell/ridgeline/internal/store"
)

// envelope is the JSON wrapper an observer publishes for each received packet.
// Field names mirror the MeshCore observer convention. Note that real
// observers encode SNR/RSSI as JSON strings (e.g. "11", "-45"), so those use
// a lenient numeric type that accepts both strings and numbers.
type envelope struct {
	Raw      string   `json:"raw"`
	SNR      optFloat `json:"SNR"`
	RSSI     optFloat `json:"RSSI"`
	Origin   string   `json:"origin"`    // friendly observer name
	OriginID string   `json:"origin_id"` // observer public key
	Region   string   `json:"region"`
}

// optFloat is a float that unmarshals from a JSON number or a quoted numeric
// string, tracking whether a usable value was present.
type optFloat struct {
	set bool
	val float64
}

func (o *optFloat) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	if s == "" || s == "null" {
		return nil
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return nil // tolerate non-numeric values rather than dropping the packet
	}
	o.set, o.val = true, v
	return nil
}

func (o optFloat) ptr() *float64 {
	if !o.set {
		return nil
	}
	v := o.val
	return &v
}

// Ingestor subscribes to the broker and writes decoded packets to the store.
type Ingestor struct {
	cfg   config.MQTT
	store *store.Store
	log   *slog.Logger

	// OnObservation, if set, is called for each successfully stored
	// observation — used to feed the live WebSocket.
	OnObservation func(store.Observation)

	client mqtt.Client
}

// New creates an Ingestor.
func New(cfg config.MQTT, st *store.Store, log *slog.Logger) *Ingestor {
	return &Ingestor{cfg: cfg, store: st, log: log}
}

// Start connects to the broker and subscribes. It returns once the
// subscriptions are established; the client reconnects automatically on drop.
func (in *Ingestor) Start() error {
	opts := mqtt.NewClientOptions().
		AddBroker(in.cfg.Broker).
		SetClientID(in.cfg.ClientID).
		SetAutoReconnect(true).
		SetConnectRetry(true).
		SetCleanSession(true)
	if in.cfg.Username != "" {
		opts.SetUsername(in.cfg.Username)
		opts.SetPassword(in.cfg.Password)
	}
	opts.SetOnConnectHandler(func(c mqtt.Client) {
		for _, topic := range in.cfg.Topics {
			if tok := c.Subscribe(topic, 0, in.handle); tok.Wait() && tok.Error() != nil {
				in.log.Error("subscribe failed", "topic", topic, "err", tok.Error())
			} else {
				in.log.Info("subscribed", "topic", topic)
			}
		}
	})

	in.client = mqtt.NewClient(opts)
	tok := in.client.Connect()
	// With ConnectRetry + AutoReconnect, paho keeps (re)connecting in the
	// background. Don't block the whole daemon — the HTTP server, /api/health and
	// the web UI — on the broker being reachable at startup. Wait a short bounded
	// time for a fast connect (so the common case still logs "subscribed" before
	// serving), then proceed regardless; the connection completes/retries later.
	if !tok.WaitTimeout(10 * time.Second) {
		in.log.Warn("mqtt connect still pending; retrying in background", "broker", in.cfg.Broker)
		return nil
	}
	return tok.Error()
}

// Stop disconnects from the broker.
func (in *Ingestor) Stop() {
	if in.client != nil {
		in.client.Disconnect(250)
	}
}

func (in *Ingestor) handle(_ mqtt.Client, msg mqtt.Message) {
	// This callback runs in a paho goroutine on untrusted, attacker-influenceable
	// payloads (raw packet hex, observer status). An unrecovered panic here would
	// be fatal to the whole daemon, so contain it: log and drop the one message.
	defer func() {
		if rec := recover(); rec != nil {
			in.log.Error("recovered from panic in ingest handler", "topic", msg.Topic(), "panic", rec)
		}
	}()

	if strings.HasSuffix(msg.Topic(), "/status") {
		in.handleStatus(msg)
		return
	}
	var env envelope
	if err := json.Unmarshal(msg.Payload(), &env); err != nil {
		in.log.Debug("bad envelope", "topic", msg.Topic(), "err", err)
		return
	}
	if env.Raw == "" {
		return
	}

	packet, err := meshcore.DecodeHex(env.Raw)
	if err != nil {
		in.log.Debug("decode error", "err", err)
		return
	}

	observerKey, region := topicMeta(msg.Topic())
	if region == "" {
		region = env.Region
	}
	// Prefer the friendly observer name for display; fall back to the
	// topic-derived public key.
	observerID := env.Origin
	if observerID == "" {
		observerID = observerKey
	}

	// Drop blocklisted traffic (quarantined RF bridges / rogue MQTT publishers)
	// before it ever reaches the store.
	if in.store.ShouldDrop(packet, observerID) {
		in.log.Debug("dropped blocklisted packet", "observer", observerID)
		return
	}

	obs := store.Observation{
		Packet:         packet,
		RawHex:         env.Raw,
		ObserverID:     observerID,
		ObserverPubkey: env.OriginID,
		Region:         region,
		SNR:            env.SNR.ptr(),
		RSSI:           env.RSSI.ptr(),
		// Server clock owns ordering; the envelope timestamp is untrusted
		// (observers with skewed clocks would poison ordering).
		ReceivedAt: time.Now(),
	}
	if err := in.store.Record(obs); err != nil {
		in.log.Error("store record failed", "err", err)
		return
	}
	// Node-ownership claim verification: a signature-valid advert whose node has
	// a pending claim may carry the verification code in its name. The signature
	// check is essential — it proves the advert came from the node's own key, so
	// a rogue observer can't forge a claim by injecting the code. Gated by the
	// in-memory pending-claim set, so the overwhelming common case costs nothing.
	if a := packet.Advert; a != nil && a.SignatureValid && in.store.HasPendingClaim(a.PublicKey) {
		if verified, err := in.store.VerifyPendingClaims(a.PublicKey, a.Name); err != nil {
			in.log.Warn("claim verification failed", "node", a.PublicKey, "err", err)
		} else {
			for _, v := range verified {
				in.log.Info("node ownership claim verified", "node", a.PublicKey, "user", v.UserID)
			}
		}
	}
	if in.OnObservation != nil {
		in.OnObservation(obs)
	}
}

// statusEnvelope is the JSON an observer publishes on its /status topic: device
// identity, radio config ("freq,bw,sf,cr") and a stats block. Field names mirror
// the real MeshCore observer status messages.
type statusEnvelope struct {
	Status          string `json:"status"`
	Origin          string `json:"origin"`
	OriginID        string `json:"origin_id"`
	Region          string `json:"region"`
	Radio           string `json:"radio"`
	Model           string `json:"model"`
	FirmwareVersion string `json:"firmware_version"`
	ClientVersion   string `json:"client_version"`
	Stats           struct {
		BatteryMV  *int     `json:"battery_mv"`
		UptimeSecs *int64   `json:"uptime_secs"`
		NoiseFloor *float64 `json:"noise_floor"`
		TxAirSecs  *float64 `json:"tx_air_secs"`
		RxAirSecs  *float64 `json:"rx_air_secs"`
		RecvErrors *int     `json:"recv_errors"`
		Errors     *int     `json:"errors"` // some clients use "errors"
		QueueLen   *int     `json:"queue_len"`
	} `json:"stats"`
}

// handleStatus parses an observer /status message and stores its latest device
// telemetry. The observer is keyed by its friendly origin name (matching the
// packet path), so status attaches to the same observer row.
func (in *Ingestor) handleStatus(msg mqtt.Message) {
	var env statusEnvelope
	if err := json.Unmarshal(msg.Payload(), &env); err != nil {
		in.log.Debug("bad status envelope", "topic", msg.Topic(), "err", err)
		return
	}
	observerKey, region := topicMeta(msg.Topic())
	if region == "" {
		region = env.Region
	}
	observerID := env.Origin
	if observerID == "" {
		observerID = observerKey
	}
	if observerID == "" {
		return
	}

	st := store.ObserverStatus{
		State:         env.Status,
		Radio:         env.Radio,
		Model:         env.Model,
		Firmware:      env.FirmwareVersion,
		ClientVersion: env.ClientVersion,
		BatteryMV:     env.Stats.BatteryMV,
		UptimeSecs:    env.Stats.UptimeSecs,
		NoiseFloor:    env.Stats.NoiseFloor,
		TxAirSecs:     env.Stats.TxAirSecs,
		RxAirSecs:     env.Stats.RxAirSecs,
		RecvErrors:    firstNonNil(env.Stats.RecvErrors, env.Stats.Errors),
		QueueLen:      env.Stats.QueueLen,
	}
	parseRadio(env.Radio, &st)

	b, err := json.Marshal(st)
	if err != nil {
		return
	}
	pubkey := env.OriginID
	if pubkey == "" {
		pubkey = observerKey
	}
	now := time.Now().UTC().Format(time.RFC3339)
	// A retained status is the broker replaying an observer's last known value on
	// every reconnect, not a live sighting — it may refresh an observer we already
	// know, but it must never create one, or decommissioned observers reappear.
	// Returning early also skips the telemetry append below: stamping a replayed
	// battery/noise reading with the reconnect time invents a data point that was
	// never measured, one per reconnect, for as long as the retained message lives.
	if msg.Retained() {
		found, err := in.store.UpdateObserverStatusIfPresent(observerID, region, pubkey, string(b), env.Radio, now)
		if err != nil {
			in.log.Error("update observer status failed", "err", err)
		} else if !found {
			in.log.Debug("ignored retained status for unknown observer", "observer", observerID)
		}
		return
	}
	if err := in.store.UpsertObserverStatus(observerID, region, pubkey, string(b), env.Radio, now); err != nil {
		in.log.Error("store observer status failed", "err", err)
	}
	// Append a point to the telemetry time series (rate-floored in the store) so
	// battery/noise/airtime can be trended — the observer row only keeps the latest.
	if err := in.store.RecordObserverTelemetry(observerID, now, st); err != nil {
		in.log.Error("store observer telemetry failed", "err", err)
	}
}

// parseRadio splits the "freq,bw,sf,cr" radio string into typed fields.
func parseRadio(radio string, st *store.ObserverStatus) {
	parts := strings.Split(strings.TrimSpace(radio), ",")
	if len(parts) >= 1 {
		if f, err := strconv.ParseFloat(parts[0], 64); err == nil {
			st.FreqMHz = &f
		}
	}
	if len(parts) >= 2 {
		if f, err := strconv.ParseFloat(parts[1], 64); err == nil {
			st.BandwidthKHz = &f
		}
	}
	if len(parts) >= 3 {
		if n, err := strconv.Atoi(strings.TrimSpace(parts[2])); err == nil {
			st.SpreadingFactor = &n
		}
	}
	if len(parts) >= 4 {
		if n, err := strconv.Atoi(strings.TrimSpace(parts[3])); err == nil {
			st.CodingRate = &n
		}
	}
}

func firstNonNil(a, b *int) *int {
	if a != nil {
		return a
	}
	return b
}

// topicMeta extracts region and observer id from a meshcore/{region}/{observer}/packets
// topic. Missing segments yield empty strings.
func topicMeta(topic string) (observerID, region string) {
	parts := strings.Split(topic, "/")
	if len(parts) >= 2 {
		region = parts[1]
	}
	if len(parts) >= 3 {
		observerID = parts[2]
	}
	return observerID, region
}

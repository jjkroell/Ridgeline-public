// Package api serves ridgelined's REST endpoints and the live WebSocket feed
// over the store.
package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"

	"github.com/jjkroell/ridgeline/internal/analytics"
	"github.com/jjkroell/ridgeline/internal/meshcore"
	"github.com/jjkroell/ridgeline/internal/store"
)

// mailSender is the subset of the mailer the API uses. An interface (rather than
// the concrete *mail.Mailer) so tests can inject a capturing fake and never send
// real email.
type mailSender interface {
	Enabled() bool
	BaseURL() string
	SendAsync(kind, to, subject, text, html string)
}

// Server holds API dependencies and serves HTTP.
type Server struct {
	store     *store.Store
	log       *slog.Logger
	version   string
	webDir    string
	hub       *hub
	up        websocket.Upgrader
	analytics *analytics.Engine
	keyChal   *keyChallengeStore // pending private-key ownership challenges
	mail      mailSender         // outbound transactional email (nil/disabled ok)
}

// SetAnalytics attaches the analytics engine used by the node-detail endpoint.
func (s *Server) SetAnalytics(e *analytics.Engine) { s.analytics = e }

// SetMailer attaches the outbound mailer. When nil or disabled, email features
// (verification, note notifications) degrade to no-ops.
func (s *Server) SetMailer(m mailSender) { s.mail = m }

// mailEnabled reports whether outbound email is configured.
func (s *Server) mailEnabled() bool { return s.mail != nil && s.mail.Enabled() }

// New creates an API Server. If webDir is non-empty and exists, the built SPA
// is served from it with an index.html fallback for client routes. The admin
// console is gated by the is_admin account flag (session auth), not a token.
func New(st *store.Store, log *slog.Logger, version, webDir string) *Server {
	return &Server{
		store:   st,
		log:     log,
		version: version,
		webDir:  webDir,
		hub:     newHub(),
		// Dev: allow any origin. Tighten before exposing publicly.
		up:      websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }},
		keyChal: newKeyChallengeStore(),
	}
}

// Handler returns the configured HTTP handler.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", s.health)
	mux.HandleFunc("GET /api/stats", s.stats)
	mux.HandleFunc("GET /api/nodes", s.nodes)
	mux.HandleFunc("GET /api/nodes/{pubkey}", s.nodeDetail)
	mux.HandleFunc("GET /api/nodes/{pubkey}/history", s.nodeHistory)
	mux.HandleFunc("GET /api/nodes/{pubkey}/observers", s.nodeObservers)
	mux.HandleFunc("GET /api/nodes/{pubkey}/heatmap", s.nodeHeatmap)
	mux.HandleFunc("GET /api/nodes/{pubkey}/claim", s.nodeClaimStatus)
	mux.HandleFunc("GET /api/nodes/{pubkey}/notes", s.nodeNotes)
	mux.HandleFunc("GET /api/mesh-analytics", s.meshAnalytics)
	mux.HandleFunc("GET /api/observers", s.observers)
	mux.HandleFunc("GET /api/observers/{id}/analytics", s.observerAnalytics)
	mux.HandleFunc("GET /api/observers/{id}/telemetry", s.observerTelemetry)
	mux.HandleFunc("GET /api/observations", s.observations)
	mux.HandleFunc("GET /api/recent", s.recent)
	mux.HandleFunc("GET /api/packets/{hash}", s.packet)
	mux.HandleFunc("GET /api/channels/recent", s.channelsRecent)
	mux.HandleFunc("GET /api/live", s.live)

	// Accounts: registration, login/logout, current-user probe. Mutating auth
	// endpoints are same-origin POSTs; CSRF is enforced on authenticated
	// mutations elsewhere (requireUser) once a session exists.
	mux.HandleFunc("POST /api/auth/register", s.authRegister)
	mux.HandleFunc("POST /api/auth/login", s.authLogin)
	mux.HandleFunc("POST /api/auth/logout", s.authLogout)
	mux.HandleFunc("GET /api/auth/me", s.authMe)
	mux.HandleFunc("POST /api/auth/verify", s.authVerifyEmail)
	mux.HandleFunc("POST /api/auth/resend-verification", s.authResendVerification)
	// Self-service account editing (authenticated + CSRF via requireUser).
	mux.HandleFunc("PUT /api/account/profile", s.requireUser(s.accountUpdateProfile))
	mux.HandleFunc("POST /api/account/password", s.requireUser(s.accountChangePassword))
	mux.HandleFunc("POST /api/account/email", s.requireUser(s.accountChangeEmail))
	mux.HandleFunc("POST /api/account/delete", s.requireUser(s.accountDelete))

	// Node ownership claims (authenticated; creating requires the can_claim gate).
	mux.HandleFunc("POST /api/claims", s.requireUser(s.claimCreate))
	mux.HandleFunc("GET /api/claims/mine", s.requireUser(s.claimsMine))
	mux.HandleFunc("DELETE /api/claims/{pubkey}", s.requireUser(s.claimDelete))
	// Alternative ownership proof: sign a server challenge with the node's private key.
	mux.HandleFunc("POST /api/nodes/{pubkey}/claim/key-challenge", s.requireUser(s.claimKeyChallenge))
	mux.HandleFunc("POST /api/nodes/{pubkey}/claim/key-verify", s.requireUser(s.claimKeyVerify))

	// Node private exact location (owner-only, all methods). Kept entirely
	// separate from the public node data — never joined into /api/nodes or WS.
	mux.HandleFunc("GET /api/nodes/{pubkey}/private-location", s.requireUser(s.privateLocationGet))
	mux.HandleFunc("PUT /api/nodes/{pubkey}/private-location", s.requireUser(s.privateLocationSet))
	mux.HandleFunc("DELETE /api/nodes/{pubkey}/private-location", s.requireUser(s.privateLocationDelete))

	// Sharing a node's private location with specific registered users (owner-only).
	mux.HandleFunc("GET /api/nodes/{pubkey}/location-shares", s.requireUser(s.locationSharesList))
	mux.HandleFunc("POST /api/nodes/{pubkey}/location-shares", s.requireUser(s.locationShareCreate))
	mux.HandleFunc("DELETE /api/nodes/{pubkey}/location-shares/{userId}", s.requireUser(s.locationShareDelete))

	// User autocomplete for the share picker (signed-in; returns id + name only).
	mux.HandleFunc("GET /api/users/search", s.requireUser(s.usersSearch))

	// Grantee-facing: nodes shared with me + mark-them-seen (clears the badge).
	mux.HandleFunc("GET /api/shares/mine", s.requireUser(s.sharesMine))
	mux.HandleFunc("POST /api/shares/mark-seen", s.requireUser(s.sharesMarkSeen))

	// Node notes (public/private/team). Reading is public; writing needs a login.
	mux.HandleFunc("POST /api/nodes/{pubkey}/notes", s.requireUser(s.noteCreate))
	mux.HandleFunc("PATCH /api/notes/{id}", s.requireUser(s.noteUpdate))
	mux.HandleFunc("DELETE /api/notes/{id}", s.requireUser(s.noteDelete))

	// The admin console is one session-gated area for any is_admin account:
	// member administration + injection detection / quarantine / purge. (The old
	// static admin-token gate has been removed in favour of the account login.)
	mux.HandleFunc("GET /api/admin/users", s.requireAdminUser(s.adminListUsers))
	mux.HandleFunc("POST /api/admin/users/flags", s.requireAdminUser(s.adminSetUserFlags))
	mux.HandleFunc("POST /api/admin/users/block", s.requireAdminUser(s.adminBlockUser))
	mux.HandleFunc("POST /api/admin/users/delete", s.requireAdminUser(s.adminDeleteUser))
	mux.HandleFunc("GET /api/admin/detect", s.requireAdminUser(s.adminDetect))
	mux.HandleFunc("GET /api/admin/blocklist", s.requireAdminUser(s.adminBlocklist))
	mux.HandleFunc("POST /api/admin/block", s.requireAdminUser(s.adminBlock))
	mux.HandleFunc("DELETE /api/admin/block", s.requireAdminUser(s.adminUnblock))
	mux.HandleFunc("POST /api/admin/purge", s.requireAdminUser(s.adminPurge))
	mux.HandleFunc("POST /api/admin/delete", s.requireAdminUser(s.adminDelete))

	if s.webDir != "" {
		if info, err := os.Stat(s.webDir); err == nil && info.IsDir() {
			mux.HandleFunc("/", staticHandler(s.webDir))
			s.log.Info("serving web UI", "dir", s.webDir)
		} else {
			s.log.Warn("web dir not found, serving API only", "dir", s.webDir)
		}
	}
	return mux
}

// LiveEvent is the JSON shape broadcast to WebSocket clients per observation.
type LiveEvent struct {
	MessageHash    string `json:"messageHash"`
	RouteType      string `json:"routeType"`
	PayloadType    string `json:"payloadType"`
	PayloadVersion uint8  `json:"payloadVersion"`
	PathHops       int    `json:"pathHops"`
	HashSize       int    `json:"hashSize"`
	// Path holds the per-hop relay key prefixes (uppercase hex) the packet
	// accumulated as it flooded — the chain of repeaters that relayed it.
	Path           []string   `json:"path,omitempty"`
	TransportCodes *[2]uint16 `json:"transportCodes,omitempty"`
	PayloadRaw     string     `json:"payloadRaw,omitempty"`
	Raw            string     `json:"raw,omitempty"`
	// GroupText channel fields. ChannelHash is always set for GroupText; the
	// rest are populated only when the message decrypts (e.g. public channel).
	ChannelHash string   `json:"channelHash,omitempty"`
	Channel     string   `json:"channel,omitempty"`
	Sender      string   `json:"sender,omitempty"`
	Text        string   `json:"text,omitempty"`
	ObserverID  string   `json:"observerId,omitempty"`
	Region      string   `json:"region,omitempty"`
	SNR         *float64 `json:"snr,omitempty"`
	RSSI        *float64 `json:"rssi,omitempty"`
	ReceivedAt  string   `json:"receivedAt"`
	// Node is populated for Advert packets.
	Node *LiveNode `json:"node,omitempty"`
}

// LiveNode summarizes the node announced by an Advert.
type LiveNode struct {
	PublicKey string   `json:"publicKey"`
	Name      string   `json:"name"`
	Role      string   `json:"role"`
	Latitude  *float64 `json:"latitude,omitempty"`
	Longitude *float64 `json:"longitude,omitempty"`
	Timestamp uint32   `json:"timestamp,omitempty"` // advertised unix time
}

// newLiveEvent builds the JSON event shape from a decoded packet and its
// reception envelope. Shared by the live broadcast and the /api/recent replay
// so both render identically.
func newLiveEvent(pkt *meshcore.Packet, rawHex, observerID, region, receivedAt string, snr, rssi *float64) LiveEvent {
	ev := LiveEvent{
		MessageHash:    pkt.MessageHash,
		RouteType:      pkt.RouteType.String(),
		PayloadType:    pkt.PayloadType.String(),
		PayloadVersion: pkt.PayloadVersion,
		PathHops:       pkt.PathHopCount,
		HashSize:       pkt.PathHashSize,
		Path:           pkt.Path,
		TransportCodes: pkt.TransportCodes,
		PayloadRaw:     pkt.PayloadRaw,
		Raw:            strings.ToUpper(rawHex),
		ObserverID:     observerID,
		Region:         region,
		SNR:            snr,
		RSSI:           rssi,
		ReceivedAt:     receivedAt,
	}
	if a := pkt.Advert; a != nil {
		n := &LiveNode{
			PublicKey: a.PublicKey,
			Name:      a.Name,
			Role:      a.DeviceRole.String(),
			Timestamp: a.Timestamp,
		}
		if a.HasLocation {
			lat, lon := a.Latitude, a.Longitude
			n.Latitude, n.Longitude = &lat, &lon
		}
		ev.Node = n
	}
	if gt := pkt.GroupText; gt != nil {
		ev.ChannelHash = gt.ChannelHash
		if gt.Decrypted {
			ev.Channel = gt.Channel
			ev.Sender = gt.Sender
			ev.Text = gt.Message
		}
	}
	return ev
}

// Broadcast pushes an observation to live WebSocket subscribers.
func (s *Server) Broadcast(o store.Observation) {
	ev := newLiveEvent(o.Packet, o.RawHex, o.ObserverID, o.Region,
		o.ReceivedAt.UTC().Format(time.RFC3339Nano), o.SNR, o.RSSI)
	b, err := json.Marshal(ev)
	if err != nil {
		return
	}
	s.hub.broadcast(b)
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, map[string]string{"status": "ok", "version": s.version})
}

func (s *Server) stats(w http.ResponseWriter, _ *http.Request) {
	st, err := s.store.Stats()
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, st)
}

// nodeWithLiveness augments a stored node with its recent relay activity so the
// UI can compute a true "alive in the mesh" status (advert recency OR traffic).
type nodeWithLiveness struct {
	store.Node
	LastRelayed  string `json:"lastRelayed,omitempty"`
	RelayCount1h int    `json:"relayCount1h,omitempty"`
	// Claimed reports that the node has a verified owner (public "claimed" badge).
	Claimed bool `json:"claimed,omitempty"`
}

func (s *Server) nodes(w http.ResponseWriter, _ *http.Request) {
	nodes, err := s.store.ListNodes()
	if err != nil {
		s.fail(w, err)
		return
	}
	var live map[string]analytics.LiveSignal
	if s.analytics != nil {
		live = s.analytics.Liveness()
	}
	claimed, err := s.store.ClaimedNodeKeys()
	if err != nil {
		s.fail(w, err)
		return
	}
	out := make([]nodeWithLiveness, 0, len(nodes))
	for _, n := range nodes {
		if s.store.IsNodeBlocked(n.PublicKey) {
			continue // quarantined injected node — hidden from public views
		}
		nw := nodeWithLiveness{Node: n}
		if sig, ok := live[n.PublicKey]; ok {
			nw.LastRelayed = sig.LastRelayed
			nw.RelayCount1h = sig.RelayCount1h
		}
		nw.Claimed = claimed[strings.ToUpper(n.PublicKey)]
		out = append(out, nw)
	}
	writeJSON(w, out)
}

// nodeDetail returns one node's row plus its computed analytics snapshot.
func (s *Server) nodeDetail(w http.ResponseWriter, r *http.Request) {
	pubkey := strings.ToUpper(r.PathValue("pubkey"))
	if s.store.IsNodeBlocked(pubkey) {
		// Quarantined — return a marker (not the node) so the UI can show a
		// "suspected bridge" notice instead of node detail.
		writeJSON(w, struct {
			Quarantined bool              `json:"quarantined"`
			Block       *store.BlockEntry `json:"block,omitempty"`
		}{Quarantined: true, Block: s.store.NodeBlock(pubkey)})
		return
	}
	nodes, err := s.store.ListNodes()
	if err != nil {
		s.fail(w, err)
		return
	}
	var node *store.Node
	for i := range nodes {
		if strings.ToUpper(nodes[i].PublicKey) == pubkey {
			node = &nodes[i]
			break
		}
	}
	resp := struct {
		Node        *store.Node           `json:"node"`
		Detail      *analytics.NodeDetail `json:"detail"`
		GeneratedAt string                `json:"generatedAt,omitempty"`
	}{Node: node}
	if s.analytics != nil && node != nil {
		d, gen := s.analytics.Get(node.PublicKey)
		resp.Detail = d
		if !gen.IsZero() {
			resp.GeneratedAt = gen.UTC().Format(time.RFC3339)
		}
		// Fall back to the radio of the observers that heard this node when the
		// stored value isn't set yet (e.g. node hasn't re-advertised since the
		// observer's radio became known). Most common wins.
		if node.Radio == "" && d != nil && len(d.Observers) > 0 {
			node.Radio = resolveRadioFromObservers(s.store, d.Observers)
		}
	}
	writeJSON(w, resp)
}

// resolveRadioFromObservers returns the most common radio config among the
// observers that heard a node, used when the node's own stored radio is unset.
func resolveRadioFromObservers(st *store.Store, heard []analytics.ObserverStat) string {
	obs, err := st.ListObservers()
	if err != nil {
		return ""
	}
	radioByID := make(map[string]string, len(obs))
	for _, o := range obs {
		if o.Status != nil && o.Status.Radio != "" {
			radioByID[o.ID] = o.Status.Radio
		}
	}
	counts := map[string]int{}
	best, bestN := "", 0
	for _, h := range heard {
		if r := radioByID[h.ID]; r != "" {
			counts[r]++
			if counts[r] > bestN {
				best, bestN = r, counts[r]
			}
		}
	}
	return best
}

// nodeHistory returns a node's stored observations over an arbitrary time range
// (its own adverts + packets it relayed), newest first. Unlike nodeDetail's
// fixed-window analytics snapshot, this queries the database on demand.
func (s *Server) nodeHistory(w http.ResponseWriter, r *http.Request) {
	pubkey := strings.ToUpper(r.PathValue("pubkey"))
	sinceSec := queryInt(r, "since", 86400, 1, 7*86400)
	limit := queryInt(r, "limit", 200, 1, 1000)
	cutoff := time.Now().Add(-time.Duration(sinceSec) * time.Second).UTC().Format(time.RFC3339Nano)

	nodes, err := s.store.ListNodes()
	if err != nil {
		s.fail(w, err)
		return
	}
	entries, err := analytics.NodeHistory(s.store, nodes, pubkey, cutoff, 0, limit)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, entries)
}

// nodeObservers returns, per observer, the reception of a node's adverts over a
// selectable time range (default 3 days). Computed on demand — the fixed-window
// snapshot's "Heard by" list only spans a few hours, which rarely catches a
// node's ~30h advert cadence, so the detail page lets the user widen the range.
func (s *Server) nodeObservers(w http.ResponseWriter, r *http.Request) {
	pubkey := strings.ToUpper(r.PathValue("pubkey"))
	sinceSec := queryInt(r, "since", 3*86400, 1, 7*86400)
	cutoff := time.Now().Add(-time.Duration(sinceSec) * time.Second).UTC().Format(time.RFC3339Nano)

	nodes, err := s.store.ListNodes()
	if err != nil {
		s.fail(w, err)
		return
	}
	obs, err := analytics.NodeObservers(s.store, nodes, pubkey, cutoff, 0)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, obs)
}

// nodeHeatmap returns a node's weekday×hour activity grid over the last `days`.
func (s *Server) nodeHeatmap(w http.ResponseWriter, r *http.Request) {
	pubkey := strings.ToUpper(r.PathValue("pubkey"))
	days := queryInt(r, "days", 7, 1, 30)
	cutoff := time.Now().Add(-time.Duration(days) * 24 * time.Hour).UTC().Format(time.RFC3339Nano)
	nodes, err := s.store.ListNodes()
	if err != nil {
		s.fail(w, err)
		return
	}
	grid, err := analytics.NodeHeatmap(s.store, nodes, pubkey, cutoff, 0, days)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, grid)
}

// meshAnalytics returns a mesh-wide aggregate (traffic mix, link/RF health,
// channel utilisation, busiest relays) over a selectable window. Computed on
// demand from stored raw_hex, like nodeHistory.
func (s *Server) meshAnalytics(w http.ResponseWriter, r *http.Request) {
	sinceSec := queryInt(r, "since", 6*3600, 1, 24*3600)
	bucketMin := queryInt(r, "bucket", 10, 1, 1440)
	cutoff := time.Now().Add(-time.Duration(sinceSec) * time.Second).UTC().Format(time.RFC3339Nano)

	nodes, err := s.store.ListNodes()
	if err != nil {
		s.fail(w, err)
		return
	}
	summary, err := analytics.MeshSummary(s.store, nodes, cutoff, 0, analytics.DefaultRadio(), bucketMin)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, summary)
}

func (s *Server) observers(w http.ResponseWriter, _ *http.Request) {
	obs, err := s.store.ListObservers()
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, obs)
}

// observerAnalytics returns one observer's feed metrics (throughput, payload mix,
// SNR, RF neighbours, clock skew) over the last `since` seconds. On demand.
func (s *Server) observerAnalytics(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	sinceSec := queryInt(r, "since", 24*3600, 1, 7*86400)
	cutoff := time.Now().Add(-time.Duration(sinceSec) * time.Second).UTC().Format(time.RFC3339Nano)
	nodes, err := s.store.ListNodes()
	if err != nil {
		s.fail(w, err)
		return
	}
	summary, err := analytics.ObserverSummary(s.store, nodes, id, cutoff, 0)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, summary)
}

// observerTelemetry returns an observer's device-telemetry time series (battery,
// noise floor, airtime, errors) plus a derived health summary (battery/noise
// trends, reboot count) over the last `since` seconds.
func (s *Server) observerTelemetry(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	sinceSec := queryInt(r, "since", 24*3600, 1, 7*86400)
	cutoff := time.Now().Add(-time.Duration(sinceSec) * time.Second).UTC().Format(time.RFC3339Nano)
	points, err := s.store.ObserverTelemetry(id, cutoff, 0)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, analytics.ObserverTelemetryReport{
		ID:      id,
		Points:  points,
		Summary: analytics.SummarizeTelemetry(points),
	})
}

func (s *Server) observations(w http.ResponseWriter, r *http.Request) {
	limit := queryInt(r, "limit", 100, 1, 500)
	obs, err := s.store.RecentObservations(limit)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, obs)
}

// packet returns every observation of ONE transmission (by message hash) in the
// same live-event shape as /api/recent, so a shared link can re-open that exact
// packet in the feed modal. Public — the feed is public. Empty array if the hash
// is unknown or has aged out of storage.
func (s *Server) packet(w http.ResponseWriter, r *http.Request) {
	hash := r.PathValue("hash")
	raws, err := s.store.RawByHash(hash)
	if err != nil {
		s.fail(w, err)
		return
	}
	out := make([]LiveEvent, 0, len(raws))
	for _, ro := range raws {
		pkt, err := meshcore.DecodeHex(ro.RawHex)
		if err != nil || pkt == nil {
			continue
		}
		if s.store.ShouldDrop(pkt, ro.ObserverID) {
			continue // keep quarantined traffic hidden here too
		}
		out = append(out, newLiveEvent(pkt, ro.RawHex, ro.ObserverID, ro.Region, ro.ReceivedAt, ro.SNR, ro.RSSI))
	}
	writeJSON(w, out)
}

// recent returns the last `since` seconds (default 1h, max 6h) of observations
// re-decoded into the same shape as live WebSocket events, newest first, so the
// feed can render history identically without waiting for fresh packets.
func (s *Server) recent(w http.ResponseWriter, r *http.Request) {
	sinceSec := queryInt(r, "since", 3600, 1, 6*3600)
	cutoff := time.Now().Add(-time.Duration(sinceSec) * time.Second).UTC().Format(time.RFC3339Nano)

	raws, err := s.store.RecentRaw(cutoff, 3000)
	if err != nil {
		s.fail(w, err)
		return
	}

	out := make([]LiveEvent, 0, len(raws))
	for _, ro := range raws {
		pkt, err := meshcore.DecodeHex(ro.RawHex)
		if err != nil || pkt == nil {
			continue
		}
		// Hide quarantined traffic from feed history too (live WS is already
		// clean — blocked packets are dropped at ingest before broadcast).
		if s.store.ShouldDrop(pkt, ro.ObserverID) {
			continue
		}
		out = append(out, newLiveEvent(pkt, ro.RawHex, ro.ObserverID, ro.Region, ro.ReceivedAt, ro.SNR, ro.RSSI))
	}
	writeJSON(w, out)
}

// channelsRecent returns channel (GroupText) message history in the live-event
// shape, newest first, over the last `since` seconds (default & max 24h). Unlike
// /api/recent it filters to GroupText and returns one row per distinct message
// (collapsing observer copies), so the chat reader can show a full day of
// messages without the all-traffic row cap truncating history to ~30 minutes.
func (s *Server) channelsRecent(w http.ResponseWriter, r *http.Request) {
	sinceSec := queryInt(r, "since", 24*3600, 1, 24*3600)
	cutoff := time.Now().Add(-time.Duration(sinceSec) * time.Second).UTC().Format(time.RFC3339Nano)

	raws, err := s.store.RecentGroupText(cutoff, 10000)
	if err != nil {
		s.fail(w, err)
		return
	}

	out := make([]LiveEvent, 0, len(raws))
	for _, ro := range raws {
		pkt, err := meshcore.DecodeHex(ro.RawHex)
		if err != nil || pkt == nil {
			continue
		}
		if s.store.ShouldDrop(pkt, ro.ObserverID) {
			continue
		}
		out = append(out, newLiveEvent(pkt, ro.RawHex, ro.ObserverID, ro.Region, ro.ReceivedAt, ro.SNR, ro.RSSI))
	}
	writeJSON(w, out)
}

func (s *Server) live(w http.ResponseWriter, r *http.Request) {
	conn, err := s.up.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	ch := s.hub.add(conn)
	defer s.hub.remove(conn)

	// Reader pump: discard input, detect close.
	go func() {
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				conn.Close()
				return
			}
		}
	}()

	for msg := range ch {
		if err := conn.WriteMessage(websocket.TextMessage, msg); err != nil {
			return
		}
	}
}

func (s *Server) fail(w http.ResponseWriter, err error) {
	s.log.Error("api error", "err", err)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusInternalServerError)
	json.NewEncoder(w).Encode(map[string]string{"error": "internal error"})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

// queryInt reads an integer query parameter, falling back to def when absent or
// unparseable, then clamps the result to [min, max].
func queryInt(r *http.Request, key string, def, min, max int) int {
	v := def
	if s := r.URL.Query().Get(key); s != "" {
		if n, err := strconv.Atoi(s); err == nil {
			v = n
		}
	}
	if v < min {
		v = min
	}
	if v > max {
		v = max
	}
	return v
}

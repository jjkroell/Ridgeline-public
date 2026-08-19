package api

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/jjkroell/ridgeline/internal/auth"
	"github.com/jjkroell/ridgeline/internal/store"
)

// MQTTAuthConfig configures the endpoints the JWT broker calls to authenticate
// observers. Disabled (and every endpoint 404s) until Audience is set.
type MQTTAuthConfig struct {
	// Audience is the value observers must carry in their token's "aud" claim —
	// the hostname they were told to use via `set mqttN.audience`. A token minted
	// for another broker is refused, so this must match exactly.
	Audience string
	// ConsumerUsername is ridgelined's own ingest client. It is not an observer:
	// it holds no node key, subscribes rather than publishes, and so is checked
	// by password and granted superuser instead of going through token
	// verification. Empty disables the account entirely.
	ConsumerUsername string
	ConsumerPassword string
}

// Enabled reports whether observer token auth is configured.
func (c MQTTAuthConfig) Enabled() bool { return c.Audience != "" }

// mosquitto-go-auth's ACL access levels, from mosquitto's plugin header. Only
// WRITE matters to us: observers publish and never read.
const (
	mosqACLRead      = 1
	mosqACLWrite     = 2
	mosqACLSubscribe = 4
)

// mqttAuthReply is mosquitto-go-auth's json response mode: a 2XX status plus
// {Ok, Error}. Both must be satisfied, so a denial is a 200 with Ok=false —
// that way the reason reaches the broker log instead of being flattened into a
// bare status code.
type mqttAuthReply struct {
	Ok    bool   `json:"Ok"`
	Error string `json:"Error,omitempty"`
}

// observerSeen records an observer that has authenticated since this process
// started.
type observerSeen struct {
	PublicKey string    `json:"pubkey"`
	FirstAuth time.Time `json:"firstAuth"`
	LastAuth  time.Time `json:"lastAuth"`
	Count     int       `json:"count"`
}

// mqttAuthState tracks which observers have moved onto the authenticated broker.
// In memory by design: it answers "who has migrated so far", which is a question
// about the live transition, and it resets with the daemon rather than pretending
// to be history.
type mqttAuthState struct {
	mu   sync.Mutex
	seen map[string]*observerSeen
}

func newMQTTAuthState() *mqttAuthState {
	return &mqttAuthState{seen: make(map[string]*observerSeen)}
}

func (m *mqttAuthState) record(pubkey string, now time.Time) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.seen[pubkey]
	if !ok {
		m.seen[pubkey] = &observerSeen{PublicKey: pubkey, FirstAuth: now, LastAuth: now, Count: 1}
		return true // first time this process has seen it
	}
	e.LastAuth = now
	e.Count++
	return false
}

func (m *mqttAuthState) list() []observerSeen {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]observerSeen, 0, len(m.seen))
	for _, e := range m.seen {
		out = append(out, *e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].LastAuth.After(out[j].LastAuth) })
	return out
}

// SetMQTTAuth configures observer token authentication for the JWT broker.
func (s *Server) SetMQTTAuth(cfg MQTTAuthConfig) { s.mqttAuth = cfg }

// mqttAuthUser handles the broker's user check: is this username/password pair
// a legitimate observer (or our own ingest consumer)?
//
// SECURITY: this endpoint must not be reachable from the public internet. The
// broker reaches ridgelined directly over the compose network; the public edge
// (caddy) blocks /api/mqtt-auth/*. It is an oracle for token validity, and the
// consumer password is checked here.
func (s *Server) mqttAuthUser(w http.ResponseWriter, r *http.Request) {
	if !s.mqttAuth.Enabled() {
		http.NotFound(w, r)
		return
	}
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
		ClientID string `json:"clientid"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&req); err != nil {
		mqttAuthDeny(w, "malformed request")
		return
	}

	// ridgelined's own ingest client, which has no node identity to prove.
	if s.isMQTTConsumer(req.Username, req.Password) {
		writeJSON(w, mqttAuthReply{Ok: true})
		return
	}

	if !strings.HasPrefix(strings.ToLower(req.Username), strings.ToLower(auth.ObserverUsernamePrefix)) {
		// Log the username here too. A client whose username is simply shaped
		// differently -- a non-MeshCore observer implementation, say -- is
		// indistinguishable from silence if this path stays quiet, and the
		// broker only reports the bare reason without saying who sent it.
		s.log.Info("mqtt auth rejected: username is not an observer identity",
			"username", req.Username, "clientid", req.ClientID,
			"want_prefix", auth.ObserverUsernamePrefix)
		mqttAuthDeny(w, "unknown account")
		return
	}

	claims, err := auth.VerifyObserverToken(req.Username, req.Password, s.mqttAuth.Audience, time.Now())
	if err != nil {
		// Logged at info, not warn: during the migration a failure here is far
		// more likely to be a misconfigured node than an attack, and this fires
		// on every reconnect attempt.
		s.log.Info("mqtt auth rejected", "username", req.Username, "clientid", req.ClientID, "err", err)
		mqttAuthDeny(w, err.Error())
		return
	}

	if first := s.mqttAuthSeen.record(claims.PublicKey, time.Now()); first {
		s.log.Info("mqtt observer authenticated for the first time",
			"pubkey", claims.PublicKey, "owner", claims.Owner, "client", claims.Client)
	}
	// Persist it too, so "which observers have migrated?" survives a restart and
	// can be shown on the observer itself rather than only in this process's
	// memory. Throttled and a no-op for an observer with no row yet.
	s.store.RecordObserverJWTAuth(claims.PublicKey)
	writeJSON(w, mqttAuthReply{Ok: true})
}

// mqttAuthSuperuser grants the ingest consumer a blanket pass so it can
// subscribe across every observer's topics. Observers are never superusers.
func (s *Server) mqttAuthSuperuser(w http.ResponseWriter, r *http.Request) {
	if !s.mqttAuth.Enabled() {
		http.NotFound(w, r)
		return
	}
	var req struct {
		Username string `json:"username"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&req); err != nil {
		mqttAuthDeny(w, "malformed request")
		return
	}
	// Username-only request: the password was already checked at the user stage,
	// so matching the configured consumer name is what identifies it here.
	if s.mqttAuth.ConsumerUsername != "" && req.Username == s.mqttAuth.ConsumerUsername {
		writeJSON(w, mqttAuthReply{Ok: true})
		return
	}
	writeJSON(w, mqttAuthReply{Ok: false})
}

// mqttAuthACL binds an authenticated observer to its own topic subtree. Without
// this, any node holding a valid token of its own could publish under another
// observer's public key.
func (s *Server) mqttAuthACL(w http.ResponseWriter, r *http.Request) {
	if !s.mqttAuth.Enabled() {
		http.NotFound(w, r)
		return
	}
	var req struct {
		Username string `json:"username"`
		ClientID string `json:"clientid"`
		Topic    string `json:"topic"`
		Acc      int    `json:"acc"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&req); err != nil {
		mqttAuthDeny(w, "malformed request")
		return
	}

	if s.mqttAuth.ConsumerUsername != "" && req.Username == s.mqttAuth.ConsumerUsername {
		writeJSON(w, mqttAuthReply{Ok: true})
		return
	}

	// The username was proven to match the token's key at the user stage, so it
	// is the identity to bind against here.
	pubkey := strings.ToUpper(strings.TrimPrefix(
		strings.ToUpper(req.Username), strings.ToUpper(auth.ObserverUsernamePrefix)))

	write := req.Acc == mosqACLWrite
	if !auth.AuthorizeObserverTopic(pubkey, req.Topic, write) {
		s.log.Info("mqtt acl denied", "username", req.Username, "topic", req.Topic, "acc", req.Acc)
		mqttAuthDeny(w, "topic does not belong to this observer")
		return
	}
	writeJSON(w, mqttAuthReply{Ok: true})
}

// adminMQTTAuth reports which observers have authenticated against the JWT
// broker since this process started — the migration progress readout.
func (s *Server) adminMQTTAuth(w http.ResponseWriter, _ *http.Request, _ store.User) {
	writeJSON(w, map[string]any{
		"enabled":   s.mqttAuth.Enabled(),
		"audience":  s.mqttAuth.Audience,
		"observers": s.mqttAuthSeen.list(),
	})
}

func (s *Server) isMQTTConsumer(user, pass string) bool {
	if s.mqttAuth.ConsumerUsername == "" || s.mqttAuth.ConsumerPassword == "" {
		return false
	}
	// Constant-time on both: the username is as much a secret as the password
	// for a machine account nobody types.
	userOK := subtle.ConstantTimeCompare([]byte(user), []byte(s.mqttAuth.ConsumerUsername)) == 1
	passOK := subtle.ConstantTimeCompare([]byte(pass), []byte(s.mqttAuth.ConsumerPassword)) == 1
	return userOK && passOK
}

func mqttAuthDeny(w http.ResponseWriter, reason string) {
	writeJSON(w, mqttAuthReply{Ok: false, Error: reason})
}

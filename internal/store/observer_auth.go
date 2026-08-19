package store

import (
	"sync"
	"time"
)

// jwtAuthTouchInterval throttles the write behind RecordObserverJWTAuth. A
// broker re-checks credentials on every CONNECT, and a flapping client can
// reconnect repeatedly — the exact second of the latest auth is not worth a
// write each time, since all anyone asks of this column is "has this observer
// moved to the authenticated broker, and recently enough to still count?".
const jwtAuthTouchInterval = 5 * time.Minute

var (
	jwtAuthMu   sync.Mutex
	jwtAuthSeen map[string]time.Time
)

// RecordObserverJWTAuth notes that observerID authenticated with a token signed
// by its own node key.
//
// It deliberately does NOT create an observer row. An observer exists once it
// has published something we kept; authenticating only proves it connected, and
// a node that authenticates but never publishes should not appear on the site as
// a listening post. So this updates an existing row and quietly does nothing
// otherwise — the row appears on its first packet and picks the flag up on the
// next auth.
func (s *Store) RecordObserverJWTAuth(observerID string) {
	if observerID == "" {
		return
	}
	now := time.Now()

	jwtAuthMu.Lock()
	touch := now.Sub(jwtAuthSeen[observerID]) >= jwtAuthTouchInterval
	if touch {
		if jwtAuthSeen == nil {
			jwtAuthSeen = map[string]time.Time{}
		}
		jwtAuthSeen[observerID] = now
	}
	jwtAuthMu.Unlock()
	if !touch {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	// Case-insensitive on id: the topic carries the key in whatever case the
	// publisher used, while the username is uppercase by convention, and the two
	// must land on the same row.
	s.db.Exec(
		`UPDATE observers SET jwt_auth_at = ? WHERE UPPER(id) = UPPER(?)`,
		now.UTC().Format(time.RFC3339Nano), observerID,
	)
}

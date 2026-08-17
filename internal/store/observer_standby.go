package store

// Standby: an observer that stays connected but whose packets are discarded at
// ingest. It is the reversible middle ground between the two things that already
// existed — a blocklist entry (permanent quarantine, treats the publisher as
// rogue) and retirement (hides the receiver but keeps ingesting everything it
// reports).
//
// The use case is a receiver the operator does not want influencing the data
// right now: one being moved, re-sited, bench-tested, or running a firmware
// build under suspicion. Its /status messages are still processed, so it keeps
// reporting online with live battery/noise telemetry — the operator can watch
// the device while none of what it hears reaches the database.
//
// Nothing already stored is touched. Coming off standby resumes ingest with no
// backfill: packets discarded while on standby are gone, which is the point.

import "time"

// loadStandby refreshes the in-memory standby set from the observers table.
// Consulted on the hot ingest path, so it is a cache like the blocklist rather
// than a query per packet.
func (s *Store) loadStandby() error {
	rows, err := s.db.Query(`SELECT id FROM observers WHERE standby_since IS NOT NULL`)
	if err != nil {
		return err
	}
	defer rows.Close()
	set := map[string]bool{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return err
		}
		set[id] = true
	}
	if err := rows.Err(); err != nil {
		return err
	}
	s.standbyMu.Lock()
	s.standbyObservers = set
	// Drop counters for observers no longer on standby; a counter only means
	// anything for the current stand-down.
	for id := range s.standbyDropped {
		if !set[id] {
			delete(s.standbyDropped, id)
		}
	}
	s.standbyMu.Unlock()
	return nil
}

// ObserverOnStandby reports whether an observer's packets should be discarded.
// Deliberately NOT folded into ShouldDrop: that answers "is this traffic
// blocklisted", which carries a quarantine meaning this does not, and a caller
// reading either one should not have to know about the other.
func (s *Store) ObserverOnStandby(observerID string) bool {
	if observerID == "" {
		return false
	}
	s.standbyMu.RLock()
	defer s.standbyMu.RUnlock()
	return s.standbyObservers[observerID]
}

// standbyTouchInterval is how often a discarded packet is allowed to refresh an
// observer's last_seen. Once a minute is far finer than anything that reads the
// column (freshness is minutes, retention is an hour) and turns a per-packet
// write into a negligible one.
const standbyTouchInterval = time.Minute

// RecordStandbyDrop records that one packet was discarded for an observer on
// standby, and keeps the observer's last_seen current.
//
// The count is in memory on purpose: incrementing a SQLite counter on every
// discarded packet would cost more than storing the packet would have.
//
// last_seen, though, MUST keep advancing. We really did hear from the observer —
// we just threw away what it said — and last_seen is what the whole app uses to
// decide an observer is alive. Leaving it frozen would make a stood-down
// observer read as "Silent" within minutes and, far worse, DeleteStaleObservers
// would sweep the row away after an hour, taking the stand-down with it. The
// write is throttled to standbyTouchInterval so the hot path stays cheap.
func (s *Store) RecordStandbyDrop(observerID, at string) {
	s.standbyMu.Lock()
	if s.standbyDropped == nil {
		s.standbyDropped = map[string]int64{}
	}
	s.standbyDropped[observerID]++
	if s.standbySeen == nil {
		s.standbySeen = map[string]time.Time{}
	}
	now := time.Now()
	touch := now.Sub(s.standbySeen[observerID]) >= standbyTouchInterval
	if touch {
		s.standbySeen[observerID] = now
	}
	s.standbyMu.Unlock()

	if !touch {
		return
	}
	// Taken after releasing standbyMu: the write lock must never be acquired
	// while holding the hot-path lock the ingest goroutine spins on.
	s.mu.Lock()
	s.db.Exec(`UPDATE observers SET last_seen = ? WHERE id = ?`, at, observerID)
	s.mu.Unlock()
}

// StandbyDropped returns how many packets have been discarded for each observer
// currently on standby. Counts are since this daemon started, or since the
// observer was placed on standby if that is later — they are a live signal that
// the stand-down is working, not an audited total.
func (s *Store) StandbyDropped() map[string]int64 {
	s.standbyMu.RLock()
	defer s.standbyMu.RUnlock()
	out := make(map[string]int64, len(s.standbyDropped))
	for k, v := range s.standbyDropped {
		out[k] = v
	}
	return out
}

// SetObserverStandby places an observer on standby from the given RFC3339 time.
// Like RetireObserver this only sets a column: UpsertObserverStatus's ON
// CONFLICT branch never touches standby_since, so a retained /status replay
// cannot quietly return an observer to service.
func (s *Store) SetObserverStandby(id, at string) error {
	s.mu.Lock()
	_, err := s.db.Exec(`UPDATE observers SET standby_since = ? WHERE id = ?`, at, id)
	s.mu.Unlock()
	if err != nil {
		return err
	}
	return s.loadStandby()
}

// ClearObserverStandby returns an observer to service. Ingest resumes with the
// next packet; nothing discarded while it was on standby is recovered.
func (s *Store) ClearObserverStandby(id string) error {
	s.mu.Lock()
	_, err := s.db.Exec(`UPDATE observers SET standby_since = NULL WHERE id = ?`, id)
	s.mu.Unlock()
	if err != nil {
		return err
	}
	return s.loadStandby()
}

// NowRFC3339 is the timestamp format every lifecycle column in this package
// uses (retired_at, standby_since).
func NowRFC3339() string {
	return time.Now().UTC().Format(time.RFC3339)
}

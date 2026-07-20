package store

import (
	"database/sql"
	"fmt"
	"sort"
	"time"
)

// Observers used to be identified by their friendly name, which is a label the
// operator can change at will. Renaming one therefore started a whole new
// identity: the old name kept every observation and telemetry sample recorded
// under it, and the renamed observer began from nothing. The name is also not
// reliably distinct — a device publishing "Foo " and "Foo" is two observers.
//
// The public key is the stable identity. It comes from the MQTT topic
// (meshcore/{REGION}/{OBSERVER_PUBKEY}/…) on every message, so it is always
// available at ingest, and it survives any number of renames.
//
// migrateObserversToPubkey re-keys the existing data once: history recorded
// under each old name is repointed at that observer's public key, rows that were
// separate identities purely because of a rename are merged into one, and the
// friendly name is demoted to the `name` label column.
//
// Observers whose public key is unknown (rows predating the pubkey column, or a
// status message that never carried one) are left keyed by name — there is
// nothing better to key them by, and dropping them would lose their history.
func migrateObserversToPubkey(db *sql.DB) error {
	// Nothing to do once every observer that HAS a key is keyed by it.
	var pending int
	if err := db.QueryRow(`
		SELECT COUNT(*) FROM observers
		WHERE pubkey IS NOT NULL AND pubkey != '' AND id != pubkey`).Scan(&pending); err != nil {
		// The column may not exist yet on a brand-new database; migrate() creates
		// the table first, so a real error here is worth reporting.
		return fmt.Errorf("store: check observer identity migration: %w", err)
	}
	if pending == 0 {
		return nil
	}

	type observer struct {
		id, name, region, pubkey        string
		firstSeen, lastSeen             string
		packetCount                     int64
		statusJSON, lastStatusAt, radio sql.NullString
		retiredAt                       sql.NullString
	}

	rows, err := db.Query(`
		SELECT id, COALESCE(name,''), COALESCE(region,''), COALESCE(pubkey,''),
		       first_seen, last_seen, packet_count,
		       status_json, last_status_at, radio, retired_at
		FROM observers`)
	if err != nil {
		return fmt.Errorf("store: read observers for migration: %w", err)
	}
	var all []observer
	for rows.Next() {
		var o observer
		if err := rows.Scan(&o.id, &o.name, &o.region, &o.pubkey,
			&o.firstSeen, &o.lastSeen, &o.packetCount,
			&o.statusJSON, &o.lastStatusAt, &o.radio, &o.retiredAt); err != nil {
			rows.Close()
			return fmt.Errorf("store: scan observer for migration: %w", err)
		}
		all = append(all, o)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("store: read observers for migration: %w", err)
	}

	// Merge every row that shares a public key. A rename produced two rows for one
	// physical receiver, so the merged row must span both: earliest first_seen,
	// latest last_seen, summed packet count.
	//
	// Everything that describes the observer's CURRENT identity — its label, live
	// status, region, radio, and whether it is retired — is taken wholesale from
	// the most recent row, rather than combined field by field. That keeps the
	// merged row internally consistent (a label from one era can't end up beside a
	// status from another), and it means a receiver that was retired under an old
	// name and has since come back reporting is NOT left hidden: the current row
	// isn't retired, so the merged observer isn't either.
	groups := map[string][]observer{}
	order := []string{}
	for _, o := range all {
		key := o.pubkey
		if key == "" {
			key = o.id // no public key: keep it keyed by name
		}
		if _, seen := groups[key]; !seen {
			order = append(order, key)
		}
		groups[key] = append(groups[key], o)
	}

	merged := map[string]*observer{}
	for key, rows := range groups {
		// Most recent last. Timestamps are PARSED, not string-compared: the packet
		// path writes RFC3339Nano and the status path plain RFC3339, and as strings
		// '.' (0x2E) sorts before 'Z' (0x5A) — so within the same second a
		// fractional timestamp would compare as EARLIER than a whole-second one.
		sort.SliceStable(rows, func(i, j int) bool {
			return obsTimeLess(rows[i].lastSeen, rows[j].lastSeen)
		})
		cur := rows[len(rows)-1]
		if cur.name == "" {
			cur.name = cur.id // the old id WAS the friendly name
		}
		cur.id = key
		cur.packetCount = 0
		for _, o := range rows {
			if obsTimeLess(o.firstSeen, cur.firstSeen) {
				cur.firstSeen = o.firstSeen
			}
			if obsTimeLess(cur.lastSeen, o.lastSeen) {
				cur.lastSeen = o.lastSeen
			}
			cur.packetCount += o.packetCount
		}
		merged[key] = &cur
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Repoint the history. Both tables reference the observer by the same string,
	// so an old name maps to the public key it belonged to.
	for _, o := range all {
		if o.pubkey == "" || o.id == o.pubkey {
			continue
		}
		for _, q := range []string{
			`UPDATE observations SET observer_id = ? WHERE observer_id = ?`,
			`UPDATE observer_telemetry SET observer_id = ? WHERE observer_id = ?`,
			`UPDATE blocklist SET key = ? WHERE kind = 'observer' AND key = ?`,
		} {
			if _, err := tx.Exec(q, o.pubkey, o.id); err != nil {
				return fmt.Errorf("store: repoint observer history: %w", err)
			}
		}
	}

	if _, err := tx.Exec(`DELETE FROM observers`); err != nil {
		return fmt.Errorf("store: clear observers for migration: %w", err)
	}
	for _, key := range order {
		o := merged[key]
		if _, err := tx.Exec(`
			INSERT INTO observers
				(id, name, region, pubkey, first_seen, last_seen, packet_count,
				 status_json, last_status_at, radio, retired_at)
			VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
			o.id, o.name, nullStr(o.region), nullStr(o.pubkey),
			o.firstSeen, o.lastSeen, o.packetCount,
			o.statusJSON, o.lastStatusAt, o.radio, o.retiredAt,
		); err != nil {
			return fmt.Errorf("store: rewrite observer %q: %w", o.id, err)
		}
	}
	return tx.Commit()
}

// obsTimeLess reports whether observer timestamp a is earlier than b.
//
// Observer timestamps are not written in one format: the packet path stamps
// RFC3339Nano and the status path plain RFC3339. Compared as strings those
// interleave wrongly — '.' (0x2E) sorts before 'Z' (0x5A), so within the same
// second a fractional timestamp reads as EARLIER than a whole-second one. Parse
// both; fall back to a string compare only if either is unparseable.
func obsTimeLess(a, b string) bool {
	ta, errA := time.Parse(time.RFC3339, a)
	tb, errB := time.Parse(time.RFC3339, b)
	if errA != nil || errB != nil {
		return a < b
	}
	return ta.Before(tb)
}

package store

import "testing"

// TestMigrateObserversToPubkey covers the re-key: history recorded under an old
// friendly name is repointed at the observer's public key, rows that were
// separate identities only because of a rename are merged into one, and an
// observer with no key is left alone.
func TestMigrateObserversToPubkey(t *testing.T) {
	st := testStore(t)
	pk := "AA11BB22CC33DD44EE55FF66AA11BB22CC33DD44EE55FF66AA11BB22CC33DD44"

	// Old-style rows: keyed by friendly name. "Obs Old" and "Obs New" are the same
	// physical receiver either side of a rename, so they share a public key.
	ins := func(id, pubkey, first, last string, packets int) {
		if _, err := st.db.Exec(`
			INSERT INTO observers (id, region, pubkey, first_seen, last_seen, packet_count)
			VALUES (?,?,?,?,?,?)`, id, "R1", pubkey, first, last, packets); err != nil {
			t.Fatalf("insert observer %s: %v", id, err)
		}
	}
	ins("Obs Old", pk, "2026-01-01T00:00:00Z", "2026-02-01T00:00:00Z", 10)
	ins("Obs New", pk, "2026-02-02T00:00:00Z", "2026-03-01T00:00:00Z", 5)
	ins("Keyless", "", "2026-01-01T00:00:00Z", "2026-01-02T00:00:00Z", 3)

	for _, id := range []string{"Obs Old", "Obs New", "Keyless"} {
		if _, err := st.db.Exec(`
			INSERT INTO observations (message_hash, raw_hex, route_type, payload_type, path_hops, observer_id, received_at)
			VALUES ('h','00','Flood','Advert',0,?, '2026-01-01T00:00:00Z')`, id); err != nil {
			t.Fatalf("insert observation %s: %v", id, err)
		}
		if _, err := st.db.Exec(`
			INSERT INTO observer_telemetry (observer_id, recorded_at, battery_mv)
			VALUES (?, '2026-01-01T00:00:00Z', 4000)`, id); err != nil {
			t.Fatalf("insert telemetry %s: %v", id, err)
		}
	}
	if _, err := st.db.Exec(`
		INSERT INTO blocklist (kind, key, name, reason, created_at)
		VALUES ('observer','Obs Old','Obs Old','test','2026-01-01T00:00:00Z')`); err != nil {
		t.Fatalf("insert blocklist: %v", err)
	}

	if err := migrateObserversToPubkey(st.db); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	// The two renamed rows collapse into one observer keyed by the public key,
	// spanning both: earliest first_seen, latest last_seen, summed packets. The
	// label is the most recent name.
	var name, first, last string
	var packets int
	if err := st.db.QueryRow(`
		SELECT COALESCE(name,''), first_seen, last_seen, packet_count
		FROM observers WHERE id = ?`, pk).Scan(&name, &first, &last, &packets); err != nil {
		t.Fatalf("merged observer not found: %v", err)
	}
	if name != "Obs New" {
		t.Errorf("name = %q, want the most recent label %q", name, "Obs New")
	}
	if first != "2026-01-01T00:00:00Z" || last != "2026-03-01T00:00:00Z" {
		t.Errorf("merged span = %s..%s, want 2026-01-01..2026-03-01", first, last)
	}
	if packets != 15 {
		t.Errorf("packet_count = %d, want 15 (10+5)", packets)
	}

	// History from BOTH old names now resolves to the one identity.
	var obs, tel int
	st.db.QueryRow(`SELECT COUNT(*) FROM observations WHERE observer_id = ?`, pk).Scan(&obs)
	st.db.QueryRow(`SELECT COUNT(*) FROM observer_telemetry WHERE observer_id = ?`, pk).Scan(&tel)
	if obs != 2 || tel != 2 {
		t.Errorf("repointed history = %d observations / %d telemetry, want 2 / 2", obs, tel)
	}

	// A blocklist entry keyed by the old name follows too, or a quarantined
	// observer would silently come off the blocklist.
	var blocked int
	st.db.QueryRow(`SELECT COUNT(*) FROM blocklist WHERE kind='observer' AND key = ?`, pk).Scan(&blocked)
	if blocked != 1 {
		t.Errorf("blocklist entries for the key = %d, want 1", blocked)
	}

	// The keyless observer keeps its name as identity — there is nothing better,
	// and dropping it would lose its history.
	var keyless int
	st.db.QueryRow(`SELECT COUNT(*) FROM observers WHERE id = 'Keyless'`).Scan(&keyless)
	if keyless != 1 {
		t.Errorf("keyless observer rows = %d, want 1 (left keyed by name)", keyless)
	}

	// Running again is a no-op: everything that has a key is already keyed by it.
	if err := migrateObserversToPubkey(st.db); err != nil {
		t.Fatalf("second migrate: %v", err)
	}
	var total int
	st.db.QueryRow(`SELECT COUNT(*) FROM observers`).Scan(&total)
	if total != 2 {
		t.Errorf("observers after re-run = %d, want 2 (merged + keyless)", total)
	}
}

// TestMigrateMergeTakesCurrentIdentity verifies that the merged observer's
// identity fields come from the MOST RECENT row. In particular a receiver that
// was retired under an old name and has since come back reporting must NOT stay
// hidden — the current row isn't retired, so the merged observer isn't either.
func TestMigrateMergeTakesCurrentIdentity(t *testing.T) {
	st := testStore(t)
	pk := "BB22CC33DD44EE55FF66AA11BB22CC33DD44EE55FF66AA11BB22CC33DD44EE55"

	// Retired under the old name...
	if _, err := st.db.Exec(`
		INSERT INTO observers (id, region, pubkey, first_seen, last_seen, packet_count, retired_at)
		VALUES ('Old Name','R1',?, '2026-01-01T00:00:00Z','2026-02-01T00:00:00Z',4,'2026-02-01T00:00:00Z')`, pk); err != nil {
		t.Fatalf("insert retired row: %v", err)
	}
	// ...then back on the air under a new one, not retired.
	if _, err := st.db.Exec(`
		INSERT INTO observers (id, region, pubkey, first_seen, last_seen, packet_count)
		VALUES ('New Name','R1',?, '2026-03-01T00:00:00Z','2026-04-01T00:00:00Z',6)`, pk); err != nil {
		t.Fatalf("insert active row: %v", err)
	}

	if err := migrateObserversToPubkey(st.db); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	var name string
	var retired *string
	var packets int
	if err := st.db.QueryRow(`
		SELECT COALESCE(name,''), retired_at, packet_count FROM observers WHERE id = ?`, pk).
		Scan(&name, &retired, &packets); err != nil {
		t.Fatalf("merged observer not found: %v", err)
	}
	if name != "New Name" {
		t.Errorf("name = %q, want the current label %q", name, "New Name")
	}
	if retired != nil {
		t.Errorf("retired_at = %v, want nil — the receiver is reporting again", *retired)
	}
	if packets != 10 {
		t.Errorf("packet_count = %d, want 10 (4+6 across both names)", packets)
	}
}

// TestObsTimeLessHandlesMixedPrecision guards the timestamp comparison. The
// packet path writes RFC3339Nano and the status path plain RFC3339; compared as
// strings '.' sorts before 'Z', so a fractional timestamp would read as earlier
// than a whole-second one inside the same second.
func TestObsTimeLessHandlesMixedPrecision(t *testing.T) {
	nano := "2026-07-20T04:13:05.999999Z"
	whole := "2026-07-20T04:13:05Z"

	if nano < whole != true {
		t.Fatal("precondition: expected the naive string compare to be wrong")
	}
	if obsTimeLess(nano, whole) {
		t.Errorf("obsTimeLess(%s, %s) = true, want false — the fractional time is LATER", nano, whole)
	}
	if !obsTimeLess(whole, nano) {
		t.Errorf("obsTimeLess(%s, %s) = false, want true", whole, nano)
	}
	// Unparseable input still gives a deterministic ordering.
	if obsTimeLess("zzz", "aaa") {
		t.Error("unparseable timestamps should fall back to a string compare")
	}
}

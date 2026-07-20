// Package store is ridgelined's SQLite persistence layer. A single writer (the
// ingest loop) holds the write mutex; API reads run concurrently against the
// same WAL-mode database.
package store

import (
	"database/sql"
	"fmt"
	"sync"
	"time"

	"github.com/jjkroell/ridgeline/internal/meshcore"

	_ "modernc.org/sqlite"
)

const schema = `
CREATE TABLE IF NOT EXISTS nodes (
	pubkey       TEXT PRIMARY KEY,
	name         TEXT,
	role         TEXT,
	latitude     REAL,
	longitude    REAL,
	has_location INTEGER NOT NULL DEFAULT 0,
	first_seen   TEXT NOT NULL,
	last_seen    TEXT NOT NULL,
	last_advert  TEXT,
	advert_count INTEGER NOT NULL DEFAULT 0,
	advert_tx_count INTEGER NOT NULL DEFAULT 0,
	hash_size    INTEGER NOT NULL DEFAULT 0,
	-- Display name of the node's last verified owner, retained after that owner
	-- deleted their account (so the node can show "previously owned by …").
	-- Cleared when the node gains a new verified owner.
	prev_owner_name TEXT
);

CREATE TABLE IF NOT EXISTS observers (
	id           TEXT PRIMARY KEY,
	region       TEXT,
	pubkey       TEXT,
	first_seen   TEXT NOT NULL,
	last_seen    TEXT NOT NULL,
	packet_count INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS observations (
	id           INTEGER PRIMARY KEY AUTOINCREMENT,
	message_hash TEXT NOT NULL,
	raw_hex      TEXT NOT NULL,
	route_type   TEXT NOT NULL,
	payload_type TEXT NOT NULL,
	path_hops    INTEGER NOT NULL DEFAULT 0,
	observer_id  TEXT,
	region       TEXT,
	snr          REAL,
	rssi         REAL,
	received_at  TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_obs_hash ON observations(message_hash);
CREATE INDEX IF NOT EXISTS idx_obs_received ON observations(received_at DESC);

-- observer_telemetry is an append-only time series of each observer's
-- self-reported device telemetry. The observers row only keeps the LATEST
-- /status (overwritten in place); this log preserves history so battery/noise/
-- airtime can be trended. Only the non-reconstructable device fields live here
-- (radio config is static and stays on the observers row).
CREATE TABLE IF NOT EXISTS observer_telemetry (
	id           INTEGER PRIMARY KEY AUTOINCREMENT,
	observer_id  TEXT NOT NULL,
	recorded_at  TEXT NOT NULL,
	battery_mv   INTEGER,
	uptime_secs  INTEGER,
	noise_floor  REAL,
	tx_air_secs  REAL,
	rx_air_secs  REAL,
	recv_errors  INTEGER,
	queue_len    INTEGER
);
CREATE INDEX IF NOT EXISTS idx_tel_obs_time ON observer_telemetry(observer_id, recorded_at DESC);

-- blocklist holds nodes/observers/bridges an admin has quarantined as injected
-- traffic (RF bridge or rogue MQTT publisher). Entries drop matching data at
-- ingest and hide it from the API; purging additionally hard-deletes stored rows.
CREATE TABLE IF NOT EXISTS blocklist (
	kind       TEXT NOT NULL,            -- observer | bridge | node | allow (allow = dismissed candidate)
	key        TEXT NOT NULL,            -- observer id, or node/bridge pubkey
	name       TEXT,                     -- friendly label captured at block time
	reason     TEXT,
	created_at TEXT NOT NULL,
	PRIMARY KEY (kind, key)
);

-- users holds registered accounts. Registration is open, but the sensitive
-- features (claiming nodes, storing a node's private exact location) are gated
-- behind can_claim, which an admin grants. The very first account created is
-- bootstrapped as admin + can_claim so the deployment has an owner.
CREATE TABLE IF NOT EXISTS users (
	id            INTEGER PRIMARY KEY AUTOINCREMENT,
	email         TEXT NOT NULL UNIQUE,     -- stored lowercased
	password_hash TEXT NOT NULL,            -- Argon2id PHC string
	display_name  TEXT,                     -- callsign / handle shown on public notes
	is_admin      INTEGER NOT NULL DEFAULT 0,
	can_claim     INTEGER NOT NULL DEFAULT 0,
	blocked       INTEGER NOT NULL DEFAULT 0, -- suspended: cannot log in, existing sessions void
	protected     INTEGER NOT NULL DEFAULT 0, -- the initial/owner admin: cannot be demoted/blocked/removed
	created_at    TEXT NOT NULL,
	last_login    TEXT
);

-- sessions are server-side so they can be revoked. token_hash is the SHA-256 of
-- the opaque token in the user's cookie; the plaintext is never stored. csrf is
-- the double-submit token echoed back on mutating requests.
CREATE TABLE IF NOT EXISTS sessions (
	token_hash TEXT PRIMARY KEY,
	user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	csrf       TEXT NOT NULL,
	created_at TEXT NOT NULL,
	expires_at TEXT NOT NULL,
	last_seen  TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_sessions_user ON sessions(user_id);
CREATE INDEX IF NOT EXISTS idx_sessions_expires ON sessions(expires_at);

-- email_verifications holds pending account-verification tokens. token_hash is
-- the SHA-256 of the opaque token emailed to the user; the plaintext is never
-- stored. One row per outstanding request (a resend replaces prior rows).
CREATE TABLE IF NOT EXISTS email_verifications (
	token_hash TEXT PRIMARY KEY,
	user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	created_at TEXT NOT NULL,
	expires_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_email_verif_user ON email_verifications(user_id);

-- password_resets holds pending password-reset tokens. Same shape as
-- email_verifications: token_hash is the SHA-256 of the opaque token emailed to
-- the user (plaintext never stored), one row per outstanding request (a new
-- request replaces prior rows), short TTL.
CREATE TABLE IF NOT EXISTS password_resets (
	token_hash TEXT PRIMARY KEY,
	user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	created_at TEXT NOT NULL,
	expires_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_pw_reset_user ON password_resets(user_id);

-- node_claims records a user's claim on a node. A pending claim carries a
-- verification code the owner temporarily embeds in the node's advertised name;
-- the ingest verifier promotes it to 'verified' when a signature-valid advert
-- from that node carries the code. One verified owner per node (partial unique
-- index below); one claim row per (node,user).
CREATE TABLE IF NOT EXISTS node_claims (
	id          INTEGER PRIMARY KEY AUTOINCREMENT,
	node_pubkey TEXT NOT NULL,           -- uppercase hex
	user_id     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	code        TEXT NOT NULL,
	status      TEXT NOT NULL,           -- pending | verified
	created_at  TEXT NOT NULL,
	expires_at  TEXT NOT NULL,           -- pending-challenge expiry
	verified_at TEXT,
	UNIQUE(node_pubkey, user_id)
);
CREATE INDEX IF NOT EXISTS idx_claims_node ON node_claims(node_pubkey);
CREATE UNIQUE INDEX IF NOT EXISTS idx_claims_one_owner ON node_claims(node_pubkey) WHERE status = 'verified';
-- No two open (pending) claims may share a verification code, so a code is
-- globally unambiguous while it's live. Verified claims keep their (now spent)
-- code but are excluded here, so they never block a new pending code.
CREATE UNIQUE INDEX IF NOT EXISTS idx_claims_pending_code ON node_claims(code) WHERE status = 'pending';

-- node_notes are user-authored annotations on a node. Public notes are visible
-- to everyone on the node page; private notes are visible only to their author.
CREATE TABLE IF NOT EXISTS node_notes (
	id          INTEGER PRIMARY KEY AUTOINCREMENT,
	node_pubkey TEXT NOT NULL,             -- uppercase hex
	user_id     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	visibility  TEXT NOT NULL,             -- public | private
	body        TEXT NOT NULL,
	created_at  TEXT NOT NULL,
	updated_at  TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_notes_node ON node_notes(node_pubkey);
CREATE INDEX IF NOT EXISTS idx_notes_user ON node_notes(user_id);

-- node_private_locations holds a node's owner-set exact coordinates. This is
-- SENSITIVE (a ham operator's real antenna/home location) and is DELIBERATELY
-- separate from the nodes table: it is never joined into /api/nodes, node detail,
-- the live WebSocket, or analytics. Only the node's verified owner (and, in a
-- later phase, users they explicitly share with) may read it. One row per node.
CREATE TABLE IF NOT EXISTS node_private_locations (
	node_pubkey TEXT PRIMARY KEY,            -- uppercase hex
	user_id     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE, -- who set it
	latitude    REAL NOT NULL,
	longitude   REAL NOT NULL,
	label       TEXT NOT NULL DEFAULT '',    -- optional owner note ("rooftop", "repeater site")
	updated_at  TEXT NOT NULL
);

-- location_shares grants specific registered users READ access to a node's
-- private exact location. Only the node's verified owner may grant/revoke; a
-- grantee can read the location but never edit it or re-share it. Rows are
-- dropped when the owner releases the node (see claimDelete) or either user is
-- deleted (cascade).
CREATE TABLE IF NOT EXISTS location_shares (
	node_pubkey     TEXT NOT NULL,           -- uppercase hex
	owner_user_id   INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	grantee_user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	created_at      TEXT NOT NULL,
	seen            INTEGER NOT NULL DEFAULT 0, -- grantee has seen this share (badge/alert)
	PRIMARY KEY (node_pubkey, grantee_user_id)
);
CREATE INDEX IF NOT EXISTS idx_location_shares_grantee ON location_shares(grantee_user_id);
`

// Store wraps a SQLite database.
type Store struct {
	db *sql.DB
	mu sync.Mutex // serializes writes (single-writer model)

	// needAdvertTxBackfill is set on open when the advert_tx_count column was
	// just added, so a caller can seed it once from history.
	needAdvertTxBackfill bool

	// Blocklist cache, consulted on the hot ingest path. Guarded separately
	// from mu so reads don't contend with writes. Refreshed from the table on
	// open and after every mutation.
	blockMu          sync.RWMutex
	blockedObservers map[string]bool // observer id (exact)
	blockedNodes     map[string]bool // node/bridge pubkey (UPPER) — origin-advert block
	blockedBridges   []string        // bridge pubkeys (UPPER) — path-prefix block
	allowedNodes     map[string]bool // node pubkey (UPPER) — dismissed detection candidates
	knownBridges     map[string]bool // node pubkey (UPPER) — sanctioned bridges, labelled not hidden

	// Set of node pubkeys (UPPER) with an open pending ownership claim. Consulted
	// on the hot ingest path so the advert verifier only touches the DB for nodes
	// that actually have a claim awaiting a code. Refreshed on claim mutations.
	claimMu           sync.RWMutex
	pendingClaimNodes map[string]bool
}

// Open opens (creating if needed) the SQLite database at path, enables WAL
// mode, and applies the schema.
func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("store: open: %w", err)
	}
	// Pin to a single connection. SQLite pragmas (busy_timeout, WAL) are
	// per-connection, so with database/sql's default pool the timeout never
	// reaches the extra connections it opens for concurrent queries — which is
	// what produced spurious SQLITE_BUSY errors. One connection serializes all
	// access (writes are already single-writer via s.mu, and the workload is
	// low-volume), and guarantees every statement runs with the pragmas below.
	db.SetMaxOpenConns(1)
	// WAL keeps reads fast; busy_timeout is belt-and-suspenders for any retry.
	for _, pragma := range []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA busy_timeout=5000",
		"PRAGMA foreign_keys=ON",
	} {
		if _, err := db.Exec(pragma); err != nil {
			db.Close()
			return nil, fmt.Errorf("store: %s: %w", pragma, err)
		}
	}
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("store: schema: %w", err)
	}
	// Lightweight migrations for databases created before a column existed.
	// Errors are expected (and ignored) when the column is already present.
	db.Exec(`ALTER TABLE observers ADD COLUMN pubkey TEXT`)
	db.Exec(`ALTER TABLE nodes ADD COLUMN hash_size INTEGER NOT NULL DEFAULT 0`)
	db.Exec(`ALTER TABLE observers ADD COLUMN status_json TEXT`)
	db.Exec(`ALTER TABLE observers ADD COLUMN last_status_at TEXT`)
	db.Exec(`ALTER TABLE observers ADD COLUMN radio TEXT`)
	db.Exec(`ALTER TABLE nodes ADD COLUMN radio TEXT`)
	// advert_tx_count counts actual advert *transmissions* (re-flood/multi-observer
	// copies of one advert collapsed by a 30s gap), vs advert_count which counts
	// raw observations. If the column is new on an existing DB, flag a one-time
	// backfill from the stored observation history.
	needAdvertTxBackfill := !columnExists(db, "nodes", "advert_tx_count")
	db.Exec(`ALTER TABLE nodes ADD COLUMN advert_tx_count INTEGER NOT NULL DEFAULT 0`)
	// prev_owner_name records a node's last verified owner after they delete their
	// account, so the public page can show "previously owned by …".
	db.Exec(`ALTER TABLE nodes ADD COLUMN prev_owner_name TEXT`)
	// Retiring an observer hides it from the observers page without touching the
	// packets it reported — a decommissioned receiver stops being presented as
	// part of the network while its history stays attributable to it. NULL means
	// active; a retirement stamps the RFC3339 time it was retired.
	db.Exec(`ALTER TABLE observers ADD COLUMN retired_at TEXT`)
	// User account status columns (added after the initial users table shipped).
	db.Exec(`ALTER TABLE users ADD COLUMN blocked INTEGER NOT NULL DEFAULT 0`)
	db.Exec(`ALTER TABLE users ADD COLUMN protected INTEGER NOT NULL DEFAULT 0`)
	// Email verification: new accounts must confirm their address before logging
	// in. Accounts that predate this feature are grandfathered as verified on the
	// one-time column add, so nobody is locked out.
	if !columnExists(db, "users", "email_verified") {
		db.Exec(`ALTER TABLE users ADD COLUMN email_verified INTEGER NOT NULL DEFAULT 0`)
		db.Exec(`UPDATE users SET email_verified = 1`)
	}
	// Claiming is now universal (no admin approval) — grant every existing account
	// the can_claim right so members created before this change can claim too.
	db.Exec(`UPDATE users SET can_claim = 1 WHERE can_claim = 0`)
	// Per-grantee "seen" flag on shares, for the Shared-with-me badge. Existing
	// shares are treated as already seen so they don't retroactively alert.
	if !columnExists(db, "location_shares", "seen") {
		db.Exec(`ALTER TABLE location_shares ADD COLUMN seen INTEGER NOT NULL DEFAULT 0`)
		db.Exec(`UPDATE location_shares SET seen = 1`)
	}
	// Backfill: if the protected/owner flag exists on nobody yet but accounts do,
	// mark the first-registered account (lowest id = the bootstrap admin) as the
	// protected owner. Idempotent — once one row is protected this is a no-op.
	db.Exec(`UPDATE users SET protected = 1
		WHERE id = (SELECT MIN(id) FROM users)
		  AND (SELECT COUNT(*) FROM users) > 0
		  AND NOT EXISTS (SELECT 1 FROM users WHERE protected = 1)`)
	s := &Store{db: db, needAdvertTxBackfill: needAdvertTxBackfill}
	if err := s.loadBlocklist(); err != nil {
		db.Close()
		return nil, fmt.Errorf("store: load blocklist: %w", err)
	}
	if err := s.loadPendingClaims(); err != nil {
		db.Close()
		return nil, fmt.Errorf("store: load pending claims: %w", err)
	}
	return s, nil
}

// UpsertObserverStatus records an observer's latest self-reported status (radio
// config + device telemetry) from its /status message, creating the observer row
// if a status arrives before any packet. statusJSON is the marshalled
// ObserverStatus; receivedAt is the server's receipt time (RFC3339).
func (s *Store) UpsertObserverStatus(id, region, pubkey, statusJSON, radio, receivedAt string) error {
	_, err := s.db.Exec(`
		INSERT INTO observers (id, region, pubkey, first_seen, last_seen, packet_count, status_json, last_status_at, radio)
		VALUES (?,?,?,?,?,0,?,?,?)
		ON CONFLICT(id) DO UPDATE SET
			status_json    = excluded.status_json,
			last_status_at = excluded.last_status_at,
			radio          = COALESCE(NULLIF(excluded.radio,''), observers.radio),
			region         = COALESCE(NULLIF(excluded.region,''), observers.region),
			pubkey         = COALESCE(NULLIF(excluded.pubkey,''), observers.pubkey)`,
		id, region, pubkey, receivedAt, receivedAt, statusJSON, receivedAt, radio)
	return err
}

// RetireObserver hides an observer from the observers page without deleting
// anything it reported. Retiring KEEPS the row on purpose: a retained /status
// message replayed by the broker takes the ON CONFLICT branch of
// UpsertObserverStatus and leaves retired_at alone, so the observer stays
// hidden. Deleting the row instead would let that same replay re-INSERT it with
// a fresh first_seen/last_seen — which is exactly how decommissioned observers
// used to keep coming back.
func (s *Store) RetireObserver(id, at string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(`UPDATE observers SET retired_at = ? WHERE id = ?`, at, id)
	return err
}

// UnretireObserver returns a retired observer to the observers page.
func (s *Store) UnretireObserver(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(`UPDATE observers SET retired_at = NULL WHERE id = ?`, id)
	return err
}

// UpdateObserverStatusIfPresent refreshes an existing observer's status without
// ever creating a row, and reports whether one was updated.
//
// This is the path for RETAINED status messages. Observers publish /status with
// the retain flag, and the broker replays that message to the daemon on every
// reconnect — for as long as it exists, whether or not the device is still on
// the air. Feeding a replay through UpsertObserverStatus would INSERT a row for
// a decommissioned observer, stamping first_seen/last_seen with the reconnect
// time and resurrecting it on the observers page. A retained status is a stale
// last-known value, never evidence the device is live, so it may refresh an
// observer that already exists but must not conjure one.
func (s *Store) UpdateObserverStatusIfPresent(id, region, pubkey, statusJSON, radio, receivedAt string) (bool, error) {
	res, err := s.db.Exec(`
		UPDATE observers SET
			status_json    = ?,
			last_status_at = ?,
			radio          = COALESCE(NULLIF(?,''), radio),
			region         = COALESCE(NULLIF(?,''), region),
			pubkey         = COALESCE(NULLIF(?,''), pubkey)
		WHERE id = ?`,
		statusJSON, receivedAt, radio, region, pubkey, id)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

// DeleteStaleObservers removes observer rows whose last_seen is older than the
// given RFC3339 cutoff, returning the ids removed. Only the observers row is
// deleted — the observations (packets) it reported, and its telemetry history,
// are left intact so past traffic and "heard by" attribution survive. A removed
// observer reappears the moment it publishes again (its next packet or status
// re-creates the row), so this only clears observers that have genuinely gone
// silent.
//
// Retired observers are skipped: their row is deliberately kept so replayed
// retained status messages can't re-INSERT them (see RetireObserver), and
// sweeping it away would undo the retirement.
func (s *Store) DeleteStaleObservers(cutoff string) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	rows, err := s.db.Query(`SELECT id FROM observers WHERE last_seen < ? AND retired_at IS NULL`, cutoff)
	if err != nil {
		return nil, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, nil
	}
	if _, err := s.db.Exec(`DELETE FROM observers WHERE last_seen < ? AND retired_at IS NULL`, cutoff); err != nil {
		return nil, err
	}
	return ids, nil
}

// Close closes the underlying database.
func (s *Store) Close() error { return s.db.Close() }

// Observation is one observer's sighting of one packet, ready to persist.
type Observation struct {
	Packet         *meshcore.Packet
	RawHex         string // full raw packet hex as received
	ObserverID     string
	ObserverPubkey string // observer node public key (origin_id), for geo-locating
	Region         string
	SNR            *float64
	RSSI           *float64
	ReceivedAt     time.Time // server ingest time — authoritative for ordering
}

// Record persists an observation: it inserts the observation row, updates the
// observer, and (for Adverts) upserts the announcing node.
func (s *Store) Record(o Observation) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	ts := o.ReceivedAt.UTC().Format(time.RFC3339Nano)
	p := o.Packet

	rawHex := o.RawHex
	if rawHex == "" {
		rawHex = p.PayloadRaw
	}
	if _, err := tx.Exec(`
		INSERT INTO observations
			(message_hash, raw_hex, route_type, payload_type, path_hops,
			 observer_id, region, snr, rssi, received_at)
		VALUES (?,?,?,?,?,?,?,?,?,?)`,
		p.MessageHash, rawHex, p.RouteType.String(), p.PayloadType.String(),
		p.PathHopCount, nullStr(o.ObserverID), nullStr(o.Region),
		o.SNR, o.RSSI, ts,
	); err != nil {
		return fmt.Errorf("store: insert observation: %w", err)
	}

	if o.ObserverID != "" {
		if _, err := tx.Exec(`
			INSERT INTO observers (id, region, pubkey, first_seen, last_seen, packet_count)
			VALUES (?,?,?,?,?,1)
			ON CONFLICT(id) DO UPDATE SET
				last_seen    = excluded.last_seen,
				region       = COALESCE(NULLIF(excluded.region,''), observers.region),
				pubkey       = COALESCE(NULLIF(excluded.pubkey,''), observers.pubkey),
				packet_count = observers.packet_count + 1`,
			o.ObserverID, nullStr(o.Region), nullStr(o.ObserverPubkey), ts, ts,
		); err != nil {
			return fmt.Errorf("store: upsert observer: %w", err)
		}
	}

	// Only a signature-valid advert may create or mutate the authoritative node
	// row (name, role, location, hash size, counts). The observation itself is
	// already stored above, so a corrupt or forged advert still shows in the live
	// feed — it just can't deface a node's identity. Without this guard a single
	// RF-garbled copy (e.g. "UBCV//Zenith" arriving as "P#e//jnitm") overwrites the
	// real name, and corrupt copies also spawn phantom nodes.
	if a := p.Advert; a != nil && a.PublicKey != "" && a.SignatureValid {
		var lat, lon interface{}
		if a.HasLocation {
			lat, lon = a.Latitude, a.Longitude
		}
		// A node inherits the radio config of the observer that heard it — its
		// own freq/bw/sf/cr aren't in the packet (they're a PHY setting), but the
		// observing observer reports them via /status. Prep for network filtering.
		var observerRadio string
		if o.ObserverID != "" {
			tx.QueryRow(`SELECT COALESCE(radio,'') FROM observers WHERE id = ?`, o.ObserverID).Scan(&observerRadio)
		}
		// Count actual advert transmissions, not raw observations: re-floods and
		// multi-observer copies of one advert all arrive within a few seconds, so
		// this counts a new transmission only when it lands >30s after the node's
		// previous advert. (Mirrors analytics.summarizeAdverts.)
		txInc := advertTxIncrement(tx, a.PublicKey, o.ReceivedAt)
		// The advert's path-length byte carries the originating node's own
		// hash size (1, 2, or 3 bytes) — the length of the key prefix by which
		// this node is identified in packet paths. A zero-hop (direct) advert
		// is sent with path_len=0, which always decodes as size 1 regardless of
		// the node's setting, so it carries no usable hash-size signal: feed 0
		// (unknown) for those and let a flood advert (or the periodic consensus
		// pass) establish the real size.
		advertHashSize := 0
		if p.RouteType.IsFlood() {
			advertHashSize = p.PathHashSize
		}
		if _, err := tx.Exec(`
			INSERT INTO nodes
				(pubkey, name, role, latitude, longitude, has_location,
				 first_seen, last_seen, last_advert, advert_count, advert_tx_count, hash_size, radio)
			VALUES (?,?,?,?,?,?,?,?,?,1,?,?,?)
			ON CONFLICT(pubkey) DO UPDATE SET
				name         = COALESCE(NULLIF(excluded.name,''), nodes.name),
				role         = excluded.role,
				latitude     = COALESCE(excluded.latitude, nodes.latitude),
				longitude    = COALESCE(excluded.longitude, nodes.longitude),
				has_location = nodes.has_location | excluded.has_location,
				last_seen    = excluded.last_seen,
				last_advert  = excluded.last_advert,
				advert_count = nodes.advert_count + 1,
				advert_tx_count = nodes.advert_tx_count + ?,
				-- Only set hash_size from an advert while it's still unknown. A
				-- single advert with a corrupt path-length byte must not flip an
				-- established size; the periodic consensus pass (analytics) owns
				-- corrections from there, voting over many adverts.
				hash_size    = CASE WHEN nodes.hash_size = 0 THEN excluded.hash_size ELSE nodes.hash_size END,
				radio        = COALESCE(NULLIF(excluded.radio,''), nodes.radio)`,
			a.PublicKey, nullStr(a.Name), a.DeviceRole.String(),
			lat, lon, boolInt(a.HasLocation), ts, ts, ts, txInc, advertHashSize, nullStr(observerRadio),
			txInc,
		); err != nil {
			return fmt.Errorf("store: upsert node: %w", err)
		}
	}

	return tx.Commit()
}

// advertTxGap is how far apart two adverts must land to count as separate
// transmissions; closer ones are re-flood / multi-observer copies of the same
// broadcast (late reflood through distant hops can trail the first copy by a
// minute). Keep in sync with analytics.advertTxGap (backfill + cadence grouping).
const advertTxGap = 90 * time.Second

// advertTxIncrement returns 1 when this advert (at receivedAt) starts a new
// transmission for the node — a brand-new node, or an advert landing more than
// advertTxGap after the node's previous one — and 0 when it's just another
// observation (re-flood / different observer) of the current transmission.
func advertTxIncrement(tx *sql.Tx, pubkey string, receivedAt time.Time) int {
	var prev string
	tx.QueryRow(`SELECT COALESCE(last_advert,'') FROM nodes WHERE pubkey = ?`, pubkey).Scan(&prev)
	if prev == "" {
		return 1
	}
	pt, err := time.Parse(time.RFC3339Nano, prev)
	if err != nil {
		return 1
	}
	if receivedAt.UTC().Sub(pt) > advertTxGap {
		return 1
	}
	return 0
}

// columnExists reports whether a table already has a column (used to gate
// one-time backfills on a freshly-migrated column).
func columnExists(db *sql.DB, table, col string) bool {
	rows, err := db.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		return false
	}
	defer rows.Close()
	for rows.Next() {
		var cid, notnull, pk int
		var name, ctype string
		var dflt interface{}
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return false
		}
		if name == col {
			return true
		}
	}
	return false
}

func nullStr(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

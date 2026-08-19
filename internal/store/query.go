package store

import (
	"encoding/json"
	"math"
	"sort"
	"strings"

	"github.com/jjkroell/ridgeline/internal/meshcore"
)

// Node is a row from the nodes table, shaped for API responses.
type Node struct {
	PublicKey   string   `json:"publicKey"`
	Name        string   `json:"name"`
	Role        string   `json:"role"`
	Latitude    *float64 `json:"latitude,omitempty"`
	Longitude   *float64 `json:"longitude,omitempty"`
	HasLocation bool     `json:"hasLocation"`
	FirstSeen   string   `json:"firstSeen"`
	LastSeen    string   `json:"lastSeen"`
	LastAdvert  string   `json:"lastAdvert,omitempty"`
	AdvertCount int      `json:"advertCount"`
	// AdvertTxCount is the number of actual advert *transmissions* — re-flood and
	// multi-observer copies of one advert collapsed by a 30s gap — vs AdvertCount
	// which counts every observation.
	AdvertTxCount int `json:"advertTxCount"`
	// HashSize is the node's path-hash length in bytes (1, 2, or 3), learned
	// from its advert. 0 means not yet known.
	HashSize int `json:"hashSize"`
	// RetiredAt is set when the node's verified owner has withdrawn it from the
	// map and node lists. Everything it sent is kept. ListNodes still RETURNS
	// retired nodes — they remain subject to retention and still resolve as relay
	// hops — so the public API filters them out at its own boundary.
	RetiredAt string `json:"retiredAt,omitempty"`
	// GpsSuspect marks a located node whose coordinates can't be plotted: either
	// structurally invalid (null island, out of range) or implausibly far from
	// the rest of the mesh. See flagGpsOutliers.
	GpsSuspect bool `json:"gpsSuspect"`
	// Radio is the node's "freq,bw,sf,cr" config, inherited from the observer
	// that heard it (nodes don't broadcast their own). Empty until known.
	//
	// ⚠ For a node beyond a bridge this describes a receiver on THIS side and is
	// therefore wrong for the node. The API blanks it whenever ViaBridge is set
	// and reports ViaBridgeRadio instead.
	Radio string `json:"radio,omitempty"`
	// ViaBridge is the uppercase pubkey of the sanctioned bridge this node is
	// reached through — set only when the node lives on the far segment, i.e. it
	// is never heard directly and essentially all of its traffic crosses.
	ViaBridge string `json:"viaBridge,omitempty"`
	// ViaBridgeName / ViaBridgeRadio are resolved for display: the bridge's near
	// end, and the far segment's OPERATOR-DECLARED radio config (unobservable —
	// nothing on this side can hear the far side's settings).
	ViaBridgeName  string `json:"viaBridgeName,omitempty"`
	ViaBridgeRadio string `json:"viaBridgeRadio,omitempty"`
	// ViaBridgeConfidence is "confirmed" (>=2-byte path hops proved the crossing)
	// or "probable" (a 1-byte path, where only the far end's byte is unique).
	ViaBridgeConfidence string `json:"viaBridgeConfidence,omitempty"`
}

// gpsFloorKm is the radius around the mesh centroid inside which a node is
// NEVER flagged as an outlier, however tightly the rest of the population is
// clustered. A mesh with one dense metro cluster produces a very small spread,
// and a real regional group sitting outside it is not corrupt GPS — it is
// coverage. Without this floor the north-island group (~330 km out) was being
// hidden from every map by a whisker it missed by under a kilometre.
const gpsFloorKm = 500

// validCoords reports whether a node carries coordinates worth plotting at all.
// Null island is the common corrupt-GPS value — a node that has never had a fix
// reports 0,0 — and it is a structural error, not a statistical one, so it is
// tested directly rather than left to the outlier maths to catch by luck.
func validCoords(lat, lon float64) bool {
	if lat == 0 && lon == 0 {
		return false
	}
	return math.Abs(lat) <= 90 && math.Abs(lon) <= 180
}

// flagGpsOutliers marks located nodes whose coordinates can't be trusted. Two
// separate tests: structurally invalid coordinates always fail, and valid ones
// fail only when they sit both beyond gpsFloorKm and beyond the 3×IQR whisker
// of the population's distances from the mesh centroid.
//
// Distance from a median centroid is one number, so a node that is moderately
// north AND moderately west no longer compounds two independent axis tests into
// a rejection. Invalid coordinates are excluded from the centroid and the
// whisker — 0,0 rows would otherwise drag both, and the flagging of real nodes
// would depend on how many broken ones happened to be on air.
func flagGpsOutliers(nodes []Node) {
	var lats, lons []float64
	for i := range nodes {
		if nodes[i].Latitude == nil || nodes[i].Longitude == nil {
			continue
		}
		lat, lon := *nodes[i].Latitude, *nodes[i].Longitude
		if !validCoords(lat, lon) {
			nodes[i].GpsSuspect = true
			continue
		}
		lats = append(lats, lat)
		lons = append(lons, lon)
	}
	if len(lats) < 8 {
		return // too few to judge a distance outlier
	}
	cLat, cLon := median(lats), median(lons)

	dists := make([]float64, 0, len(lats))
	for i := range lats {
		dists = append(dists, haversineKm(cLat, cLon, lats[i], lons[i]))
	}
	limit := math.Max(gpsFloorKm, iqrUpperWhisker(dists))

	for i := range nodes {
		if nodes[i].Latitude == nil || nodes[i].Longitude == nil || nodes[i].GpsSuspect {
			continue
		}
		if haversineKm(cLat, cLon, *nodes[i].Latitude, *nodes[i].Longitude) > limit {
			nodes[i].GpsSuspect = true
		}
	}
}

// median of v (v is not modified). The centroid is a median rather than a mean
// so a handful of wild coordinates can't drag the centre out to sea.
func median(v []float64) float64 {
	s := append([]float64(nil), v...)
	sort.Float64s(s)
	return s[int(math.Round(float64(len(s)-1)*0.5))]
}

func iqrUpperWhisker(v []float64) float64 {
	s := append([]float64(nil), v...)
	sort.Float64s(s)
	q := func(p float64) float64 { return s[int(math.Round(float64(len(s)-1)*p))] }
	q1, q3 := q(0.25), q(0.75)
	return q3 + 3*(q3-q1)
}

const earthRadiusKm = 6371

// haversineKm is the great-circle distance between two points in kilometres.
func haversineKm(lat1, lon1, lat2, lon2 float64) float64 {
	rad := math.Pi / 180
	dLat := (lat2 - lat1) * rad
	dLon := (lon2 - lon1) * rad
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1*rad)*math.Cos(lat2*rad)*math.Sin(dLon/2)*math.Sin(dLon/2)
	return 2 * earthRadiusKm * math.Asin(math.Min(1, math.Sqrt(a)))
}

// Stats is a high-level snapshot of the database.
type Stats struct {
	Nodes        int    `json:"nodes"`
	Observers    int    `json:"observers"`
	Observations int    `json:"observations"`
	LastPacketAt string `json:"lastPacketAt,omitempty"`
}

// ListNodes returns all known nodes, most recently seen first.
func (s *Store) ListNodes() ([]Node, error) {
	rows, err := s.db.Query(`
		SELECT pubkey, COALESCE(name,''), COALESCE(role,''),
		       latitude, longitude, has_location,
		       first_seen, last_seen, COALESCE(last_advert,''), advert_count, advert_tx_count,
		       COALESCE(hash_size, 0), COALESCE(radio,''), COALESCE(retired_at,''),
		       COALESCE(via_bridge,''), COALESCE(via_bridge_conf,'')
		FROM nodes
		ORDER BY last_seen DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	nodes := []Node{}
	for rows.Next() {
		var n Node
		var hasLoc int
		if err := rows.Scan(&n.PublicKey, &n.Name, &n.Role,
			&n.Latitude, &n.Longitude, &hasLoc,
			&n.FirstSeen, &n.LastSeen, &n.LastAdvert, &n.AdvertCount, &n.AdvertTxCount, &n.HashSize, &n.Radio, &n.RetiredAt,
			&n.ViaBridge, &n.ViaBridgeConfidence); err != nil {
			return nil, err
		}
		n.HasLocation = hasLoc != 0
		nodes = append(nodes, n)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	flagGpsOutliers(nodes)
	if err := s.annotateBridgeSegments(nodes); err != nil {
		return nil, err
	}
	return nodes, nil
}

// annotateBridgeSegments fills in the display fields for nodes beyond a bridge
// and BLANKS their inherited radio.
//
// nodes.radio is copied from whichever observer heard the node. For a far-side
// node that observer is on this side of the bridge, so the value describes the
// listener and is simply wrong for the node — reporting it would state a
// falsehood with more confidence than reporting nothing. The operator-declared
// far-segment config replaces it, and is absent until they declare one.
func (s *Store) annotateBridgeSegments(nodes []Node) error {
	var any bool
	for i := range nodes {
		if nodes[i].ViaBridge != "" {
			any = true
			break
		}
	}
	if !any {
		return nil
	}
	links, err := s.KnownBridgeLinks()
	if err != nil {
		return err
	}
	byNear := make(map[string]BridgeLink, len(links))
	for _, l := range links {
		byNear[l.Near] = l
	}
	for i := range nodes {
		key := nodes[i].ViaBridge
		if key == "" {
			continue
		}
		nodes[i].Radio = ""
		if l, ok := byNear[key]; ok {
			nodes[i].ViaBridgeName = l.NearName
			nodes[i].ViaBridgeRadio = l.PeerRadio
		}
	}
	return nil
}

// Stats returns counts and the most recent packet time.
func (s *Store) Stats() (Stats, error) {
	var st Stats
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM nodes`).Scan(&st.Nodes); err != nil {
		return st, err
	}
	// Retired observers are excluded so this agrees with the observers page,
	// which lists only the active ones.
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM observers`).Scan(&st.Observers); err != nil {
		return st, err
	}
	var last *string
	if err := s.db.QueryRow(
		`SELECT COUNT(*), MAX(received_at) FROM observations`,
	).Scan(&st.Observations, &last); err != nil {
		return st, err
	}
	if last != nil {
		st.LastPacketAt = *last
	}
	return st, nil
}

// Observer is a row from the observers table. Latitude/Longitude are resolved
// by matching the observer's public key to an advertised node, when available.
type Observer struct {
	// ID is the observer's stable identity — its public key (or, for rows that
	// never carried one, its name). Use Name for anything shown to a person.
	ID string `json:"id"`
	// Name is the operator-chosen label. It changes freely and identifies nothing.
	Name         string          `json:"name,omitempty"`
	Region       string          `json:"region"`
	PublicKey    string          `json:"publicKey,omitempty"`
	Latitude     *float64        `json:"latitude,omitempty"`
	Longitude    *float64        `json:"longitude,omitempty"`
	FirstSeen    string          `json:"firstSeen"`
	LastSeen     string          `json:"lastSeen"`
	PacketCount  int             `json:"packetCount"`
	Status       *ObserverStatus `json:"status,omitempty"`
	LastStatusAt string          `json:"lastStatusAt,omitempty"`
	// StandbySince is set while an observer is connected but having its packets
	// discarded at ingest. It does NOT hide the observer — staying visible is the
	// point, so the operator can see the stand-down.
	StandbySince string `json:"standbySince,omitempty"`
	// StandbyDropped counts packets discarded since this daemon started (see
	// Store.StandbyDropped). Only meaningful while StandbySince is set.
	StandbyDropped int64 `json:"standbyDropped,omitempty"`
}

// ObserverStatus is an observer's latest self-reported device telemetry, parsed
// from its /status message (radio config + battery/uptime/noise/airtime/errors).
type ObserverStatus struct {
	State           string   `json:"state,omitempty"` // online | offline
	Radio           string   `json:"radio,omitempty"` // raw "freq,bw,sf,cr"
	FreqMHz         *float64 `json:"freqMhz,omitempty"`
	BandwidthKHz    *float64 `json:"bandwidthKhz,omitempty"`
	SpreadingFactor *int     `json:"spreadingFactor,omitempty"`
	CodingRate      *int     `json:"codingRate,omitempty"`
	Model           string   `json:"model,omitempty"`
	Firmware        string   `json:"firmware,omitempty"`
	ClientVersion   string   `json:"clientVersion,omitempty"`
	BatteryMV       *int     `json:"batteryMv,omitempty"`
	UptimeSecs      *int64   `json:"uptimeSecs,omitempty"`
	NoiseFloor      *float64 `json:"noiseFloor,omitempty"`
	TxAirSecs       *float64 `json:"txAirSecs,omitempty"`
	RxAirSecs       *float64 `json:"rxAirSecs,omitempty"`
	RecvErrors      *int     `json:"recvErrors,omitempty"`
	QueueLen        *int     `json:"queueLen,omitempty"`
}

// ListObservers returns every known observer, most recently active first, with a
// location joined from the nodes table when the observer's key has advertised.
//
// There is no hidden set. Observer retirement was removed in v0.9.9: an observer
// is in service, on standby (visible, badged, packets discarded), or deleted.
// A receiver that simply goes quiet is swept by DeleteStaleObservers and returns
// on its next packet, which is what retirement was really being used for.
func (s *Store) ListObservers() ([]Observer, error) {
	return s.listObservers()
}

func (s *Store) listObservers() ([]Observer, error) {
	rows, err := s.db.Query(`
		SELECT o.id, COALESCE(o.name,''), COALESCE(o.region,''), COALESCE(o.pubkey,''),
		       n.latitude, n.longitude, o.first_seen, o.last_seen, o.packet_count,
		       o.status_json, COALESCE(o.last_status_at,''),
		       COALESCE(o.standby_since,'')
		FROM observers o
		LEFT JOIN nodes n ON n.pubkey = o.pubkey
		ORDER BY o.last_seen DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []Observer{}
	for rows.Next() {
		var o Observer
		var statusJSON *string
		if err := rows.Scan(&o.ID, &o.Name, &o.Region, &o.PublicKey,
			&o.Latitude, &o.Longitude, &o.FirstSeen, &o.LastSeen, &o.PacketCount,
			&statusJSON, &o.LastStatusAt, &o.StandbySince); err != nil {
			return nil, err
		}
		if statusJSON != nil && *statusJSON != "" {
			var st ObserverStatus
			if json.Unmarshal([]byte(*statusJSON), &st) == nil {
				o.Status = &st
			}
		}
		out = append(out, o)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// Discard counts live in memory, so they are joined in after the query.
	dropped := s.StandbyDropped()
	for i := range out {
		if out[i].StandbySince != "" {
			out[i].StandbyDropped = dropped[out[i].ID]
		}
	}
	return out, nil
}

// RecentObservation is a lightweight view of a recent packet sighting.
type RecentObservation struct {
	MessageHash string   `json:"messageHash"`
	RouteType   string   `json:"routeType"`
	PayloadType string   `json:"payloadType"`
	PathHops    int      `json:"pathHops"`
	ObserverID  string   `json:"observerId,omitempty"`
	Region      string   `json:"region,omitempty"`
	SNR         *float64 `json:"snr,omitempty"`
	RSSI        *float64 `json:"rssi,omitempty"`
	ReceivedAt  string   `json:"receivedAt"`
}

// RecentObservations returns the most recent observations, newest first.
func (s *Store) RecentObservations(limit int) ([]RecentObservation, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.db.Query(`
		SELECT message_hash, route_type, payload_type, path_hops,
		       COALESCE(observer_id,''), COALESCE(region,''), snr, rssi, received_at
		FROM observations
		ORDER BY id DESC
		LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []RecentObservation{}
	for rows.Next() {
		var o RecentObservation
		if err := rows.Scan(&o.MessageHash, &o.RouteType, &o.PayloadType, &o.PathHops,
			&o.ObserverID, &o.Region, &o.SNR, &o.RSSI, &o.ReceivedAt); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// RawObservation carries the stored raw packet hex plus reception envelope,
// for callers that re-decode history into the full live-event shape.
type RawObservation struct {
	RawHex     string
	ObserverID string
	Region     string
	SNR        *float64
	RSSI       *float64
	ReceivedAt string // stored RFC3339Nano UTC string
}

// RawWindow returns raw observations received at or after sinceISO, newest
// first, capped at max. Like RecentRaw but allows a larger cap for analytics
// passes (which decode the whole window).
func (s *Store) RawWindow(sinceISO string, max int) ([]RawObservation, error) {
	if max <= 0 || max > 500000 {
		max = 200000
	}
	return s.rawSince(sinceISO, max)
}

// RecentRaw returns raw observations received at or after sinceISO (an
// RFC3339Nano UTC string), newest first, capped at limit.
func (s *Store) RecentRaw(sinceISO string, limit int) ([]RawObservation, error) {
	if limit <= 0 || limit > 5000 {
		limit = 2000
	}
	return s.rawSince(sinceISO, limit)
}

// RawByHash returns every raw observation of ONE transmission (all observer
// copies sharing a message_hash), newest first. Backs the shareable per-packet
// deep link — re-opening a specific packet from a copied URL.
func (s *Store) RawByHash(hash string) ([]RawObservation, error) {
	rows, err := s.db.Query(`
		SELECT raw_hex, COALESCE(observer_id,''), COALESCE(region,''), snr, rssi, received_at
		FROM observations
		WHERE message_hash = ?
		ORDER BY received_at DESC
		LIMIT 500`, hash)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []RawObservation{}
	for rows.Next() {
		var o RawObservation
		if err := rows.Scan(&o.RawHex, &o.ObserverID, &o.Region, &o.SNR, &o.RSSI, &o.ReceivedAt); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// RecentGroupText returns one raw observation per distinct channel message
// (grouped by message_hash), newest first, for GroupText payloads received at
// or after sinceISO. Collapsing the ~N observer copies of each transmission
// keeps a long (24h) channel-history window compact — the channel reader dedups
// by message hash anyway, so it only needs one copy of each.
func (s *Store) RecentGroupText(sinceISO string, limit int) ([]RawObservation, error) {
	if limit <= 0 || limit > 20000 {
		limit = 10000
	}
	rows, err := s.db.Query(`
		SELECT raw_hex, COALESCE(observer_id,''), COALESCE(region,''), snr, rssi, received_at, MAX(id)
		FROM observations
		WHERE payload_type = 'GroupText' AND received_at >= ?
		GROUP BY message_hash
		ORDER BY MAX(id) DESC
		LIMIT ?`, sinceISO, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []RawObservation{}
	for rows.Next() {
		var o RawObservation
		var id int64 // MAX(id): drives newest-per-hash ordering, not returned
		if err := rows.Scan(&o.RawHex, &o.ObserverID, &o.Region, &o.SNR, &o.RSSI, &o.ReceivedAt, &id); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// rawSince runs the shared "raw observations since a timestamp, newest first"
// query used by both RawWindow and RecentRaw.
func (s *Store) rawSince(sinceISO string, limit int) ([]RawObservation, error) {
	rows, err := s.db.Query(`
		SELECT raw_hex, COALESCE(observer_id,''), COALESCE(region,''), snr, rssi, received_at
		FROM observations
		WHERE received_at >= ?
		ORDER BY id DESC
		LIMIT ?`, sinceISO, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []RawObservation{}
	for rows.Next() {
		var o RawObservation
		if err := rows.Scan(&o.RawHex, &o.ObserverID, &o.Region, &o.SNR, &o.RSSI, &o.ReceivedAt); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// RelayHopPrefixesSince returns the set of distinct relay-hop identifiers
// (uppercase hex, as they appear in packet paths) seen in any multi-hop
// observation since sinceISO. It's used by the node-retention sweep to detect a
// node that stopped advertising but is still relaying traffic within the window
// — the hop identifier is the relay's hash-ID prefix (1/2/3 bytes), so a node
// whose pubkey starts with one of these prefixes was relaying. Zero-hop packets
// carry no relay hops and are skipped (path_hops = 0), which also keeps the scan
// cheap since most adverts are zero-hop.
func (s *Store) RelayHopPrefixesSince(sinceISO string) (map[string]bool, error) {
	rows, err := s.db.Query(`SELECT raw_hex FROM observations WHERE received_at >= ? AND path_hops > 0`, sinceISO)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	set := make(map[string]bool)
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		pkt, err := meshcore.DecodeHex(raw)
		if err != nil || pkt == nil {
			continue
		}
		for _, hop := range pkt.RelayPath() {
			if hop != "" {
				set[strings.ToUpper(hop)] = true
			}
		}
	}
	return set, rows.Err()
}

// NeedsAdvertTxBackfill reports whether the advert_tx_count column was just added
// on this open and should be seeded from history.
func (s *Store) NeedsAdvertTxBackfill() bool { return s.needAdvertTxBackfill }

// AdvertObservationsChrono returns every stored advert observation's raw hex and
// reception time, oldest first — for the one-time backfill of advert_tx_count.
func (s *Store) AdvertObservationsChrono() ([]RawObservation, error) {
	rows, err := s.db.Query(`
		SELECT raw_hex, received_at
		FROM observations
		WHERE payload_type = 'Advert'
		ORDER BY received_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []RawObservation{}
	for rows.Next() {
		var o RawObservation
		if err := rows.Scan(&o.RawHex, &o.ReceivedAt); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// AdvertObservationsSince returns FLOOD advert observations received at or after
// sinceISO, oldest first, with raw hex and reception time — for the periodic
// hash-size consensus vote. Only flood adverts carry the originator's configured
// hash size; zero-hop (direct) adverts always encode size 1, so they're filtered
// out here. Scanning only those rows in a bounded window keeps a wide (multi-day)
// pass cheap versus decoding every stored packet.
func (s *Store) AdvertObservationsSince(sinceISO string) ([]RawObservation, error) {
	rows, err := s.db.Query(`
		SELECT raw_hex, received_at
		FROM observations
		WHERE payload_type = 'Advert'
		  AND route_type IN ('Flood', 'TransportFlood')
		  AND received_at >= ?
		ORDER BY received_at ASC`, sinceISO)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []RawObservation{}
	for rows.Next() {
		var o RawObservation
		if err := rows.Scan(&o.RawHex, &o.ReceivedAt); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// SetAdvertTxCounts bulk-updates per-node advert transmission counts (keyed by
// pubkey) in a single transaction.
func (s *Store) SetAdvertTxCounts(counts map[string]int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.Prepare(`UPDATE nodes SET advert_tx_count = ? WHERE pubkey = ?`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for k, v := range counts {
		if _, err := stmt.Exec(v, k); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// SetHashSizes bulk-updates per-node hash sizes (keyed by pubkey) in a single
// transaction. Used by the periodic consensus correction to repair a stored
// hash size that a corrupt advert (flipped path-length byte) flipped at ingest.
func (s *Store) SetHashSizes(sizes map[string]int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.Prepare(`UPDATE nodes SET hash_size = ? WHERE pubkey = ?`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for k, v := range sizes {
		if _, err := stmt.Exec(v, k); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ObserverNames maps each observer's stable id (its public key) to the friendly
// label to show for it. Observations store the id, so anything presenting an
// observer to a person resolves the label through this.
func (s *Store) ObserverNames() (map[string]string, error) {
	rows, err := s.db.Query(`SELECT id, COALESCE(NULLIF(name,''), id) FROM observers`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var id, name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, err
		}
		out[id] = name
	}
	return out, rows.Err()
}

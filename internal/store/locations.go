package store

import (
	"database/sql"
	"errors"
	"strings"
	"time"
)

// PrivateLocation is a node's owner-set exact coordinates. It is sensitive and
// only ever returned to the node's verified owner (or, later, shared users) —
// the API layer enforces that gate; the store just persists it.
type PrivateLocation struct {
	NodePubkey string  `json:"nodePubkey"`
	UserID     int64   `json:"userId"`
	Latitude   float64 `json:"latitude"`
	Longitude  float64 `json:"longitude"`
	Label      string  `json:"label"`
	UpdatedAt  string  `json:"updatedAt"`
}

// GetPrivateLocation returns a node's stored private location, if one is set.
func (s *Store) GetPrivateLocation(nodePubkey string) (PrivateLocation, bool, error) {
	nodePubkey = strings.ToUpper(nodePubkey)
	var p PrivateLocation
	err := s.db.QueryRow(`
		SELECT node_pubkey, user_id, latitude, longitude, label, updated_at
		FROM node_private_locations WHERE node_pubkey = ?`, nodePubkey).
		Scan(&p.NodePubkey, &p.UserID, &p.Latitude, &p.Longitude, &p.Label, &p.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return PrivateLocation{}, false, nil
	}
	if err != nil {
		return PrivateLocation{}, false, err
	}
	return p, true, nil
}

// SetPrivateLocation upserts a node's private location, recording which user set
// it. Ownership is verified by the caller (API layer) before this is invoked.
func (s *Store) SetPrivateLocation(nodePubkey string, userID int64, lat, lon float64, label string) (PrivateLocation, error) {
	nodePubkey = strings.ToUpper(nodePubkey)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.db.Exec(`
		INSERT INTO node_private_locations (node_pubkey, user_id, latitude, longitude, label, updated_at)
		VALUES (?,?,?,?,?,?)
		ON CONFLICT(node_pubkey) DO UPDATE SET
			user_id    = excluded.user_id,
			latitude   = excluded.latitude,
			longitude  = excluded.longitude,
			label      = excluded.label,
			updated_at = excluded.updated_at`,
		nodePubkey, userID, lat, lon, label, now)
	if err != nil {
		return PrivateLocation{}, err
	}
	return PrivateLocation{
		NodePubkey: nodePubkey, UserID: userID, Latitude: lat, Longitude: lon,
		Label: label, UpdatedAt: now,
	}, nil
}

// DeletePrivateLocation removes a node's private location. Returns whether a row
// was removed.
func (s *Store) DeletePrivateLocation(nodePubkey string) (bool, error) {
	nodePubkey = strings.ToUpper(nodePubkey)
	res, err := s.db.Exec(`DELETE FROM node_private_locations WHERE node_pubkey = ?`, nodePubkey)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

package store

import (
	"context"
	"database/sql"
	"errors"
)

// ShareNode is intentionally narrower than Node: no location, radio inference,
// ownership, notes, telemetry or session-dependent data can enter a public preview.
type ShareNode struct {
	PublicKey string
	Name      string
	Role      string
}

// NodeSharePreview performs a single indexed lookup. Retired, quarantined and
// missing nodes all have the same public result. Callers supply an uppercase key.
func (s *Store) NodeSharePreview(ctx context.Context, key string) (*ShareNode, error) {
	if s.IsNodeBlocked(key) {
		return nil, nil
	}
	var n ShareNode
	err := s.db.QueryRowContext(ctx, `SELECT pubkey, COALESCE(name,''), COALESCE(role,'') FROM nodes WHERE pubkey = ? AND COALESCE(retired_at,'') = ''`, key).Scan(&n.PublicKey, &n.Name, &n.Role)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &n, nil
}

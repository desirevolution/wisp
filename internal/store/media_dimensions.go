package store

import (
	"context"
	"strings"
)

// MediaLayoutPayloads reads retained descriptors, including messages saved before
// layout dimensions were exposed. No download or database migration is required.
func (s *Store) MediaLayoutPayloads(ctx context.Context, ids []string) (map[string][]byte, error) {
	out := make(map[string][]byte)
	if len(ids) == 0 {
		return out, nil
	}
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	ph := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	rows, err := s.DB.QueryContext(ctx, `SELECT m.id,COALESCE(p.payload,c.raw_payload) FROM messages m LEFT JOIN media_payload p ON p.message_id=m.id LEFT JOIN message_context c ON c.message_id=m.id WHERE m.id IN (`+ph+`)`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var raw []byte
		if err := rows.Scan(&id, &raw); err != nil {
			return nil, err
		}
		out[id] = raw
	}
	return out, rows.Err()
}

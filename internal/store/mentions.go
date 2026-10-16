package store

import (
	"context"
	"encoding/json"
	"strings"
)

// A present empty list means the original message explicitly had no mentions.
// An absent entry is a legacy row for which WhatsApp context was not retained.
func (s *Store) MessageMentions(ctx context.Context, ids []string) (map[string][]string, error) {
	out := make(map[string][]string)
	if len(ids) == 0 {
		return out, nil
	}
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	rows, err := s.DB.QueryContext(ctx, `SELECT message_id,jids_json FROM message_mentions WHERE message_id IN (`+placeholders+`)`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id, raw string
		if err := rows.Scan(&id, &raw); err != nil {
			return nil, err
		}
		var jids []string
		if err := json.Unmarshal([]byte(raw), &jids); err != nil {
			return nil, err
		}
		out[id] = jids
	}
	return out, rows.Err()
}

// Group participants often have no private chat/contact entry, but Wisp may
// already know their name from a message they sent in this or another group.
func (s *Store) KnownContactName(ctx context.Context, jid string) string {
	var name string
	_ = s.DB.QueryRowContext(ctx, `SELECT COALESCE(name,'') FROM chats WHERE id=? AND type='private'`, jid).Scan(&name)
	if strings.TrimSpace(name) != "" {
		return name
	}
	_ = s.DB.QueryRowContext(ctx, `SELECT sender_name FROM messages WHERE sender_id=? AND TRIM(COALESCE(sender_name,''))<>'' ORDER BY timestamp DESC,id DESC LIMIT 1`, jid).Scan(&name)
	return name
}

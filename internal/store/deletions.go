package store

import (
	"context"
	"wisp/internal/model"
)

type Deletion struct {
	ChatID    string `json:"chat_id"`
	MessageID string `json:"message_id"`
	Timestamp int64  `json:"timestamp"`
}

// Revoke is monotonic and independent of original-message arrival order.
func (s *Store) SaveDeletion(d Deletion, sender string, fromMe bool) (bool, error) {
	tx, err := s.DB.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	res, err := tx.Exec(`INSERT OR IGNORE INTO message_deletions(chat_id,message_id,timestamp) VALUES(?,?,?)`, d.ChatID, d.MessageID, d.Timestamp)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	// Unknown originals get a placeholder without reordering existing chats.
	_, err = tx.Exec(`INSERT OR IGNORE INTO chats(id,name,type,last_message_at,created_at,updated_at) VALUES(?,NULL,CASE WHEN ? LIKE '%@g.us' THEN 'group' ELSE 'private' END,?,?,?)`, d.ChatID, d.ChatID, d.Timestamp, d.Timestamp, d.Timestamp)
	if err != nil {
		return false, err
	}
	_, err = tx.Exec(`INSERT OR IGNORE INTO messages(id,whatsapp_id,chat_id,sender_id,from_me,type,body,has_media,timestamp,created_at) VALUES(?,?,?,?,?,'deleted','',0,?,?)`, d.MessageID, d.MessageID, d.ChatID, sender, fromMe, d.Timestamp, d.Timestamp)
	if err != nil {
		return false, err
	}
	return n > 0, tx.Commit()
}
func (s *Store) IsDeleted(chat, id string) bool {
	var n int
	_ = s.DB.QueryRow(`SELECT COUNT(*) FROM message_deletions WHERE chat_id=? AND message_id=?`, chat, id).Scan(&n)
	return n > 0
}
func (s *Store) Deletions(ctx context.Context, chat string) ([]Deletion, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT chat_id,message_id,timestamp FROM message_deletions WHERE chat_id=?`, chat)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Deletion{}
	for rows.Next() {
		var d Deletion
		if err = rows.Scan(&d.ChatID, &d.MessageID, &d.Timestamp); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}
func (s *Store) DecorateDeletion(m *model.Message) {
	m.Deleted = s.IsDeleted(m.ChatID, m.ID)
	if m.Reply != nil {
		m.Reply.Deleted = s.IsDeleted(m.Reply.ChatID, m.Reply.MessageID)
	}
}

package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"wisp/internal/model"
)

// Capture an original once and reapply the latest edit in the same transaction as
// a history/live save. Late old history can never restore the old message body.
func applyStoredEdits(tx *sql.Tx, chat, id string) error {
	var body, mentions string
	err := tx.QueryRow(`SELECT body,mentions FROM message_edits WHERE chat_id=? AND message_id=? ORDER BY timestamp_ms DESC,digest DESC LIMIT 1`, chat, id).Scan(&body, &mentions)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}
	_, err = tx.Exec(`INSERT OR IGNORE INTO message_edit_originals(chat_id,message_id,body,type,timestamp_ms,mentions) SELECT chat_id,id,COALESCE(body,''),type,timestamp*1000,COALESCE((SELECT jids_json FROM message_mentions WHERE message_id=messages.id),'[]') FROM messages WHERE chat_id=? AND id=? AND type<>'deleted'`, chat, id)
	if err != nil {
		return err
	}
	_, err = tx.Exec(`UPDATE messages SET body=? WHERE chat_id=? AND id=? AND type<>'deleted'`, body, chat, id)
	if err != nil {
		return err
	}
	_, err = tx.Exec(`INSERT INTO message_mentions(message_id,jids_json) SELECT id,? FROM messages WHERE chat_id=? AND id=? AND type<>'deleted' ON CONFLICT(message_id) DO UPDATE SET jids_json=excluded.jids_json`, mentions, chat, id)
	return err
}
func (s *Store) SaveEdit(e model.MessageEdit) (bool, error) {
	tx, err := s.DB.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	mentions, err := json.Marshal(e.MentionJIDs)
	if err != nil {
		return false, err
	}
	// Ignore delivery envelope IDs: history/live can wrap the same edit differently.
	hash := sha256.Sum256(e.Raw)
	digest := hex.EncodeToString(hash[:])
	res, err := tx.Exec(`INSERT OR IGNORE INTO message_edits(chat_id,message_id,timestamp_ms,digest,body,type,mentions,raw_payload) VALUES(?,?,?,?,?,?,?,?)`, e.ChatID, e.MessageID, e.TimestampMS, digest, e.Body, e.Type, string(mentions), e.Raw)
	if err != nil {
		return false, err
	}
	if err = applyStoredEdits(tx, e.ChatID, e.MessageID); err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, tx.Commit()
}
func (s *Store) EditState(ctx context.Context, chat, id string) (*model.EditState, error) {
	tx, err := s.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	state := &model.EditState{ChatID: chat, MessageID: id, History: []model.MessageVersion{}}
	var v model.MessageVersion
	var mentions string
	err = tx.QueryRowContext(ctx, `SELECT body,type,timestamp_ms,mentions FROM message_edit_originals WHERE chat_id=? AND message_id=?`, chat, id).Scan(&v.Body, &v.Type, &v.TimestampMS, &mentions)
	if err != nil && err != sql.ErrNoRows {
		return nil, err
	}
	if err == nil {
		v.Original = true
		_ = json.Unmarshal([]byte(mentions), &v.MentionJIDs)
		state.History = append(state.History, v)
		state.OriginalAvailable = true
		state.Version++
	}
	rows, err := tx.QueryContext(ctx, `SELECT body,type,timestamp_ms,mentions,raw_payload FROM message_edits WHERE chat_id=? AND message_id=? ORDER BY timestamp_ms,digest`, chat, id)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var item model.MessageVersion
		var raw []byte
		if err = rows.Scan(&item.Body, &item.Type, &item.TimestampMS, &mentions, &raw); err != nil {
			rows.Close()
			return nil, err
		}
		if err = json.Unmarshal([]byte(mentions), &item.MentionJIDs); err != nil {
			rows.Close()
			return nil, err
		}
		state.History = append(state.History, item)
		state.Body = item.Body
		state.EditedAt = item.TimestampMS
		state.MentionJIDs = item.MentionJIDs
		state.CurrentRaw = raw
		state.Version++
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	// The newest snapshot is already visible as the message itself.
	if state.EditedAt > 0 {
		state.History = state.History[:len(state.History)-1]
	}
	return state, tx.Commit()
}
func (s *Store) LatestEditTime(chat, id string) int64 {
	var ts int64
	_ = s.DB.QueryRow(`SELECT COALESCE(MAX(timestamp_ms),0) FROM message_edits WHERE chat_id=? AND message_id=?`, chat, id).Scan(&ts)
	return ts
}
func (s *Store) DecorateEdit(m *model.Message) error {
	state, err := s.EditState(context.Background(), m.ChatID, m.ID)
	if err != nil {
		return err
	}
	if state.EditedAt > 0 {
		m.Edited = true
		m.Body = state.Body
		m.EditedAt = state.EditedAt
		m.EditVersion = state.Version
		m.EditHistory = state.History
		m.EditOriginalAvailable = state.OriginalAvailable
		m.CurrentRaw = state.CurrentRaw
		m.MentionJIDs = state.MentionJIDs
	}
	if m.Reply != nil {
		chat := m.Reply.ChatID
		if chat == "" {
			chat = m.ChatID
		}
		m.Reply.OriginalEdited = s.LatestEditTime(chat, m.Reply.MessageID) > m.Timestamp*1000
	}
	return nil
}

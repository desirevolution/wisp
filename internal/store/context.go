package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"wisp/internal/model"
)

func (s *Store) LoadMessageContext(m *model.Message) error {
	defer s.DecorateDeletion(m)
	var raw string
	err := s.DB.QueryRow(`SELECT raw_payload,info_json,history_payload,original_chat,original_sender,reply_json FROM message_context WHERE message_id=?`, m.ID).Scan(&m.Raw, &m.OriginalInfo, &m.HistoryRaw, &m.OriginalChat, &m.OriginalSender, &raw)
	if err == sql.ErrNoRows {
		return s.DecorateEdit(m)
	}
	if err != nil {
		return err
	}
	if err := json.Unmarshal([]byte(raw), &m.Reply); err != nil {
		return err
	}
	return s.DecorateEdit(m)
}

// Empty emoji is a retained tombstone: old history must not resurrect a removal.
func (s *Store) SaveReaction(r model.Reaction) (bool, error) {
	tx, err := s.DB.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	result, err := tx.Exec(`INSERT INTO reactions(chat_id,message_id,actor_id,actor_name,from_me,emoji,timestamp_ms,event_id,raw_payload,raw_kind) VALUES(?,?,?,?,?,?,?,?,?,?) ON CONFLICT(chat_id,message_id,actor_id) DO UPDATE SET actor_name=excluded.actor_name,from_me=excluded.from_me,emoji=excluded.emoji,timestamp_ms=excluded.timestamp_ms,event_id=excluded.event_id,raw_payload=excluded.raw_payload,raw_kind=excluded.raw_kind WHERE excluded.timestamp_ms>reactions.timestamp_ms OR (excluded.timestamp_ms=reactions.timestamp_ms AND excluded.event_id>reactions.event_id)`, r.ChatID, r.MessageID, r.ActorID, r.ActorName, r.FromMe, r.Emoji, r.TimestampMS, r.EventID, r.Raw, r.RawKind)
	if err != nil {
		return false, err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	if n > 0 {
		if _, err = tx.Exec(`INSERT INTO reaction_versions(chat_id,message_id,version) VALUES(?,?,1) ON CONFLICT(chat_id,message_id) DO UPDATE SET version=version+1`, r.ChatID, r.MessageID); err != nil {
			return false, err
		}
	}
	return n > 0, tx.Commit()
}
func (s *Store) Reactions(ctx context.Context, chat, id string) ([]model.Reaction, error) {
	state, err := s.ReactionState(ctx, chat, id)
	if err != nil {
		return nil, err
	}
	return state.Reactions, nil
}
func (s *Store) ReactionState(ctx context.Context, chat, id string) (*model.ReactionState, error) {
	tx, err := s.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	state := &model.ReactionState{ChatID: chat, MessageID: id, Reactions: []model.Reaction{}}
	err = tx.QueryRowContext(ctx, `SELECT version FROM reaction_versions WHERE chat_id=? AND message_id=?`, chat, id).Scan(&state.Version)
	if err != nil && err != sql.ErrNoRows {
		return nil, err
	}
	rows, err := tx.QueryContext(ctx, `SELECT actor_id,actor_name,from_me,emoji,timestamp_ms FROM reactions WHERE chat_id=? AND message_id=? AND emoji<>'' ORDER BY timestamp_ms,actor_id`, chat, id)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		r := model.Reaction{ChatID: chat, MessageID: id}
		if err := rows.Scan(&r.ActorID, &r.ActorName, &r.FromMe, &r.Emoji, &r.TimestampMS); err != nil {
			rows.Close()
			return nil, err
		}
		state.Reactions = append(state.Reactions, r)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	return state, tx.Commit()
}
func (s *Store) ReactionTimestamp(chat, id, actor string) int64 {
	var ts int64
	_ = s.DB.QueryRow(`SELECT timestamp_ms FROM reactions WHERE chat_id=? AND message_id=? AND actor_id=?`, chat, id, actor).Scan(&ts)
	return ts
}
func (s *Store) EnrichMessageRows(ctx context.Context, rows []map[string]any) error {
	for _, row := range rows {
		id, _ := row["id"].(string)
		chat, _ := row["chat_id"].(string)
		if id == "" || chat == "" {
			continue
		}
		var raw string
		err := s.DB.QueryRowContext(ctx, `SELECT reply_json FROM message_context WHERE message_id=?`, id).Scan(&raw)
		if err != nil && err != sql.ErrNoRows {
			return err
		}
		if raw != "" && raw != "null" {
			var q model.Quote
			if err := json.Unmarshal([]byte(raw), &q); err != nil {
				return err
			}
			q.Deleted = s.IsDeleted(q.ChatID, q.MessageID)
			row["reply"] = &q
		}
		state, err := s.ReactionState(ctx, chat, id)
		if err != nil {
			return err
		}
		edit, err := s.EditState(ctx, chat, id)
		if err != nil {
			return err
		}
		row["edited"] = edit.EditedAt > 0
		row["edited_at"] = edit.EditedAt
		row["edit_version"] = edit.Version
		row["edit_history"] = edit.History
		row["edit_original_available"] = edit.OriginalAvailable
		if edit.EditedAt > 0 {
			row["body"] = edit.Body
		}
		if q, ok := row["reply"].(*model.Quote); ok {
			qchat := q.ChatID
			if qchat == "" {
				qchat = chat
			}
			ts, _ := row["timestamp"].(int64)
			q.OriginalEdited = s.LatestEditTime(qchat, q.MessageID) > ts*1000
		}
		row["deleted"] = s.IsDeleted(chat, id)
		row["reactions"] = state.Reactions
		row["reaction_version"] = state.Version
	}
	return nil
}

// Reconcile an old LID identity once WhatsApp has supplied its phone mapping.
func (s *Store) ReactionActors(ctx context.Context, chat, id string) ([]string, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT actor_id FROM reactions WHERE chat_id=? AND message_id=?`, chat, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var actor string
		if err := rows.Scan(&actor); err != nil {
			return nil, err
		}
		ids = append(ids, actor)
	}
	return ids, rows.Err()
}
func (s *Store) MergeReactor(chat, id, alias, canonical string) error {
	if alias == canonical {
		return nil
	}
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.Exec(`INSERT INTO reactions(chat_id,message_id,actor_id,actor_name,from_me,emoji,timestamp_ms,event_id,raw_payload,raw_kind) SELECT chat_id,message_id,?,actor_name,from_me,emoji,timestamp_ms,event_id,raw_payload,raw_kind FROM reactions WHERE chat_id=? AND message_id=? AND actor_id=? ON CONFLICT(chat_id,message_id,actor_id) DO UPDATE SET actor_name=excluded.actor_name,from_me=excluded.from_me,emoji=excluded.emoji,timestamp_ms=excluded.timestamp_ms,event_id=excluded.event_id,raw_payload=excluded.raw_payload,raw_kind=excluded.raw_kind WHERE excluded.timestamp_ms>reactions.timestamp_ms OR (excluded.timestamp_ms=reactions.timestamp_ms AND excluded.event_id>reactions.event_id)`, canonical, chat, id, alias)
	if err != nil {
		return err
	}
	res, err := tx.Exec(`DELETE FROM reactions WHERE chat_id=? AND message_id=? AND actor_id=?`, chat, id, alias)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n > 0 {
		if _, err = tx.Exec(`INSERT INTO reaction_versions(chat_id,message_id,version) VALUES(?,?,1) ON CONFLICT(chat_id,message_id) DO UPDATE SET version=version+1`, chat, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

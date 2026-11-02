package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

const (
	ReadModeLocal    = "wisp"
	ReadModeSelf     = "devices"
	ReadModeReceipts = "receipts"
)

func ValidReadMode(mode string) bool {
	return mode == ReadModeLocal || mode == ReadModeSelf || mode == ReadModeReceipts
}
func (s *Store) ReadMode() string {
	mode := s.ConfigString("readMode", ReadModeLocal)
	if !ValidReadMode(mode) {
		return ReadModeLocal
	}
	return mode
}

// Persist the original WhatsApp addresses: the displayed chat may have a PN/LID alias.
var ErrInvalidReadBoundary = errors.New("invalid read message boundary")

type ReadSyncMessage struct {
	FromMe      bool   `json:"from_me,omitempty"`
	SkipReceipt bool   `json:"skip_receipt,omitempty"`
	Boundary    bool   `json:"boundary,omitempty"`
	Through     int64  `json:"through,omitempty"`
	ID          string `json:"id"`
	Chat        string `json:"chat"`
	Sender      string `json:"sender"`
	Timestamp   int64  `json:"timestamp"`
}
type ReadSyncJob struct {
	ID         int64
	Chat, Mode string
	Messages   []ReadSyncMessage
}

func queueReadSync(tx *sql.Tx, chat string, after, through int64, lastID string) error {
	var boundary ReadSyncMessage
	var boundaryLocalID string
	if lastID != "" {
		// Resolve only within this chat; never trust an ID from another conversation.
		var sequence sql.NullInt64
		var kind string
		var id sql.NullString
		err := tx.QueryRow(`SELECT m.id,m.whatsapp_id,COALESCE(NULLIF(c.original_chat,''),m.chat_id),COALESCE(NULLIF(c.original_sender,''),m.sender_id,''),m.timestamp,m.from_me,u.sequence,m.type FROM messages m LEFT JOIN message_context c ON c.message_id=m.id LEFT JOIN unread_messages u ON u.message_id=m.id WHERE m.chat_id=? AND m.id=?`, chat, lastID).Scan(&boundaryLocalID, &id, &boundary.Chat, &boundary.Sender, &boundary.Timestamp, &boundary.FromMe, &sequence, &kind)
		if err == sql.ErrNoRows {
			return ErrInvalidReadBoundary
		}
		if err != nil {
			return err
		}
		if !id.Valid || id.String == "" || (sequence.Valid && sequence.Int64 > through) {
			return ErrInvalidReadBoundary
		}
		boundary.SkipReceipt = kind == "deleted"
		boundary.ID = id.String
		boundary.Boundary = true
		boundary.Through = through
	}
	if through < after {
		return nil
	}
	var mode string
	err := tx.QueryRow(`SELECT value FROM app_config WHERE key='readMode'`).Scan(&mode)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}
	if mode != ReadModeSelf && mode != ReadModeReceipts {
		return nil
	}
	if lastID != "" {
		// Deduplicate only pending work. Explicitly reopening an already-sent chat
		// may repair its native unread state without replaying on every render.
		var found int
		err = tx.QueryRow(`SELECT 1 FROM read_sync_jobs WHERE chat_id=? AND mode=? AND json_extract(payload,'$[#-1].boundary')=1 AND json_extract(payload,'$[#-1].id')=? AND COALESCE(json_extract(payload,'$[#-1].through'),0)=? LIMIT 1`, chat, mode, boundary.ID, through).Scan(&found)
		if err == nil {
			return nil
		}
		if err != sql.ErrNoRows {
			return err
		}
	}
	query := `SELECT m.whatsapp_id,COALESCE(NULLIF(c.original_chat,''),m.chat_id),COALESCE(NULLIF(c.original_sender,''),m.sender_id,''),m.timestamp FROM unread_messages u JOIN messages m ON m.id=u.message_id LEFT JOIN message_context c ON c.message_id=m.id WHERE u.chat_id=? AND u.sequence>? AND u.sequence<=? AND m.from_me=0 AND m.type<>'deleted'`
	args := []any{chat, after, through}
	if lastID != "" {
		query += ` AND (m.timestamp<? OR (m.timestamp=? AND m.id<=?))`
		args = append(args, boundary.Timestamp, boundary.Timestamp, boundaryLocalID)
	}
	query += ` ORDER BY u.sequence`
	rows, err := tx.Query(query, args...)
	if err != nil {
		return err
	}
	messages := []ReadSyncMessage{}
	for rows.Next() {
		var m ReadSyncMessage
		var id sql.NullString
		if err = rows.Scan(&id, &m.Chat, &m.Sender, &m.Timestamp); err != nil {
			rows.Close()
			return err
		}
		m.ID = id.String
		if m.ID != "" {
			messages = append(messages, m)
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	if lastID != "" {
		// At least the latest incoming message needs an own-device receipt even if
		// it was already read locally or imported as history (no unread ledger row).
		var latest ReadSyncMessage
		var id sql.NullString
		err = tx.QueryRow(`SELECT m.whatsapp_id,COALESCE(NULLIF(c.original_chat,''),m.chat_id),COALESCE(NULLIF(c.original_sender,''),m.sender_id,''),m.timestamp FROM messages m LEFT JOIN message_context c ON c.message_id=m.id WHERE m.chat_id=? AND m.from_me=0 AND m.type<>'deleted' AND (m.timestamp<? OR (m.timestamp=? AND m.id<=?)) AND NOT EXISTS(SELECT 1 FROM unread_messages u WHERE u.message_id=m.id AND u.sequence>?) ORDER BY m.timestamp DESC,m.id DESC LIMIT 1`, chat, boundary.Timestamp, boundary.Timestamp, boundaryLocalID, through).Scan(&id, &latest.Chat, &latest.Sender, &latest.Timestamp)
		if err != nil && err != sql.ErrNoRows {
			return err
		}
		if err == nil && id.Valid && id.String != "" {
			latest.ID = id.String
			found := false
			for _, m := range messages {
				if m.ID == latest.ID {
					found = true
					break
				}
			}
			if !found {
				messages = append(messages, latest)
			}
		}
		// Keep the exact displayed boundary last, with the correct own-message flag.
		filtered := messages[:0]
		for _, m := range messages {
			if m.ID != boundary.ID {
				filtered = append(filtered, m)
			}
		}
		messages = append(filtered, boundary)
	}
	if len(messages) == 0 {
		return nil
	}
	raw, err := json.Marshal(messages)
	if err != nil {
		return err
	}
	_, err = tx.Exec(`INSERT INTO read_sync_jobs(chat_id,mode,payload) VALUES(?,?,?)`, chat, mode, string(raw))
	return err
}

func (s *Store) ReadSyncJobs() ([]ReadSyncJob, error) {
	rows, err := s.DB.Query(`SELECT id,chat_id,mode,payload FROM read_sync_jobs ORDER BY id LIMIT 100`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	jobs := []ReadSyncJob{}
	for rows.Next() {
		var j ReadSyncJob
		var raw string
		if err = rows.Scan(&j.ID, &j.Chat, &j.Mode, &raw); err != nil {
			return nil, err
		}
		if err = json.Unmarshal([]byte(raw), &j.Messages); err != nil {
			return nil, err
		}
		jobs = append(jobs, j)
	}
	return jobs, rows.Err()
}
func (s *Store) CancelReadSync() error {
	_, err := s.DB.Exec(`DELETE FROM read_sync_jobs`)
	return err
}
func (s *Store) DeleteReadSyncJob(id int64) error {
	_, err := s.DB.Exec(`DELETE FROM read_sync_jobs WHERE id=?`, id)
	return err
}

// Changing a privacy mode cancels queued work from the previous mode. Enabling
// sync never retroactively sends receipts for messages read in local-only mode.
func (s *Store) SetReadMode(mode string) error {
	if !ValidReadMode(mode) {
		return fmt.Errorf("invalid read mode")
	}
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var old string
	err = tx.QueryRow(`SELECT value FROM app_config WHERE key='readMode'`).Scan(&old)
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	if old == mode {
		return nil
	}
	if _, err = tx.Exec(`DELETE FROM read_sync_jobs`); err != nil {
		return err
	}
	if _, err = tx.Exec(`INSERT INTO app_config(key,value,updated_at) VALUES('readMode',?,unixepoch()) ON CONFLICT(key) DO UPDATE SET value=excluded.value,updated_at=excluded.updated_at`, mode); err != nil {
		return err
	}
	return tx.Commit()
}

// Resolve a read boundary to our monotonic live sequence, never to "everything
// currently unread". Unknown IDs are ignored. Timestamp-only app-state updates
// use a strict boundary because WhatsApp message timestamps have second precision.
func (s *Store) RemoteReadBoundary(chat string, ids []string, before, readAt int64) (ChatState, error) {
	chat = s.ResolveChat(chat)
	var through int64
	for _, id := range ids {
		var sequence int64
		err := s.DB.QueryRow(`SELECT COALESCE(MAX(u.sequence),0) FROM unread_messages u JOIN messages m ON m.id=u.message_id WHERE u.chat_id=? AND m.from_me=0 AND (m.whatsapp_id=? OR m.id=?)`, chat, id, id).Scan(&sequence)
		if err != nil {
			return ChatState{}, err
		}
		if sequence == 0 && readAt > 0 {
			// Native clients may describe a whole-chat read using their last own
			// message. Resolve that exact stored key, including same-second order.
			err = s.DB.QueryRow(`SELECT COALESCE(MAX(u.sequence),0) FROM messages boundary JOIN messages m ON m.chat_id=boundary.chat_id JOIN unread_messages u ON u.message_id=m.id WHERE boundary.chat_id=? AND (boundary.id=? OR boundary.whatsapp_id=?) AND boundary.from_me=1 AND (m.timestamp<boundary.timestamp OR (m.timestamp=boundary.timestamp AND m.id<=boundary.id)) AND m.created_at<?`, chat, id, id, readAt).Scan(&sequence)
			if err != nil {
				return ChatState{}, err
			}
		}
		if sequence > through {
			through = sequence
		}
	}
	if before > 0 && readAt > 0 && through == 0 {
		rows, err := s.DB.Query(`SELECT u.sequence,m.timestamp,m.created_at FROM unread_messages u JOIN messages m ON m.id=u.message_id WHERE u.chat_id=? ORDER BY u.sequence`, chat)
		if err != nil {
			return ChatState{}, err
		}
		for rows.Next() {
			var sequence, timestamp, created int64
			if err = rows.Scan(&sequence, &timestamp, &created); err != nil {
				rows.Close()
				return ChatState{}, err
			}
			// Stop at the first ambiguous boundary, never jump across a newer arrival.
			if timestamp >= before || created >= readAt {
				break
			}
			through = sequence
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return ChatState{}, err
		}
	}
	return s.MarkChatRead(chat, through)
}

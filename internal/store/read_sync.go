package store

import (
	"database/sql"
	"encoding/json"
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
type ReadSyncMessage struct {
	ID        string `json:"id"`
	Chat      string `json:"chat"`
	Sender    string `json:"sender"`
	Timestamp int64  `json:"timestamp"`
}
type ReadSyncJob struct {
	ID         int64
	Chat, Mode string
	Messages   []ReadSyncMessage
}

func queueReadSync(tx *sql.Tx, chat string, after, through int64) error {
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
	rows, err := tx.Query(`SELECT m.whatsapp_id,COALESCE(NULLIF(c.original_chat,''),m.chat_id),COALESCE(NULLIF(c.original_sender,''),m.sender_id,''),m.timestamp FROM unread_messages u JOIN messages m ON m.id=u.message_id LEFT JOIN message_context c ON c.message_id=m.id WHERE u.chat_id=? AND u.sequence>? AND u.sequence<=? AND m.from_me=0 AND m.type<>'deleted' ORDER BY u.sequence`, chat, after, through)
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
	if err != nil || len(messages) == 0 {
		return err
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

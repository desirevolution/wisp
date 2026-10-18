package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Chat state is Wisp-only. Keep it in app_config so existing databases need no
// schema migration, and retain draft tombstones even when a chat is pruned.
const chatStatePrefix = "chat-state:"
const openChatSQL = `NOT EXISTS (SELECT 1 FROM app_config p WHERE p.key='chat-state:'||c.id AND json_extract(p.value,'$.closed')=1)`

type ChatState struct {
	ChatID        string `json:"chat_id"`
	Closed        bool   `json:"closed"`
	MuteUntil     int64  `json:"mute_until"` // -1 forever, 0 off, otherwise Unix seconds
	DraftRevision int64  `json:"draft_revision"`
	Version       int64  `json:"version"`
}

func (p ChatState) MutedAt(now int64) bool { return p.MuteUntil == -1 || p.MuteUntil > now }
func readChatState(q interface{ QueryRow(string, ...any) *sql.Row }, chat string) (ChatState, error) {
	p := ChatState{ChatID: chat}
	var raw string
	err := q.QueryRow(`SELECT value FROM app_config WHERE key=?`, chatStatePrefix+chat).Scan(&raw)
	if err == sql.ErrNoRows {
		return p, nil
	}
	if err != nil {
		return p, err
	}
	err = json.Unmarshal([]byte(raw), &p)
	p.ChatID = chat
	return p, err
}
func (s *Store) ChatState(chat string) (ChatState, error) {
	return readChatState(s.DB, s.ResolveChat(chat))
}
func (s *Store) ChatStates() ([]ChatState, error) {
	rows, err := s.DB.Query(`SELECT value FROM app_config WHERE substr(key,1,?)=?`, len(chatStatePrefix), chatStatePrefix)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ChatState{}
	for rows.Next() {
		var raw string
		var p ChatState
		if err = rows.Scan(&raw); err != nil {
			return nil, err
		}
		if err = json.Unmarshal([]byte(raw), &p); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
func writeChatState(tx *sql.Tx, p ChatState) error {
	raw, err := json.Marshal(p)
	if err != nil {
		return err
	}
	_, err = tx.Exec(`INSERT INTO app_config(key,value,updated_at) VALUES(?,?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value,updated_at=excluded.updated_at`, chatStatePrefix+p.ChatID, string(raw), time.Now().Unix())
	return err
}

// ChangeChatState serializes independent actions to avoid losing mute/close or
// draft-revision updates when several devices act at the same time.
func (s *Store) ChangeChatState(chat, action string, until int64) (ChatState, error) {
	chat = s.ResolveChat(chat)
	tx, err := s.DB.Begin()
	if err != nil {
		return ChatState{}, err
	}
	defer tx.Rollback()
	p, err := readChatState(tx, chat)
	if err != nil {
		return p, err
	}
	var exists int
	if err = tx.QueryRow(`SELECT 1 FROM chats WHERE id=?`, chat).Scan(&exists); err != nil {
		return p, err
	}
	switch action {
	case "close":
		p.Closed = true
		p.DraftRevision++
	case "open":
		p.Closed = false
	case "mute":
		if until < -1 {
			return p, fmt.Errorf("invalid mute deadline")
		}
		p.MuteUntil = until
	default:
		return p, fmt.Errorf("invalid chat action")
	}
	p.Version++
	if err = writeChatState(tx, p); err != nil {
		return p, err
	}
	return p, tx.Commit()
}

// Called only for a newly accepted live message, never history, edits or reactions.
func (s *Store) LiveChatMessage(chat string) (ChatState, bool, error) {
	chat = s.ResolveChat(chat)
	tx, err := s.DB.Begin()
	if err != nil {
		return ChatState{}, false, err
	}
	defer tx.Rollback()
	p, err := readChatState(tx, chat)
	if err != nil {
		return p, false, err
	}
	changed := p.Closed && !p.MutedAt(time.Now().Unix())
	if changed {
		p.Closed = false
		p.Version++
		if err = writeChatState(tx, p); err != nil {
			return p, false, err
		}
	}
	return p, changed, tx.Commit()
}
func (s *Store) ChatMuted(chat string) bool {
	p, err := s.ChatState(chat)
	return err != nil || p.MutedAt(time.Now().Unix())
}
func (s *Store) OpenChat(chat, name, kind string) error {
	if strings.TrimSpace(chat) == "" {
		return fmt.Errorf("empty chat")
	}
	now := time.Now().Unix()
	_, err := s.DB.Exec(`INSERT INTO chats(id,name,type,last_message_at,created_at,updated_at) VALUES(?,?,?,0,?,?) ON CONFLICT(id) DO NOTHING`, chat, name, kind, now, now)
	return err
}

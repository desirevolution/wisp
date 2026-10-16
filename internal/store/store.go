package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"wisp/internal/model"
)

// SchemaVersion is an internal storage version, independent of app releases.
const SchemaVersion = 150

type Store struct {
	DB         *sql.DB
	overrideMu sync.RWMutex
	overrides  map[string]string
}

func Open(path string) (*Store, error) {
	db, e := sql.Open("sqlite3", "file:"+path+"?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(10000)")
	if e != nil {
		return nil, e
	}
	db.SetMaxOpenConns(1)
	s := &Store{DB: db, overrides: make(map[string]string)}
	e = s.init()
	if e != nil {
		_ = db.Close()
		return nil, e
	}
	return s, nil
}
func (s *Store) init() error {
	// Schema initialization is atomic. Never reset an incompatible database.
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var version, existing int
	if err := tx.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		return err
	}
	if err := tx.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='messages'`).Scan(&existing); err != nil {
		return err
	}
	if version > SchemaVersion || (existing > 0 && version != SchemaVersion) {
		return fmt.Errorf("unsupported Wisp database schema %d (expected %d); keep your data, use a compatible Wisp version or restore a complete backup; see docs/operations.md", version, SchemaVersion)
	}
	_, e := tx.Exec(`
CREATE TABLE IF NOT EXISTS chats(id TEXT PRIMARY KEY,name TEXT,type TEXT NOT NULL,last_message_at INTEGER,created_at INTEGER NOT NULL,updated_at INTEGER NOT NULL,history_exhausted INTEGER NOT NULL DEFAULT 0);
CREATE TABLE IF NOT EXISTS chat_aliases(alias TEXT PRIMARY KEY,chat_id TEXT NOT NULL,kind TEXT,FOREIGN KEY(chat_id) REFERENCES chats(id) ON DELETE CASCADE);
CREATE TABLE IF NOT EXISTS messages(id TEXT PRIMARY KEY,whatsapp_id TEXT UNIQUE,chat_id TEXT NOT NULL,sender_id TEXT,sender_name TEXT,from_me INTEGER NOT NULL DEFAULT 0,type TEXT NOT NULL,body TEXT,has_media INTEGER NOT NULL DEFAULT 0,media_mime TEXT,media_filename TEXT,timestamp INTEGER NOT NULL,created_at INTEGER NOT NULL,FOREIGN KEY(chat_id) REFERENCES chats(id));
CREATE TABLE IF NOT EXISTS message_edits(chat_id TEXT NOT NULL,message_id TEXT NOT NULL,timestamp_ms INTEGER NOT NULL,digest TEXT NOT NULL,body TEXT NOT NULL,type TEXT NOT NULL,mentions TEXT NOT NULL,raw_payload BLOB NOT NULL,PRIMARY KEY(chat_id,message_id,timestamp_ms,digest));
CREATE TABLE IF NOT EXISTS message_edit_originals(chat_id TEXT NOT NULL,message_id TEXT NOT NULL,body TEXT NOT NULL,type TEXT NOT NULL,timestamp_ms INTEGER NOT NULL,mentions TEXT NOT NULL,PRIMARY KEY(chat_id,message_id));
CREATE TRIGGER IF NOT EXISTS delete_message_edits AFTER DELETE ON messages BEGIN DELETE FROM message_edits WHERE chat_id=OLD.chat_id AND message_id=OLD.id; DELETE FROM message_edit_originals WHERE chat_id=OLD.chat_id AND message_id=OLD.id; END;
CREATE TRIGGER IF NOT EXISTS delete_chat_edits AFTER DELETE ON chats BEGIN DELETE FROM message_edits WHERE chat_id=OLD.id; DELETE FROM message_edit_originals WHERE chat_id=OLD.id; END;
CREATE TABLE IF NOT EXISTS message_deletions(chat_id TEXT NOT NULL,message_id TEXT NOT NULL,timestamp INTEGER NOT NULL,PRIMARY KEY(chat_id,message_id));
CREATE TRIGGER IF NOT EXISTS delete_message_deletion AFTER DELETE ON messages BEGIN DELETE FROM message_deletions WHERE chat_id=OLD.chat_id AND message_id=OLD.id; END;
CREATE TABLE IF NOT EXISTS message_context(message_id TEXT PRIMARY KEY,raw_payload BLOB,info_json BLOB,history_payload BLOB,original_chat TEXT NOT NULL,original_sender TEXT NOT NULL,reply_json TEXT NOT NULL,FOREIGN KEY(message_id) REFERENCES messages(id) ON DELETE CASCADE);
CREATE TABLE IF NOT EXISTS reaction_versions(chat_id TEXT NOT NULL,message_id TEXT NOT NULL,version INTEGER NOT NULL,PRIMARY KEY(chat_id,message_id));
CREATE TABLE IF NOT EXISTS reactions(chat_id TEXT NOT NULL,message_id TEXT NOT NULL,actor_id TEXT NOT NULL,actor_name TEXT NOT NULL,from_me INTEGER NOT NULL,emoji TEXT NOT NULL,timestamp_ms INTEGER NOT NULL,event_id TEXT NOT NULL,raw_payload BLOB,raw_kind TEXT NOT NULL,PRIMARY KEY(chat_id,message_id,actor_id));
CREATE INDEX IF NOT EXISTS idx_reactions_message ON reactions(chat_id,message_id);
CREATE TRIGGER IF NOT EXISTS delete_message_reactions AFTER DELETE ON messages BEGIN DELETE FROM reactions WHERE chat_id=OLD.chat_id AND message_id=OLD.id; DELETE FROM reaction_versions WHERE chat_id=OLD.chat_id AND message_id=OLD.id; END;
CREATE TRIGGER IF NOT EXISTS delete_chat_reactions AFTER DELETE ON chats BEGIN DELETE FROM reactions WHERE chat_id=OLD.id; DELETE FROM reaction_versions WHERE chat_id=OLD.id; END;
CREATE TABLE IF NOT EXISTS message_mentions(message_id TEXT PRIMARY KEY,jids_json TEXT NOT NULL,FOREIGN KEY(message_id) REFERENCES messages(id) ON DELETE CASCADE);
CREATE INDEX IF NOT EXISTS idx_messages_chat_time ON messages(chat_id,timestamp DESC,id DESC);
CREATE INDEX IF NOT EXISTS idx_messages_sender ON messages(sender_id);
CREATE TABLE IF NOT EXISTS media(id TEXT PRIMARY KEY,message_id TEXT NOT NULL UNIQUE,chat_id TEXT NOT NULL,type TEXT NOT NULL,status TEXT NOT NULL DEFAULT 'pending',mime_type TEXT,filename TEXT,path TEXT,size INTEGER,error TEXT,created_at INTEGER NOT NULL,updated_at INTEGER NOT NULL,FOREIGN KEY(message_id) REFERENCES messages(id) ON DELETE CASCADE);
CREATE INDEX IF NOT EXISTS idx_media_chat ON media(chat_id,type);
CREATE INDEX IF NOT EXISTS idx_media_chat_status_type ON media(chat_id,status,type);
CREATE TABLE IF NOT EXISTS media_payload(message_id TEXT PRIMARY KEY,payload BLOB NOT NULL,FOREIGN KEY(message_id) REFERENCES messages(id) ON DELETE CASCADE);
CREATE TABLE IF NOT EXISTS app_config(key TEXT PRIMARY KEY,value TEXT NOT NULL,updated_at INTEGER NOT NULL);
CREATE TABLE IF NOT EXISTS push_subscriptions(endpoint TEXT PRIMARY KEY,subscription_json TEXT NOT NULL,created_at INTEGER NOT NULL,updated_at INTEGER NOT NULL);
`)
	if e != nil {
		return e
	}
	if _, err := tx.Exec(fmt.Sprintf("PRAGMA user_version=%d", SchemaVersion)); err != nil {
		return err
	}
	return tx.Commit()
}
func (s *Store) Close() error { return s.DB.Close() }

func (s *Store) Save(m model.Message, chatName, chatType string) (bool, error) {
	now := time.Now().Unix()
	tx, e := s.DB.Begin()
	if e != nil {
		return false, e
	}
	defer tx.Rollback()
	_, e = tx.Exec(`INSERT INTO chats(id,name,type,last_message_at,created_at,updated_at) VALUES(?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET name=CASE WHEN excluded.name<>'' THEN excluded.name ELSE chats.name END,type=excluded.type,last_message_at=MAX(COALESCE(chats.last_message_at,0),excluded.last_message_at),updated_at=excluded.updated_at`, m.ChatID, chatName, chatType, m.Timestamp, now, now)
	if e != nil {
		return false, e
	}
	// Fill a deletion placeholder when history later supplies its original.
	restoredResult, e := tx.Exec(`UPDATE messages SET sender_id=?,sender_name=?,from_me=?,type=?,body=?,has_media=?,timestamp=? WHERE id=? AND chat_id=? AND type='deleted'`, m.SenderID, m.SenderName, m.FromMe, m.Type, m.Body, m.HasMedia, m.Timestamp, m.ID, m.ChatID)
	if e != nil {
		return false, e
	}
	restoredCount, _ := restoredResult.RowsAffected()
	res, e := tx.Exec(`INSERT OR IGNORE INTO messages(id,whatsapp_id,chat_id,sender_id,sender_name,from_me,type,body,has_media,timestamp,created_at) VALUES(?,?,?,?,?,?,?,?,?,?,?)`, m.ID, m.WhatsAppID, m.ChatID, m.SenderID, m.SenderName, m.FromMe, m.Type, m.Body, m.HasMedia, m.Timestamp, now)
	if e != nil {
		return false, e
	}
	quote, err := json.Marshal(m.Reply)
	if err != nil {
		return false, err
	}
	_, e = tx.Exec(`INSERT INTO message_context(message_id,raw_payload,info_json,history_payload,original_chat,original_sender,reply_json) VALUES(?,?,?,?,?,?,?) ON CONFLICT(message_id) DO UPDATE SET raw_payload=COALESCE(excluded.raw_payload,message_context.raw_payload),info_json=COALESCE(excluded.info_json,message_context.info_json),history_payload=COALESCE(excluded.history_payload,message_context.history_payload),original_chat=CASE WHEN excluded.original_chat<>'' THEN excluded.original_chat ELSE message_context.original_chat END,original_sender=CASE WHEN excluded.original_sender<>'' THEN excluded.original_sender ELSE message_context.original_sender END,reply_json=CASE WHEN excluded.reply_json<>'null' THEN excluded.reply_json ELSE message_context.reply_json END`, m.ID, m.Raw, m.OriginalInfo, m.HistoryRaw, m.OriginalChat, m.OriginalSender, string(quote))
	if e != nil {
		return false, e
	}
	mentions, err := json.Marshal(m.MentionJIDs)
	if err != nil {
		return false, err
	}
	// Preserve metadata across history snapshots and live redelivery.
	// Preserve an already-known mention list if a later partial echo omits it.
	_, e = tx.Exec(`INSERT INTO message_mentions(message_id,jids_json) VALUES(?,?) ON CONFLICT(message_id) DO UPDATE SET jids_json=CASE WHEN excluded.jids_json NOT IN ('null','[]') THEN excluded.jids_json ELSE message_mentions.jids_json END`, m.ID, string(mentions))
	if e != nil {
		return false, e
	}
	if n, _ := res.RowsAffected(); (n > 0 || restoredCount > 0) && m.HasMedia != 0 {
		_, e = tx.Exec(`INSERT OR IGNORE INTO media(id,message_id,chat_id,type,status,created_at,updated_at) VALUES(?,?,?,?,'pending',?,?)`, m.ID, m.ID, m.ChatID, m.Type, now, now)
		if e != nil {
			return false, e
		}
	}
	if e := applyStoredEdits(tx, m.ChatID, m.ID); e != nil {
		return false, e
	}
	if e := tx.Commit(); e != nil {
		return false, e
	}
	n, _ := res.RowsAffected()
	return n > 0 || restoredCount > 0, nil
}
func (s *Store) AddAlias(alias, canonical, kind string) error {
	if alias == "" || canonical == "" {
		return nil
	}
	_, e := s.DB.Exec(`INSERT INTO chat_aliases(alias,chat_id,kind) VALUES(?,?,?) ON CONFLICT(alias) DO UPDATE SET chat_id=excluded.chat_id,kind=excluded.kind`, alias, canonical, kind)
	return e
}
func (s *Store) ResolveChat(id string) string {
	var canonical string
	if e := s.DB.QueryRow(`SELECT chat_id FROM chat_aliases WHERE alias=?`, id).Scan(&canonical); e == nil {
		return canonical
	}
	return id
}
func (s *Store) TrimChat(chat string, keep int) error {
	if keep < 1 {
		return nil
	}
	_, e := s.DB.Exec(`DELETE FROM messages WHERE chat_id=? AND id NOT IN (SELECT id FROM messages WHERE chat_id=? ORDER BY timestamp DESC,id DESC LIMIT ?)`, chat, chat, keep)
	return e
}

// PruneChats keeps only the most recently active chats. It is used only while
// processing WhatsApp's initial history bootstrap; live chats are never pruned.
func (s *Store) PruneChats(keep int) (int, error) {
	if keep < 1 {
		return 0, nil
	}
	tx, err := s.DB.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	rows, err := tx.Query(`SELECT id FROM chats ORDER BY COALESCE(last_message_at,0) DESC,id DESC LIMIT -1 OFFSET ?`, keep)
	if err != nil {
		return 0, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return 0, err
		}
		ids = append(ids, id)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return 0, err
	}
	rows.Close()
	for _, id := range ids {
		if _, err = tx.Exec(`DELETE FROM messages WHERE chat_id=?`, id); err != nil {
			return 0, err
		}
		if _, err = tx.Exec(`DELETE FROM chats WHERE id=?`, id); err != nil {
			return 0, err
		}
	}
	if err = tx.Commit(); err != nil {
		return 0, err
	}
	return len(ids), nil
}
func (s *Store) MessageByID(id string) (*model.Message, error) {
	var m model.Message
	var sid, sn sql.NullString
	var fm int
	err := s.DB.QueryRow(`SELECT id,whatsapp_id,chat_id,sender_id,sender_name,from_me,type,COALESCE(body,''),has_media,timestamp FROM messages WHERE id=?`, id).
		Scan(&m.ID, &m.WhatsAppID, &m.ChatID, &sid, &sn, &fm, &m.Type, &m.Body, &m.HasMedia, &m.Timestamp)
	if err != nil {
		return nil, err
	}
	if sid.Valid {
		m.SenderID = &sid.String
	}
	if sn.Valid {
		m.SenderName = &sn.String
	}
	m.FromMe = fm
	if err := s.LoadMessageContext(&m); err != nil {
		return nil, err
	}
	return &m, nil
}

func (s *Store) OldestMessage(chat string) (*model.Message, error) {
	chat = s.ResolveChat(chat)
	var m model.Message
	var sid, sn sql.NullString
	var fm int
	e := s.DB.QueryRow(`SELECT id,whatsapp_id,chat_id,sender_id,sender_name,from_me,type,COALESCE(body,''),has_media,timestamp FROM messages WHERE chat_id=? ORDER BY timestamp ASC,id ASC LIMIT 1`, chat).Scan(&m.ID, &m.WhatsAppID, &m.ChatID, &sid, &sn, &fm, &m.Type, &m.Body, &m.HasMedia, &m.Timestamp)
	if e != nil {
		return nil, e
	}
	if sid.Valid {
		m.SenderID = &sid.String
	}
	if sn.Valid {
		m.SenderName = &sn.String
	}
	m.FromMe = fm
	return &m, nil
}
func (s *Store) SetHistoryExhausted(chat string, exhausted bool) {
	v := 0
	if exhausted {
		v = 1
	}
	_, _ = s.DB.Exec(`UPDATE chats SET history_exhausted=? WHERE id=?`, v, s.ResolveChat(chat))
}
func (s *Store) HistoryExhausted(chat string) bool {
	var v int
	_ = s.DB.QueryRow(`SELECT history_exhausted FROM chats WHERE id=?`, s.ResolveChat(chat)).Scan(&v)
	return v != 0
}

func (s *Store) Chats() ([]map[string]any, error) {
	r, e := s.DB.Query(`SELECT c.id,c.name,c.type,c.last_message_at,m.body,m.type,m.from_me,m.sender_name,m.id FROM chats c LEFT JOIN messages m ON m.id=(SELECT id FROM messages WHERE chat_id=c.id ORDER BY timestamp DESC,id DESC LIMIT 1) ORDER BY c.last_message_at DESC`)
	if e != nil {
		return nil, e
	}
	defer r.Close()
	var out []map[string]any
	for r.Next() {
		var id, typ string
		var name, body, mt, sn, mid sql.NullString
		var ts, fm sql.NullInt64
		if e = r.Scan(&id, &name, &typ, &ts, &body, &mt, &fm, &sn, &mid); e != nil {
			return nil, e
		}
		out = append(out, map[string]any{"id": id, "name": nullable(name), "type": typ, "last_message_at": nullableInt(ts), "last_body": nullable(body), "last_type": nullable(mt), "last_from_me": nullableInt(fm), "last_sender_name": nullable(sn), "last_message_id": nullable(mid)})
	}
	return out, r.Err()
}
func nullable(v sql.NullString) any {
	if v.Valid {
		return v.String
	}
	return nil
}
func nullableInt(v sql.NullInt64) any {
	if v.Valid {
		return v.Int64
	}
	return nil
}

type HistoryGap struct {
	UpperID string
	UpperTS int64
	LowerID string
	LowerTS int64
	Seconds int64
}

func (s *Store) LargestHistoryGap(chat string, minSeconds int64) (*HistoryGap, error) {
	chat = s.ResolveChat(chat)
	const q = `
WITH ordered AS (
 SELECT id,timestamp,
        LAG(id) OVER (ORDER BY timestamp ASC,id ASC) AS lower_id,
        LAG(timestamp) OVER (ORDER BY timestamp ASC,id ASC) AS lower_ts
 FROM messages WHERE chat_id=?
)
SELECT id,timestamp,lower_id,lower_ts,timestamp-lower_ts
FROM ordered
WHERE lower_ts IS NOT NULL AND timestamp-lower_ts>=?
ORDER BY timestamp-lower_ts DESC LIMIT 1`
	var g HistoryGap
	err := s.DB.QueryRow(q, chat, minSeconds).Scan(&g.UpperID, &g.UpperTS, &g.LowerID, &g.LowerTS, &g.Seconds)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &g, nil
}

func (s *Store) Messages(chat string, limit int, before *int64, beforeID string) ([]map[string]any, error) {
	chat = s.ResolveChat(chat)
	q := `SELECT m.id,m.whatsapp_id,m.chat_id,m.sender_id,m.sender_name,m.from_me,m.type,m.body,m.has_media,m.media_mime,m.media_filename,m.timestamp,m.created_at,md.id,md.status,md.mime_type,md.filename FROM messages m LEFT JOIN media md ON md.message_id=m.id WHERE m.chat_id=?`
	a := []any{chat}
	if before != nil && beforeID != "" {
		q += ` AND (m.timestamp<? OR (m.timestamp=? AND m.id<?))`
		a = append(a, *before, *before, beforeID)
	} else if before != nil {
		q += ` AND m.timestamp<?`
		a = append(a, *before)
	}
	q += ` ORDER BY m.timestamp DESC,m.id DESC LIMIT ?`
	a = append(a, limit)
	r, e := s.DB.Query(q, a...)
	if e != nil {
		return nil, e
	}
	defer r.Close()
	var out []map[string]any
	for r.Next() {
		var id, cid, typ string
		var wa, sid, sn, body, mm, mf, mediaID, mediaStatus, mimeType, filename sql.NullString
		var fm, hm, ts, created int64
		if e = r.Scan(&id, &wa, &cid, &sid, &sn, &fm, &typ, &body, &hm, &mm, &mf, &ts, &created, &mediaID, &mediaStatus, &mimeType, &filename); e != nil {
			return nil, e
		}
		out = append(out, map[string]any{"id": id, "whatsapp_id": nullable(wa), "chat_id": cid, "sender_id": nullable(sid), "sender_name": nullable(sn), "from_me": fm, "type": typ, "body": nullable(body), "has_media": hm, "media_mime": nullable(mm), "media_filename": nullable(mf), "timestamp": ts, "created_at": created, "media_id": nullable(mediaID), "media_status": nullable(mediaStatus), "mime_type": nullable(mimeType), "filename": nullable(filename)})
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out, r.Err()
}
func (s *Store) Health() error {
	var result string
	if err := s.DB.QueryRow(`PRAGMA quick_check`).Scan(&result); err != nil {
		return err
	}
	if result != "ok" {
		return fmt.Errorf("sqlite quick_check: %s", result)
	}
	return nil
}
func (s *Store) Stats() map[string]any {
	var c, m, media int64
	s.DB.QueryRow(`SELECT COUNT(*) FROM chats`).Scan(&c)
	s.DB.QueryRow(`SELECT COUNT(*) FROM messages`).Scan(&m)
	s.DB.QueryRow(`SELECT COUNT(*) FROM media WHERE status='ready'`).Scan(&media)
	return map[string]any{"chats": c, "messages": m, "media": media}
}
func (s *Store) String() string { return fmt.Sprintf("store(%p)", s) }

// UpdateChatName only replaces a chat name with a non-empty value. This makes
// metadata updates monotonic: a useful name is never overwritten by an empty
// event later on.
func (s *Store) MissingSenderNameIDs() ([]string, error) {
	rows, err := s.DB.Query(`SELECT DISTINCT sender_id FROM messages WHERE from_me=0 AND sender_id IS NOT NULL AND sender_id<>'' AND (sender_name IS NULL OR sender_name='')`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (s *Store) UpdateSenderName(senderID, name string) (int, error) {
	if senderID == "" || name == "" {
		return 0, nil
	}
	res, err := s.DB.Exec(`UPDATE messages SET sender_name=? WHERE sender_id=? AND (sender_name IS NULL OR sender_name='')`, name, senderID)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

func (s *Store) UpdateChatName(chat, name string) error {
	if name == "" {
		return nil
	}
	chat = s.ResolveChat(chat)
	_, err := s.DB.Exec(`UPDATE chats SET name=?,updated_at=? WHERE id=?`, name, time.Now().Unix(), chat)
	return err
}

type MediaRow struct {
	ID, MessageID, ChatID, Type, Status string
	MimeType, Filename, Path, Error     sql.NullString
	Size                                sql.NullInt64
}

func (s *Store) MediaReady(messageID, mimeType, filename, path string, size int64) (*MediaRow, error) {
	now := time.Now().Unix()
	_, err := s.DB.Exec(`UPDATE media SET mime_type=?,filename=?,path=?,size=?,status='ready',error=NULL,updated_at=? WHERE message_id=?`, mimeType, filename, path, size, now, messageID)
	if err != nil {
		return nil, err
	}
	_, _ = s.DB.Exec(`UPDATE messages SET media_mime=?,media_filename=? WHERE id=?`, mimeType, filename, messageID)
	return s.MediaByMessage(messageID)
}
func (s *Store) MediaPending(messageID string) error {
	_, err := s.DB.Exec(`UPDATE media SET status='pending',error=NULL,updated_at=? WHERE message_id=?`, time.Now().Unix(), messageID)
	return err
}
func (s *Store) MediaFailed(messageID string, cause error) error {
	_, err := s.DB.Exec(`UPDATE media SET status='failed',error=?,updated_at=? WHERE message_id=?`, fmt.Sprint(cause), time.Now().Unix(), messageID)
	return err
}
func (s *Store) MediaByMessage(messageID string) (*MediaRow, error) {
	var x MediaRow
	err := s.DB.QueryRow(`SELECT id,message_id,chat_id,type,status,mime_type,filename,path,size,error FROM media WHERE message_id=?`, messageID).Scan(&x.ID, &x.MessageID, &x.ChatID, &x.Type, &x.Status, &x.MimeType, &x.Filename, &x.Path, &x.Size, &x.Error)
	return &x, err
}
func (s *Store) Media(id string) (*MediaRow, error) {
	var x MediaRow
	err := s.DB.QueryRow(`SELECT id,message_id,chat_id,type,status,mime_type,filename,path,size,error FROM media WHERE id=?`, id).Scan(&x.ID, &x.MessageID, &x.ChatID, &x.Type, &x.Status, &x.MimeType, &x.Filename, &x.Path, &x.Size, &x.Error)
	return &x, err
}

var urlRE = regexp.MustCompile(`https?://[^\s<>()]+`)

func (s *Store) Search(q string, limit int) (map[string]any, error) {
	q = strings.TrimSpace(q)
	if limit < 1 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}
	like := "%" + q + "%"
	cr, err := s.DB.Query(`SELECT id,name,type,last_message_at FROM chats WHERE COALESCE(name,'') LIKE ? COLLATE NOCASE ORDER BY COALESCE(last_message_at,0) DESC LIMIT ?`, like, limit)
	if err != nil {
		return nil, err
	}
	defer cr.Close()
	chats := []map[string]any{}
	for cr.Next() {
		var id, typ string
		var name sql.NullString
		var ts sql.NullInt64
		if err = cr.Scan(&id, &name, &typ, &ts); err != nil {
			return nil, err
		}
		chats = append(chats, map[string]any{"id": id, "name": nullable(name), "type": typ, "last_message_at": nullableInt(ts)})
	}
	mr, err := s.DB.Query(`SELECT m.id,m.chat_id,c.name,m.body,m.timestamp FROM messages m JOIN chats c ON c.id=m.chat_id WHERE COALESCE(m.body,'') LIKE ? COLLATE NOCASE ORDER BY m.timestamp DESC LIMIT ?`, like, limit)
	if err != nil {
		return nil, err
	}
	defer mr.Close()
	messages := []map[string]any{}
	for mr.Next() {
		var id, cid, body string
		var cn sql.NullString
		var ts int64
		if err = mr.Scan(&id, &cid, &cn, &body, &ts); err != nil {
			return nil, err
		}
		messages = append(messages, map[string]any{"id": id, "chat_id": cid, "chat_name": nullable(cn), "body": body, "timestamp": ts})
	}
	return map[string]any{"chats": chats, "messages": messages}, nil
}

func (s *Store) GalleryMedia(chat, kind string, limit int) ([]map[string]any, error) {
	chat = s.ResolveChat(chat)
	if limit < 1 {
		limit = 100
	}
	if limit > 200 {
		limit = 200
	}
	types := []string{"image", "video", "sticker"}
	if kind == "audio" {
		types = []string{"audio", "ptt"}
	} else if kind == "documents" {
		types = []string{"document"}
	}
	ph := strings.TrimRight(strings.Repeat("?,", len(types)), ",")
	args := []any{chat}
	for _, t := range types {
		args = append(args, t)
	}
	args = append(args, limit)
	q := `SELECT md.id,md.message_id,md.type,md.status,md.mime_type,md.filename,m.timestamp FROM media md JOIN messages m ON m.id=md.message_id WHERE md.chat_id=? AND md.status='ready' AND md.path IS NOT NULL AND md.path<>'' AND md.type IN (` + ph + `) ORDER BY m.timestamp DESC LIMIT ?`
	r, e := s.DB.Query(q, args...)
	if e != nil {
		return nil, e
	}
	defer r.Close()
	out := []map[string]any{}
	for r.Next() {
		var id, mid, typ, status string
		var mime, fn sql.NullString
		var ts int64
		if e = r.Scan(&id, &mid, &typ, &status, &mime, &fn, &ts); e != nil {
			return nil, e
		}
		out = append(out, map[string]any{"id": id, "message_id": mid, "type": typ, "status": status, "mime_type": nullable(mime), "filename": nullable(fn), "timestamp": ts})
	}
	return out, r.Err()
}
func (s *Store) Links(chat string, limit int) ([]map[string]any, error) {
	chat = s.ResolveChat(chat)
	if limit < 1 {
		limit = 100
	}
	rows, e := s.DB.Query(`SELECT id,COALESCE(body,''),timestamp FROM messages WHERE chat_id=? AND body LIKE '%http%' ORDER BY timestamp DESC LIMIT ?`, chat, limit*4)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, body string
		var ts int64
		if e = rows.Scan(&id, &body, &ts); e != nil {
			return nil, e
		}
		for _, u := range urlRE.FindAllString(body, -1) {
			u = strings.TrimRight(u, ".,;:!?)]}")
			out = append(out, map[string]any{"message_id": id, "url": u, "timestamp": ts})
			if len(out) >= limit {
				return out, nil
			}
		}
	}
	return out, rows.Err()
}
func (s *Store) MessageContext(chat, id string, radius int) ([]map[string]any, error) {
	chat = s.ResolveChat(chat)
	var ts int64
	if e := s.DB.QueryRow(`SELECT timestamp FROM messages WHERE id=? AND chat_id=?`, id, chat).Scan(&ts); e != nil {
		return nil, e
	}
	if radius < 1 {
		radius = 25
	}
	// Reuse Messages around target by selecting a bounded window and returning chronological rows.
	r, e := s.DB.Query(`SELECT id FROM messages WHERE chat_id=? ORDER BY ABS(timestamp-?) ASC LIMIT ?`, chat, ts, radius*2+1)
	if e != nil {
		return nil, e
	}
	ids := []string{}
	for r.Next() {
		var x string
		_ = r.Scan(&x)
		ids = append(ids, x)
	}
	r.Close()
	if len(ids) == 0 {
		return []map[string]any{}, nil
	}
	ph := strings.TrimRight(strings.Repeat("?,", len(ids)), ",")
	args := []any{chat}
	for _, x := range ids {
		args = append(args, x)
	}
	q := `SELECT m.id,m.whatsapp_id,m.chat_id,m.sender_id,m.sender_name,m.from_me,m.type,m.body,m.has_media,m.media_mime,m.media_filename,m.timestamp,m.created_at,md.id,md.status,md.mime_type,md.filename FROM messages m LEFT JOIN media md ON md.message_id=m.id WHERE m.chat_id=? AND m.id IN (` + ph + `) ORDER BY m.timestamp ASC,m.id ASC`
	rr, e := s.DB.Query(q, args...)
	if e != nil {
		return nil, e
	}
	defer rr.Close()
	out := []map[string]any{}
	for rr.Next() {
		var iid, cid, typ string
		var wa, sid, sn, body, mm, mf, mediaID, mediaStatus, mimeType, filename sql.NullString
		var fm, hm, t, created int64
		if e = rr.Scan(&iid, &wa, &cid, &sid, &sn, &fm, &typ, &body, &hm, &mm, &mf, &t, &created, &mediaID, &mediaStatus, &mimeType, &filename); e != nil {
			return nil, e
		}
		out = append(out, map[string]any{"id": iid, "whatsapp_id": nullable(wa), "chat_id": cid, "sender_id": nullable(sid), "sender_name": nullable(sn), "from_me": fm, "type": typ, "body": nullable(body), "has_media": hm, "media_mime": nullable(mm), "media_filename": nullable(mf), "timestamp": t, "created_at": created, "media_id": nullable(mediaID), "media_status": nullable(mediaStatus), "mime_type": nullable(mimeType), "filename": nullable(filename)})
	}
	return out, rr.Err()
}
func (s *Store) SaveMediaPayload(messageID string, payload []byte) error {
	if len(payload) == 0 {
		return nil
	}
	_, e := s.DB.Exec(`INSERT INTO media_payload(message_id,payload) VALUES(?,?) ON CONFLICT(message_id) DO UPDATE SET payload=excluded.payload`, messageID, payload)
	return e
}
func (s *Store) MediaPayload(messageID string) ([]byte, error) {
	var b []byte
	e := s.DB.QueryRow(`SELECT payload FROM media_payload WHERE message_id=?`, messageID).Scan(&b)
	return b, e
}

func (s *Store) ConfigBool(key string, fallback bool) bool {
	if value, ok := s.ConfigOverride(key); ok {
		v, err := strconv.ParseBool(value)
		if err == nil {
			return v
		}
	}
	var value string
	if err := s.DB.QueryRow(`SELECT value FROM app_config WHERE key=?`, key).Scan(&value); err != nil {
		return fallback
	}
	v, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return v
}
func (s *Store) SetConfigBool(key string, value bool) error {
	_, err := s.DB.Exec(`INSERT INTO app_config(key,value,updated_at) VALUES(?,?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value,updated_at=excluded.updated_at`, key, strconv.FormatBool(value), time.Now().Unix())
	return err
}

func (s *Store) ConfigString(key, fallback string) string {
	if value, ok := s.ConfigOverride(key); ok {
		return value
	}
	var value string
	if err := s.DB.QueryRow(`SELECT value FROM app_config WHERE key=?`, key).Scan(&value); err != nil {
		return fallback
	}
	return value
}
func (s *Store) SetConfigOverride(key, value string) {
	s.overrideMu.Lock()
	defer s.overrideMu.Unlock()
	s.overrides[key] = value
}
func (s *Store) ConfigOverride(key string) (string, bool) {
	s.overrideMu.RLock()
	defer s.overrideMu.RUnlock()
	v, ok := s.overrides[key]
	return v, ok
}
func (s *Store) ConfigManaged(key string) bool { _, ok := s.ConfigOverride(key); return ok }

func (s *Store) SetConfigString(key, value string) error {
	_, err := s.DB.Exec(`INSERT INTO app_config(key,value,updated_at) VALUES(?,?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value,updated_at=excluded.updated_at`, key, value, time.Now().Unix())
	return err
}

func (s *Store) SavePushSubscription(endpoint string, raw []byte) error {
	if endpoint == "" {
		return fmt.Errorf("push subscription endpoint is empty")
	}
	now := time.Now().Unix()
	_, err := s.DB.Exec(`INSERT INTO push_subscriptions(endpoint,subscription_json,created_at,updated_at) VALUES(?,?,?,?) ON CONFLICT(endpoint) DO UPDATE SET subscription_json=excluded.subscription_json,updated_at=excluded.updated_at`, endpoint, string(raw), now, now)
	return err
}
func (s *Store) DeletePushSubscription(endpoint string) error {
	_, err := s.DB.Exec(`DELETE FROM push_subscriptions WHERE endpoint=?`, endpoint)
	return err
}
func (s *Store) PushSubscriptions() (map[string]string, error) {
	rows, err := s.DB.Query(`SELECT endpoint,subscription_json FROM push_subscriptions ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var endpoint, raw string
		if err := rows.Scan(&endpoint, &raw); err != nil {
			return nil, err
		}
		out[endpoint] = raw
	}
	return out, rows.Err()
}
func (s *Store) ChatName(chat string) string {
	chat = s.ResolveChat(chat)
	var name string
	if err := s.DB.QueryRow(`SELECT COALESCE(name,'') FROM chats WHERE id=?`, chat).Scan(&name); err != nil {
		return ""
	}
	return name
}
func (s *Store) ChatType(chat string) string {
	chat = s.ResolveChat(chat)
	var typ string
	if err := s.DB.QueryRow(`SELECT COALESCE(type,'') FROM chats WHERE id=?`, chat).Scan(&typ); err != nil {
		return ""
	}
	return typ
}

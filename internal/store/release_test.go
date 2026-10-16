package store

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"wisp/internal/model"
)

func TestReopenPreservesMessagesSettingsAndSubscriptions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "wisp.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Save(model.Message{ID: "kept", WhatsAppID: "kept", ChatID: "group@g.us", Type: "chat", Body: "keep this", Timestamp: 1}, "Group", "group"); err != nil {
		t.Fatal(err)
	}
	if err = s.SetConfigString("pushVapidPrivateKey", "kept-key"); err != nil {
		t.Fatal(err)
	}
	if err = s.SavePushSubscription("https://push.example/device", []byte(`{"endpoint":"https://push.example/device"}`)); err != nil {
		t.Fatal(err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	m, err := reopened.MessageByID("kept")
	if err != nil || m.Body != "keep this" {
		t.Fatal("message lost", err)
	}
	if reopened.ConfigString("pushVapidPrivateKey", "") != "kept-key" {
		t.Fatal("setting lost")
	}
	subs, err := reopened.PushSubscriptions()
	if err != nil || len(subs) != 1 {
		t.Fatal("subscription lost", err)
	}
	var version int
	if err = reopened.DB.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil || version != SchemaVersion {
		t.Fatal(version, err)
	}
}
func TestFutureSchemaIsRejectedWithoutResettingData(t *testing.T) {
	path := filepath.Join(t.TempDir(), "wisp.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Save(model.Message{ID: "kept", WhatsAppID: "kept", ChatID: "g", Type: "chat", Body: "untouched"}, "Group", "group")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.Exec(`PRAGMA user_version=9999`); err != nil {
		t.Fatal(err)
	}
	s.Close()
	if next, err := Open(path); err == nil {
		next.Close()
		t.Fatal("future schema accepted")
	} else if !strings.Contains(err.Error(), "keep your data") {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var body string
	if err = db.QueryRow(`SELECT body FROM messages WHERE id='kept'`).Scan(&body); err != nil || body != "untouched" {
		t.Fatal(body, err)
	}
	var version int
	_ = db.QueryRow(`PRAGMA user_version`).Scan(&version)
	if version != 9999 {
		t.Fatal("schema changed", version)
	}
}
func TestSchemaInitializationRollsBackOnFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "partial.db")
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`CREATE TABLE messages(id TEXT PRIMARY KEY); PRAGMA user_version=150;`); err != nil {
		t.Fatal(err)
	}
	db.Close()
	if next, err := Open(path); err == nil {
		next.Close()
		t.Fatal("broken schema accepted")
	}
	db, err = sql.Open("sqlite3", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var count int
	if err = db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type='table' AND name='chats'`).Scan(&count); err != nil || count != 0 {
		t.Fatal("partial schema committed", count, err)
	}
}

package store

import (
	"path/filepath"
	"testing"
	"wisp/internal/model"
)

func TestReadSyncSnapshotDurableAndPrivacyChanges(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	st, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	save := func(id string, ts int64) {
		t.Helper()
		if _, err := st.SaveLive(model.Message{ID: id, WhatsAppID: id, ChatID: "123@s.whatsapp.net", Timestamp: ts, Type: "chat"}, "C", "private"); err != nil {
			t.Fatal(err)
		}
	}
	save("local", 10)
	if _, err = st.MarkChatReadAndSync("123@s.whatsapp.net", 1); err != nil {
		t.Fatal(err)
	}
	jobs, _ := st.ReadSyncJobs()
	if len(jobs) != 0 {
		t.Fatal(jobs)
	}
	if err = st.SetReadMode(ReadModeSelf); err != nil {
		t.Fatal(err)
	}
	save("one", 20)
	save("two", 21)
	if _, err = st.MarkChatReadAndSync("123@s.whatsapp.net", 2); err != nil {
		t.Fatal(err)
	}
	// Repeating the same read does not duplicate a receipt or include message two.
	st.MarkChatReadAndSync("123@s.whatsapp.net", 2)
	jobs, err = st.ReadSyncJobs()
	if err != nil || len(jobs) != 1 || len(jobs[0].Messages) != 1 || jobs[0].Messages[0].ID != "one" || jobs[0].Mode != ReadModeSelf {
		t.Fatal(jobs, err)
	}
	st.Close()
	st, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	jobs, err = st.ReadSyncJobs()
	if err != nil || len(jobs) != 1 {
		t.Fatal(jobs, err)
	}
	if err = st.SetReadMode(ReadModeReceipts); err != nil {
		t.Fatal(err)
	}
	jobs, _ = st.ReadSyncJobs()
	if len(jobs) != 0 {
		t.Fatal("old work escalated", jobs)
	}
	if _, err = st.MarkChatReadAndSync("123@s.whatsapp.net", 3); err != nil {
		t.Fatal(err)
	}
	jobs, _ = st.ReadSyncJobs()
	if len(jobs) != 1 || jobs[0].Mode != ReadModeReceipts || jobs[0].Messages[0].ID != "two" {
		t.Fatal(jobs)
	}
	if err = st.SetReadMode("invalid"); err == nil {
		t.Fatal("invalid mode accepted")
	}
	st.SetReadMode(ReadModeLocal)
	jobs, _ = st.ReadSyncJobs()
	if len(jobs) != 0 {
		t.Fatal(jobs)
	}
}
func TestRemoteReadBoundaryNeverClearsLaterMessages(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	for _, row := range []struct {
		id        string
		timestamp int64
	}{{"first", 10}, {"second", 20}, {"late", 20}} {
		if _, err = st.SaveLive(model.Message{ID: row.id, WhatsAppID: row.id, ChatID: "c", Timestamp: row.timestamp, Type: "chat"}, "C", "private"); err != nil {
			t.Fatal(err)
		}
	}
	p, err := st.RemoteReadBoundary("c", []string{"second", "first"}, 99, 9999999999)
	if err != nil || p.ReadSequence != 2 || p.FirstUnreadID != "late" {
		t.Fatal(p, err)
	}
	p, err = st.RemoteReadBoundary("c", []string{"first", "unknown"}, 0, 0)
	if err != nil || p.ReadSequence != 2 {
		t.Fatal(p, err)
	}
	// Timestamp-only updates retain same-second and newly delivered messages.
	st.DB.Exec(`UPDATE messages SET created_at=1`)
	p, err = st.RemoteReadBoundary("c", nil, 20, 100)
	if err != nil || p.ReadSequence != 2 {
		t.Fatal(p, err)
	}
	// Whole-chat reads from other devices do not enter the outgoing queue.
	jobs, _ := st.ReadSyncJobs()
	if len(jobs) != 0 {
		t.Fatal(jobs)
	}
}

func TestTimestampOnlyReadStopsAtAmbiguousOrLaterArrival(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	for _, row := range []struct {
		id        string
		timestamp int64
	}{{"first", 10}, {"second", 20}, {"late-backdated", 5}} {
		if _, err = st.SaveLive(model.Message{ID: row.id, WhatsAppID: row.id, ChatID: "c", Timestamp: row.timestamp, Type: "chat"}, "C", "private"); err != nil {
			t.Fatal(err)
		}
	}
	st.DB.Exec(`UPDATE messages SET created_at=1`)
	st.DB.Exec(`UPDATE messages SET created_at=200 WHERE id='late-backdated'`)
	p, err := st.RemoteReadBoundary("c", nil, 15, 100)
	if err != nil || p.ReadSequence != 1 || p.FirstUnreadID != "second" {
		t.Fatal(p, err)
	}
	// Even when the boundary moves beyond second, a message delivered after
	// the remote read remains unread despite its older WhatsApp timestamp.
	p, err = st.RemoteReadBoundary("c", nil, 30, 100)
	if err != nil || p.ReadSequence != 2 || p.FirstUnreadID != "late-backdated" {
		t.Fatal(p, err)
	}
}

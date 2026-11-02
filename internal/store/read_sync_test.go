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

func TestVisibleReadBoundaryRepairsHistoryAndOwnOnlyChats(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	st.SetReadMode(ReadModeSelf)
	// Imported history has no unread ledger, and the latest visible message is own.
	for _, m := range []model.Message{
		{ID: "history", WhatsAppID: "history", ChatID: "c", Type: "chat", Timestamp: 10},
		{ID: "mine", WhatsAppID: "mine", ChatID: "c", Type: "chat", Timestamp: 20, FromMe: 1},
	} {
		if _, err = st.Save(m, "C", "private"); err != nil {
			t.Fatal(err)
		}
	}
	p, err := st.MarkChatReadThrough("c", 0, "mine")
	if err != nil || p.UnreadSequence != 0 || p.ReadSequence != 0 {
		t.Fatal(p, err)
	}
	jobs, err := st.ReadSyncJobs()
	if err != nil || len(jobs) != 1 || len(jobs[0].Messages) != 2 {
		t.Fatal(jobs, err)
	}
	last := jobs[0].Messages[1]
	if !last.Boundary || !last.FromMe || last.ID != "mine" || last.Through != 0 {
		t.Fatal(last)
	}
	st.MarkChatReadThrough("c", 0, "mine")
	jobs, _ = st.ReadSyncJobs()
	if len(jobs) != 1 {
		t.Fatal("duplicate pending snapshot", jobs)
	}
	st.DeleteReadSyncJob(jobs[0].ID)
	// Reopening can retry a native read marker that was later changed elsewhere.
	st.MarkChatReadThrough("c", 0, "mine")
	jobs, _ = st.ReadSyncJobs()
	if len(jobs) != 1 {
		t.Fatal(jobs)
	}
	if _, err = st.MarkChatReadThrough("c", 0, "missing"); err != ErrInvalidReadBoundary {
		t.Fatal(err)
	}
	st.Save(model.Message{ID: "other", WhatsAppID: "other", ChatID: "other", Type: "chat", Timestamp: 30}, "Other", "private")
	if _, err = st.MarkChatReadThrough("c", 0, "other"); err != ErrInvalidReadBoundary {
		t.Fatal("foreign boundary accepted", err)
	}
}
func TestReadBoundarySnapshotDoesNotIncludeLaterIncoming(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	st.SetReadMode(ReadModeSelf)
	for i, id := range []string{"first", "second"} {
		if _, err = st.SaveLive(model.Message{ID: id, WhatsAppID: id, ChatID: "c", Type: "chat", Timestamp: int64(10 + i)}, "C", "private"); err != nil {
			t.Fatal(err)
		}
	}
	p, err := st.MarkChatReadThrough("c", 1, "first")
	if err != nil || p.ReadSequence != 1 || p.FirstUnreadID != "second" {
		t.Fatal(p, err)
	}
	jobs, _ := st.ReadSyncJobs()
	if len(jobs) != 1 || len(jobs[0].Messages) != 1 || jobs[0].Messages[0].ID != "first" {
		t.Fatal(jobs)
	}
	if _, err = st.MarkChatReadThrough("c", 1, "second"); err != ErrInvalidReadBoundary {
		t.Fatal("newer unread boundary accepted", err)
	}
}

func TestNativeReadAtOwnBoundaryIncludesSameSecondIncoming(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	st.SaveLive(model.Message{ID: "a-incoming", WhatsAppID: "a-incoming", ChatID: "c", Type: "chat", Timestamp: 10}, "C", "private")
	st.Save(model.Message{ID: "b-own", WhatsAppID: "b-own", ChatID: "c", Type: "chat", Timestamp: 10, FromMe: 1}, "C", "private")
	st.SaveLive(model.Message{ID: "c-later", WhatsAppID: "c-later", ChatID: "c", Type: "chat", Timestamp: 10}, "C", "private")
	st.DB.Exec(`UPDATE messages SET created_at=1`)
	p, err := st.RemoteReadBoundary("c", []string{"b-own"}, 10, 100)
	if err != nil || p.ReadSequence != 1 || p.FirstUnreadID != "c-later" {
		t.Fatal(p, err)
	}
}

func TestStaleDeviceBoundaryCannotQueueBehindNewerReadPosition(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	st.SetReadMode(ReadModeSelf)
	for i, id := range []string{"first", "second"} {
		st.SaveLive(model.Message{ID: id, WhatsAppID: id, ChatID: "c", Type: "chat", Timestamp: int64(10 + i)}, "C", "private")
	}
	st.MarkChatRead("c", 2)
	p, err := st.MarkChatReadThrough("c", 1, "first")
	if err != nil || p.ReadSequence != 2 {
		t.Fatal(p, err)
	}
	jobs, _ := st.ReadSyncJobs()
	if len(jobs) != 0 {
		t.Fatal("stale read queued", jobs)
	}
}

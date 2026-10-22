package store

import (
	"path/filepath"
	"testing"
	"wisp/internal/model"
)

func TestUnreadPersistsAndOnlyCountsFirstLiveIncoming(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	save := func(id string, live bool, own int) {
		t.Helper()
		m := model.Message{ID: id, WhatsAppID: id, ChatID: "c", Type: "chat", FromMe: own, Timestamp: 1}
		var err error
		if live {
			_, err = s.SaveLive(m, "C", "private")
		} else {
			_, err = s.Save(m, "C", "private")
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	save("history", false, 0)
	save("mine", true, 1)
	p, _ := s.ChatState("c")
	if p.UnreadSequence != 0 {
		t.Fatal(p)
	}
	save("first", true, 0)
	save("first", true, 0)
	save("first", false, 0)
	p, _ = s.ChatState("c")
	if p.UnreadSequence != 1 || p.ReadSequence != 0 || p.LastIncomingID != "first" || p.FirstUnreadID != "first" {
		t.Fatal(p)
	}
	// A second message after a client's snapshot must survive its delayed read.
	save("second", true, 0)
	p, err = s.MarkChatRead("c", 1)
	if err != nil || p.ReadSequence != 1 || p.UnreadSequence != 2 || p.FirstUnreadID != "second" {
		t.Fatal(p, err)
	}
	s.Close()
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	p, _ = s.ChatState("c")
	if p.UnreadSequence-p.ReadSequence != 1 || p.FirstUnreadID != "second" {
		t.Fatal(p)
	}
	p, err = s.MarkChatRead("c", 2)
	if err != nil || p.ReadSequence != 2 || p.FirstUnreadID != "" {
		t.Fatal(p, err)
	}
	var indexed int
	if err = s.DB.QueryRow(`SELECT count(*) FROM unread_messages`).Scan(&indexed); err != nil || indexed != 0 {
		t.Fatal("read index was not cleared", indexed, err)
	}
	p, err = s.MarkChatRead("c", 1)
	if err != nil || p.ReadSequence != 2 {
		t.Fatal("stale device moved read position", p, err)
	}
	if _, err = s.MarkChatRead("c", 3); err == nil {
		t.Fatal("future read accepted")
	}
	if _, err = s.MarkChatRead("c", -1); err == nil {
		t.Fatal("negative read accepted")
	}
	// Close reads only in Wisp, mute neither clears nor reopens a closed chat.
	_, err = s.SaveLive(model.Message{ID: "third", WhatsAppID: "third", ChatID: "c", Type: "chat", Timestamp: 2}, "C", "private")
	if err != nil {
		t.Fatal(err)
	}
	p, _ = s.ChangeChatState("c", "mute", -1)
	if p.UnreadSequence-p.ReadSequence != 1 {
		t.Fatal(p)
	}
	p, _ = s.ChangeChatState("c", "close", 0)
	if p.ReadSequence != p.UnreadSequence || p.FirstUnreadID != "" {
		t.Fatal(p)
	}
	if err = s.DB.QueryRow(`SELECT count(*) FROM unread_messages`).Scan(&indexed); err != nil || indexed != 0 {
		t.Fatal("close retained unread index", indexed, err)
	}
	var schema int
	s.DB.QueryRow("PRAGMA user_version").Scan(&schema)
	if schema != 150 {
		t.Fatal(schema)
	}
}

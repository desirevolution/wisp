package store

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"
	"wisp/internal/model"
)

func TestChatCloseMuteAndHistory(t *testing.T) {
	s := contextStore(t)
	chat := "chat@s.whatsapp.net"
	if err := s.OpenChat(chat, "Arthur", "private"); err != nil {
		t.Fatal(err)
	}
	p, err := s.ChangeChatState(chat, "close", 0)
	if err != nil || !p.Closed || p.DraftRevision != 1 {
		t.Fatal(p, err)
	}
	// Importing old messages, reactions and edits must not reopen a closed chat.
	if _, err = s.Save(model.Message{ID: "history", WhatsAppID: "history", ChatID: chat, Body: "secret text", Type: "chat", Timestamp: 1}, "Arthur", "private"); err != nil {
		t.Fatal(err)
	}
	s.SaveReaction(model.Reaction{ChatID: chat, MessageID: "history", ActorID: "a", Emoji: "👍", EventID: "r", TimestampMS: 2})
	p, _ = s.ChatState(chat)
	if !p.Closed {
		t.Fatal("history reopened chat")
	}
	result, err := s.Search("secret", 50)
	if err != nil || len(result["messages"].([]map[string]any)) != 0 {
		t.Fatal(result, err)
	}
	result, err = s.Search("Arthur", 50)
	if err != nil || len(result["chats"].([]map[string]any)) != 0 {
		t.Fatal(result, err)
	}
	p, changed, err := s.LiveChatMessage(chat)
	if err != nil || !changed || p.Closed {
		t.Fatal(p, changed, err)
	}
	p, _ = s.ChangeChatState(chat, "mute", -1)
	if !p.MutedAt(time.Now().Unix()) {
		t.Fatal(p)
	}
	p, _ = s.ChangeChatState(chat, "close", 0)
	p, changed, err = s.LiveChatMessage(chat)
	if err != nil || changed || !p.Closed || p.DraftRevision != 2 {
		t.Fatal(p, changed, err)
	}
	// Manual open keeps mute, and does not restore discarded drafts.
	p, _ = s.ChangeChatState(chat, "open", 0)
	if p.Closed || p.MuteUntil != -1 || p.DraftRevision != 2 {
		t.Fatal(p)
	}
	s.ChangeChatState(chat, "close", 0)
	s.ChangeChatState(chat, "mute", time.Now().Unix()-10)
	p, _ = s.ChatState(chat)
	if !p.Closed || p.MutedAt(time.Now().Unix()) {
		t.Fatal(p)
	}
	p, changed, err = s.LiveChatMessage(chat)
	if err != nil || !changed || p.Closed {
		t.Fatal(p, changed, err)
	}
	result, err = s.Search("secret", 50)
	if err != nil || len(result["messages"].([]map[string]any)) != 1 {
		t.Fatal(result, err)
	}
	states, err := s.ChatStates()
	if err != nil || len(states) != 1 {
		t.Fatal(states, err)
	}
	if _, err = s.ReactionState(context.Background(), chat, "history"); err != nil {
		t.Fatal(err)
	}
}
func TestChatStatePersistsWithoutSchemaChangeAndSerializesDevices(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	s.OpenChat("c", "Contact", "private")
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, e := s.ChangeChatState("c", "close", 0); e != nil {
				t.Error(e)
			}
		}()
	}
	wg.Wait()
	s.ChangeChatState("c", "mute", -1)
	s.Close()
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	p, err := s.ChatState("c")
	if err != nil || p.DraftRevision != 20 || p.Version != 21 || p.MuteUntil != -1 || !p.Closed {
		t.Fatal(p, err)
	}
	var version int
	s.DB.QueryRow(`PRAGMA user_version`).Scan(&version)
	if version != 150 {
		t.Fatal(version)
	}
}

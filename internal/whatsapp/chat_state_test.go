package whatsapp

import (
	"context"
	"go.mau.fi/whatsmeow"
	wastore "go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/types"
	"testing"
	"time"
	"wisp/internal/model"
)

type chooserContacts struct{ wastore.ContactStore }

func (chooserContacts) GetAllContacts(context.Context) (map[types.JID]types.ContactInfo, error) {
	return map[types.JID]types.ContactInfo{
		types.NewJID("49123", types.DefaultUserServer): {FullName: "Bea"},
		types.NewJID("49124", types.DefaultUserServer): {PushName: "Chris"},
		types.NewJID("status", types.BroadcastServer):  {FullName: "Status"},
	}, nil
}
func TestChooserContactsAndKnownChatValidation(t *testing.T) {
	s := mentionService(t)
	s.client = &whatsmeow.Client{Store: &wastore.Device{Contacts: chooserContacts{}}}
	s.store.OpenChat("group@g.us", "Family", "group")
	s.store.ChangeChatState("group@g.us", "close", 0)
	rows, err := s.ChatOptions(context.Background(), "")
	if err != nil || len(rows) != 3 {
		t.Fatal(rows, err)
	}
	if rows[0].ID != "group@g.us" || !rows[0].Closed {
		t.Fatal(rows)
	}
	rows, err = s.ChatOptions(context.Background(), "bea")
	if err != nil || len(rows) != 1 || !rows[0].Contact {
		t.Fatal(rows, err)
	}
	if err = s.OpenChat(context.Background(), "49123@s.whatsapp.net"); err != nil {
		t.Fatal(err)
	}
	if s.store.ChatName("49123@s.whatsapp.net") != "Bea" {
		t.Fatal("contact not created")
	}
	if err = s.OpenChat(context.Background(), "999@s.whatsapp.net"); err == nil {
		t.Fatal("arbitrary number accepted")
	}
}
func TestMutedLiveMessageRetainsInternalEventWithoutReopening(t *testing.T) {
	s := mentionService(t)
	chat := "group@g.us"
	s.store.OpenChat(chat, "Group", "group")
	s.store.ChangeChatState(chat, "mute", -1)
	s.store.ChangeChatState(chat, "close", 0)
	id, ch := s.bus.Subscribe()
	defer s.bus.Unsubscribe(id)
	s.publishLiveMessage(model.Message{ID: "new", ChatID: chat, Body: "hello"})
	select {
	case e := <-ch:
		if e.Type != "message" || !e.Data.(model.Message).Silent {
			t.Fatal(e)
		}
	case <-time.After(time.Second):
		t.Fatal("UI sync lost")
	}
	p, _ := s.store.ChatState(chat)
	if !p.Closed {
		t.Fatal("muted chat reopened")
	}
	s.store.ChangeChatState(chat, "mute", 0)
	s.publishLiveMessage(model.Message{ID: "next", ChatID: chat})
	if e := <-ch; e.Type != "chat-state" {
		t.Fatal(e)
	}
	if e := <-ch; e.Type != "message" || e.Data.(model.Message).Silent {
		t.Fatal(e)
	}
}

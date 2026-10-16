package whatsapp

import (
	"context"
	waCommon "go.mau.fi/whatsmeow/proto/waCommon"
	waE2E "go.mau.fi/whatsmeow/proto/waE2E"
	waHistorySync "go.mau.fi/whatsmeow/proto/waHistorySync"
	waWeb "go.mau.fi/whatsmeow/proto/waWeb"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"
	"testing"
	"time"
	"wisp/internal/model"
)

func editEvent(chat, actor, id, body string, ts int64) *events.Message {
	j, _ := types.ParseJID(chat)
	a, _ := types.ParseJID(actor)
	return &events.Message{Info: types.MessageInfo{MessageSource: types.MessageSource{Chat: j, Sender: a, IsGroup: true}, Timestamp: time.UnixMilli(ts)}, Message: &waE2E.Message{EditedMessage: &waE2E.FutureProofMessage{Message: &waE2E.Message{ProtocolMessage: &waE2E.ProtocolMessage{Type: waE2E.ProtocolMessage_MESSAGE_EDIT.Enum(), Key: &waCommon.MessageKey{ID: proto.String(id), RemoteJID: proto.String(chat)}, TimestampMS: proto.Int64(ts), EditedMessage: &waE2E.Message{Conversation: proto.String(body)}}}}}}
}
func TestLiveEditIsSilentAndNewRepliesQuoteCurrentText(t *testing.T) {
	s, sender := sendingService(t)
	id, ch := s.bus.Subscribe()
	defer s.bus.Unsubscribe(id)
	s.onMessage(editEvent("g@g.us", "12@lid", "original", "corrected question", 30000))
	select {
	case e := <-ch:
		if e.Type != "edit" {
			t.Fatal(e)
		}
		state := e.Data.(*model.EditState)
		if state.Body != "corrected question" || len(state.History) != 1 || state.History[0].Body != "original question" {
			t.Fatal(state)
		}
	default:
		t.Fatal("no edit event")
	}
	select {
	case e := <-ch:
		t.Fatal("extra event", e)
	default:
	}
	s.onMessage(editEvent("g@g.us", "12@lid", "original", "corrected question", 30000))
	select {
	case e := <-ch:
		t.Fatal("duplicate event", e)
	default:
	}
	_, err := s.SendText(context.Background(), "g@g.us", "answer", "original")
	if err != nil {
		t.Fatal(err)
	}
	if sender.messages[0].GetExtendedTextMessage().GetContextInfo().GetQuotedMessage().GetConversation() != "corrected question" {
		t.Fatal("reply quoted stale original")
	}
}
func TestEditRejectsOtherChatOrSender(t *testing.T) {
	s, _ := sendingService(t)
	s.onMessage(editEvent("other@g.us", "12@lid", "original", "wrong chat", 30000))
	s.onMessage(editEvent("g@g.us", "999@lid", "original", "wrong sender", 30000))
	m, err := s.store.MessageByID("original")
	if err != nil || m.Edited || m.Body != "original question" {
		t.Fatal(m, err)
	}
}
func TestUnwrappedEditAndEmptyCaption(t *testing.T) {
	s, _ := sendingService(t)
	if _, err := s.store.DB.Exec(`UPDATE messages SET type='image',has_media=1,body='old caption' WHERE id='original'`); err != nil {
		t.Fatal(err)
	}
	evt := editEvent("g@g.us", "12@lid", "original", "", 30000)
	evt.Message = evt.Message.GetEditedMessage().GetMessage()
	evt.Message.ProtocolMessage.EditedMessage = &waE2E.Message{ImageMessage: &waE2E.ImageMessage{Caption: proto.String("")}}
	s.onMessage(evt)
	m, err := s.store.MessageByID("original")
	if err != nil || !m.Edited || m.Body != "" {
		t.Fatal(m, err)
	}
}

func TestHistoryEditIsNotImportedAsNewMessage(t *testing.T) {
	s, _ := sendingService(t)
	e := editEvent("g@g.us", "12@lid", "original", "history correction", 40000)
	conv := &waHistorySync.Conversation{ID: proto.String("g@g.us"), Name: proto.String("Group"), Messages: []*waHistorySync.HistorySyncMsg{
		{Message: &waWeb.WebMessageInfo{Key: &waCommon.MessageKey{ID: proto.String("edit-envelope"), Participant: proto.String("12@lid")}, Message: e.Message, MessageTimestamp: proto.Uint64(40)}},
		{Message: &waWeb.WebMessageInfo{Key: &waCommon.MessageKey{ID: proto.String("original"), Participant: proto.String("12@lid")}, Message: &waE2E.Message{Conversation: proto.String("original question")}, MessageTimestamp: proto.Uint64(10)}},
	}}
	s.importConversation(conv, true)
	m, err := s.store.MessageByID("original")
	if err != nil || !m.Edited || m.Body != "history correction" {
		t.Fatal(m, err)
	}
	if _, err = s.store.MessageByID("edit-envelope"); err == nil {
		t.Fatal("edit became a new message")
	}
	if len(m.EditHistory) != 1 || m.EditHistory[0].Body != "original question" {
		t.Fatal(m.EditHistory)
	}
}

// ParseWebMessage is used by whatsmeow for edits synced from another own device.
// It strips the protocol envelope from Message, retaining it only in RawMessage.
func TestOwnDeviceEditParsedByWhatsmeow(t *testing.T) {
	for _, chat := range []string{"g@g.us", "88@s.whatsapp.net", "88@lid"} {
		t.Run(chat, func(t *testing.T) {
			s, _ := sendingService(t)
			own := s.client.Store.ID.ToNonAD().String()
			s.client.Store.LID = types.NewJID("777", types.HiddenUserServer)
			raw, _ := proto.Marshal(&waE2E.Message{Conversation: proto.String("my original")})
			_, err := s.store.Save(model.Message{ID: "mine", WhatsAppID: "mine", ChatID: chat, SenderID: &own, FromMe: 1, Body: "my original", Type: "chat", Raw: raw}, "Chat", "private")
			if err != nil {
				t.Fatal(err)
			}
			source := editEvent(chat, own, "mine", "my correction", 30000)
			source.Message.GetEditedMessage().GetMessage().ProtocolMessage.Key.FromMe = proto.Bool(true)
			// Device-sent wrapper is how a companion receives another own device's message.
			web := &waWeb.WebMessageInfo{Key: &waCommon.MessageKey{ID: proto.String("edit-envelope"), RemoteJID: proto.String(chat), FromMe: proto.Bool(true)}, Message: &waE2E.Message{DeviceSentMessage: &waE2E.DeviceSentMessage{DestinationJID: proto.String(chat), Message: source.Message}}, MessageTimestamp: proto.Uint64(30)}
			jid, _ := types.ParseJID(chat)
			evt, err := s.client.ParseWebMessage(jid, web)
			if err != nil {
				t.Fatal(err)
			}
			if evt.Message.GetConversation() != "my correction" {
				t.Fatal("fixture was not normalized by whatsmeow")
			}
			sub, ch := s.bus.Subscribe()
			defer s.bus.Unsubscribe(sub)
			s.onMessage(evt)
			m, err := s.store.MessageByID("mine")
			if err != nil || !m.Edited || m.Body != "my correction" || m.FromMe != 1 || len(m.EditHistory) != 1 || m.EditHistory[0].Body != "my original" {
				t.Fatalf("edit missing: %#v %v", m, err)
			}
			select {
			case e := <-ch:
				if e.Type != "edit" {
					t.Fatal(e)
				}
			default:
				t.Fatal("no edit event")
			}
			s.onMessage(evt)
			select {
			case e := <-ch:
				t.Fatal("duplicate/notification event", e)
			default:
			}
		})
	}
}

func TestOwnEditCannotModifyOtherAuthorsMessage(t *testing.T) {
	s, _ := sendingService(t)
	evt := editEvent("g@g.us", "49@s.whatsapp.net", "original", "invalid", 30000)
	evt.Info.IsFromMe = true
	s.onMessage(evt)
	m, err := s.store.MessageByID("original")
	if err != nil || m.Edited {
		t.Fatal(m, err)
	}
}

func TestOwnHistoryEditWithDifferentSelfAddress(t *testing.T) {
	s, _ := sendingService(t)
	own := s.client.Store.ID.ToNonAD().String()
	raw, _ := proto.Marshal(&waE2E.Message{Conversation: proto.String("my original")})
	_, err := s.store.Save(model.Message{ID: "mine", WhatsAppID: "mine", ChatID: "g@g.us", SenderID: &own, FromMe: 1, Type: "chat", Body: "my original", Raw: raw}, "Group", "group")
	if err != nil {
		t.Fatal(err)
	}
	evt := editEvent("g@g.us", "777@lid", "mine", "history correction", 40000)
	conv := &waHistorySync.Conversation{ID: proto.String("g@g.us"), Messages: []*waHistorySync.HistorySyncMsg{
		{Message: &waWeb.WebMessageInfo{Key: &waCommon.MessageKey{ID: proto.String("edit-history"), FromMe: proto.Bool(true), Participant: proto.String("777@lid")}, Message: evt.Message, MessageTimestamp: proto.Uint64(40)}},
	}}
	s.importConversation(conv, true)
	m, err := s.store.MessageByID("mine")
	if err != nil || !m.Edited || m.Body != "history correction" {
		t.Fatal("own history edit not applied", err)
	}
	// A remote author must not update our message, even if it uses our sender ID.
	foreign := editEvent("g@g.us", own, "mine", "invalid", 50000)
	s.onMessage(foreign)
	m, err = s.store.MessageByID("mine")
	if err != nil || m.Body != "history correction" {
		t.Fatal("foreign edit changed own message", err)
	}
}

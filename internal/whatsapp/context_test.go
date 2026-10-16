package whatsapp

import (
	"context"
	"errors"
	"go.mau.fi/whatsmeow"
	waCommon "go.mau.fi/whatsmeow/proto/waCommon"
	waE2E "go.mau.fi/whatsmeow/proto/waE2E"
	waWeb "go.mau.fi/whatsmeow/proto/waWeb"
	wastore "go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"
	"testing"
	"time"
	"wisp/internal/model"
)

type captureSender struct {
	messages []*waE2E.Message
	fail     bool
}

func (*captureSender) IsConnected() bool { return true }
func (*captureSender) IsLoggedIn() bool  { return true }
func (s *captureSender) SendMessage(ctx context.Context, j types.JID, m *waE2E.Message, _ ...whatsmeow.SendRequestExtra) (whatsmeow.SendResponse, error) {
	if s.fail {
		return whatsmeow.SendResponse{}, errors.New("provider failed")
	}
	s.messages = append(s.messages, proto.Clone(m).(*waE2E.Message))
	return whatsmeow.SendResponse{ID: "sent", Timestamp: time.Now()}, nil
}
func sendingService(t *testing.T) (*Service, *captureSender) {
	s := mentionService(t)
	own := types.NewJID("49", types.DefaultUserServer)
	s.client = &whatsmeow.Client{Store: &wastore.Device{ID: &own}}
	sender := &captureSender{}
	s.sender = sender
	s.groupNames["g@g.us"] = "Group"
	raw, _ := proto.Marshal(&waE2E.Message{Conversation: proto.String("original question")})
	sid := "12@lid"
	name := "Arthur"
	_, err := s.store.Save(model.Message{ID: "original", WhatsAppID: "original", ChatID: "g@g.us", SenderID: &sid, SenderName: &name, Body: "original question", Type: "chat", Raw: raw, OriginalChat: "g@g.us", OriginalSender: sid}, "Group", "group")
	if err != nil {
		t.Fatal(err)
	}
	return s, sender
}
func TestSendReplyUsesOriginalContextAndPersistsSnapshot(t *testing.T) {
	s, sender := sendingService(t)
	id, err := s.SendText(context.Background(), "g@g.us", "answer", "original")
	if err != nil {
		t.Fatal(err)
	}
	c := sender.messages[0].GetExtendedTextMessage().GetContextInfo()
	if c.GetStanzaID() != "original" || c.GetParticipant() != "12@lid" || c.GetQuotedMessage().GetConversation() != "original question" {
		t.Fatal(c)
	}
	m, err := s.store.MessageByID(id)
	if err != nil || m.Reply == nil || m.Reply.Body != "original question" || len(m.Raw) == 0 {
		t.Fatalf("missing saved reply: %#v %v", m, err)
	}
	if _, err = s.SendText(context.Background(), "other@g.us", "answer", "original"); !errors.Is(err, ErrInvalidAction) {
		t.Fatal("cross-chat reply accepted", err)
	}
	if len(sender.messages) != 1 {
		t.Fatal("invalid reply sent")
	}
}
func TestSendReactionSubsetRemovalAndMirrorPolicy(t *testing.T) {
	s, sender := sendingService(t)
	ctx := context.Background()
	if _, err := s.SendReaction(ctx, "g@g.us", "original", "🦊"); !errors.Is(err, ErrInvalidAction) {
		t.Fatal(err)
	}
	state, err := s.SendReaction(ctx, "g@g.us", "original", "👍")
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Reactions) != 1 || !state.Reactions[0].FromMe {
		t.Fatal(state)
	}
	reaction := sender.messages[0].GetReactionMessage()
	if reaction.GetKey().GetID() != "original" || reaction.GetKey().GetFromMe() || reaction.GetKey().GetParticipant() != "12@lid" {
		t.Fatal(reaction)
	}
	state, err = s.SendReaction(ctx, "g@g.us", "original", "")
	if err != nil || len(state.Reactions) != 0 {
		t.Fatal(state, err)
	}
	s.store.SetConfigOverride("mirrorOnly", "true")
	if _, err = s.SendReaction(ctx, "g@g.us", "original", "👍"); !errors.Is(err, ErrInvalidAction) {
		t.Fatal(err)
	}
	if _, err = s.SendText(ctx, "g@g.us", "test", ""); !errors.Is(err, ErrInvalidAction) {
		t.Fatal(err)
	}
	if len(sender.messages) != 2 {
		t.Fatal("unexpected send")
	}
}
func TestReactionSendFailureDoesNotClaimSuccess(t *testing.T) {
	s, sender := sendingService(t)
	sender.fail = true
	if _, err := s.SendReaction(context.Background(), "g@g.us", "original", "👍"); err == nil {
		t.Fatal("missing failure")
	}
	state, _ := s.store.ReactionState(context.Background(), "g@g.us", "original")
	if len(state.Reactions) != 0 || state.Version != 0 {
		t.Fatal("failed reaction saved")
	}
}
func TestIncomingReactionOnlyPublishesReactionEvent(t *testing.T) {
	s, _ := sendingService(t)
	id, ch := s.bus.Subscribe()
	defer s.bus.Unsubscribe(id)
	before, _ := s.store.Chats()
	event := &events.Message{Info: types.MessageInfo{MessageSource: types.MessageSource{Chat: types.NewJID("g", types.GroupServer), Sender: types.NewJID("12", types.HiddenUserServer), IsGroup: true}, ID: "react", Timestamp: time.Now(), PushName: "Arthur"}, Message: &waE2E.Message{ReactionMessage: &waE2E.ReactionMessage{Key: &waCommon.MessageKey{ID: proto.String("original")}, Text: proto.String("🦊"), SenderTimestampMS: proto.Int64(100)}}}
	s.onMessage(event)
	s.onMessage(event)
	select {
	case e := <-ch:
		if e.Type != "reaction" {
			t.Fatal(e.Type)
		}
	default:
		t.Fatal("missing reaction event")
	}
	select {
	case e := <-ch:
		t.Fatal("duplicate/message event", e.Type)
	default:
	}
	after, _ := s.store.Chats()
	if len(before) != len(after) || before[0]["last_message_at"] != after[0]["last_message_at"] {
		t.Fatal("reaction reordered chat")
	}
	if _, err := s.store.MessageByID("react"); err == nil {
		t.Fatal("reaction inserted as message")
	}
}
func TestHistoryReactionsDoNotOverwriteNewerLiveState(t *testing.T) {
	s, _ := sendingService(t)
	s.store.SaveReaction(model.Reaction{ChatID: "g@g.us", MessageID: "original", ActorID: "12@lid", Emoji: "", TimestampMS: 200, EventID: "new"})
	w := &waWeb.WebMessageInfo{Reactions: []*waWeb.Reaction{{Key: &waCommon.MessageKey{Participant: proto.String("12@lid"), ID: proto.String("old")}, Text: proto.String("👍"), SenderTimestampMS: proto.Int64(100)}}}
	s.importReactions(context.Background(), "g@g.us", "original", w)
	state, _ := s.store.ReactionState(context.Background(), "g@g.us", "original")
	if len(state.Reactions) != 0 {
		t.Fatal("history resurrected removed reaction")
	}
}

func TestGroupSenderNamesAgreeForHistoryLiveAndStoredRows(t *testing.T) {
	s, _ := sendingService(t)
	s.client.Store.Contacts = mentionContacts{}
	s.client.Store.LIDs = mentionLIDs{}
	chat := types.NewJID("g", types.GroupServer)
	actor := types.NewJID("222676958327013", types.HiddenUserServer)
	raw := &waE2E.Message{Conversation: proto.String("hello")}
	historical, reason := s.normalizeHistoryMessage(context.Background(), &waWeb.WebMessageInfo{
		Key:     &waCommon.MessageKey{ID: proto.String("history-name"), Participant: proto.String(actor.String())},
		Message: raw, PushName: proto.String("Profile nickname"), MessageTimestamp: proto.Uint64(100),
	}, chat, chat.String())
	if historical == nil {
		t.Fatal(reason)
	}
	if historical.Message.SenderName == nil || *historical.Message.SenderName != "Arthur" {
		t.Fatalf("history: %#v", historical.Message.SenderName)
	}
	s.onMessage(&events.Message{Info: types.MessageInfo{MessageSource: types.MessageSource{Chat: chat, Sender: actor, IsGroup: true}, ID: "live-name", PushName: "Profile nickname", Timestamp: time.Unix(101, 0)}, Message: raw})
	live, err := s.store.MessageByID("live-name")
	if err != nil || live.SenderName == nil || *live.SenderName != "Arthur" {
		t.Fatalf("live: %#v %v", live, err)
	}
	// Old stored names are corrected for display without rewriting their snapshots.
	saveNamedSender(t, s, "old-name", actor.String(), "Old profile")
	rows, err := s.store.Messages("group@g.us", 50, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	s.ResolveMentionRows(context.Background(), rows, "chat_id", "body", "id")
	if len(rows) != 1 || rows[0]["sender_name"] != "Arthur" {
		t.Fatalf("stored display: %#v", rows)
	}
	original, err := s.store.MessageByID("old-name")
	if err != nil || *original.SenderName != "Old profile" {
		t.Fatal("stored snapshot modified", err)
	}
	if got := s.senderDisplayName(context.Background(), types.NewJID("999", types.DefaultUserServer), "Fallback profile"); got != "Fallback profile" {
		t.Fatal(got)
	}
}

func TestRevokeOnlyEmitsDeletionAndReactionNamesMatch(t *testing.T) {
	s, _ := sendingService(t)
	id, ch := s.bus.Subscribe()
	defer s.bus.Unsubscribe(id)
	raw := &waE2E.Message{ProtocolMessage: &waE2E.ProtocolMessage{Type: waE2E.ProtocolMessage_REVOKE.Enum(), Key: &waCommon.MessageKey{ID: proto.String("original"), Participant: proto.String("12@lid")}}}
	s.onMessage(&events.Message{Info: types.MessageInfo{MessageSource: types.MessageSource{Chat: types.NewJID("g", types.GroupServer), IsGroup: true}, Timestamp: time.Unix(100, 0)}, Message: raw})
	select {
	case event := <-ch:
		if event.Type != "deletion" {
			t.Fatal(event)
		}
	default:
		t.Fatal("no deletion event")
	}
	select {
	case event := <-ch:
		t.Fatal("unexpected event", event)
	default:
	}
	m, err := s.store.MessageByID("original")
	if err != nil || !m.Deleted || m.Body != "original question" {
		t.Fatal(m, err)
	}
	s.client.Store.Contacts = mentionContacts{}
	s.client.Store.LIDs = mentionLIDs{}
	rows := []model.Reaction{{ActorID: "222676958327013@lid", ActorName: "Old profile", Emoji: "👍"}}
	s.decorateReactions(context.Background(), rows)
	if rows[0].ActorName != "Arthur" {
		t.Fatal(rows)
	}
}

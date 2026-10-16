package whatsapp

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"go.mau.fi/whatsmeow"
	waE2E "go.mau.fi/whatsmeow/proto/waE2E"
	wastore "go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"
	"wisp/internal/events"
	"wisp/internal/model"
	"wisp/internal/store"
)

func TestMentionTokenBoundaries(t *testing.T) {
	cases := map[string]string{
		"Hi @123!":           "Hi @Arthur!",
		"(@123), @123\n@456": "(@Arthur), @Arthur\n@Bea",
		"@1234 @999":         "@1234 @999",
		"a@123.com https://example.org/@123 www.example.org/@123": "a@123.com https://example.org/@123 www.example.org/@123",
		"hello@123 @123abc @@123 /@123":                           "hello@123 @123abc @@123 /@123",
		"ä@123 @123ä":                                             "ä@123 @123ä",
		"🙂 @123 and @456.":                                        "🙂 @Arthur and @Bea.",
	}
	for input, want := range cases {
		t.Run(input, func(t *testing.T) {
			got := replaceMentionTokens(input, func(id string) string { return map[string]string{"123": "Arthur", "456": "Bea"}[id] })
			if got != want {
				t.Fatalf("got %q, want %q", got, want)
			}
		})
	}
}
func TestMentionContextUnwrapsTextAndCaptions(t *testing.T) {
	ctx := &waE2E.ContextInfo{MentionedJID: []string{"123@lid"}}
	messages := []*waE2E.Message{
		{ExtendedTextMessage: &waE2E.ExtendedTextMessage{Text: proto.String("@123"), ContextInfo: ctx}},
		{ImageMessage: &waE2E.ImageMessage{Caption: proto.String("@123"), ContextInfo: ctx}},
		{VideoMessage: &waE2E.VideoMessage{Caption: proto.String("@123"), ContextInfo: ctx}},
		{EphemeralMessage: &waE2E.FutureProofMessage{Message: &waE2E.Message{ExtendedTextMessage: &waE2E.ExtendedTextMessage{ContextInfo: ctx}}}},
	}
	for _, m := range messages {
		got := messageMentionJIDs(m)
		if len(got) != 1 || got[0] != "123@lid" {
			t.Fatalf("missing context: %v", got)
		}
	}
	if len(messageMentionJIDs(nil)) != 0 || len(messageMentionJIDs(&waE2E.Message{Conversation: proto.String("hello")})) != 0 {
		t.Fatal("invented mentions")
	}
}
func mentionService(t *testing.T) *Service {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "wisp.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return New(st, events.New(), Options{})
}
func saveNamedSender(t *testing.T, s *Service, id, jid, name string) {
	t.Helper()
	_, err := s.store.Save(model.Message{ID: id, WhatsAppID: id, ChatID: "group@g.us", SenderID: &jid, SenderName: &name, Type: "chat", Body: "hello", Timestamp: 1}, "Group", "group")
	if err != nil {
		t.Fatal(err)
	}
}
func TestMentionResolutionPreservesRawBody(t *testing.T) {
	s := mentionService(t)
	saveNamedSender(t, s, "sender", "123@lid", "Arthur")
	body := "Hi @123 and @999"
	for _, id := range []string{"new", "literal"} {
		m := model.Message{ID: id, WhatsAppID: id, ChatID: "group@g.us", Type: "chat", Body: body, Timestamp: 2}
		if id == "new" {
			m.MentionJIDs = []string{"123@lid", "999@lid"}
		}
		if _, err := s.store.Save(m, "Group", "group"); err != nil {
			t.Fatal(err)
		}
	}
	rows, err := s.store.Messages("group@g.us", 50, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	s.ResolveMentionRows(context.Background(), rows, "chat_id", "body", "id")
	for _, row := range rows {
		id := row["id"].(string)
		if id == "sender" {
			continue
		}
		want := "Hi @Arthur and @999"
		if id == "literal" {
			want = body
		}
		if row["body"] != want {
			t.Fatalf("%s: got %q, want %q", id, row["body"], want)
		}
		stored, err := s.store.MessageByID(id)
		if err != nil || stored.Body != body {
			t.Fatal("stored body was changed", err)
		}
	}
	if got := s.mentionBody(context.Background(), "private@s.whatsapp.net", "@123", []string{"123@lid"}, true); got != "@123" {
		t.Fatal("changed private chat")
	}
}
func TestUndeclaredAndAmbiguousMentionsStayUnchanged(t *testing.T) {
	s := mentionService(t)
	saveNamedSender(t, s, "lid", "123@lid", "Arthur")
	saveNamedSender(t, s, "pn", "123@s.whatsapp.net", "Bea")
	if got := s.mentionBody(context.Background(), "group@g.us", "@123", nil, false); got != "@123" {
		t.Fatal(got)
	}
	if got := s.mentionBody(context.Background(), "group@g.us", "@123", []string{"123@lid"}, true); got != "@Arthur" {
		t.Fatal(got)
	}
}
func TestMentionMetadataBackfillAndCleanup(t *testing.T) {
	s := mentionService(t)
	m := model.Message{ID: "m", WhatsAppID: "m", ChatID: "group@g.us", Type: "chat", Body: "@123", Timestamp: 2}
	if _, err := s.store.Save(m, "Group", "group"); err != nil {
		t.Fatal(err)
	}
	m.MentionJIDs = []string{"123@lid"}
	if inserted, err := s.store.Save(m, "Group", "group"); err != nil || inserted {
		t.Fatal("duplicate was not safely enriched", err)
	}
	m.MentionJIDs = nil
	if _, err := s.store.Save(m, "Group", "group"); err != nil {
		t.Fatal(err)
	}
	info, err := s.store.MessageMentions(context.Background(), []string{"m"})
	if err != nil || strings.Join(info["m"], ",") != "123@lid" {
		t.Fatal("metadata lost", info, err)
	}
	if _, err := s.store.DB.Exec(`DELETE FROM messages WHERE id='m'`); err != nil {
		t.Fatal(err)
	}
	info, err = s.store.MessageMentions(context.Background(), []string{"m"})
	if err != nil || len(info) != 0 {
		t.Fatal("metadata was not removed", err)
	}
}

type mentionContacts struct{ wastore.ContactStore }

func (mentionContacts) GetContact(ctx context.Context, jid types.JID) (types.ContactInfo, error) {
	if jid.String() == "491234@s.whatsapp.net" {
		return types.ContactInfo{Found: true, FullName: "Arthur", PushName: "Other"}, nil
	}
	return types.ContactInfo{}, nil
}

type mentionLIDs struct{ wastore.LIDStore }

func (mentionLIDs) GetPNForLID(ctx context.Context, jid types.JID) (types.JID, error) {
	if jid.User == "222676958327013" {
		return types.NewJID("491234", types.DefaultUserServer), nil
	}
	return types.JID{}, nil
}
func TestLIDMentionUsesMappedContactName(t *testing.T) {
	s := mentionService(t)
	s.client = &whatsmeow.Client{Store: &wastore.Device{Contacts: mentionContacts{}, LIDs: mentionLIDs{}}}
	got := s.mentionBody(context.Background(), "group@g.us", "Hi @222676958327013!", []string{"222676958327013@lid"}, true)
	if got != "Hi @Arthur!" {
		t.Fatal(got)
	}
}

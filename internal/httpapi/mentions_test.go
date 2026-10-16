package httpapi

import (
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"wisp/internal/events"
	"wisp/internal/model"
	"wisp/internal/store"
	"wisp/internal/whatsapp"
)

func TestMentionNamesAcrossMessageAPIAndPreviews(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "wisp.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	jid, name := "123@lid", "Arthur"
	messages := []model.Message{
		{ID: "sender", WhatsAppID: "sender", ChatID: "group@g.us", SenderID: &jid, SenderName: &name, Type: "chat", Body: "hello", Timestamp: 1},
		{ID: "mention", WhatsAppID: "mention", ChatID: "group@g.us", Type: "chat", Body: "Hi @123", MentionJIDs: []string{jid}, Timestamp: 2},
	}
	for _, m := range messages {
		if _, err := st.Save(m, "Group", "group"); err != nil {
			t.Fatal(err)
		}
	}
	b := events.New()
	wa := whatsapp.New(st, b, whatsapp.Options{})
	handler := New(st, wa, b, nil, nil, nil, "").Handler()
	for _, path := range []string{
		"/api/chats", "/api/chats/group@g.us/messages",
		"/api/chats/group@g.us/messages/mention/context", "/api/search?q=Hi",
	} {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest("GET", path, nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != 200 {
				t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
			}
			if !json.Valid(rec.Body.Bytes()) || !strings.Contains(rec.Body.String(), "Hi @Arthur") {
				t.Fatal(rec.Body.String())
			}
		})
	}
	m, err := st.MessageByID("mention")
	if err != nil || m.Body != "Hi @123" {
		t.Fatal("stored text changed", err)
	}
}

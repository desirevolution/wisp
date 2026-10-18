package httpapi

import (
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"wisp/internal/events"
	"wisp/internal/store"
	"wisp/internal/whatsapp"
)

func TestChatStateEndpointsAndFilteredLists(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	st.OpenChat("c@g.us", "Family", "group")
	b := events.New()
	wa := whatsapp.New(st, b, whatsapp.Options{})
	handler := New(st, wa, b, nil, nil, nil, "").Handler()
	request := func(method, path, body string, code int) string {
		t.Helper()
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(method, path, strings.NewReader(body)))
		if rec.Code != code {
			t.Fatalf("%s %s: %d %s", method, path, rec.Code, rec.Body.String())
		}
		return rec.Body.String()
	}
	for _, value := range []string{"8h", "1w", "forever", "off"} {
		raw := request("POST", "/api/chats/c@g.us/state", `{"action":"mute","mute_for":"`+value+`"}`, 200)
		var p store.ChatState
		json.Unmarshal([]byte(raw), &p)
		if value == "forever" && p.MuteUntil != -1 {
			t.Fatal(p)
		}
		if value == "off" && p.MuteUntil != 0 {
			t.Fatal(p)
		}
		if (value == "8h" || value == "1w") && p.MuteUntil <= time.Now().Unix() {
			t.Fatal(p)
		}
	}
	request("POST", "/api/chats/c@g.us/state", `{"action":"mute","mute_for":"bad"}`, 400)
	request("POST", "/api/chats/c@g.us/state", `{"action":"close"}`, 200)
	if raw := request("GET", "/api/chats", "", 200); strings.TrimSpace(raw) != "[]" {
		t.Fatal(raw)
	}
	if raw := request("GET", "/api/chat-options?q=Family", "", 200); !strings.Contains(raw, `"closed":true`) {
		t.Fatal(raw)
	}
	request("POST", "/api/chats/c@g.us/state", `{"action":"open"}`, 200)
	if raw := request("GET", "/api/chats", "", 200); !strings.Contains(raw, "Family") {
		t.Fatal(raw)
	}
	request("GET", "/api/chats/c@g.us/state", "", 405)
	request("POST", "/api/chats/not-a-contact/state", `{"action":"open"}`, 404)
	request("POST", "/api/chats/not-a-contact/state", `{"action":"close"}`, 404)
}

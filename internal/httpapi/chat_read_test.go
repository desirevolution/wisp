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

func TestReadEndpointAndServerSnapshot(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	for _, id := range []string{"one", "two"} {
		if _, err = st.SaveLive(model.Message{ID: id, WhatsAppID: id, ChatID: "c", Type: "chat"}, "C", "private"); err != nil {
			t.Fatal(err)
		}
	}
	bus := events.New()
	wa := whatsapp.New(st, bus, whatsapp.Options{})
	handler := New(st, wa, bus, nil, nil, nil, "").Handler()
	id, ch := bus.Subscribe()
	defer bus.Unsubscribe(id)
	req := func(method, path, body string, code int) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRecorder()
		handler.ServeHTTP(r, httptest.NewRequest(method, path, strings.NewReader(body)))
		if r.Code != code {
			t.Fatal(r.Code, r.Body.String())
		}
		return r
	}
	req("GET", "/api/chats/c/read", "", 405)
	for _, body := range []string{`{}`, `{"through_sequence":-1}`, `{"through_sequence":3}`, `{"through_sequence":1,"other":true}`} {
		req("POST", "/api/chats/c/read", body, 400)
	}
	req("POST", "/api/chats/missing/read", `{"through_sequence":0}`, 404)
	raw := req("POST", "/api/chats/c/read", `{"through_sequence":1}`, 200)
	var p store.ChatState
	if err = json.Unmarshal(raw.Body.Bytes(), &p); err != nil || p.UnreadSequence-p.ReadSequence != 1 {
		t.Fatal(p, err)
	}
	if event := <-ch; event.Type != "unread" {
		t.Fatal(event)
	}
	states, err := st.ChatStates()
	if err != nil || len(states) != 1 || states[0].ReadSequence != 1 {
		t.Fatal(states, err)
	}
	// GET overview never reads chats, nor does any WhatsApp operation occur.
	req("GET", "/api/chats", "", 200)
	p, _ = st.ChatState("c")
	if p.ReadSequence != 1 {
		t.Fatal(p)
	}
}

func TestReadModeAPIValidationAndQueuedRead(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	bus := events.New()
	wa := whatsapp.New(st, bus, whatsapp.Options{})
	handler := New(st, wa, bus, nil, nil, nil, "").Handler()
	request := func(method, path, body string, code int) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRecorder()
		handler.ServeHTTP(r, httptest.NewRequest(method, path, strings.NewReader(body)))
		if r.Code != code {
			t.Fatal(r.Code, r.Body.String())
		}
		return r
	}
	raw := request("GET", "/api/config", "", 200)
	if !strings.Contains(raw.Body.String(), `"readMode":"wisp"`) {
		t.Fatal(raw.Body.String())
	}
	request("POST", "/api/config", `{"readMode":"invalid","followNewMessages":true}`, 400)
	if st.ConfigBool("followNewMessages", false) {
		t.Fatal("partial invalid config applied")
	}
	subscriber, ch := bus.Subscribe()
	defer bus.Unsubscribe(subscriber)
	request("POST", "/api/config", `{"readMode":"devices"}`, 200)
	if event := <-ch; event.Type != "config" {
		t.Fatal(event)
	}
	st.SaveLive(model.Message{ID: "a", WhatsAppID: "a", ChatID: "123@s.whatsapp.net", Type: "chat", Timestamp: 10}, "C", "private")
	request("POST", "/api/chats/123@s.whatsapp.net/read", `{"through_sequence":1}`, 200)
	jobs, err := st.ReadSyncJobs()
	if err != nil || len(jobs) != 1 || jobs[0].Mode != store.ReadModeSelf {
		t.Fatal(jobs, err)
	}
	request("POST", "/api/config", `{"readMode":"wisp"}`, 200)
	jobs, _ = st.ReadSyncJobs()
	if len(jobs) != 0 {
		t.Fatal(jobs)
	}
}

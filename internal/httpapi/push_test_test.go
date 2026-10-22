package httpapi

import (
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"wisp/internal/events"
	"wisp/internal/pushsvc"
	"wisp/internal/store"
	"wisp/internal/whatsapp"
)

func TestPushTestEndpointCannotBeEnabledThroughUIConfig(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "wisp.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	bus := events.New()
	push, err := pushsvc.New(st, bus)
	if err != nil {
		t.Fatal(err)
	}
	api := New(st, whatsapp.New(st, bus, whatsapp.Options{}), bus, nil, push, nil, "")
	handler := api.Handler()
	request := func(method, url, body string) *httptest.ResponseRecorder {
		t.Helper()
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(method, url, strings.NewReader(body)))
		return rec
	}
	st.SetConfigOverride("pushTestEnabled", "false")
	if rec := request("POST", "/api/push/test", ""); rec.Code != 403 {
		t.Fatal(rec.Code, rec.Body.String())
	}
	if rec := request("GET", "/api/push/test", ""); rec.Code != 405 || rec.Header().Get("Allow") != "POST" {
		t.Fatal(rec.Code, rec.Header())
	}
	request("POST", "/api/config", `{"pushTestEnabled":true}`)
	if rec := request("POST", "/api/push/test", ""); rec.Code != 403 {
		t.Fatal("UI enabled deployment policy", rec.Code)
	}
	st.SetConfigOverride("pushTestEnabled", "true")
	rec := request("POST", "/api/push/test", "")
	var report pushsvc.TestReport
	if err := json.Unmarshal(rec.Body.Bytes(), &report); err != nil || rec.Code != 200 || report.Total != 0 || report.Results == nil || report.TestID == "" {
		t.Fatal(rec.Code, rec.Body.String(), err)
	}
	// No WhatsApp Start/pairing was needed. Status advertises only deployment policy.
	rec = request("GET", "/api/status", "")
	var status struct {
		Push pushsvc.Status `json:"push"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &status); err != nil || !status.Push.TestEnabled {
		t.Fatal(rec.Code, rec.Body.String(), err)
	}
	st.SetConfigOverride("pushTestEnabled", "false")
	if rec = request("POST", "/api/push/test", ""); rec.Code != 403 {
		t.Fatal(rec.Code)
	}
}

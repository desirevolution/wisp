package httpapi

import (
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"wisp/internal/appconfig"
	"wisp/internal/events"
	"wisp/internal/mqttsvc"
	"wisp/internal/pushsvc"
	"wisp/internal/store"
	"wisp/internal/whatsapp"
)

func TestReleaseVersionAndUnpairedHealth(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "wisp.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	bus := events.New()
	wa := whatsapp.New(st, bus, whatsapp.Options{})
	push, err := pushsvc.New(st, bus)
	if err != nil {
		t.Fatal(err)
	}
	mq := mqttsvc.New(st, bus)
	handler := New(st, wa, bus, mq, push, nil, "").Handler()
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/api/status", nil))
	var status struct {
		Version string `json:"version"`
	}
	if err = json.Unmarshal(rec.Body.Bytes(), &status); err != nil || rec.Code != 200 || status.Version != appconfig.Version {
		t.Fatal(rec.Code, status, err)
	}
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/health", nil))
	var health struct {
		Database string `json:"database"`
	}
	if err = json.Unmarshal(rec.Body.Bytes(), &health); err != nil || rec.Code != 503 || health.Database != "ok" {
		t.Fatal(rec.Code, health, err)
	}
}

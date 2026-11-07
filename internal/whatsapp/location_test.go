package whatsapp

import (
	"context"
	"math"
	"testing"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"google.golang.org/protobuf/proto"

	"wisp/internal/model"
)

func TestLocationNormalizationAndFirstLiveSnapshot(t *testing.T) {
	s := mentionService(t)
	normal := &waE2E.Message{LocationMessage: &waE2E.LocationMessage{
		DegreesLatitude: proto.Float64(0), DegreesLongitude: proto.Float64(0), Name: proto.String(" Test place "),
	}}
	var m model.Message
	if !normalizeContent(&m, normal) || m.Type != "location" || m.Body != "Test place" || m.Location.Live {
		t.Fatalf("invalid regular location: %#v", m)
	}
	normal.LocationMessage.DegreesLongitude = proto.Float64(math.NaN())
	if normalizeContent(&m, normal) {
		t.Fatal("invalid coordinates were accepted")
	}
	for _, lat := range []float64{50, 51} {
		payload := &waE2E.Message{LiveLocationMessage: &waE2E.LiveLocationMessage{
			DegreesLatitude: proto.Float64(lat), DegreesLongitude: proto.Float64(7),
		}}
		m = model.Message{ID: "location", WhatsAppID: "location", ChatID: "chat", Timestamp: 100}
		if !normalizeContent(&m, payload) || m.Type != "live_location" || !m.Location.Live {
			t.Fatalf("invalid live location: %#v", m)
		}
		m.Raw, _ = proto.Marshal(payload)
		if _, err := s.store.Save(m, "Chat", "private"); err != nil {
			t.Fatal(err)
		}
	}
	for _, load := range []func() ([]map[string]any, error){
		func() ([]map[string]any, error) { return s.store.Messages("chat", 10, nil, "") },
		func() ([]map[string]any, error) { return s.store.MessageContext("chat", "location", 3) },
	} {
		rows, err := load()
		if err != nil || len(rows) != 1 {
			t.Fatalf("load returned %d rows: %v", len(rows), err)
		}
		s.ResolveLocationRows(context.Background(), rows)
		point, ok := rows[0]["location"].(*model.Location)
		if !ok || !point.Live || point.Latitude != 50 || point.Longitude != 7 {
			t.Fatalf("first snapshot changed: %#v", rows[0])
		}
	}
}

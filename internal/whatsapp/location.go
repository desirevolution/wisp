package whatsapp

import (
	"context"
	"math"
	"strings"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"google.golang.org/protobuf/proto"

	"wisp/internal/model"
)

func messageLocation(m *waE2E.Message) *model.Location {
	m, _ = unwrapContent(m)
	if m == nil {
		return nil
	}
	if p := m.GetLocationMessage(); p != nil {
		location := checkedLocation(p.DegreesLatitude, p.DegreesLongitude, p.GetIsLive())
		if location != nil {
			location.Name = strings.TrimSpace(p.GetName())
			location.Address = strings.TrimSpace(p.GetAddress())
		}
		return location
	}
	if p := m.GetLiveLocationMessage(); p != nil {
		return checkedLocation(p.DegreesLatitude, p.DegreesLongitude, true)
	}
	return nil
}

func checkedLocation(lat, lon *float64, live bool) *model.Location {
	if lat == nil || lon == nil || math.IsNaN(*lat) || math.IsNaN(*lon) ||
		math.Abs(*lat) > 90 || math.Abs(*lon) > 180 {
		return nil
	}
	return &model.Location{Latitude: *lat, Longitude: *lon, Live: live}
}

func normalizeContent(m *model.Message, raw *waE2E.Message) bool {
	m.Location = messageLocation(raw)
	if m.Location == nil {
		m.Type, m.Body, m.HasMedia = summary(raw)
		return isVisibleMessage(m.Type, m.Body, m.HasMedia)
	}
	m.Type, m.Body = "location", "Location shared"
	if m.Location.Live {
		m.Type, m.Body = "live_location", "Live location shared"
	}
	if m.Location.Name != "" {
		m.Body = m.Location.Name
	} else if m.Location.Address != "" {
		m.Body = m.Location.Address
	}
	return true
}

// Read the retained WhatsApp descriptor so older Wisp databases need no migration.
func (s *Service) ResolveLocationRows(ctx context.Context, rows []map[string]any) {
	var ids []string
	for _, row := range rows {
		kind, _ := row["type"].(string)
		if kind == "location" || kind == "live_location" {
			if id, ok := row["id"].(string); ok {
				ids = append(ids, id)
			}
		}
	}
	if len(ids) == 0 {
		return
	}
	payloads, err := s.store.MediaLayoutPayloads(ctx, ids)
	if err != nil {
		return
	}
	for _, row := range rows {
		id, _ := row["id"].(string)
		if raw := payloads[id]; len(raw) != 0 {
			var m waE2E.Message
			if proto.Unmarshal(raw, &m) == nil {
				if location := messageLocation(&m); location != nil {
					row["location"] = location
				}
			}
		}
	}
}

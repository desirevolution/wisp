package whatsapp

import (
	"context"
	"fmt"

	waE2E "go.mau.fi/whatsmeow/proto/waE2E"
	"google.golang.org/protobuf/proto"
)

func mediaDimensions(raw *waE2E.Message) (uint32, uint32) {
	m, _ := unwrapContent(raw)
	var width, height uint32
	switch {
	case m.GetImageMessage() != nil:
		width, height = m.GetImageMessage().GetWidth(), m.GetImageMessage().GetHeight()
	case m.GetVideoMessage() != nil:
		width, height = m.GetVideoMessage().GetWidth(), m.GetVideoMessage().GetHeight()
	case m.GetStickerMessage() != nil:
		width, height = m.GetStickerMessage().GetWidth(), m.GetStickerMessage().GetHeight()
	}
	if width == 0 || height == 0 {
		return 0, 0
	}
	return width, height
}

// ResolveMediaDimensions exposes only layout sizes, never private media keys or
// descriptors. Existing retained messages benefit immediately, before download.
func (s *Service) ResolveMediaDimensions(ctx context.Context, rows []map[string]any) {
	if s.store == nil {
		return
	}
	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		kind, _ := row["type"].(string)
		id, _ := row["id"].(string)
		if id != "" && (kind == "image" || kind == "video" || kind == "gif" || kind == "sticker") {
			ids = append(ids, id)
		}
	}
	payloads, err := s.store.MediaLayoutPayloads(ctx, ids)
	if err != nil {
		fmt.Printf("[media] layout metadata: %v\n", err)
		return
	}
	for _, row := range rows {
		id, _ := row["id"].(string)
		raw := payloads[id]
		if len(raw) == 0 {
			continue
		}
		var message waE2E.Message
		if proto.Unmarshal(raw, &message) != nil {
			continue
		}
		if width, height := mediaDimensions(&message); width > 0 && height > 0 {
			row["media_width"], row["media_height"] = width, height
		}
	}
}

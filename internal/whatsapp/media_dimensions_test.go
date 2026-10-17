package whatsapp

import (
	"context"
	"testing"

	waE2E "go.mau.fi/whatsmeow/proto/waE2E"
	"google.golang.org/protobuf/proto"
	"wisp/internal/model"
)

func TestMediaDimensionsUnwrapAndRejectIncompleteSizes(t *testing.T) {
	cases := []struct {
		message       *waE2E.Message
		width, height uint32
	}{
		{nil, 0, 0},
		{&waE2E.Message{ImageMessage: &waE2E.ImageMessage{Width: proto.Uint32(600), Height: proto.Uint32(1200)}}, 600, 1200},
		{&waE2E.Message{VideoMessage: &waE2E.VideoMessage{Width: proto.Uint32(1920), Height: proto.Uint32(1080)}}, 1920, 1080},
		{&waE2E.Message{StickerMessage: &waE2E.StickerMessage{Width: proto.Uint32(64), Height: proto.Uint32(64)}}, 64, 64},
		{&waE2E.Message{ImageMessage: &waE2E.ImageMessage{Width: proto.Uint32(600)}}, 0, 0},
		{&waE2E.Message{EphemeralMessage: &waE2E.FutureProofMessage{Message: &waE2E.Message{ImageMessage: &waE2E.ImageMessage{Width: proto.Uint32(400), Height: proto.Uint32(800)}}}}, 400, 800},
	}
	for _, c := range cases {
		if w, h := mediaDimensions(c.message); w != c.width || h != c.height {
			t.Fatalf("got %dx%d, want %dx%d", w, h, c.width, c.height)
		}
	}
}

func TestStoredMediaDimensionsWorkBeforeDownloadWithoutSchemaChange(t *testing.T) {
	s := mentionService(t)
	ctx := context.Background()
	for _, id := range []string{"descriptor", "original", "missing", "corrupt"} {
		m := model.Message{ID: id, WhatsAppID: id, ChatID: "g@g.us", Type: "image", HasMedia: 1, Timestamp: 1}
		if id == "original" {
			m.Raw, _ = proto.Marshal(&waE2E.Message{ImageMessage: &waE2E.ImageMessage{Width: proto.Uint32(600), Height: proto.Uint32(1200)}})
		}
		// Each fixture needs its own WhatsApp ID: an empty string also participates
		// in the unique constraint and would silently skip subsequent inserts.
		if inserted, err := s.store.Save(m, "Group", "group"); err != nil {
			t.Fatal(err)
		} else if !inserted {
			t.Fatalf("fixture %s was not inserted", id)
		}
	}
	raw, _ := proto.Marshal(&waE2E.Message{ImageMessage: &waE2E.ImageMessage{Width: proto.Uint32(1600), Height: proto.Uint32(900)}})
	if err := s.store.SaveMediaPayload("descriptor", raw); err != nil {
		t.Fatal(err)
	}
	if err := s.store.SaveMediaPayload("corrupt", []byte{255}); err != nil {
		t.Fatal(err)
	}
	rows, err := s.store.Messages("g@g.us", 10, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 4 {
		t.Fatalf("got %d fixture rows, want 4", len(rows))
	}
	s.ResolveMediaDimensions(ctx, rows)
	for _, row := range rows {
		switch row["id"] {
		case "descriptor":
			if row["media_width"] != uint32(1600) || row["media_height"] != uint32(900) {
				t.Fatal(row)
			}
		case "original":
			if row["media_width"] != uint32(600) || row["media_height"] != uint32(1200) {
				t.Fatal(row)
			}
		default:
			if _, ok := row["media_width"]; ok {
				t.Fatal(row)
			}
		}
	}
	// Live/own sends use the same dimensions before the media-ready event.
	var live model.Message
	if err := s.addContext(ctx, &live, &waE2E.Message{ImageMessage: &waE2E.ImageMessage{Width: proto.Uint32(300), Height: proto.Uint32(500)}}, "g@g.us", "sender"); err != nil {
		t.Fatal(err)
	}
	if live.MediaWidth != 300 || live.MediaHeight != 500 {
		t.Fatal(live)
	}
}

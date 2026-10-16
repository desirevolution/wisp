package whatsapp

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"go.mau.fi/whatsmeow"
	"hash/crc32"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"strings"
	"testing"
	"wisp/internal/model"
)

type captureImageSender struct {
	captureSender
	uploaded    []byte
	uploadFails bool
}

func (s *captureImageSender) Upload(_ context.Context, data []byte, kind whatsmeow.MediaType) (whatsmeow.UploadResponse, error) {
	if s.uploadFails {
		return whatsmeow.UploadResponse{}, errors.New("upload failed")
	}
	if kind != whatsmeow.MediaImage {
		return whatsmeow.UploadResponse{}, errors.New("wrong upload type")
	}
	s.uploaded = append([]byte(nil), data...)
	return whatsmeow.UploadResponse{URL: "https://example/image", DirectPath: "/image", MediaKey: bytes.Repeat([]byte{1}, 32), FileSHA256: bytes.Repeat([]byte{2}, 32), FileEncSHA256: bytes.Repeat([]byte{3}, 32), FileLength: uint64(len(data))}, nil
}
func testPNG(t *testing.T) []byte {
	t.Helper()
	im := image.NewNRGBA(image.Rect(0, 0, 8, 6))
	im.Set(1, 1, color.NRGBA{R: 255, A: 255})
	var b bytes.Buffer
	if err := png.Encode(&b, im); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}
func TestSendImageReplyAndLocalMedia(t *testing.T) {
	s, _ := sendingService(t)
	s.mediaRoot = t.TempDir()
	sender := &captureImageSender{}
	s.sender = sender
	s.onMessage(editEvent("g@g.us", "12@lid", "original", "updated question", 30000))
	sub, ch := s.bus.Subscribe()
	defer s.bus.Unsubscribe(sub)
	data := testPNG(t)
	id, err := s.SendImage(context.Background(), "g@g.us", " Caption ", "original", data)
	if err != nil {
		t.Fatal(err)
	}
	m := sender.messages[0].GetImageMessage()
	if m.GetCaption() != "Caption" || m.GetMimetype() != "image/png" || m.GetWidth() != 8 || m.GetHeight() != 6 || len(m.GetJPEGThumbnail()) == 0 || m.GetURL() != "https://example/image" || !bytes.Equal(sender.uploaded, data) {
		t.Fatal(m)
	}
	if m.GetContextInfo().GetStanzaID() != "original" || m.GetContextInfo().GetQuotedMessage().GetConversation() != "updated question" {
		t.Fatal("missing reply", m)
	}
	stored, err := s.store.MessageByID(id)
	if err != nil || stored.FromMe != 1 || stored.Type != "image" || stored.Reply == nil || stored.Body != "Caption" {
		t.Fatal(stored, err)
	}
	media, err := s.store.MediaByMessage(id)
	if err != nil || media.Status != "ready" {
		t.Fatal(media, err)
	}
	cached, err := os.ReadFile(media.Path.String)
	if err != nil || !bytes.Equal(cached, data) {
		t.Fatal("local media missing", err)
	}
	if e := <-ch; e.Type != "message" || e.Data.(model.Message).HasMedia != 1 {
		t.Fatal(e)
	}
	if e := <-ch; e.Type != "media" {
		t.Fatal(e)
	}
}
func TestSendImageFailuresDoNotSendOrRetainFiles(t *testing.T) {
	for _, kind := range []string{"upload", "send", "mirror", "crosschat", "invalid", "oversize"} {
		t.Run(kind, func(t *testing.T) {
			s, _ := sendingService(t)
			s.mediaRoot = t.TempDir()
			sender := &captureImageSender{}
			s.sender = sender
			data := testPNG(t)
			chat, reply := "g@g.us", ""
			switch kind {
			case "upload":
				sender.uploadFails = true
			case "send":
				sender.fail = true
			case "mirror":
				s.store.SetConfigOverride("mirrorOnly", "true")
			case "crosschat":
				chat = "other@g.us"
				reply = "original"
			case "invalid":
				data = []byte("not an image")
			case "oversize":
				data = make([]byte, MaxImageBytes+1)
			}
			if _, err := s.SendImage(context.Background(), chat, "", reply, data); err == nil {
				t.Fatal("expected rejection")
			}
			if len(sender.messages) != 0 {
				t.Fatal("unexpected send")
			}
			files, _ := os.ReadDir(s.mediaRoot)
			if len(files) != 0 {
				t.Fatal("temporary files leaked")
			}
		})
	}
}
func TestInspectImageJPEGAndCorruption(t *testing.T) {
	var b bytes.Buffer
	_ = jpeg.Encode(&b, image.NewRGBA(image.Rect(0, 0, 3, 4)), nil)
	d, err := inspectImage(b.Bytes())
	if err != nil || d.mime != "image/jpeg" || d.width != 3 {
		t.Fatal(d, err)
	}
	data := testPNG(t)
	if _, err = inspectImage(data[:len(data)-12]); !errors.Is(err, ErrInvalidAction) {
		t.Fatal("truncated image accepted", err)
	}
}

func TestImagePixelLimitWithoutFullDecode(t *testing.T) {
	data := testPNG(t)
	binary.BigEndian.PutUint32(data[16:20], 5000)
	binary.BigEndian.PutUint32(data[20:24], 5000)
	binary.BigEndian.PutUint32(data[29:33], crc32.ChecksumIEEE(data[12:29]))
	if _, err := inspectImage(data); err == nil || !strings.Contains(err.Error(), "20 megapixels") {
		t.Fatal("large decoded image accepted", err)
	}
}
func TestImageOnlySend(t *testing.T) {
	s, _ := sendingService(t)
	s.mediaRoot = t.TempDir()
	sender := &captureImageSender{}
	s.sender = sender
	if _, err := s.SendImage(context.Background(), "88@s.whatsapp.net", "", "", testPNG(t)); err != nil {
		t.Fatal(err)
	}
	if len(sender.messages) != 1 || sender.messages[0].GetImageMessage().GetCaption() != "" || sender.messages[0].GetImageMessage().ContextInfo != nil {
		t.Fatal("image-only descriptor invalid")
	}
}

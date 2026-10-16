package whatsapp

import (
	"context"
	"errors"
	"go.mau.fi/whatsmeow"
	waE2E "go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/proto/waMmsRetry"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"
	"testing"
	"wisp/internal/model"
)

type fakeMedia struct {
	requests, downloads       int
	info                      *types.MessageInfo
	downloadErr, errorReceipt error
	path                      string
}

func (f *fakeMedia) Download(_ context.Context, d whatsmeow.DownloadableMessage) ([]byte, error) {
	f.downloads++
	f.path = d.GetDirectPath()
	return nil, f.downloadErr
}
func (f *fakeMedia) SendMediaRetryReceipt(_ context.Context, i *types.MessageInfo, _ []byte) error {
	f.requests++
	f.info = i
	return f.errorReceipt
}
func mediaTestService(t *testing.T) (*Service, *fakeMedia, *waE2E.ImageMessage) {
	s := mentionService(t)
	s.mediaRoot = t.TempDir()
	d := &waE2E.ImageMessage{MediaKey: make([]byte, 32), FileEncSHA256: make([]byte, 32), FileSHA256: make([]byte, 32), DirectPath: proto.String("/old")}
	raw, _ := proto.Marshal(&waE2E.Message{ImageMessage: d})
	sid := "49123@s.whatsapp.net"
	_, err := s.store.Save(model.Message{ID: "m", WhatsAppID: "m", ChatID: "group@g.us", SenderID: &sid, Type: "image", HasMedia: 1, OriginalChat: "group@g.us", OriginalSender: "123@lid", Raw: raw}, "Group", "group")
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeMedia{downloadErr: whatsmeow.ErrInvalidMediaEncSHA256}
	s.mediaIO = f
	t.Cleanup(func() {
		s.mediaMu.Lock()
		defer s.mediaMu.Unlock()
		for _, a := range s.mediaRetryPending {
			if a.timer != nil {
				a.timer.Stop()
			}
		}
	})
	return s, f, d
}
func TestMediaRetryIsBoundedAndPreservesDescriptor(t *testing.T) {
	s, f, d := mediaTestService(t)
	s.downloadMedia("m", "group@g.us", d)
	if f.requests != 1 || f.downloads != 1 {
		t.Fatal(f)
	}
	if f.info.Sender.String() != "123@lid" {
		t.Fatal("normalized participant used", f.info)
	}
	a := s.mediaRetryPending["m"]
	if a == nil {
		t.Fatal("missing attempt")
	}
	if !s.requestMediaRetry("m", d) || f.requests != 1 {
		t.Fatal("duplicate receipt")
	}
	s.completeMediaRetry("m", a, &waMmsRetry.MediaRetryNotification{Result: waMmsRetry.MediaRetryNotification_SUCCESS.Enum(), DirectPath: proto.String("/new")})
	if f.requests != 1 || f.downloads != 2 || f.path != "/new" {
		t.Fatal("retry loop or stale path", f)
	}
	if s.mediaRetryActive("m") {
		t.Fatal("pending stuck")
	}
	if d.GetDirectPath() != "/old" {
		t.Fatal("shared descriptor mutated")
	}
	row, _ := s.store.MediaByMessage("m")
	if row.Status != "failed" {
		t.Fatal(row)
	}
	raw, err := s.store.MediaPayload("m")
	if err != nil {
		t.Fatal(err)
	}
	var saved waE2E.Message
	proto.Unmarshal(raw, &saved)
	if saved.GetImageMessage().GetDirectPath() != "/new" || len(saved.GetImageMessage().GetFileEncSHA256()) != 32 {
		t.Fatal("descriptor not persisted")
	}
}
func TestMediaRetryFailureAndOldAttemptCleanup(t *testing.T) {
	s, f, d := mediaTestService(t)
	f.errorReceipt = errors.New("offline")
	if s.requestMediaRetry("m", d) || s.mediaRetryActive("m") {
		t.Fatal("failed receipt stuck")
	}
	f.errorReceipt = nil
	s.requestMediaRetry("m", d)
	old := s.mediaRetryPending["m"]
	s.finishMediaRetry("m", old)
	s.requestMediaRetry("m", d)
	current := s.mediaRetryPending["m"]
	s.finishMediaRetry("m", old)
	if s.mediaRetryPending["m"] != current {
		t.Fatal("old completion removed new attempt")
	}
	s.completeMediaRetry("m", current, &waMmsRetry.MediaRetryNotification{Result: waMmsRetry.MediaRetryNotification_GENERAL_ERROR.Enum(), DirectPath: proto.String("/ignored")})
	if f.downloads != 0 || s.mediaRetryActive("m") {
		t.Fatal("unsuccessful response downloaded")
	}
}

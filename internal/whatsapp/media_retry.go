package whatsapp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"go.mau.fi/whatsmeow"
	waE2E "go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/proto/waMmsRetry"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"
	"time"
	buspkg "wisp/internal/events"
	"wisp/internal/model"
)

type mediaTransport interface {
	Download(context.Context, whatsmeow.DownloadableMessage) ([]byte, error)
	SendMediaRetryReceipt(context.Context, *types.MessageInfo, []byte) error
}
type mediaRetryAttempt struct {
	dl         whatsmeow.DownloadableMessage
	processing bool
	timer      *time.Timer
}

func (s *Service) mediaTransport() mediaTransport {
	if s.mediaIO != nil {
		return s.mediaIO
	}
	return s.client
}
func (s *Service) mediaRetryActive(id string) bool {
	s.mediaMu.Lock()
	defer s.mediaMu.Unlock()
	return s.mediaRetryPending[id] != nil
}
func (s *Service) finishMediaRetry(id string, a *mediaRetryAttempt) {
	s.mediaMu.Lock()
	defer s.mediaMu.Unlock()
	if s.mediaRetryPending[id] == a {
		if a.timer != nil {
			a.timer.Stop()
		}
		delete(s.mediaRetryPending, id)
	}
}
func (s *Service) failMediaRetry(id string, err error) {
	_ = s.store.MediaFailed(id, err)
	fmt.Printf("[media-retry] failed message=%s: %v\n", id, err)
	if row, e := s.store.MediaByMessage(id); e == nil {
		s.bus.Publish(buspkg.Event{Type: "media", Data: map[string]any{"messageId": id, "chatId": row.ChatID, "status": "failed"}})
	}
}
func retryMessageInfo(m *model.Message) (*types.MessageInfo, error) {
	var info types.MessageInfo
	if len(m.OriginalInfo) > 0 {
		_ = json.Unmarshal(m.OriginalInfo, &info)
	}
	chat := m.OriginalChat
	if chat == "" {
		chat = m.ChatID
	}
	sender := m.OriginalSender
	if sender == "" && m.SenderID != nil {
		sender = *m.SenderID
	}
	var err error
	if info.Chat.IsEmpty() {
		info.Chat, err = types.ParseJID(chat)
		if err != nil || info.Chat.IsEmpty() {
			return nil, fmt.Errorf("missing retry chat")
		}
	}
	if info.Sender.IsEmpty() {
		info.Sender, _ = types.ParseJID(sender)
	}
	info.IsGroup = info.Chat.Server == types.GroupServer
	if info.IsGroup && info.Sender.IsEmpty() {
		return nil, fmt.Errorf("missing group retry participant")
	}
	if info.Sender.IsEmpty() {
		info.Sender = info.Chat
	}
	info.ID = m.WhatsAppID
	if info.ID == "" {
		info.ID = m.ID
	}
	info.IsFromMe = m.FromMe != 0
	info.Timestamp = time.Unix(m.Timestamp, 0)
	return &info, nil
}
func (s *Service) requestMediaRetry(id string, dl whatsmeow.DownloadableMessage) bool {
	if s.mediaRetryActive(id) {
		return true
	}
	msg := downloadableToMessage(dl)
	if msg == nil || len(mediaKey(dl)) != 32 {
		return false
	}
	// The asynchronous response must not mutate a descriptor being downloaded elsewhere.
	clone := downloadable(proto.Clone(msg).(*waE2E.Message))
	m, err := s.store.MessageByID(id)
	if err != nil {
		return false
	}
	info, err := retryMessageInfo(m)
	if err != nil {
		s.failMediaRetry(id, err)
		return false
	}
	a := &mediaRetryAttempt{dl: clone}
	s.mediaMu.Lock()
	if s.mediaRetryPending[id] != nil {
		s.mediaMu.Unlock()
		return true
	}
	s.mediaRetryPending[id] = a
	s.mediaMu.Unlock()
	if err = s.store.MediaPending(id); err != nil {
		s.finishMediaRetry(id, a)
		return false
	}
	// Diagnose descriptors without logging keys, URLs or message contents.
	if len(m.Raw) > 0 {
		var original waE2E.Message
		if proto.Unmarshal(m.Raw, &original) == nil {
			if d := downloadable(&original); d != nil {
				fmt.Printf("[media-retry] descriptor message=%s original_key_match=%t original_enc_hash_match=%t original_plain_hash_match=%t original_ids=%t\n", id, bytes.Equal(d.GetMediaKey(), clone.GetMediaKey()), bytes.Equal(d.GetFileEncSHA256(), clone.GetFileEncSHA256()), bytes.Equal(d.GetFileSHA256(), clone.GetFileSHA256()), m.OriginalChat != "" && m.OriginalSender != "")
			}
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err = s.mediaTransport().SendMediaRetryReceipt(ctx, info, mediaKey(clone)); err != nil {
		s.failMediaRetry(id, err)
		s.finishMediaRetry(id, a)
		return false
	}
	s.mediaMu.Lock()
	if s.mediaRetryPending[id] == a && !a.processing {
		a.timer = time.AfterFunc(20*time.Second, func() {
			s.mediaMu.Lock()
			active := s.mediaRetryPending[id] == a && !a.processing
			if active {
				s.failMediaRetry(id, fmt.Errorf("WhatsApp media retry timed out"))
				delete(s.mediaRetryPending, id)
			}
			s.mediaMu.Unlock()
		})
	}
	s.mediaMu.Unlock()
	return true
}
func (s *Service) onMediaRetry(evt *events.MediaRetry) {
	id := string(evt.MessageID)
	s.mediaMu.Lock()
	a := s.mediaRetryPending[id]
	if a == nil || a.processing {
		s.mediaMu.Unlock()
		return
	}
	a.processing = true
	if a.timer != nil {
		a.timer.Stop()
	}
	s.mediaMu.Unlock()
	data, err := whatsmeow.DecryptMediaRetryNotification(evt, mediaKey(a.dl))
	if err != nil {
		s.failMediaRetry(id, err)
		s.finishMediaRetry(id, a)
		return
	}
	go s.completeMediaRetry(id, a, data)
}
func (s *Service) completeMediaRetry(id string, a *mediaRetryAttempt, data *waMmsRetry.MediaRetryNotification) {
	defer s.finishMediaRetry(id, a)
	if data.GetResult() != waMmsRetry.MediaRetryNotification_SUCCESS || data.GetDirectPath() == "" || (data.GetStanzaID() != "" && data.GetStanzaID() != id) {
		s.failMediaRetry(id, fmt.Errorf("unusable media retry response: result=%s", data.GetResult()))
		return
	}
	path := data.GetDirectPath()
	fmt.Printf("[media-retry] response message=%s result=%s path_changed=%t usable_path=true\n", id, data.GetResult(), path != mediaDirectPath(a.dl))
	if !setDirectPath(a.dl, path) {
		s.failMediaRetry(id, fmt.Errorf("unsupported media type"))
		return
	}
	raw, err := proto.Marshal(downloadableToMessage(a.dl))
	if err == nil {
		err = s.store.SaveMediaPayload(id, raw)
	}
	if err != nil {
		s.failMediaRetry(id, err)
		return
	}
	s.rememberMedia(id, a.dl)
	row, err := s.store.MediaByMessage(id)
	if err != nil {
		s.failMediaRetry(id, err)
		return
	}
	// Exactly one post-refresh download; never start another receipt from this failure.
	s.downloadMediaAttempt(id, row.ChatID, a.dl, false)
}

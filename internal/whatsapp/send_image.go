package whatsapp

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	_ "image/png"
	"os"
	"strings"
	"time"

	"go.mau.fi/whatsmeow"
	waE2E "go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"
	buspkg "wisp/internal/events"
	"wisp/internal/model"
)

const MaxImageBytes = 10 << 20
const maxImagePixels = 20_000_000

type imageUploader interface {
	Upload(context.Context, []byte, whatsmeow.MediaType) (whatsmeow.UploadResponse, error)
}

type imageDetails struct {
	mime          string
	width, height int
	thumbnail     []byte
}

// Inspect the actual file, not the browser-provided MIME type or filename.
func inspectImage(data []byte) (imageDetails, error) {
	invalid := func(reason string) (imageDetails, error) {
		return imageDetails{}, fmt.Errorf("%w: %s", ErrInvalidAction, reason)
	}
	if len(data) == 0 || len(data) > MaxImageBytes {
		return invalid("image must be between 1 byte and 10 MiB")
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || (format != "png" && format != "jpeg") {
		return invalid("only valid PNG and JPEG images are supported")
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || int64(cfg.Width)*int64(cfg.Height) > maxImagePixels {
		return invalid("image exceeds 20 megapixels")
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return invalid("image data is damaged")
	}
	w, h := cfg.Width, cfg.Height
	if w > 320 || h > 320 {
		if w >= h {
			h = max(1, h*320/w)
			w = 320
		} else {
			w = max(1, w*320/h)
			h = 320
		}
	}
	thumb := image.NewRGBA(image.Rect(0, 0, w, h))
	b := img.Bounds()
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			pixel := color.NRGBAModel.Convert(img.At(b.Min.X+x*cfg.Width/w, b.Min.Y+y*cfg.Height/h)).(color.NRGBA)
			a := uint32(pixel.A)
			thumb.SetRGBA(x, y, color.RGBA{uint8((uint32(pixel.R)*a + 255*(255-a)) / 255), uint8((uint32(pixel.G)*a + 255*(255-a)) / 255), uint8((uint32(pixel.B)*a + 255*(255-a)) / 255), 255})
		}
	}
	var out bytes.Buffer
	if err = jpeg.Encode(&out, thumb, &jpeg.Options{Quality: 75}); err != nil {
		return imageDetails{}, err
	}
	mime := "image/png"
	if format == "jpeg" {
		mime = "image/jpeg"
	}
	return imageDetails{mime, cfg.Width, cfg.Height, out.Bytes()}, nil
}

func (s *Service) SendImage(ctx context.Context, chat, caption, replyTo string, data []byte) (string, error) {
	if s.store.ConfigBool("mirrorOnly", false) {
		return "", fmt.Errorf("%w: mirror-only mode", ErrInvalidAction)
	}
	caption = strings.TrimSpace(caption)
	if len([]rune(caption)) > 4000 {
		return "", fmt.Errorf("%w: caption is too long", ErrInvalidAction)
	}
	details, err := inspectImage(data)
	if err != nil {
		return "", err
	}
	transport := s.transport()
	if transport == nil || !transport.IsConnected() || !transport.IsLoggedIn() {
		return "", fmt.Errorf("whatsapp is not connected")
	}
	uploader, ok := transport.(imageUploader)
	if !ok {
		return "", fmt.Errorf("image upload is unavailable")
	}
	chat = s.store.ResolveChat(chat)
	jid, err := types.ParseJID(chat)
	if err != nil || jid.IsEmpty() || (jid.Server != types.DefaultUserServer && jid.Server != types.HiddenUserServer && jid.Server != types.GroupServer) {
		return "", fmt.Errorf("%w: invalid chat", ErrInvalidAction)
	}
	var reply *waE2E.ContextInfo
	if replyTo != "" {
		target, originalChat, sender, e := s.actionTarget(ctx, chat, replyTo)
		if e != nil {
			return "", e
		}
		raw := target.Raw
		if len(target.CurrentRaw) > 0 {
			raw = target.CurrentRaw
		}
		var original waE2E.Message
		if len(raw) == 0 || proto.Unmarshal(raw, &original) != nil {
			return "", fmt.Errorf("%w: original content is not available", ErrInvalidAction)
		}
		content, _ := unwrapContent(&original)
		reply = &waE2E.ContextInfo{StanzaID: proto.String(target.WhatsAppID), Participant: proto.String(sender.ToNonAD().String()), RemoteJID: proto.String(originalChat.String()), QuotedMessage: content}
		jid = originalChat
	}
	// Stage the local copy before sending so disk errors cannot cause a resend.
	if err = os.MkdirAll(s.mediaRoot, 0755); err != nil {
		return "", fmt.Errorf("cannot store image locally: %w", err)
	}
	file, err := os.CreateTemp(s.mediaRoot, "sent-image-*")
	if err != nil {
		return "", err
	}
	path := file.Name()
	keep := false
	defer func() {
		if !keep {
			_ = os.Remove(path)
		}
	}()
	_, err = file.Write(data)
	closeErr := file.Close()
	if err != nil {
		return "", err
	}
	if closeErr != nil {
		return "", closeErr
	}
	uploaded, err := uploader.Upload(ctx, data, whatsmeow.MediaImage)
	if err != nil {
		return "", fmt.Errorf("image upload failed: %w", err)
	}
	imageMsg := &waE2E.ImageMessage{Caption: proto.String(caption), Mimetype: proto.String(details.mime), URL: proto.String(uploaded.URL), DirectPath: proto.String(uploaded.DirectPath), MediaKey: uploaded.MediaKey, FileEncSHA256: uploaded.FileEncSHA256, FileSHA256: uploaded.FileSHA256, FileLength: proto.Uint64(uploaded.FileLength), Width: proto.Uint32(uint32(details.width)), Height: proto.Uint32(uint32(details.height)), JPEGThumbnail: details.thumbnail, ContextInfo: reply}
	outgoing := &waE2E.Message{ImageMessage: imageMsg}
	resp, err := transport.SendMessage(ctx, jid, outgoing)
	if err != nil {
		return "", err
	}
	// Once WhatsApp acknowledges the message, do not report a retryable send
	// failure for local persistence problems: that would send the image twice.
	id := string(resp.ID)
	self := s.selfJID(ctx).String()
	var sender *string
	if self != "" {
		sender = &self
	}
	ts := resp.Timestamp.Unix()
	if ts <= 0 {
		ts = time.Now().Unix()
	}
	m := model.Message{ID: id, WhatsAppID: id, ChatID: chat, SenderID: sender, FromMe: 1, Type: "image", Body: caption, HasMedia: 1, Timestamp: ts}
	name := s.contactName(ctx, jid, "")
	kind := "private"
	if jid.Server == types.GroupServer {
		kind = "group"
		name = s.groupName(ctx, jid, name)
	}
	if err = s.addContext(ctx, &m, outgoing, jid.String(), self); err != nil {
		fmt.Printf("[send-image] acknowledged message=%s context_error=%v\n", id, err)
		return id, nil
	}
	inserted, err := s.store.Save(m, name, kind)
	if err != nil {
		fmt.Printf("[send-image] acknowledged message=%s persistence_error=%v\n", id, err)
		return id, nil
	}
	_ = s.store.SaveMediaPayload(id, m.Raw)
	s.rememberMedia(id, imageMsg)
	row, err := s.store.MediaReady(id, details.mime, "image", path, int64(len(data)))
	if err == nil {
		keep = true
	} else {
		fmt.Printf("[send-image] cache failed message=%s: %v\n", id, err)
	}
	if inserted {
		s.bus.Publish(buspkg.Event{Type: "message", Data: m})
	}
	if err == nil {
		s.bus.Publish(buspkg.Event{Type: "media", Data: map[string]any{"messageId": id, "chatId": chat, "media": row}})
	}
	return id, nil
}

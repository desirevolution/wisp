package mqttsvc

import (
	"encoding/json"
	"path/filepath"
	"testing"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"wisp/internal/events"
	"wisp/internal/model"
	"wisp/internal/store"
)

type payloadClient struct {
	mqtt.Client
	payload  []byte
	qos      byte
	retained bool
	topic    string
}

func (c *payloadClient) IsConnectionOpen() bool { return true }
func (c *payloadClient) Publish(topic string, qos byte, retained bool, payload interface{}) mqtt.Token {
	c.topic = topic
	c.qos = qos
	c.retained = retained
	c.payload = append([]byte{}, payload.([]byte)...)
	return completedToken{}
}
func TestPublishedMediaPayloadPreservesBodyAndDescribesEmptyMedia(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "wisp.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if err = st.OpenChat("family@g.us", "Family", "group"); err != nil {
		t.Fatal(err)
	}
	s := New(st, events.New())
	c := &payloadClient{}
	s.client = c
	s.enabled = true
	s.prefix = "wisp"
	sender := "Bea"
	for _, test := range []struct {
		name, kind, body, want string
		media                  int
	}{
		{"image without caption", "image", "", "Image", 1},
		{"image with caption", "image", "  A photo 📷\nFrom today  ", "  A photo 📷\nFrom today  ", 1},
		{"image whitespace caption", "image", " \n\t", "Image", 1},
		{"video", "video", "", "Video", 1},
		{"video with caption", "video", "Look at this", "Look at this", 1},
		{"voice message", "ptt", "", "Voice message", 1},
		{"audio", "audio", "", "Audio", 1},
		{"sticker", "sticker", "", "Sticker", 1},
		{"document", "document", "", "Document", 1},
		{"document filename", "document", "invoice.pdf", "invoice.pdf", 1},
		{"gif", "gif", "", "GIF", 1},
		{"text unchanged", "chat", "Hello <world> & \"you\" 👋", "Hello <world> & \"you\" 👋", 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			c.payload = nil
			s.handle(events.Event{Type: "message", Data: model.Message{ID: "message-123", ChatID: "family@g.us", SenderName: &sender, Type: test.kind, Body: test.body, HasMedia: test.media, Timestamp: 1792692000}})
			var payload map[string]any
			if err = json.Unmarshal(c.payload, &payload); err != nil {
				t.Fatal(err)
			}
			if payload["text"] != test.want || payload["body"] != test.body || payload["type"] != test.kind || payload["hasMedia"] != (test.media != 0) {
				t.Fatal(payload)
			}
			if payload["chatName"] != "Family" || payload["senderName"] != "Bea" || payload["isGroup"] != true || payload["chatId"] != "family@g.us" || payload["messageId"] != "message-123" || payload["timestamp"] != float64(1792692000) {
				t.Fatal(payload)
			}
			if c.topic != "wisp/message" || c.qos != 1 || c.retained {
				t.Fatal("MQTT routing changed", c)
			}
		})
	}
	// Applying the richer payload must not bypass mute or outgoing filtering.
	c.payload = nil
	st.ChangeChatState("family@g.us", "mute", -1)
	s.handle(events.Event{Type: "message", Data: model.Message{ID: "muted", ChatID: "family@g.us", Type: "image", HasMedia: 1}})
	if c.payload != nil {
		t.Fatal("muted media published")
	}
	st.ChangeChatState("family@g.us", "mute", 0)
	s.handle(events.Event{Type: "message", Data: model.Message{ID: "own", ChatID: "family@g.us", Type: "image", HasMedia: 1, FromMe: 1}})
	if c.payload != nil {
		t.Fatal("outgoing media published")
	}
}

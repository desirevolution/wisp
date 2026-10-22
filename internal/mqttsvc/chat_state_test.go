package mqttsvc

import (
	mqtt "github.com/eclipse/paho.mqtt.golang"
	"path/filepath"
	"testing"
	"time"
	"wisp/internal/events"
	"wisp/internal/model"
	"wisp/internal/store"
)

type recordingClient struct {
	mqtt.Client
	topics []string
}

func (c *recordingClient) IsConnectionOpen() bool { return true }
func (c *recordingClient) Publish(topic string, qos byte, retained bool, payload interface{}) mqtt.Token {
	c.topics = append(c.topics, topic)
	return completedToken{}
}
func TestMuteSuppressesMQTTButPreservesUnmutedMessages(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	st.OpenChat("c", "Contact", "private")
	c := &recordingClient{}
	s := New(st, events.New())
	s.client = c
	s.enabled = true
	s.prefix = "wisp"
	run := func(m model.Message) {
		ch := make(chan events.Event, 1)
		ch <- events.Event{Type: "message", Data: m}
		close(ch)
		s.run(ch)
	}
	st.ChangeChatState("c", "mute", -1)
	run(model.Message{ChatID: "c", Body: "muted"})
	st.ChangeChatState("c", "mute", 0)
	run(model.Message{ChatID: "c", Silent: true, Body: "queued muted"})
	if len(c.topics) != 0 {
		t.Fatal("muted MQTT publication", c.topics)
	}
	run(model.Message{ChatID: "c", Body: "new"})
	if len(c.topics) != 1 || c.topics[0] != "wisp/message" {
		t.Fatal(c.topics)
	}
}

type completedToken struct{}

func (completedToken) Wait() bool                     { return true }
func (completedToken) WaitTimeout(time.Duration) bool { return true }
func (completedToken) Done() <-chan struct{}          { ch := make(chan struct{}); close(ch); return ch }
func (completedToken) Error() error                   { return nil }

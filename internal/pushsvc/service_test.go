package pushsvc

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	webpush "github.com/SherClockHolmes/webpush-go"
	"wisp/internal/events"
	"wisp/internal/model"
	"wisp/internal/store"
)

func testService(t *testing.T, endpoints ...string) *Service {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "wisp.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	s, err := New(st, events.New())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.cancel)
	for _, endpoint := range endpoints {
		raw, _ := json.Marshal(map[string]any{"endpoint": endpoint, "keys": map[string]string{"p256dh": "test", "auth": "test"}})
		if err := s.Subscribe(raw); err != nil {
			t.Fatal(err)
		}
	}
	return s
}
func response(code int) *http.Response {
	return &http.Response{StatusCode: code, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(""))}
}
func TestPayloadLimit(t *testing.T) {
	for _, body := range []string{strings.Repeat("😀", 10000), strings.Repeat("<>&\n", 5000), "hello"} {
		b, err := marshalPayload(payload{Title: strings.Repeat("<", 1000), Body: body, ChatID: "chat", MessageID: "message"})
		if err != nil {
			t.Fatal(err)
		}
		if len(b) > 3000 || !utf8.Valid(b) {
			t.Fatalf("invalid payload size=%d", len(b))
		}
		var p payload
		if err := json.Unmarshal(b, &p); err != nil {
			t.Fatal(err)
		}
		if p.ChatID != "chat" || p.MessageID != "message" {
			t.Fatal("lost deep link")
		}
	}
}
func TestFanoutDoesNotWaitForSlowDevice(t *testing.T) {
	s := testService(t, "https://push.example/slow", "https://push.example/fast")
	fast := make(chan struct{})
	s.send = func(ctx context.Context, b []byte, sub *webpush.Subscription, options *webpush.Options) (*http.Response, error) {
		if options.TTL != 86400 || options.Urgency != webpush.UrgencyHigh {
			t.Error("wrong delivery options")
		}
		if strings.HasSuffix(sub.Endpoint, "/slow") {
			select {
			case <-fast:
			case <-time.After(time.Second):
				t.Error("slow device blocked fast device")
			}
		} else {
			close(fast)
		}
		return response(201), nil
	}
	s.notify(model.Message{ID: "m", ChatID: "c", Body: "hello"})
}
func TestTransientFailureRetriesButExpiredEndpointIsRemoved(t *testing.T) {
	s := testService(t, "https://push.example/retry", "https://push.example/gone")
	var mu sync.Mutex
	attempts := map[string]int{}
	s.send = func(ctx context.Context, b []byte, sub *webpush.Subscription, options *webpush.Options) (*http.Response, error) {
		mu.Lock()
		defer mu.Unlock()
		attempts[sub.Endpoint]++
		if strings.HasSuffix(sub.Endpoint, "/gone") {
			return response(410), nil
		}
		if attempts[sub.Endpoint] == 1 {
			return response(503), nil
		}
		return response(201), nil
	}
	s.notify(model.Message{ID: "m", ChatID: "c", Body: "hello"})
	if attempts["https://push.example/retry"] != 2 || attempts["https://push.example/gone"] != 1 {
		t.Fatalf("unexpected attempts: %v", attempts)
	}
	subs, err := s.st.PushSubscriptions()
	if err != nil {
		t.Fatal(err)
	}
	if len(subs) != 1 || subs["https://push.example/retry"] == "" {
		t.Fatal("wrong subscription removed")
	}
}
func TestPermanentFailureKeepsSubscription(t *testing.T) {
	s := testService(t, "https://push.example/forbidden")
	calls := 0
	s.send = func(context.Context, []byte, *webpush.Subscription, *webpush.Options) (*http.Response, error) {
		calls++
		return response(403), nil
	}
	s.notify(model.Message{ID: "m", ChatID: "c"})
	subs, _ := s.st.PushSubscriptions()
	if calls != 1 || len(subs) != 1 {
		t.Fatal("403 should neither retry nor delete")
	}
}
func TestOnlyIncomingMessagesGeneratePushAndCloseCancelsSend(t *testing.T) {
	s := testService(t, "https://push.example/device")
	entered := make(chan string, 2)
	s.send = func(ctx context.Context, b []byte, sub *webpush.Subscription, options *webpush.Options) (*http.Response, error) {
		var p payload
		if err := json.Unmarshal(b, &p); err != nil {
			t.Error(err)
		}
		entered <- p.MessageID
		<-ctx.Done()
		return nil, ctx.Err()
	}
	s.Start()
	s.bus.Publish(events.Event{Type: "message", Data: model.Message{ID: "own", FromMe: 1}})
	s.bus.Publish(events.Event{Type: "media", Data: model.Message{ID: "media"}})
	s.bus.Publish(events.Event{Type: "message", Data: model.Message{ID: "incoming"}})
	select {
	case id := <-entered:
		if id != "incoming" {
			t.Fatalf("unexpected message %s", id)
		}
	case <-time.After(time.Second):
		t.Fatal("incoming message was not delivered")
	}
	done := make(chan struct{})
	go func() { s.Close(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("shutdown did not cancel provider call")
	}
}

func TestMutedAndPreviouslyMutedMessagesNeverPush(t *testing.T) {
	s := testService(t, "https://push.example/device")
	s.st.OpenChat("c", "Contact", "private")
	calls := 0
	s.send = func(context.Context, []byte, *webpush.Subscription, *webpush.Options) (*http.Response, error) {
		calls++
		return response(201), nil
	}
	s.st.ChangeChatState("c", "mute", -1)
	s.notify(model.Message{ID: "muted", ChatID: "c", Body: "quiet"})
	s.st.ChangeChatState("c", "mute", 0)
	s.notify(model.Message{ID: "queued-muted", ChatID: "c", Body: "quiet", Silent: true})
	if calls != 0 {
		t.Fatal("muted messages reached push provider")
	}
	s.notify(model.Message{ID: "new", ChatID: "c", Body: "hello"})
	if calls != 1 {
		t.Fatal("unmuted message lost", calls)
	}
}

func TestMutingStopsPendingPushRetries(t *testing.T) {
	s := testService(t, "https://push.example/device")
	s.st.OpenChat("c", "Contact", "private")
	calls := 0
	s.send = func(context.Context, []byte, *webpush.Subscription, *webpush.Options) (*http.Response, error) {
		calls++
		s.st.ChangeChatState("c", "mute", -1)
		return response(503), nil
	}
	s.notify(model.Message{ID: "retry", ChatID: "c", Body: "hello"})
	if calls != 1 {
		t.Fatal("push retried after mute", calls)
	}
}

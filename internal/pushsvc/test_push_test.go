package pushsvc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"

	webpush "github.com/SherClockHolmes/webpush-go"
)

func TestPushBroadcastDeploymentGateAndSafeResults(t *testing.T) {
	endpoints := []string{"https://push.example/ok-secret", "https://push.example/gone-secret", "https://push.example/bad-secret", "https://push.example/network-secret"}
	s := testService(t, endpoints...)
	s.send = func(context.Context, []byte, *webpush.Subscription, *webpush.Options) (*http.Response, error) {
		t.Fatal("disabled test sent a notification")
		return nil, nil
	}
	if _, err := s.TestAll(context.Background()); !errors.Is(err, ErrTestDisabled) {
		t.Fatal(err)
	}
	s.st.SetConfigOverride("pushTestEnabled", "true")
	var logs []string
	var logMu sync.Mutex
	// Fan-out logs arrive concurrently.
	s.logf = func(format string, args ...any) {
		logMu.Lock()
		defer logMu.Unlock()
		logs = append(logs, fmt.Sprintf(format, args...))
	}
	s.send = func(_ context.Context, b []byte, sub *webpush.Subscription, o *webpush.Options) (*http.Response, error) {
		var p payload
		if err := json.Unmarshal(b, &p); err != nil {
			t.Error(err)
		}
		if p.ChatID != "" || p.MessageID != "" || !strings.HasPrefix(p.TestID, "test-") || o.TTL != 300 {
			t.Error("test must not target a chat", p, o.TTL)
		}
		switch sub.Endpoint {
		case endpoints[0]:
			return response(202), nil
		case endpoints[1]:
			return response(410), nil
		case endpoints[2]:
			return response(400), nil
		default:
			return nil, fmt.Errorf("private URL %s", sub.Endpoint)
		}
	}
	busID, events := s.bus.Subscribe()
	defer s.bus.Unsubscribe(busID)
	report, err := s.TestAll(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if report.Total != 4 || report.Accepted != 1 || report.Expired != 1 || report.Failed != 2 || len(report.Results) != 4 {
		t.Fatal(report)
	}
	subscriptions, err := s.st.PushSubscriptions()
	if err != nil || len(subscriptions) != 3 {
		t.Fatal(subscriptions, err)
	}
	select {
	case event := <-events:
		t.Fatal("test published application event", event)
	default:
	}
	raw, _ := json.Marshal(report)
	all := string(raw) + strings.Join(logs, "\n")
	for _, endpoint := range endpoints {
		if strings.Contains(all, endpoint) || strings.Contains(all, strings.TrimPrefix(endpoint, "https://push.example/")) {
			t.Fatal("endpoint token leaked", all)
		}
	}
	if !strings.Contains(all, report.TestID) || !strings.Contains(all, "test_complete") {
		t.Fatal(all)
	}
}
func TestPushBroadcastBusyAndCancellation(t *testing.T) {
	s := testService(t, "https://push.example/device")
	s.st.SetConfigOverride("pushTestEnabled", "true")
	started := make(chan struct{})
	s.send = func(ctx context.Context, _ []byte, _ *webpush.Subscription, _ *webpush.Options) (*http.Response, error) {
		close(started)
		<-ctx.Done()
		return nil, ctx.Err()
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { _, err := s.TestAll(ctx); done <- err }()
	<-started
	if _, err := s.TestAll(context.Background()); !errors.Is(err, ErrTestBusy) {
		t.Fatal(err)
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

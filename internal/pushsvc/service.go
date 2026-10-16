package pushsvc

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	webpush "github.com/SherClockHolmes/webpush-go"
	"wisp/internal/events"
	"wisp/internal/model"
	"wisp/internal/store"
)

type Service struct {
	st                    *store.Store
	bus                   *events.Bus
	wg                    sync.WaitGroup
	ctx                   context.Context
	cancel                context.CancelFunc
	send                  func(context.Context, []byte, *webpush.Subscription, *webpush.Options) (*http.Response, error)
	logf                  func(string, ...any)
	publicKey, privateKey string
	subID                 int
	stop                  chan struct{}
}

type Status struct {
	Available     bool `json:"available"`
	Subscriptions int  `json:"subscriptions"`
}

type payload struct {
	Title     string `json:"title"`
	Body      string `json:"body"`
	ChatID    string `json:"chatId"`
	MessageID string `json:"messageId"`
}

func New(st *store.Store, bus *events.Bus) (*Service, error) {
	s := &Service{st: st, bus: bus, stop: make(chan struct{}), send: webpush.SendNotificationWithContext, logf: log.Printf}
	pub := st.ConfigString("pushVapidPublicKey", "")
	priv := st.ConfigString("pushVapidPrivateKey", "")
	if pub == "" || priv == "" {
		var err error
		priv, pub, err = webpush.GenerateVAPIDKeys()
		if err != nil {
			return nil, fmt.Errorf("generate VAPID keys: %w", err)
		}
		if err = st.SetConfigString("pushVapidPublicKey", pub); err != nil {
			return nil, err
		}
		if err = st.SetConfigString("pushVapidPrivateKey", priv); err != nil {
			return nil, err
		}
	}
	s.ctx, s.cancel = context.WithCancel(context.Background())
	s.publicKey, s.privateKey = pub, priv
	return s, nil
}
func (s *Service) Start() {
	id, ch := s.bus.SubscribeReliable("message")
	s.subID = id
	s.wg.Add(1)
	go func() { defer s.wg.Done(); s.run(ch) }()
}
func (s *Service) Close()            { close(s.stop); s.cancel(); s.bus.Unsubscribe(s.subID); s.wg.Wait() }
func (s *Service) PublicKey() string { return s.publicKey }
func (s *Service) Status() Status {
	subs, _ := s.st.PushSubscriptions()
	return Status{Available: s.publicKey != "" && s.privateKey != "", Subscriptions: len(subs)}
}
func (s *Service) Subscribe(raw []byte) error {
	var sub webpush.Subscription
	if err := json.Unmarshal(raw, &sub); err != nil {
		return fmt.Errorf("invalid push subscription: %w", err)
	}
	if sub.Endpoint == "" || sub.Keys.P256dh == "" || sub.Keys.Auth == "" {
		return fmt.Errorf("incomplete push subscription")
	}
	if err := s.st.SavePushSubscription(sub.Endpoint, raw); err != nil {
		return err
	}
	s.logf("[push] event=subscription_synced device=%s provider=%s", deviceID(sub.Endpoint), shortEndpoint(sub.Endpoint))
	return nil
}
func (s *Service) Unsubscribe(endpoint string) error {
	if err := s.st.DeletePushSubscription(endpoint); err != nil {
		return err
	}
	s.logf("[push] event=unsubscribed device=%s provider=%s", deviceID(endpoint), shortEndpoint(endpoint))
	return nil
}
func (s *Service) run(ch <-chan events.Event) {
	for {
		select {
		case <-s.stop:
			return
		case e, ok := <-ch:
			if !ok {
				return
			}
			if e.Type == "message" {
				if m, ok := e.Data.(model.Message); ok && m.FromMe == 0 {
					s.notify(m)
				}
			}
		}
	}
}
func (s *Service) notify(m model.Message) {
	title := strings.TrimSpace(s.st.ChatName(m.ChatID))
	if title == "" && m.SenderName != nil {
		title = strings.TrimSpace(*m.SenderName)
	}
	if title == "" {
		title = "Wisp"
	}
	body := strings.TrimSpace(m.Body)
	if body == "" {
		switch m.Type {
		case "image":
			body = "Image"
		case "video":
			body = "Video"
		case "audio":
			body = "Audio"
		case "document":
			body = "Document"
		case "sticker":
			body = "Sticker"
		default:
			body = "New message"
		}
	}
	b, err := marshalPayload(payload{Title: title, Body: body, ChatID: m.ChatID, MessageID: m.ID})
	if err != nil {
		fmt.Printf("[push] message=%s payload: %v\n", m.ID, err)
		return
	}
	subs, err := s.st.PushSubscriptions()
	if err != nil {
		fmt.Printf("[push] load subscriptions: %v\n", err)
		return
	}
	// Fan out concurrently, with a bounded number of provider requests. Finish
	// this message before the next to preserve per-device submission order.
	var workers sync.WaitGroup
	slots := make(chan struct{}, 8)
	for endpoint, raw := range subs {
		select {
		case slots <- struct{}{}:
		case <-s.ctx.Done():
			workers.Wait()
			return
		}
		workers.Add(1)
		go func(endpoint, raw string) {
			defer workers.Done()
			defer func() { <-slots }()
			var sub webpush.Subscription
			if json.Unmarshal([]byte(raw), &sub) != nil {
				fmt.Printf("[push] invalid stored subscription provider=%s\n", shortEndpoint(endpoint))
				return
			}
			s.deliver(b, &sub, m.ID)
		}(endpoint, raw)
	}
	workers.Wait()
}

// Bound the encoded JSON (including escaping and multi-byte characters), not
// just the character count. Reserve room below webpush-go's encrypted limit.
func marshalPayload(p payload) ([]byte, error) {
	p.Title = truncate(p.Title, 120)
	p.Body = truncate(p.Body, 1000)
	for {
		b, err := json.Marshal(p)
		if err != nil {
			return nil, err
		}
		if len(b) <= 3000 {
			return b, nil
		}
		if len([]rune(p.Body)) <= 1 {
			return nil, fmt.Errorf("push metadata exceeds payload limit")
		}
		p.Body = truncate(p.Body, len([]rune(p.Body))/2)
	}
}
func truncate(s string, limit int) string {
	r := []rune(s)
	if len(r) <= limit {
		return s
	}
	return string(r[:limit-1]) + "…"
}

func (s *Service) deliver(b []byte, sub *webpush.Subscription, messageID string) {
	device, provider := deviceID(sub.Endpoint), shortEndpoint(sub.Endpoint)
	for attempt := 0; attempt < 3; attempt++ {
		if s.ctx.Err() != nil {
			return
		}
		started := time.Now()
		ctx, cancel := context.WithTimeout(s.ctx, 10*time.Second)
		resp, err := s.send(ctx, b, sub, &webpush.Options{
			Subscriber: "mailto:wisp@localhost", VAPIDPublicKey: s.publicKey,
			VAPIDPrivateKey: s.privateKey, TTL: 86400, Urgency: webpush.UrgencyHigh,
		})
		status := 0
		retryAfter := ""
		if resp != nil {
			status = resp.StatusCode
			retryAfter = resp.Header.Get("Retry-After")
			if resp.Body != nil {
				resp.Body.Close()
			}
		}
		cancel()
		if err == nil && status >= 200 && status < 300 {
			s.logf("[push] event=accepted message=%q device=%s provider=%s status=%d attempt=%d duration_ms=%d", messageID, device, provider, status, attempt+1, time.Since(started).Milliseconds())
			return
		}
		if status == http.StatusNotFound || status == http.StatusGone {
			s.logf("[push] event=expired message=%q device=%s provider=%s status=%d", messageID, device, provider, status)
			if err := s.st.DeletePushSubscription(sub.Endpoint); err != nil {
				fmt.Printf("[push] remove expired subscription: %v\n", err)
			}
			return
		}
		retry := err != nil || status == 429 || status >= 500
		// Do not log endpoint tokens or HTTP errors containing subscription URLs.
		s.logf("[push] event=failed message=%q device=%s provider=%s status=%d network_error=%t attempt=%d duration_ms=%d", messageID, device, provider, status, err != nil, attempt+1, time.Since(started).Milliseconds())
		if !retry || attempt == 2 {
			return
		}
		delay := retryDelay(retryAfter, attempt)
		// Avoid blocking all following messages on a provider's long rate limit.
		if delay > 30*time.Second {
			s.logf("[push] event=retry_stopped message=%q device=%s reason=retry_after_too_long", messageID, device)
			return
		}
		timer := time.NewTimer(delay)
		select {
		case <-s.ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}
func retryDelay(header string, attempt int) time.Duration {
	delay := time.Second * time.Duration(1<<attempt)
	if n, err := strconv.Atoi(header); err == nil && n > 0 {
		return time.Duration(n) * time.Second
	}
	if t, err := http.ParseTime(header); err == nil && time.Until(t) > delay {
		return time.Until(t)
	}
	return delay
}
func shortEndpoint(endpoint string) string {
	u, err := url.Parse(endpoint)
	if err != nil || u.Hostname() == "" {
		return "unknown"
	}
	return u.Hostname()
}

// Correlate device registrations without logging endpoint tokens or keys.
func deviceID(endpoint string) string {
	digest := sha256.Sum256([]byte(endpoint))
	return fmt.Sprintf("%x", digest[:6])
}

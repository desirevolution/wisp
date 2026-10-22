package pushsvc

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"sync"
	"time"

	webpush "github.com/SherClockHolmes/webpush-go"
)

var ErrTestDisabled = errors.New("push testing is disabled by deployment policy")
var ErrTestBusy = errors.New("a push test is already running")

type TestDeviceResult struct {
	Device     string `json:"device"`
	Provider   string `json:"provider"`
	Result     string `json:"result"`
	HTTPStatus int    `json:"http_status"`
	Reason     string `json:"reason,omitempty"`
	DurationMS int64  `json:"duration_ms"`
}
type TestReport struct {
	TestID   string             `json:"test_id"`
	Total    int                `json:"total"`
	Accepted int                `json:"accepted"`
	Expired  int                `json:"expired"`
	Failed   int                `json:"failed"`
	Results  []TestDeviceResult `json:"results"`
}

// TestAll exercises Web Push directly: no WhatsApp/MQTT/message-bus events.
// One attempt per stored subscription keeps each diagnostic result unambiguous.
func (s *Service) TestAll(ctx context.Context) (TestReport, error) {
	if !s.st.ConfigBool("pushTestEnabled", false) {
		return TestReport{}, ErrTestDisabled
	}
	if !s.testMu.TryLock() {
		return TestReport{}, ErrTestBusy
	}
	defer s.testMu.Unlock()
	subscriptions, err := s.st.PushSubscriptions()
	if err != nil {
		return TestReport{}, err
	}
	var random [16]byte
	if _, err = rand.Read(random[:]); err != nil {
		return TestReport{}, err
	}
	report := TestReport{TestID: "test-" + hex.EncodeToString(random[:]), Total: len(subscriptions), Results: make([]TestDeviceResult, len(subscriptions))}
	body, err := json.Marshal(payload{Title: "Wisp – test notification", Body: "Web Push test from Wisp. No chat message was sent.", TestID: report.TestID})
	if err != nil {
		return TestReport{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	stop := context.AfterFunc(s.ctx, cancel)
	defer stop()
	endpoints := make([]string, 0, len(subscriptions))
	for endpoint := range subscriptions {
		endpoints = append(endpoints, endpoint)
	}
	sort.Strings(endpoints)
	slots := make(chan struct{}, 8)
	var workers sync.WaitGroup
	for index, endpoint := range endpoints {
		select {
		case slots <- struct{}{}:
		case <-ctx.Done():
			report.Results[index] = TestDeviceResult{Device: deviceID(endpoint), Provider: shortEndpoint(endpoint), Result: "failed", Reason: "cancelled"}
			continue
		}
		workers.Add(1)
		go func(index int, endpoint, raw string) {
			defer workers.Done()
			defer func() { <-slots }()
			result := TestDeviceResult{Device: deviceID(endpoint), Provider: shortEndpoint(endpoint), Result: "failed"}
			started := time.Now()
			var sub webpush.Subscription
			if json.Unmarshal([]byte(raw), &sub) != nil || sub.Endpoint != endpoint || sub.Keys.Auth == "" || sub.Keys.P256dh == "" {
				result.Reason = "invalid_subscription"
			} else if ctx.Err() != nil {
				result.Reason = "cancelled"
			} else {
				attemptCtx, end := context.WithTimeout(ctx, 10*time.Second)
				response, sendErr := s.send(attemptCtx, body, &sub, &webpush.Options{Subscriber: "mailto:wisp@localhost", VAPIDPublicKey: s.publicKey, VAPIDPrivateKey: s.privateKey, TTL: 300, Urgency: webpush.UrgencyHigh})
				if response != nil {
					result.HTTPStatus = response.StatusCode
					if response.Body != nil {
						response.Body.Close()
					}
				}
				end()
				switch {
				case sendErr == nil && result.HTTPStatus >= 200 && result.HTTPStatus < 300:
					result.Result = "accepted"
				case result.HTTPStatus == http.StatusNotFound || result.HTTPStatus == http.StatusGone:
					result.Result = "expired"
					result.Reason = "subscription_expired"
					if err := s.st.DeletePushSubscription(endpoint); err != nil {
						result.Reason = "cleanup_failed"
					}
				case sendErr != nil:
					result.Reason = "network_error"
				default:
					result.Reason = "provider_rejected"
				}
			}
			result.DurationMS = time.Since(started).Milliseconds()
			report.Results[index] = result
			s.logf("[push] event=test_%s test_id=%s device=%s provider=%s status=%d reason=%s duration_ms=%d", result.Result, report.TestID, result.Device, result.Provider, result.HTTPStatus, result.Reason, result.DurationMS)
		}(index, endpoint, subscriptions[endpoint])
	}
	workers.Wait()
	for _, result := range report.Results {
		switch result.Result {
		case "accepted":
			report.Accepted++
		case "expired":
			report.Expired++
		default:
			report.Failed++
		}
	}
	s.logf("[push] event=test_complete test_id=%s total=%d accepted=%d expired=%d failed=%d", report.TestID, report.Total, report.Accepted, report.Expired, report.Failed)
	return report, nil
}

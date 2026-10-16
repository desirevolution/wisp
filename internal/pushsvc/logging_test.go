package pushsvc

import (
	"context"
	"fmt"
	webpush "github.com/SherClockHolmes/webpush-go"
	"net/http"
	"strings"
	"testing"
)

func TestDeliveryLogsTechnicalResultsWithoutEndpointSecrets(t *testing.T) {
	endpoint := "https://push.example/private-token?auth=DO-NOT-LOG"
	for _, status := range []int{202, 410, 400, 503} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			s := testService(t)
			var lines []string
			s.logf = func(format string, args ...any) { lines = append(lines, fmt.Sprintf(format, args...)) }
			s.send = func(context.Context, []byte, *webpush.Subscription, *webpush.Options) (*http.Response, error) {
				r := response(status)
				if status == 503 {
					r.Header.Set("Retry-After", "60")
				}
				return r, nil
			}
			s.deliver([]byte("PRIVATE MESSAGE BODY"), &webpush.Subscription{Endpoint: endpoint}, "message-id")
			text := strings.Join(lines, "\n")
			if !strings.Contains(text, "device="+deviceID(endpoint)) || !strings.Contains(text, "provider=push.example") {
				t.Fatal(text)
			}
			if strings.Contains(text, "DO-NOT-LOG") || strings.Contains(text, "private-token") || strings.Contains(text, "PRIVATE MESSAGE BODY") {
				t.Fatal("secret logged", text)
			}
			expected := "event=failed"
			if status == 202 {
				expected = "event=accepted"
			}
			if status == 410 {
				expected = "event=expired"
			}
			if !strings.Contains(text, expected) {
				t.Fatal(text)
			}
			if status == 503 && !strings.Contains(text, "reason=retry_after_too_long") {
				t.Fatal(text)
			}
		})
	}
	if deviceID(endpoint) != deviceID(endpoint) || deviceID(endpoint) == deviceID(endpoint+"other") {
		t.Fatal("device correlation failed")
	}
}

package pushsvc

import (
	"context"
	"errors"
	"net/http"
	"net/netip"
	"testing"
)

func TestPushEndpointPolicy(t *testing.T) {
	for _, endpoint := range []string{
		"http://push.example/send", "https://localhost/send", "https://127.0.0.1/send",
		"https://[::ffff:10.0.0.1]/send", "https://user@push.example/send", "https://push.example:8443/send",
	} {
		if validatePushEndpoint(endpoint) == nil {
			t.Errorf("accepted forbidden endpoint %q", endpoint)
		}
	}
	if err := validatePushEndpoint("https://push.example/send?token=test"); err != nil {
		t.Fatal(err)
	}
	for _, address := range []string{"127.0.0.1", "169.254.169.254", "100.64.0.1", "192.168.1.1", "fd00::1", "::1", "64:ff9b::a00:1", "2002:a00:1::1"} {
		if publicPushIP(netip.MustParseAddr(address)) {
			t.Errorf("accepted non-public address %s", address)
		}
	}
	if !publicPushIP(netip.MustParseAddr("8.8.8.8")) || !publicPushIP(netip.MustParseAddr("2606:4700:4700::1111")) {
		t.Fatal("public IPv4 or IPv6 addresses were rejected")
	}
	client := pushHTTPClient()
	dial := client.Transport.(*http.Transport).DialContext
	if _, err := dial(context.Background(), "tcp", "localhost:443"); !errors.Is(err, errPushEndpoint) {
		t.Fatalf("loopback DNS answer was not blocked: %v", err)
	}
	if !errors.Is(client.CheckRedirect(nil, nil), http.ErrUseLastResponse) {
		t.Fatal("redirects must be blocked")
	}
}

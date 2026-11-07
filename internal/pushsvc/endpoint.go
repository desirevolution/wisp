package pushsvc

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
)

var errPushEndpoint = errors.New("push endpoint must be a public HTTPS URL")

var nonPublicPushRanges = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("192.88.99.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("2001::/23"),
	netip.MustParsePrefix("2001:db8::/32"),
	netip.MustParsePrefix("2002::/16"),
	netip.MustParsePrefix("3fff::/20"),
}

var globalIPv6 = netip.MustParsePrefix("2000::/3")

func publicPushIP(ip netip.Addr) bool {
	ip = ip.Unmap()
	if !ip.IsGlobalUnicast() || ip.IsPrivate() || (ip.Is6() && !globalIPv6.Contains(ip)) {
		return false
	}
	for _, prefix := range nonPublicPushRanges {
		if prefix.Contains(ip) {
			return false
		}
	}
	return true
}

func validatePushEndpoint(endpoint string) error {
	u, err := url.Parse(endpoint)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.Fragment != "" ||
		(u.Port() != "" && u.Port() != "443") {
		return errPushEndpoint
	}
	host := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
	if ip, err := netip.ParseAddr(host); err == nil {
		if publicPushIP(ip) {
			return nil
		}
		return errPushEndpoint
	}
	if !strings.Contains(host, ".") || strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".local") {
		return errPushEndpoint
	}
	return nil
}

func pushHTTPClient() *http.Client {
	dialer := &net.Dialer{}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil || port != "443" {
			return nil, errPushEndpoint
		}
		ips, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
		if err != nil {
			return nil, err
		}
		// Dial the checked address directly so DNS cannot change between the
		// check and connection. TLS still verifies the original hostname.
		lastErr := errPushEndpoint
		for _, ip := range ips {
			if publicPushIP(ip) {
				conn, err := dialer.DialContext(ctx, network, net.JoinHostPort(ip.Unmap().String(), port))
				if err == nil {
					return conn, nil
				}
				lastErr = err
			}
		}
		return nil, lastErr
	}
	return &http.Client{
		Transport: transport,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

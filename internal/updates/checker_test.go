package updates

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type memoryStorage struct{ value string }

func (m *memoryStorage) ConfigString(_, fallback string) string {
	if m.value == "" {
		return fallback
	}
	return m.value
}
func (m *memoryStorage) SetConfigString(_, value string) error { m.value = value; return nil }

func TestStableVersionComparison(t *testing.T) {
	for _, tc := range []struct {
		current, latest       string
		available, comparable bool
	}{
		{"v1.9.0", "v1.10.0", true, true},
		{"1.10.0", "v1.9.0", false, true},
		{"1.2.0-rc.1", "v1.2.0", true, true},
		{"v1.2.0+build1", "1.2.0+build2", false, true},
		{"dev-abc", "v1.2.0", false, false},
	} {
		t.Run(tc.current+"/"+tc.latest, func(t *testing.T) {
			c := New(tc.current, nil, nil)
			c.state.Latest = tc.latest
			c.compare()
			if s := c.Status(); s.Available != tc.available || s.Comparable != tc.comparable {
				t.Fatal(s)
			}
		})
	}
}

func TestDailyCheckPersistsAcrossRestartAndRecomparesCurrentBuild(t *testing.T) {
	calls, changed := 0, 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("User-Agent") == "" {
			t.Error("missing user agent")
		}
		_, _ = w.Write([]byte(`{"tag_name":"v1.10.0","html_url":"https://evil.example","draft":false,"prerelease":false}`))
	}))
	defer server.Close()
	storage := &memoryStorage{}
	c := New("v1.9.0", storage, func(State) { changed++ })
	c.endpoint = server.URL
	if err := c.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	s := c.Status()
	if !s.Available || s.URL != "https://github.com/desirevolution/wisp/releases/tag/v1.10.0" || changed != 1 {
		t.Fatal(s, changed)
	}
	if err := c.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	restarted := New("v1.10.0", storage, nil)
	restarted.endpoint = server.URL
	if err := restarted.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	if calls != 1 || restarted.Status().Available {
		t.Fatal(calls, restarted.Status())
	}
	restarted.state.CheckedAt = time.Now().Add(-Interval - time.Second).UnixMilli()
	if err := restarted.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatal(calls)
	}
}

func TestFailureRetainsLastKnownReleaseAndWaitsADay(t *testing.T) {
	for _, body := range []string{`{"tag_name":"v9.0.0-rc.1","prerelease":true}`, `{"tag_name":"v9.0.0","draft":true}`, `{"tag_name":"bad"}`, `not json`, ``} {
		t.Run(body, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if body == "" {
					w.WriteHeader(429)
				}
				_, _ = w.Write([]byte(body))
			}))
			defer server.Close()
			c := New("v1.0.0", &memoryStorage{}, nil)
			c.endpoint = server.URL
			c.state.Latest = "v1.1.0"
			if err := c.Check(context.Background()); err == nil {
				t.Fatal("invalid release was accepted")
			}
			if s := c.Status(); s.Latest != "v1.1.0" || s.Error == "" || !s.Available {
				t.Fatal(s)
			}
			_ = c.Check(context.Background())
			if calls != 1 {
				t.Fatal(calls)
			}
		})
	}
}

func TestCancelledRequestDoesNotPersistAnAttempt(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c := New("v1.0.0", nil, nil)
	if err := c.Check(ctx); err == nil || c.Status().CheckedAt != 0 {
		t.Fatal(err, c.Status())
	}
}

func TestCorruptAndFutureCacheCanRecover(t *testing.T) {
	for _, raw := range []string{`bad json`, `{"checkedAt":99999999999999}`} {
		c := New("dev", &memoryStorage{value: raw}, nil)
		if c.Status().CheckedAt != 0 {
			t.Fatal(c.Status())
		}
		if _, err := json.Marshal(c.Status()); err != nil {
			t.Fatal(err)
		}
	}
}

package updates

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"golang.org/x/mod/semver"
)

const Interval = 24 * time.Hour
const cacheKey = "releaseUpdateCheck"
const releaseAPI = "https://api.github.com/repos/desirevolution/wisp/releases/latest"

type Storage interface {
	ConfigString(string, string) string
	SetConfigString(string, string) error
}

type State struct {
	Current    string `json:"current"`
	Latest     string `json:"latest,omitempty"`
	URL        string `json:"url,omitempty"`
	CheckedAt  int64  `json:"checkedAt,omitempty"`
	Available  bool   `json:"available"`
	Comparable bool   `json:"comparable"`
	Error      string `json:"error,omitempty"`
}

type Checker struct {
	mu       sync.RWMutex
	checkMu  sync.Mutex
	state    State
	store    Storage
	client   *http.Client
	endpoint string
	changed  func(State)
}

func version(v string) string {
	if !strings.HasPrefix(v, "v") {
		v = "v" + v
	}
	return v
}

func New(current string, storage Storage, changed func(State)) *Checker {
	c := &Checker{store: storage, client: &http.Client{Timeout: 10 * time.Second}, endpoint: releaseAPI, changed: changed}
	if storage != nil {
		var cached State
		if json.Unmarshal([]byte(storage.ConfigString(cacheKey, "{}")), &cached) == nil {
			c.state = cached
		}
	}
	// A changed local version must never inherit the previous build's comparison.
	c.state.Current = current
	if c.state.CheckedAt > time.Now().Add(time.Minute).UnixMilli() {
		c.state.CheckedAt = 0
	}
	c.compare()
	return c
}

func (c *Checker) compare() {
	current, latest := version(c.state.Current), version(c.state.Latest)
	c.state.Comparable = semver.IsValid(current)
	c.state.Available = c.state.Comparable && semver.IsValid(latest) && semver.Compare(latest, current) > 0
}

func (c *Checker) Status() State {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.state
}

// Check is serialized, bounded, and rate limited even across process restarts.
// A failed check retains the last known release and is retried the next day.
func (c *Checker) Check(ctx context.Context) error {
	c.checkMu.Lock()
	defer c.checkMu.Unlock()
	s := c.Status()
	if s.CheckedAt > 0 && time.Since(time.UnixMilli(s.CheckedAt)) < Interval {
		return nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "Wisp/"+s.Current)
	var release struct {
		Tag        string `json:"tag_name"`
		Draft      bool   `json:"draft"`
		Prerelease bool   `json:"prerelease"`
	}
	response, err := c.client.Do(req)
	if err == nil {
		defer response.Body.Close()
		if response.StatusCode != http.StatusOK {
			err = fmt.Errorf("GitHub returned HTTP %d", response.StatusCode)
		} else {
			err = json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&release)
		}
	}
	if err == nil && (release.Draft || release.Prerelease || !semver.IsValid(version(release.Tag)) || semver.Prerelease(version(release.Tag)) != "") {
		err = fmt.Errorf("no valid stable release returned")
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	c.mu.Lock()
	c.state.CheckedAt = time.Now().UnixMilli()
	c.state.Error = ""
	if err != nil {
		c.state.Error = "Update check unavailable"
	} else {
		c.state.Latest = release.Tag
		c.state.URL = "https://github.com/desirevolution/wisp/releases/tag/" + url.PathEscape(release.Tag)
	}
	c.compare()
	state := c.state
	c.mu.Unlock()
	if c.store != nil {
		data, _ := json.Marshal(state)
		if saveErr := c.store.SetConfigString(cacheKey, string(data)); saveErr != nil {
			return saveErr
		}
	}
	if c.changed != nil {
		c.changed(state)
	}
	return err
}

func (c *Checker) Start(ctx context.Context) {
	go func() {
		for {
			if ctx.Err() != nil {
				return
			}
			_ = c.Check(ctx)
			wait := time.Until(time.UnixMilli(c.Status().CheckedAt).Add(Interval))
			if wait <= 0 || wait > Interval {
				wait = Interval
			}
			timer := time.NewTimer(wait)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
		}
	}()
}

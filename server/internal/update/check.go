// Package update looks for a newer Broadwave release once a day.
// The request is the whole check: no install id, no account, no extra headers.
package update

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"
)

// FeedURL is the public GitHub releases document for this repo.
const FeedURL = "https://api.github.com/repos/wolfebase/broadwave/releases/latest"

const userAgent = "Broadwave"

// Notice is what clients show when a newer release exists.
type Notice struct {
	Version  string `json:"version"`
	NotesURL string `json:"notesUrl"`
	Message  string `json:"message"`
}

// Checker polls the releases feed. A nil Enabled func means the check is on.
type Checker struct {
	Current  string
	FeedURL  string
	Interval time.Duration
	Client   *http.Client
	Now      func() time.Time
	// Enabled reports the opt-out. False performs no request.
	Enabled func(ctx context.Context) bool
	// Load restores the last result. The time is when it was fetched.
	Load func(ctx context.Context) (*Notice, time.Time)
	// Save records a result. A nil notice means this build is current.
	Save func(ctx context.Context, n *Notice, at time.Time)

	mu      sync.Mutex
	last    *Notice
	checked time.Time
	// retryAt is when a failed fetch may try again. A success clears it.
	retryAt time.Time
}

// New is the production checker. It does not start the schedule.
func New(current string) *Checker {
	return &Checker{
		Current:  current,
		FeedURL:  FeedURL,
		Interval: 24 * time.Hour,
		Client:   &http.Client{Timeout: 15 * time.Second},
	}
}

// Run checks immediately when one is due, then again after the interval.
// Opt-out skips the request and waits out the interval.
func (c *Checker) Run(ctx context.Context) {
	if c == nil {
		return
	}
	c.restore(ctx)
	timer := time.NewTimer(0)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		if c.enabled(ctx) {
			if _, err := c.Check(ctx); err != nil {
				slog.Error(fmt.Sprintf("update: %v", err))
			}
		}
		next := c.interval()
		if remain := c.wait(); remain > 0 {
			next = remain
		}
		if retry := c.retryWait(); retry > 0 && retry < next {
			next = retry
		}
		timer.Reset(next)
	}
}

// Visible is the banner to show right now. Nil when the check is off or this build is current.
func (c *Checker) Visible(ctx context.Context) *Notice {
	if c == nil || !c.enabled(ctx) {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.last == nil {
		return nil
	}
	if _, ok := newerRelease(c.last.Version, c.Current); !ok {
		return nil
	}
	n := cloneNotice(c.last)
	n.NotesURL = notesURL(n.NotesURL, "v"+n.Version)
	return n
}

// Check fetches the feed when the interval has elapsed.
// A failed fetch returns the last good notice and the error.
// Opt-out returns immediately and does not touch the network.
func (c *Checker) Check(ctx context.Context) (*Notice, error) {
	if c == nil {
		return nil, nil
	}
	if !c.enabled(ctx) {
		return nil, nil
	}
	now := c.now()
	c.mu.Lock()
	if c.Interval > 0 && !c.checked.IsZero() && now.Sub(c.checked) < c.Interval {
		n := cloneNotice(c.last)
		c.mu.Unlock()
		return n, nil
	}
	c.mu.Unlock()

	fetched, err := c.fetch(ctx)

	c.mu.Lock()
	if err != nil {
		// Leave the last success in place so a blip does not wait a full day.
		c.retryAt = now.Add(15 * time.Minute)
		n := cloneNotice(c.last)
		c.mu.Unlock()
		return n, err
	}
	c.checked = now
	c.retryAt = time.Time{}
	c.last = fetched
	n := cloneNotice(c.last)
	c.mu.Unlock()
	c.persist(ctx, n, now)
	return n, nil
}

func (c *Checker) retryWait() time.Duration {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.retryAt.IsZero() {
		return 0
	}
	remain := c.retryAt.Sub(c.now())
	if remain < 0 {
		return 0
	}
	return remain
}

func (c *Checker) restore(ctx context.Context) {
	if c.Load == nil {
		return
	}
	n, at := c.Load(ctx)
	if n != nil {
		if _, ok := newerRelease(n.Version, c.Current); !ok {
			n = nil
		}
	}
	c.mu.Lock()
	c.last = n
	c.checked = at
	c.mu.Unlock()
}

func (c *Checker) persist(ctx context.Context, n *Notice, at time.Time) {
	if c.Save == nil {
		return
	}
	c.Save(ctx, cloneNotice(n), at)
}

func (c *Checker) fetch(ctx context.Context) (*Notice, error) {
	feed := c.FeedURL
	if feed == "" {
		feed = FeedURL
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, feed, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", userAgent)
	client := c.Client
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("releases answered %d", res.StatusCode)
	}
	var doc struct {
		Tag   string `json:"tag_name"`
		Page  string `json:"html_url"`
		Draft bool   `json:"draft"`
		Pre   bool   `json:"prerelease"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, err
	}
	if doc.Draft || doc.Pre {
		return nil, nil
	}
	display, ok := newerRelease(doc.Tag, c.Current)
	if !ok {
		if _, _, _, parsed := parseVersion(doc.Tag); !parsed {
			return nil, errors.New("releases tag is not a version")
		}
		return nil, nil
	}
	return &Notice{
		Version:  display,
		NotesURL: notesURL(doc.Page, doc.Tag),
		Message:  "Broadwave " + display + " is available",
	}, nil
}

func notesURL(raw, tag string) string {
	fallback := "https://github.com/wolfebase/broadwave/releases/tag/" + url.PathEscape(strings.TrimSpace(tag))
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme != "https" || !strings.EqualFold(u.Host, "github.com") || u.User != nil {
		return fallback
	}
	if u.RawQuery != "" || u.Fragment != "" {
		return fallback
	}
	cleaned := path.Clean(u.Path)
	if cleaned != u.Path || !strings.HasPrefix(cleaned, "/wolfebase/broadwave/") {
		return fallback
	}
	u.Path = cleaned
	u.RawPath = ""
	return u.String()
}

func (c *Checker) enabled(ctx context.Context) bool {
	if c.Enabled == nil {
		return true
	}
	return c.Enabled(ctx)
}

func (c *Checker) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

func (c *Checker) interval() time.Duration {
	if c.Interval > 0 {
		return c.Interval
	}
	return 24 * time.Hour
}

func (c *Checker) wait() time.Duration {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.checked.IsZero() {
		return 0
	}
	remain := c.interval() - c.now().Sub(c.checked)
	if remain < 0 {
		return 0
	}
	return remain
}

func cloneNotice(n *Notice) *Notice {
	if n == nil {
		return nil
	}
	out := *n
	return &out
}

// newerRelease reports whether latest is a newer semver than current.
// The display string is latest without a leading v. An unversioned current
// build (a dev binary) is behind any real release.
func newerRelease(latest, current string) (string, bool) {
	lmaj, lmin, lpat, lok := parseVersion(latest)
	if !lok {
		return "", false
	}
	display := strings.TrimSpace(latest)
	display = strings.TrimPrefix(display, "v")
	display = strings.TrimPrefix(display, "V")
	if display == "" || len(display) > 32 {
		return "", false
	}
	cmaj, cmin, cpat, cok := parseVersion(current)
	if !cok {
		return display, true
	}
	if lmaj != cmaj {
		return display, lmaj > cmaj
	}
	if lmin != cmin {
		return display, lmin > cmin
	}
	return display, lpat > cpat
}

func parseVersion(raw string) (maj, min, pat int, ok bool) {
	s := strings.TrimSpace(raw)
	s = strings.TrimPrefix(s, "v")
	s = strings.TrimPrefix(s, "V")
	if i := strings.IndexAny(s, "-+"); i >= 0 {
		s = s[:i]
	}
	if s == "" {
		return 0, 0, 0, false
	}
	parts := strings.Split(s, ".")
	if len(parts) > 3 {
		return 0, 0, 0, false
	}
	nums := [3]int{}
	for i, part := range parts {
		if part == "" {
			return 0, 0, 0, false
		}
		n, err := strconv.Atoi(part)
		if err != nil || n < 0 {
			return 0, 0, 0, false
		}
		nums[i] = n
	}
	return nums[0], nums[1], nums[2], true
}

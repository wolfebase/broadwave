package update

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestNewUsesThePublicFeed(t *testing.T) {
	c := New("0.13.0")
	if c.Current != "0.13.0" {
		t.Fatalf("current %q", c.Current)
	}
	if c.FeedURL != FeedURL {
		t.Fatalf("feed %q", c.FeedURL)
	}
	if c.Interval != 24*time.Hour {
		t.Fatalf("interval %s", c.Interval)
	}
	if c.Client == nil {
		t.Fatal("client is nil")
	}
}

func TestRunReturnsWhenTheCheckerIsNil(t *testing.T) {
	done := make(chan struct{})
	go func() {
		defer close(done)
		var c *Checker
		c.Run(context.Background())
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return")
	}
}

func TestRunReturnsWhenTheContextIsCancelled(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"tag_name":"v0.14.0","html_url":"https://github.com/wolfebase/broadwave/releases/tag/v0.14.0"}`))
	}))
	t.Cleanup(srv.Close)

	fixed := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	var restored atomic.Bool
	c := &Checker{
		Current:  "0.13.0",
		FeedURL:  srv.URL,
		Interval: time.Hour,
		Client:   srv.Client(),
		Now:      func() time.Time { return fixed },
		Load: func(context.Context) (*Notice, time.Time) {
			restored.Store(true)
			return nil, time.Time{}
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		c.Run(ctx)
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
		}
	})
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return")
	}
	if !restored.Load() {
		t.Fatal("restore did not run")
	}
	if hits.Load() > 1 {
		t.Fatalf("hits %d", hits.Load())
	}
}

func TestAFailedFetchRetriesSoonerThanADay(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		http.Error(w, "nope", http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)

	fixed := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	c := &Checker{
		Current:  "0.13.0",
		FeedURL:  srv.URL,
		Interval: 0,
		Client:   srv.Client(),
		Now:      func() time.Time { return fixed },
	}
	if c.interval() != 24*time.Hour {
		t.Fatalf("interval %s", c.interval())
	}
	if c.wait() != 0 {
		t.Fatalf("wait before a check %s", c.wait())
	}
	if c.retryWait() != 0 {
		t.Fatalf("retry before a check %s", c.retryWait())
	}
	c.Interval = 24 * time.Hour
	n, err := c.Check(context.Background())
	if err == nil {
		t.Fatal("expected fetch error")
	}
	if n != nil {
		t.Fatalf("notice %+v", n)
	}
	if hits.Load() != 1 {
		t.Fatalf("hits %d", hits.Load())
	}
	retry := c.retryWait()
	if retry <= 0 || retry >= 20*time.Minute {
		t.Fatalf("retry %s", retry)
	}
	if c.wait() != 0 {
		t.Fatalf("failed check waited %s", c.wait())
	}
	if retry >= c.interval() {
		t.Fatalf("retry %s is not sooner than %s", retry, c.interval())
	}

	// Run checks once more immediately, then sits on the short retry.
	cancel, done := startRun(t, c)
	deadline := time.Now().Add(5 * time.Second)
	for hits.Load() < 2 {
		if time.Now().After(deadline) {
			t.Fatalf("hits %d", hits.Load())
		}
		time.Sleep(5 * time.Millisecond)
	}
	time.Sleep(50 * time.Millisecond)
	if hits.Load() != 2 {
		t.Fatalf("hits %d", hits.Load())
	}
	if again := c.retryWait(); again <= 0 || again >= 20*time.Minute {
		t.Fatalf("retry after Run %s", again)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return")
	}
}

func TestTheNextCheckWaitsForTheInterval(t *testing.T) {
	var hits atomic.Int32
	const notes = "https://github.com/wolfebase/broadwave/releases/tag/v0.14.0"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"tag_name":"v0.14.0","html_url":"` + notes + `"}`))
	}))
	t.Cleanup(srv.Close)

	fixed := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	c := &Checker{
		Current:  "0.13.0",
		FeedURL:  srv.URL,
		Interval: 24 * time.Hour,
		Client:   srv.Client(),
		Now:      func() time.Time { return fixed },
	}
	ctx := context.Background()
	n, err := c.Check(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if n == nil || n.Version != "0.14.0" {
		t.Fatalf("%+v", n)
	}
	if wait := c.wait(); wait != 24*time.Hour {
		t.Fatalf("wait %s", wait)
	}
	again, err := c.Check(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if again == nil || again.Version != "0.14.0" {
		t.Fatalf("cached %+v", again)
	}
	if hits.Load() != 1 {
		t.Fatalf("hits %d", hits.Load())
	}

	c.Now = func() time.Time { return fixed.Add(24*time.Hour + time.Second) }
	if c.wait() != 0 {
		t.Fatalf("elapsed wait %s", c.wait())
	}
	if _, err := c.Check(ctx); err != nil {
		t.Fatal(err)
	}
	if hits.Load() != 2 {
		t.Fatalf("hits %d", hits.Load())
	}
}

func TestRunChecksOnceThenWaits(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"tag_name":"v0.14.0","html_url":"https://github.com/wolfebase/broadwave/releases/tag/v0.14.0"}`))
	}))
	t.Cleanup(srv.Close)

	fixed := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	c := &Checker{
		Current:  "0.13.0",
		FeedURL:  srv.URL,
		Interval: time.Hour,
		Client:   srv.Client(),
		Now:      func() time.Time { return fixed },
	}
	if c.interval() != time.Hour {
		t.Fatalf("interval %s", c.interval())
	}
	cancel, done := startRun(t, c)
	deadline := time.Now().Add(5 * time.Second)
	for c.wait() <= 0 {
		if time.Now().After(deadline) {
			t.Fatal("Run did not record the check")
		}
		time.Sleep(5 * time.Millisecond)
	}
	if hits.Load() != 1 {
		t.Fatalf("hits %d", hits.Load())
	}
	if remain := c.wait(); remain != time.Hour {
		t.Fatalf("next wait %s", remain)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return")
	}
	if hits.Load() != 1 {
		t.Fatalf("hits %d", hits.Load())
	}
}

func TestRunSkipsTheFeedWhenUpdatesAreOff(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"tag_name":"v9.0.0"}`))
	}))
	t.Cleanup(srv.Close)

	asked := make(chan struct{}, 1)
	fixed := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	c := &Checker{
		Current:  "0.13.0",
		FeedURL:  srv.URL,
		Interval: time.Hour,
		Client:   srv.Client(),
		Now:      func() time.Time { return fixed },
		Enabled: func(context.Context) bool {
			select {
			case asked <- struct{}{}:
			default:
			}
			return false
		},
	}
	cancel, done := startRun(t, c)
	select {
	case <-asked:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not ask whether updates are on")
	}
	if hits.Load() != 0 {
		t.Fatalf("hits %d", hits.Load())
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return")
	}
	if hits.Load() != 0 {
		t.Fatalf("hits %d", hits.Load())
	}
}

func TestVersionCompare(t *testing.T) {
	if _, ok := newerRelease("v1.2.3-rc1", "1.2.3"); ok {
		t.Fatal("pre-release counted as newer")
	}
	maj, min, pat, ok := parseVersion("v1.2.3-rc1")
	if !ok || maj != 1 || min != 2 || pat != 3 {
		t.Fatalf("pre-release parsed %d.%d.%d %v", maj, min, pat, ok)
	}
	if _, _, _, ok := parseVersion(""); ok {
		t.Fatal("empty version parsed")
	}
	if got, ok := newerRelease("", "1.2.3"); ok || got != "" {
		t.Fatalf("empty %q %v", got, ok)
	}
	if _, _, _, ok := parseVersion("1.2.3.4"); ok {
		t.Fatal("four components parsed")
	}
	if got, ok := newerRelease("1.2.3.4", "1.0.0"); ok || got != "" {
		t.Fatalf("four components %q %v", got, ok)
	}
	got, ok := newerRelease("v2.0.0", "dev")
	if !ok || got != "2.0.0" {
		t.Fatalf("dev %q %v", got, ok)
	}
}

func TestAPreReleaseIsNotNewer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"tag_name":"v9.0.0","prerelease":true}`))
	}))
	t.Cleanup(srv.Close)
	c := &Checker{
		Current:  "0.1.0",
		FeedURL:  srv.URL,
		Interval: 0,
		Client:   srv.Client(),
	}
	n, err := c.Check(context.Background())
	if err != nil || n != nil {
		t.Fatalf("pre-release %+v %v", n, err)
	}
}

func startRun(t *testing.T, c *Checker) (context.CancelFunc, <-chan struct{}) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	done := make(chan struct{})
	go func() {
		defer close(done)
		c.Run(ctx)
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
		}
	})
	return cancel, done
}

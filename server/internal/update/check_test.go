package update

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestReleaseFeed(t *testing.T) {
	var hits int
	status := http.StatusOK
	payload := ""
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if r.Method != http.MethodGet {
			t.Errorf("method %s", r.Method)
		}
		if r.URL.RawQuery != "" {
			t.Errorf("query %s", r.URL.RawQuery)
		}
		if r.Body != nil {
			buf, _ := io.ReadAll(r.Body)
			if len(buf) != 0 {
				t.Errorf("body %q", buf)
			}
		}
		if r.Header.Get("User-Agent") != "Broadwave" {
			t.Errorf("user agent %q", r.Header.Get("User-Agent"))
		}
		if r.Header.Get("Accept") != "application/vnd.github+json" {
			t.Errorf("accept %q", r.Header.Get("Accept"))
		}
		if r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" {
			t.Errorf("identifying header auth %q cookie %q", r.Header.Get("Authorization"), r.Header.Get("Cookie"))
		}
		for name := range r.Header {
			if strings.HasPrefix(name, "X-") {
				t.Errorf("extra header %s", name)
			}
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(payload))
	}))
	t.Cleanup(srv.Close)

	const notes = "https://github.com/wolfebase/broadwave/releases/tag/v0.7"
	newer := `{"tag_name":"v0.7","html_url":"` + notes + `"}`
	ctx := context.Background()
	fresh := func(current string, on bool) *Checker {
		return &Checker{
			Current:  current,
			FeedURL:  srv.URL,
			Interval: 0,
			Client:   srv.Client(),
			Enabled:  func(context.Context) bool { return on },
		}
	}

	t.Run("newer", func(t *testing.T) {
		hits = 0
		status = http.StatusOK
		payload = newer
		n, err := fresh("0.6.0", true).Check(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if n == nil || n.Message != "Broadwave 0.7 is available" || n.NotesURL != notes || n.Version != "0.7" {
			t.Fatalf("%+v", n)
		}
		if hits != 1 {
			t.Fatalf("hits %d", hits)
		}
	})

	t.Run("same", func(t *testing.T) {
		hits = 0
		status = http.StatusOK
		payload = `{"tag_name":"v0.7.0","html_url":"https://github.com/wolfebase/broadwave/releases/tag/v0.7.0"}`
		n, err := fresh("0.7.0", true).Check(ctx)
		if err != nil || n != nil {
			t.Fatalf("same %+v %v", n, err)
		}
		payload = newer
		n, err = fresh("0.7.0", true).Check(ctx)
		if err != nil || n != nil {
			t.Fatalf("same patch %+v %v", n, err)
		}
		if hits != 2 {
			t.Fatalf("hits %d", hits)
		}
	})

	t.Run("notes stay on the release page", func(t *testing.T) {
		hits = 0
		status = http.StatusOK
		payload = `{"tag_name":"v0.7","html_url":"https://evil.example/notes"}`
		n, err := fresh("0.6.0", true).Check(ctx)
		if err != nil || n == nil || n.NotesURL != notes {
			t.Fatalf("foreign notes %+v %v", n, err)
		}
		payload = `{"tag_name":"v0.7","html_url":"http://github.com/wolfebase/broadwave/releases/tag/v0.7"}`
		n, err = fresh("0.6.0", true).Check(ctx)
		if err != nil || n == nil || n.NotesURL != notes {
			t.Fatalf("plain http notes %+v %v", n, err)
		}
		payload = `{"tag_name":"v0.7","html_url":"https://github.com/wolfebase/broadwave/../../../login"}`
		n, err = fresh("0.6.0", true).Check(ctx)
		if err != nil || n == nil || n.NotesURL != notes {
			t.Fatalf("dotdot notes %+v %v", n, err)
		}
	})

	t.Run("older", func(t *testing.T) {
		hits = 0
		status = http.StatusOK
		payload = newer
		n, err := fresh("0.8.0", true).Check(ctx)
		if err != nil || n != nil {
			t.Fatalf("older %+v %v", n, err)
		}
		if hits != 1 {
			t.Fatalf("hits %d", hits)
		}
	})

	t.Run("opt-out", func(t *testing.T) {
		hits = 0
		status = http.StatusOK
		payload = newer
		c := fresh("0.6.0", false)
		n, err := c.Check(ctx)
		if err != nil || n != nil || c.Visible(ctx) != nil {
			t.Fatalf("opt-out notice %+v %v", n, err)
		}
		if hits != 0 {
			t.Fatalf("opt-out made %d requests", hits)
		}
		c.Enabled = func(context.Context) bool { return true }
		if _, err := c.Check(ctx); err != nil {
			t.Fatal(err)
		}
		hits = 0
		c.Enabled = func(context.Context) bool { return false }
		n, err = c.Check(ctx)
		if err != nil || n != nil || hits != 0 || c.Visible(ctx) != nil {
			t.Fatalf("opt-out after a hit notice %+v err %v hits %d", n, err, hits)
		}
	})

	t.Run("failed fetch keeps last", func(t *testing.T) {
		status = http.StatusOK
		payload = newer
		c := fresh("0.6.0", true)
		first, err := c.Check(ctx)
		if err != nil || first == nil {
			t.Fatalf("first %+v %v", first, err)
		}
		status = http.StatusInternalServerError
		payload = "nope"
		second, err := c.Check(ctx)
		if err == nil {
			t.Fatal("expected fetch error")
		}
		if second == nil || second.Message != "Broadwave 0.7 is available" || second.NotesURL != notes {
			t.Fatalf("lost last good %+v", second)
		}
		if got := c.Visible(ctx); got == nil || got.Message != first.Message {
			t.Fatalf("visible %+v", got)
		}
		c.FeedURL = "http://127.0.0.1:1"
		c.Client = &http.Client{Timeout: 200 * time.Millisecond}
		third, err := c.Check(ctx)
		if err == nil {
			t.Fatal("expected connection error")
		}
		if third == nil || third.Message != "Broadwave 0.7 is available" || third.NotesURL != notes {
			t.Fatalf("lost last good after disconnect %+v", third)
		}
	})

	t.Run("reload keeps last when the next fetch fails", func(t *testing.T) {
		status = http.StatusOK
		payload = newer
		var saved *Notice
		var at time.Time
		c := fresh("0.6.0", true)
		c.Save = func(_ context.Context, n *Notice, when time.Time) {
			saved = cloneNotice(n)
			at = when
		}
		if _, err := c.Check(ctx); err != nil {
			t.Fatal(err)
		}
		if saved == nil || saved.Message != "Broadwave 0.7 is available" {
			t.Fatalf("saved %+v", saved)
		}
		status = http.StatusBadGateway
		next := fresh("0.6.0", true)
		next.Interval = time.Hour
		next.Load = func(context.Context) (*Notice, time.Time) {
			return cloneNotice(saved), at
		}
		next.Now = func() time.Time { return at.Add(2 * time.Hour) }
		next.restore(ctx)
		got, err := next.Check(ctx)
		if err == nil {
			t.Fatal("expected fetch error")
		}
		if got == nil || got.Message != "Broadwave 0.7 is available" || got.NotesURL != notes {
			t.Fatalf("reloaded %+v", got)
		}
	})
}

package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"broadwave/internal/update"
)

func TestServerUpdateField(t *testing.T) {
	st := testStore(t)
	const notes = "https://github.com/wolfebase/broadwave/releases/tag/v0.7"
	feed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"tag_name":"v0.7","html_url":"` + notes + `"}`))
	}))
	t.Cleanup(feed.Close)
	checker := &update.Checker{
		Current:  "0.6.0",
		FeedURL:  feed.URL,
		Interval: 0,
		Client:   feed.Client(),
		Enabled: func(ctx context.Context) bool {
			on, err := st.UpdatesEnabled(ctx)
			return err == nil && on
		},
	}
	if _, err := checker.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	h := (&Server{Store: st, Version: "0.6.0", Updates: checker}).Handler()
	res := get(t, h, "/api/v1/server")
	var info struct {
		Update *struct {
			Version  string `json:"version"`
			NotesURL string `json:"notesUrl"`
			Message  string `json:"message"`
		} `json:"update"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &info); err != nil {
		t.Fatal(err)
	}
	if info.Update == nil || info.Update.Message != "Broadwave 0.7 is available" || info.Update.NotesURL != notes || info.Update.Version != "0.7" {
		t.Fatalf("%s", res.Body.String())
	}
	if err := st.PutSettings(context.Background(), map[string]string{"checkUpdates": "0"}); err != nil {
		t.Fatal(err)
	}
	res = get(t, h, "/api/v1/server")
	if bytes.Contains(res.Body.Bytes(), []byte(`"update"`)) {
		t.Fatalf("banner while off: %s", res.Body.String())
	}
}

func TestSettingsHideUpdateCache(t *testing.T) {
	st := testStore(t)
	if err := st.SaveUpdate(context.Background(), "0.7", "https://github.com/wolfebase/broadwave/releases/tag/v0.7", "Broadwave 0.7 is available", time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := st.PutSettings(context.Background(), map[string]string{"checkUpdates": "0"}); err != nil {
		t.Fatal(err)
	}
	res := get(t, (&Server{Store: st}).Handler(), "/api/v1/settings")
	body := res.Body.Bytes()
	for _, secret := range []string{"updateVersion", "updateNotesURL", "updateMessage", "updateCheckedAt", "Broadwave 0.7"} {
		if bytes.Contains(body, []byte(secret)) {
			t.Fatalf("cache leaked %s: %s", secret, body)
		}
	}
	if !bytes.Contains(res.Body.Bytes(), []byte(`"checkUpdates":"0"`)) {
		t.Fatalf("%s", res.Body.String())
	}
}

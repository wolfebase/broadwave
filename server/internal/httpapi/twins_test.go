package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"broadwave/internal/hdhr"
	"broadwave/internal/hdhr/fake"
	"broadwave/internal/store"
)

// The fake FLEX 4K lineup pairs 4.1 with 104.1. The guide lists the half the
// choice keeps, and a favorite set on the hidden half follows that one.
// 115.1 is encrypted, so it stays off the guide and refuses a choice.
func TestFlex4KTwinChoiceAndFavorite(t *testing.T) {
	ctx := context.Background()
	tuner := &fake.Server{Profile: fake.ProfileFlex4K}
	base, _, err := tuner.Start()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tuner.Close)
	client := &hdhr.Client{}
	dev, err := client.FetchDevice(ctx, base)
	if err != nil {
		t.Fatal(err)
	}
	lineup, err := client.FetchLineup(ctx, dev.LineupURL)
	if err != nil {
		t.Fatal(err)
	}
	st := testStore(t)
	if err := st.UpsertDevice(ctx, dev, lineup); err != nil {
		t.Fatal(err)
	}
	h := (&Server{Store: st}).Handler()

	list := func(guide bool) []store.Channel {
		t.Helper()
		path := "/api/v1/channels"
		if guide {
			path += "?guide=1"
		}
		res := get(t, h, path)
		var body struct {
			Channels []store.Channel `json:"channels"`
		}
		if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		return body.Channels
	}
	numbers := func() []string {
		t.Helper()
		var out []string
		for _, ch := range list(true) {
			out = append(out, ch.GuideNumber)
		}
		return out
	}
	byNumber := func(guide bool) map[string]store.Channel {
		t.Helper()
		out := map[string]store.Channel{}
		for _, ch := range list(guide) {
			out[ch.GuideNumber] = ch
		}
		return out
	}
	id := func(number string) int64 {
		t.Helper()
		ch, ok := byNumber(false)[number]
		if !ok {
			t.Fatalf("lineup has no %s", number)
		}
		return ch.ID
	}
	patch := func(channel int64, body string) (int, store.Channel) {
		t.Helper()
		req := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/v1/channels/%d", channel), strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		var ch store.Channel
		_ = json.Unmarshal(rec.Body.Bytes(), &ch)
		return rec.Code, ch
	}
	expectGuide := func(choice string, want []string) {
		t.Helper()
		if got := numbers(); !slices.Equal(got, want) {
			t.Fatalf("%s lists %v, want %v", choice, got, want)
		}
	}

	expectGuide("atsc3", []string{"4.2", "5.1", "104.1"})

	code, hidden := patch(id("4.1"), `{"favorite":true}`)
	if code != http.StatusOK || !hidden.Hidden || !hidden.Favorite {
		t.Fatalf("favorite on hidden 4.1: %d %+v", code, hidden)
	}
	if shown := byNumber(true)["104.1"]; !shown.Favorite {
		t.Fatalf("104.1 %+v, want the favorite that was set on hidden 4.1", shown)
	}

	if code, _ = patch(id("4.1"), `{"twinChoice":"atsc1"}`); code != http.StatusOK {
		t.Fatalf("atsc1 %d", code)
	}
	expectGuide("atsc1", []string{"4.1", "4.2", "5.1"})
	if shown := byNumber(true)["4.1"]; !shown.Favorite {
		t.Fatalf("4.1 %+v, want the favorite after 104.1 is hidden", shown)
	}

	// The star is on the channel that stays. Clear both, then set it on the hidden 3.0 row.
	if code, _ = patch(id("4.1"), `{"favorite":false}`); code != http.StatusOK {
		t.Fatalf("clear 4.1 %d", code)
	}
	if code, _ = patch(id("104.1"), `{"favorite":false}`); code != http.StatusOK {
		t.Fatalf("clear 104.1 %d", code)
	}
	code, hidden = patch(id("104.1"), `{"favorite":true}`)
	if code != http.StatusOK || !hidden.Hidden || !hidden.Favorite {
		t.Fatalf("favorite on hidden 104.1: %d %+v", code, hidden)
	}
	if shown := byNumber(true)["4.1"]; !shown.Favorite {
		t.Fatalf("4.1 %+v, want the favorite that was set on hidden 104.1", shown)
	}

	if code, _ = patch(id("104.1"), `{"twinChoice":"both"}`); code != http.StatusOK {
		t.Fatalf("both %d", code)
	}
	expectGuide("both", []string{"4.1", "4.2", "5.1", "104.1"})

	if code, _ = patch(id("4.1"), `{"twinChoice":"atsc3"}`); code != http.StatusOK {
		t.Fatalf("atsc3 %d", code)
	}
	expectGuide("atsc3 again", []string{"4.2", "5.1", "104.1"})
	if shown := byNumber(true)["104.1"]; !shown.Favorite {
		t.Fatalf("104.1 %+v, want the favorite after 4.1 is hidden again", shown)
	}

	enc, ok := byNumber(false)["115.1"]
	if !ok || !enc.Hidden || !enc.Protected || enc.PlaysAs == 0 {
		t.Fatalf("115.1 %+v, want a hidden encrypted row that plays as 5.1", enc)
	}
	if _, onGuide := byNumber(true)["115.1"]; onGuide {
		t.Fatal("115.1 is on the guide")
	}
	for _, choice := range []string{"atsc3", "atsc1", "both"} {
		code, body := patch(enc.ID, fmt.Sprintf(`{"twinChoice":%q}`, choice))
		if code != http.StatusConflict {
			t.Fatalf("twinChoice %s on 115.1 answered %d %s", choice, code, body.GuideNumber)
		}
	}
	if _, onGuide := byNumber(true)["115.1"]; onGuide {
		t.Fatal("115.1 appeared after a refused choice")
	}
}

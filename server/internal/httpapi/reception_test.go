package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"broadwave/internal/hdhr"
	"broadwave/internal/hdhr/fake"
	"broadwave/internal/live"
)

// A TV paired with Watch access reads this during a stall. The stall it matters
// most for is a tuner that stopped answering, so that has to be an answer, not
// an error, and it has to change when the tuner comes back.
func TestReceptionAnswersAWatchTokenWhileTheTunerIsSilent(t *testing.T) {
	ctx := context.Background()
	srv := &fake.Server{Channels: []fake.Channel{{Number: "4.1", Name: "KBWV", Freq: 593000000}}}
	base, port, err := srv.Start()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(srv.Close)
	t.Setenv("HDHR_CONTROL_PORT", port)
	st := testStore(t)
	client := &hdhr.Client{}
	dev, err := client.FetchDevice(ctx, base)
	if err != nil {
		t.Fatal(err)
	}
	lineup, err := client.FetchLineup(ctx, dev.LineupURL)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertDevice(ctx, dev, lineup); err != nil {
		t.Fatal(err)
	}
	chs, err := st.Channels(ctx, false)
	if err != nil || len(chs) != 1 {
		t.Fatal(err, chs)
	}
	hub := live.New(st, t.TempDir(), "ffmpeg", "libx264")
	t.Cleanup(hub.Shutdown)
	h := (&Server{Store: st, Hub: hub}).Handler()

	turned := call(t, h, http.MethodPut, "/api/v1/settings", `{"deviceAuth":"1"}`, "")
	if turned.Code != http.StatusOK {
		t.Fatalf("enable %d %s", turned.Code, turned.Body.String())
	}
	admin := jsonField(t, turned.Body.Bytes(), "deviceToken")
	code := call(t, h, http.MethodPost, "/api/v1/pair/code", `{"scopes":["watch"],"kind":"phone"}`, admin)
	watch := jsonField(t, call(t, h, http.MethodPost, "/api/v1/pair/claim", `{"code":"`+jsonField(t, code.Body.Bytes(), "code")+`","name":"Living room","kind":"other"}`, "").Body.Bytes(), "token")

	path := fmt.Sprintf("/api/v1/channels/%d/reception", chs[0].ID)
	read := func() (int, bool) {
		got := call(t, h, http.MethodGet, path, "", watch)
		var snap struct {
			TunerAnswers bool `json:"tunerAnswers"`
		}
		_ = json.Unmarshal(got.Body.Bytes(), &snap)
		return got.Code, snap.TunerAnswers
	}
	if status, answers := read(); status != http.StatusOK || !answers {
		t.Fatalf("a working tuner: %d answers=%v", status, answers)
	}
	srv.Silence()
	start := time.Now()
	status, answers := read()
	if status != http.StatusOK {
		t.Fatalf("a silent tuner must be an answer: %d", status)
	}
	if answers {
		t.Fatal("a silent tuner reads as answering")
	}
	// The app probes on a clock. It can't wait out every timeout.
	if took := time.Since(start); took > 8*time.Second {
		t.Fatalf("the read took %s", took)
	}
	srv.Answer()
	if status, answers := read(); status != http.StatusOK || !answers {
		t.Fatalf("a tuner that came back: %d answers=%v", status, answers)
	}
}

package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"broadwave/internal/dvr"
	"broadwave/internal/hdhr"
	"broadwave/internal/live"
	"broadwave/internal/store"
)

func TestStoragePlanDeletesNothing(t *testing.T) {
	cfg := filepath.Join(t.TempDir(), "cfg")
	st, err := store.Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	ctx := context.Background()
	if err := st.PutSettings(ctx, map[string]string{"deleteWatchedDays": "1", "protectFavorites": "1"}); err != nil {
		t.Fatal(err)
	}
	work := t.TempDir()
	dir := filepath.Join(work, "recordings")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	unconfigured := httptest.NewRecorder()
	(&Server{Store: st}).Handler().ServeHTTP(unconfigured, httptest.NewRequest(http.MethodGet, "/api/v1/storage/plan", nil))
	if unconfigured.Code != http.StatusServiceUnavailable || !strings.Contains(unconfigured.Body.String(), "player is not configured") {
		t.Fatalf("unconfigured %d %s", unconfigured.Code, unconfigured.Body.String())
	}

	dev := hdhr.Device{DeviceID: "TESTBOX", FriendlyName: "Test", BaseURL: "http://203.0.113.10", LineupURL: "http://203.0.113.10/lineup.json", TunerCount: 1}
	if err := st.UpsertDevice(ctx, dev, []hdhr.Channel{{
		GuideNumber: "4.1", GuideName: "WZZZ", Favorite: true, StreamURL: "http://203.0.113.10:5004/auto/v4.1",
	}}); err != nil {
		t.Fatal(err)
	}
	channels, err := st.Channels(ctx, false)
	if err != nil || len(channels) != 1 {
		t.Fatal(err, channels)
	}

	write := func(name string) string {
		t.Helper()
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, make([]byte, 100), 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}
	add := func(title, path, status, program string, channel int64, keep bool) int64 {
		t.Helper()
		id, err := st.CreateRecording(ctx, store.Recording{
			Title: title, Path: path, Status: "recording", ChannelID: channel, ProgramID: program,
			StartedAt: time.Now().Add(-2 * time.Hour),
		})
		if err != nil {
			t.Fatal(err)
		}
		if status != "recording" {
			if err := st.FinishRecording(ctx, id, status, ""); err != nil {
				t.Fatal(err)
			}
			if err := st.SetDuration(ctx, id, 1800); err != nil {
				t.Fatal(err)
			}
			if err := st.SetWatched(ctx, id, 1); err != nil {
				t.Fatal(err)
			}
		}
		if keep {
			if err := st.SetKeep(ctx, id, true); err != nil {
				t.Fatal(err)
			}
		}
		return id
	}
	newsPath := write("news.ts")
	news := add("Evening News", newsPath, "complete", "", 0, false)
	add("Live", write("live.ts"), "recording", "", 0, false)
	add("Kept", write("kept.ts"), "complete", "", 0, true)
	moved := add("Moved", write("moved.ts"), "complete", "", 0, false)
	if err := os.Remove(filepath.Join(dir, "moved.ts")); err != nil {
		t.Fatal(err)
	}
	add("Favorite", write("favorite.ts"), "complete", "", channels[0].ID, false)
	ruined := add("Ruined", write("ruined.ts"), "complete", "EP-RUIN", 0, false)
	if err := st.SetRecordingHealth(ctx, ruined, store.RecordingHealth{LostSeconds: 12.4, Gaps: 1, ContinuityErrors: 30}); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, "ruined.ts")); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-48 * time.Hour).UTC().Format(time.RFC3339)
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(filepath.Join(cfg, "broadwave.db"))+"?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []int64{news, moved, ruined} {
		if _, err := db.Exec(`UPDATE recordings SET watched_at = ? WHERE id = ?`, old, id); err != nil {
			_ = db.Close()
			t.Fatal(err)
		}
	}
	// Kept forever is aged too, so the days rule is what it is protected from.
	keptRows, err := st.Recordings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, rec := range keptRows {
		if rec.Title != "Kept" && rec.Title != "Favorite" {
			continue
		}
		if _, err := db.Exec(`UPDATE recordings SET watched_at = ? WHERE id = ?`, old, rec.ID); err != nil {
			_ = db.Close()
			t.Fatal(err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	h := (&Server{Store: st, Hub: &live.Hub{Store: st, Dir: work}}).Handler()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/storage/plan", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), ".ts") {
		t.Fatalf("plan names a file: %s", rec.Body.String())
	}
	var plan dvr.StoragePlan
	if err := json.Unmarshal(rec.Body.Bytes(), &plan); err != nil {
		t.Fatal(err)
	}
	if !plan.ProtectFavorites || plan.DeleteWatchedDays != 1 || plan.MakeRoom {
		t.Fatalf("rules %+v", plan)
	}
	if titles(plan.Remove, "watchedDays") != "Evening News" {
		t.Fatalf("remove %+v", plan.Remove)
	}
	if plan.Remove[0].Reason != "Watched more than 1 day ago." || plan.Remove[0].When != "now" {
		t.Fatalf("reason %+v", plan.Remove[0])
	}
	if titles(plan.Held, "recording") != "Live" || titles(plan.Held, "kept") != "Kept" ||
		titles(plan.Held, "favorite") != "Favorite" || titles(plan.Held, "missing") != "Moved Ruined" {
		t.Fatalf("held %+v", plan.Held)
	}
	if len(plan.Again) != 1 || plan.Again[0].Title != "Ruined" || plan.Again[0].Rule != "lostTime" || plan.Again[0].Reason != "Signal dropped for 12 s." {
		t.Fatalf("again %+v", plan.Again)
	}
	if _, err := os.Stat(newsPath); err != nil {
		t.Fatalf("preview removed the file: %v", err)
	}
	left, err := st.Recordings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(left) != 6 {
		t.Fatalf("rows %d", len(left))
	}
}

func titles(items []dvr.StoragePlanItem, rule string) string {
	var out string
	for _, item := range items {
		if rule != "" && item.Rule != rule {
			continue
		}
		if out != "" {
			out += " "
		}
		out += item.Title
	}
	return out
}

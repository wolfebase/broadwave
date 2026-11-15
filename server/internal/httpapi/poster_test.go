package httpapi

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"broadwave/internal/live"
	"broadwave/internal/store"
)

// A poster made before 0.18.2 was the picture 2 s in, and on a recording
// that starts on an ad's end card it was one flat colour. It is made again
// from a picture, and a recording with no picture gets no poster.
func TestAFlatPosterIsMadeAgainFromAPicture(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg is not installed")
	}
	st := testStore(t)
	dir := t.TempDir()
	hub := &live.Hub{Store: st, Dir: dir, FFmpeg: "ffmpeg"}
	if err := os.MkdirAll(hub.Recordings(), 0o755); err != nil {
		t.Fatal(err)
	}
	sample := func(name, graph string) string {
		path := filepath.Join(hub.Recordings(), name)
		cmd := exec.Command("ffmpeg", "-hide_banner", "-loglevel", "error", "-f", "lavfi", "-i", graph,
			"-c:v", "libx264", "-preset", "ultrafast", "-g", "30", "-pix_fmt", "yuv420p", "-f", "mpegts", path)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("sample: %v %s", err, out)
		}
		return path
	}
	ctx := context.Background()
	show := sample("show.ts", "color=c=0x1f4fa0:s=640x360:r=30:d=7,format=yuv420p[a];"+
		"testsrc2=s=640x360:r=30:d=13,noise=alls=12:allf=t+u,format=yuv420p[b];[a][b]concat=n=2:v=1:a=0[out0]")
	id, err := st.CreateRecording(ctx, store.Recording{Title: "Show", GuideNumber: "4.1", Status: "complete", Path: show, StartedAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	card := sample("card.ts", "color=c=0x1f4fa0:s=640x360:r=30:d=10,format=yuv420p[out0]")
	flat, err := st.CreateRecording(ctx, store.Recording{Title: "Card", GuideNumber: "4.1", Status: "complete", Path: card, StartedAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	// The old poster: a flat frame at the old name.
	old := filepath.Join(dir, "posters", strconv.FormatInt(id, 10)+".jpg")
	if err := os.MkdirAll(filepath.Dir(old), 0o755); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("ffmpeg", "-hide_banner", "-loglevel", "error", "-f", "lavfi", "-i", "color=c=0x1f4fa0:s=480x270",
		"-frames:v", "1", old).CombinedOutput(); err != nil {
		t.Fatalf("old poster: %v %s", err, out)
	}
	stale, err := os.ReadFile(old)
	if err != nil {
		t.Fatal(err)
	}
	h := (&Server{Store: st, Hub: hub}).Handler()
	res := get(t, h, "/media/poster/"+strconv.FormatInt(id, 10))
	if res.Code != http.StatusOK || res.Header().Get("Content-Type") != "image/jpeg" || res.Header().Get("Cache-Control") != "public, max-age=3600" {
		t.Fatalf("poster: %d %v", res.Code, res.Header())
	}
	if bytes.Equal(res.Body.Bytes(), stale) {
		t.Fatal("served the old flat poster")
	}
	kept, err := os.ReadFile(live.PosterPath(dir, id))
	if err != nil || !bytes.Equal(kept, res.Body.Bytes()) {
		t.Fatalf("poster not kept: %v", err)
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Fatalf("old poster left behind: %v", err)
	}
	// The length was measured for the poster and is kept for the list.
	if rec, err := st.Recording(ctx, id); err != nil || rec.Duration < 19 {
		t.Fatalf("length: %v %v", rec.Duration, err)
	}
	miss := httptest.NewRecorder()
	h.ServeHTTP(miss, httptest.NewRequest(http.MethodGet, "/media/poster/"+strconv.FormatInt(flat, 10), nil))
	if miss.Code != http.StatusNotFound {
		t.Fatalf("flat recording: %d", miss.Code)
	}
	if _, err := os.Stat(live.PosterPath(dir, flat)); !os.IsNotExist(err) {
		t.Fatalf("flat poster kept: %v", err)
	}
}

// A recordings list loads its posters together; each one answers.
func TestManyPostersAtOnceAllAnswer(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg is not installed")
	}
	st := testStore(t)
	dir := t.TempDir()
	hub := &live.Hub{Store: st, Dir: dir, FFmpeg: "ffmpeg"}
	src := filepath.Join(hub.Recordings(), "show.ts")
	if err := os.MkdirAll(filepath.Dir(src), 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("ffmpeg", "-hide_banner", "-loglevel", "error", "-f", "lavfi",
		"-i", "testsrc2=s=640x360:r=30:d=10,noise=alls=12:allf=t+u,format=yuv420p",
		"-c:v", "libx264", "-preset", "ultrafast", "-g", "30", "-pix_fmt", "yuv420p", "-f", "mpegts", src)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("sample: %v %s", err, out)
	}
	ctx := context.Background()
	h := (&Server{Store: st, Hub: hub}).Handler()
	ids := make([]int64, 6)
	for i := range ids {
		id, err := st.CreateRecording(ctx, store.Recording{Title: "Show " + strconv.Itoa(i), GuideNumber: "4.1", Status: "complete", Path: src, StartedAt: time.Now()})
		if err != nil {
			t.Fatal(err)
		}
		ids[i] = id
	}
	codes := make([]int, len(ids))
	var wg sync.WaitGroup
	for i, id := range ids {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/media/poster/"+strconv.FormatInt(id, 10), nil))
			codes[i] = rec.Code
		}()
	}
	wg.Wait()
	for i, code := range codes {
		if code != http.StatusOK {
			t.Fatalf("poster %d: %d", i, code)
		}
	}
}

// A TV gets the poster full screen, and a game whose score is hidden gets
// one from before kickoff.
func TestAPosterAtFullWidthAndBeforeKickoff(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg is not installed")
	}
	st := testStore(t)
	dir := t.TempDir()
	hub := &live.Hub{Store: st, Dir: dir, FFmpeg: "ffmpeg"}
	if err := os.MkdirAll(hub.Recordings(), 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(hub.Recordings(), "game.ts")
	cmd := exec.Command("ffmpeg", "-hide_banner", "-loglevel", "error", "-f", "lavfi",
		"-i", "testsrc2=s=640x360:r=30:d=20,noise=alls=12:allf=t+u,format=yuv420p",
		"-c:v", "libx264", "-preset", "ultrafast", "-g", "30", "-pix_fmt", "yuv420p", "-f", "mpegts", path)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("sample: %v %s", err, out)
	}
	ctx := context.Background()
	started := time.Now().Add(-time.Hour)
	listed := started.Add(2 * time.Second)
	id, err := st.CreateRecording(ctx, store.Recording{Title: "Football", GuideNumber: "4.1", Status: "complete", Path: path,
		StartedAt: started, GameID: "nfl-1", Category: "Sports event", AiringStart: &listed})
	if err != nil {
		t.Fatal(err)
	}
	h := (&Server{Store: st, Hub: hub}).Handler()
	base := "/media/poster/" + strconv.FormatInt(id, 10)
	if res := get(t, h, base+"?w=1920"); res.Code != http.StatusOK {
		t.Fatalf("wide: %d", res.Code)
	}
	if _, err := os.Stat(live.StillPath(dir, id, 2, 1920)); err != nil {
		t.Fatalf("the wide poster is not the card's picture at 1920: %v", err)
	}
	if res := get(t, h, base+"?scores=hide"); res.Code != http.StatusOK {
		t.Fatalf("pregame: %d", res.Code)
	}
	// 5 s past the listed start, the only pregame time inside 20 s.
	if _, err := os.Stat(live.StillPath(dir, id, 7, 480)); err != nil {
		t.Fatalf("the pregame poster is not from the start: %v", err)
	}
	// The server's own setting hides it too; scores=show asks for the scene.
	if err := st.PutSettings(ctx, map[string]string{"hideScores": "1"}); err != nil {
		t.Fatal(err)
	}
	plain := get(t, h, base+"?scores=show").Body.Bytes()
	hidden := get(t, h, base).Body.Bytes()
	if bytes.Equal(plain, hidden) {
		t.Fatal("the server's hideScores did not pick the pregame poster")
	}
}

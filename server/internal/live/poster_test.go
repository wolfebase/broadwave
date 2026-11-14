package live

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"sync"
	"testing"
	"time"

	"broadwave/internal/store"
)

// flatThenScene writes an MPEG-TS that is one flat blue for flat seconds,
// like an ad's end card, then a moving test picture with grain, like a
// camera's, for scene seconds.
func flatThenScene(t *testing.T, path string, flat, scene int) {
	t.Helper()
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg is not installed")
	}
	graph := "color=c=0x1f4fa0:s=640x360:r=30:d=" + strconv.Itoa(flat) + ",format=yuv420p[a]"
	parts := "[a]"
	n := 1
	if scene > 0 {
		graph += ";testsrc2=s=640x360:r=30:d=" + strconv.Itoa(scene) + ",noise=alls=12:allf=t+u,format=yuv420p[b]"
		parts += "[b]"
		n = 2
	}
	graph += ";" + parts + "concat=n=" + strconv.Itoa(n) + ":v=1:a=0[out0]"
	cmd := exec.Command("ffmpeg", "-hide_banner", "-loglevel", "error", "-f", "lavfi", "-i", graph,
		"-c:v", "libx264", "-preset", "ultrafast", "-g", "30", "-pix_fmt", "yuv420p", "-f", "mpegts", path)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("sample: %v %s", err, out)
	}
}

func measured(length float64, breaks []store.Marker) func() (float64, []store.Marker) {
	return func() (float64, []store.Marker) { return length, breaks }
}

func TestPosterTimesStepOutOfBreaks(t *testing.T) {
	got := PosterTimes(1000, []store.Marker{{Start: 90, End: 200}, {Start: 590, End: 640}})
	if want := []float64{202, 250, 400, 642}; !reflect.DeepEqual(got, want) {
		t.Fatalf("times %v, want %v", got, want)
	}
	if got := PosterTimes(0, nil); !reflect.DeepEqual(got, posterTriesBlind) {
		t.Fatalf("unknown length: %v", got)
	}
}

func TestAPosterIsAPictureWhenTheRecordingStartsFlat(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "show.ts")
	flatThenScene(t, src, 7, 13)
	h := &Hub{Dir: dir, FFmpeg: "ffmpeg"}
	ctx := context.Background()
	// The sample is what the bug saw: the start is one colour.
	early, err := h.RecordingStill(ctx, 1, src, 2, 480)
	if err != nil {
		t.Fatal(err)
	}
	if score, err := pictureScore(early); err != nil || score >= flatPicture {
		t.Fatalf("the start should be flat: %.2f %v", score, err)
	}
	// A poster made the old way, 2 s in, is flat and is made again.
	if err := os.MkdirAll(filepath.Join(dir, "posters"), 0o755); err != nil {
		t.Fatal(err)
	}
	old := oldPosterPath(dir, 1)
	data, err := os.ReadFile(early)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(old, data, 0o644); err != nil {
		t.Fatal(err)
	}
	poster, err := h.RecordingPoster(ctx, 1, src, false, measured(20, nil))
	if err != nil {
		t.Fatal(err)
	}
	if poster != PosterPath(dir, 1) {
		t.Fatalf("kept at %s", poster)
	}
	if score, err := pictureScore(poster); err != nil || score < plainPicture {
		t.Fatalf("poster is not a picture: %.2f %v", score, err)
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Fatalf("the old flat poster is still there: %v", err)
	}
	// The look stopped at the first scene, 40% in.
	if _, err := os.Stat(StillPath(dir, 1, 12, 480)); !os.IsNotExist(err) {
		t.Fatalf("looked past a scene: %v", err)
	}
	// Made once: the next request reads the file, not the recording.
	again, err := h.RecordingPoster(ctx, 1, filepath.Join(dir, "gone.ts"), false, func() (float64, []store.Marker) {
		t.Fatal("measured a recording that has a poster")
		return 0, nil
	})
	if err != nil || again != poster {
		t.Fatalf("again: %q %v", again, err)
	}
}

func TestAFlatRecordingKeepsNoPosterAndIsLookedAtAgain(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "card.ts")
	flatThenScene(t, src, 10, 0)
	h := &Hub{Dir: dir, FFmpeg: "ffmpeg"}
	ctx := context.Background()
	if _, err := h.RecordingPoster(ctx, 2, src, false, measured(10, nil)); !errors.Is(err, ErrNoPoster) {
		t.Fatalf("flat recording: %v", err)
	}
	if _, err := os.Stat(PosterPath(dir, 2)); !os.IsNotExist(err) {
		t.Fatalf("a flat poster was kept: %v", err)
	}
	// Soon after, the answer is remembered and no ffmpeg runs.
	calls := 0
	count := func() (float64, []store.Marker) { calls++; return 10, nil }
	if _, err := h.RecordingPoster(ctx, 2, src, false, count); !errors.Is(err, ErrNoPoster) || calls != 0 {
		t.Fatalf("looked again at once: %v, %d", err, calls)
	}
	// Later it is looked at again: a file can be replaced or finish writing.
	h.posters.set(2, time.Now().Add(-posterRetry))
	if _, err := h.RecordingPoster(ctx, 2, src, false, count); !errors.Is(err, ErrNoPoster) || calls != 1 {
		t.Fatalf("never looked again: %v, %d", err, calls)
	}
}

func TestAGrowingRecordingKeepsNoPoster(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "live.ts")
	flatThenScene(t, src, 1, 19)
	h := &Hub{Dir: dir, FFmpeg: "ffmpeg"}
	ctx := context.Background()
	got, err := h.RecordingPoster(ctx, 3, src, true, measured(19, nil))
	if err != nil {
		t.Fatal(err)
	}
	if got == PosterPath(dir, 3) {
		t.Fatal("a recording still growing kept a poster")
	}
	if _, err := os.Stat(PosterPath(dir, 3)); !os.IsNotExist(err) {
		t.Fatalf("poster file: %v", err)
	}
	if _, err := h.RecordingPoster(ctx, 3, src, true, measured(4, nil)); !errors.Is(err, ErrNoPoster) {
		t.Fatalf("a few seconds in: %v", err)
	}
}

func TestFirstPosterRequestsAtOnceAllGetIt(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "show.ts")
	flatThenScene(t, src, 1, 9)
	h := &Hub{Dir: dir, FFmpeg: "ffmpeg"}
	var wg sync.WaitGroup
	errs := make([]error, 4)
	for i := range errs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, errs[i] = h.RecordingPoster(context.Background(), 4, src, false, measured(10, nil))
		}()
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("request %d: %v", i, err)
		}
	}
	left, _ := filepath.Glob(filepath.Join(dir, "posters", "*.part"))
	if len(left) > 0 {
		t.Fatalf("left behind: %v", left)
	}
}

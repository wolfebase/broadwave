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
	"sync/atomic"
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
	poster, err := h.RecordingPoster(ctx, 1, src, false, PosterOptions{}, measured(20, nil))
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
	again, err := h.RecordingPoster(ctx, 1, filepath.Join(dir, "gone.ts"), false, PosterOptions{}, func() (float64, []store.Marker) {
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
	if _, err := h.RecordingPoster(ctx, 2, src, false, PosterOptions{}, measured(10, nil)); !errors.Is(err, ErrNoPoster) {
		t.Fatalf("flat recording: %v", err)
	}
	if _, err := os.Stat(PosterPath(dir, 2)); !os.IsNotExist(err) {
		t.Fatalf("a flat poster was kept: %v", err)
	}
	// Soon after, the answer is remembered and no ffmpeg runs.
	calls := 0
	count := func() (float64, []store.Marker) { calls++; return 10, nil }
	if _, err := h.RecordingPoster(ctx, 2, src, false, PosterOptions{}, count); !errors.Is(err, ErrNoPoster) || calls != 0 {
		t.Fatalf("looked again at once: %v, %d", err, calls)
	}
	// Later it is looked at again: a file can be replaced or finish writing.
	h.posters.set(PosterPath(dir, 2), time.Now().Add(-posterRetry))
	if _, err := h.RecordingPoster(ctx, 2, src, false, PosterOptions{}, count); !errors.Is(err, ErrNoPoster) || calls != 1 {
		t.Fatalf("never looked again: %v, %d", err, calls)
	}
}

func TestAGrowingRecordingKeepsNoPoster(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "live.ts")
	flatThenScene(t, src, 1, 19)
	h := &Hub{Dir: dir, FFmpeg: "ffmpeg"}
	ctx := context.Background()
	got, err := h.RecordingPoster(ctx, 3, src, true, PosterOptions{}, measured(19, nil))
	if err != nil {
		t.Fatal(err)
	}
	if got == PosterPath(dir, 3) {
		t.Fatal("a recording still growing kept a poster")
	}
	if _, err := os.Stat(PosterPath(dir, 3)); !os.IsNotExist(err) {
		t.Fatalf("poster file: %v", err)
	}
	if _, err := h.RecordingPoster(ctx, 3, src, true, PosterOptions{}, measured(4, nil)); !errors.Is(err, ErrNoPoster) {
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
	var makings atomic.Int32
	count := func() (float64, []store.Marker) { makings.Add(1); return 10, nil }
	for i := range errs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, errs[i] = h.RecordingPoster(context.Background(), 4, src, false, PosterOptions{}, count)
		}()
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("request %d: %v", i, err)
		}
	}
	if n := makings.Load(); n != 1 {
		t.Fatalf("one poster was made %d times", n)
	}
	left, _ := filepath.Glob(filepath.Join(dir, "posters", "*.part"))
	if len(left) > 0 {
		t.Fatalf("left behind: %v", left)
	}
}

// A list asks for many posters at once and never asks again, so posters wait
// their turn for a still slot past the time a chapter still gives up.
func TestPostersWaitTheirTurnForASlot(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "show.ts")
	flatThenScene(t, src, 1, 9)
	h := &Hub{Dir: dir, FFmpeg: "ffmpeg"}
	ctx := context.Background()
	oldLimit := stillLimit
	stillLimit = 300 * time.Millisecond
	t.Cleanup(func() { stillLimit = oldLimit })
	// Something else holds both slots for longer than a still waits.
	h.stills.init()
	for range stillSlots {
		h.stills.slots <- struct{}{}
	}
	go func() {
		time.Sleep(time.Second)
		for range stillSlots {
			<-h.stills.slots
		}
	}()
	if _, err := h.RecordingStill(ctx, 99, src, 5, 480); !errors.Is(err, ErrStillBusy) {
		t.Fatalf("a chapter still waited: %v", err)
	}
	var wg sync.WaitGroup
	errs := make([]error, 6)
	for i := range errs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, errs[i] = h.RecordingPoster(ctx, int64(10+i), src, false, PosterOptions{}, measured(10, nil))
		}()
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("poster %d: %v", i, err)
		}
	}
	// Past its own limit a poster still gives up, and says the slots were busy.
	oldPoster := posterLimit
	posterLimit = 200 * time.Millisecond
	t.Cleanup(func() { posterLimit = oldPoster })
	for range stillSlots {
		h.stills.slots <- struct{}{}
	}
	defer func() {
		for range stillSlots {
			<-h.stills.slots
		}
	}()
	if _, err := h.RecordingPoster(ctx, 20, src, false, PosterOptions{}, measured(10, nil)); !errors.Is(err, ErrStillBusy) {
		t.Fatalf("waited past the limit: %v", err)
	}
}

// A TV asks for the poster full screen: the same picture as the card's, at
// that width, kept beside it and removed with the recording.
func TestAWidePosterIsTheSamePictureAtItsWidth(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "show.ts")
	flatThenScene(t, src, 7, 13)
	h := &Hub{Dir: dir, FFmpeg: "ffmpeg"}
	ctx := context.Background()
	card, err := h.RecordingPoster(ctx, 1, src, false, PosterOptions{}, measured(20, nil))
	if err != nil {
		t.Fatal(err)
	}
	wide, err := h.RecordingPoster(ctx, 1, src, false, PosterOptions{Width: 1920}, measured(20, nil))
	if err != nil {
		t.Fatal(err)
	}
	if wide == card || wide != posterPath(dir, 1, PosterOptions{Width: 2000}) {
		t.Fatalf("wide poster at %s", wide)
	}
	if got := jpegSize(t, wide); got != "1920,1080,1:1" {
		t.Fatalf("wide poster %s", got)
	}
	// The card's picture is the one at 8 s (40%), the first scene.
	if _, err := os.Stat(StillPath(dir, 1, 8, 1920)); err != nil {
		t.Fatalf("not the card's picture: %v", err)
	}
	removePosters(dir, 1)
	for _, path := range []string{card, wide} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("%s left: %v", path, err)
		}
	}
}

// A game whose score is hidden gets a poster from around its listed start,
// before kickoff: never later than two minutes past it, and none when the
// file starts after that.
func TestPregamePosterTimes(t *testing.T) {
	if got := PregameTimes(3600, 300, nil); !reflect.DeepEqual(got, []float64{330, 240, 390, 150, 305}) {
		t.Fatalf("times %v", got)
	}
	if got := PregameTimes(3600, 300, []store.Marker{{Start: 320, End: 430}}); !reflect.DeepEqual(got, []float64{240, 150, 305}) {
		t.Fatalf("a break past the cut-off: %v", got)
	}
	if got := PregameTimes(0, 0, nil); !reflect.DeepEqual(got, []float64{30, 90, 5}) {
		t.Fatalf("a file that starts on the listing: %v", got)
	}
	if got := PregameTimes(3600, -200, nil); len(got) != 0 {
		t.Fatalf("a file that starts after kickoff: %v", got)
	}
}

func TestAPregamePosterKeepsToTheStart(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "game.ts")
	flatThenScene(t, src, 1, 19)
	h := &Hub{Dir: dir, FFmpeg: "ffmpeg"}
	ctx := context.Background()
	got, err := h.RecordingPoster(ctx, 5, src, false, PosterOptions{Pregame: true, Lead: 2}, measured(20, nil))
	if err != nil || got != posterPath(dir, 5, PosterOptions{Pregame: true}) {
		t.Fatalf("pregame poster %q %v", got, err)
	}
	// The plain poster's first try, 10% in, is never looked at.
	if _, err := os.Stat(StillPath(dir, 5, 2, 480)); !os.IsNotExist(err) {
		t.Fatal("tried the plain poster's times")
	}
	if _, err := os.Stat(StillPath(dir, 5, 7, 480)); err != nil {
		t.Fatalf("not 5 s past the listed start: %v", err)
	}
	if _, err := h.RecordingPoster(ctx, 6, src, false, PosterOptions{Pregame: true, Lead: -300}, measured(20, nil)); !errors.Is(err, ErrNoPoster) {
		t.Fatalf("a file from after kickoff: %v", err)
	}
	// The plain poster of the same recording is not held back by it.
	if _, err := h.RecordingPoster(ctx, 6, src, false, PosterOptions{}, measured(20, nil)); err != nil {
		t.Fatal(err)
	}
}

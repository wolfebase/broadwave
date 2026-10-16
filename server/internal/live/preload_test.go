package live

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// AVPlayer turns on low latency only when the playlist names the part being
// written. The hint is always the part after the newest one.
func TestPlaylistHintsTheNextPart(t *testing.T) {
	dir := t.TempDir()
	pr, pw := io.Pipe()
	packErr := make(chan error, 1)
	go func() { packErr <- Pack(dir, pr, nil) }()
	if _, err := pw.Write(videoInit()); err != nil {
		t.Fatal(err)
	}
	if _, err := pw.Write(keyframeFragment(0, 90000)); err != nil {
		t.Fatal(err)
	}
	first := waitPlaylist(t, dir, "seg00000.m4s\n")
	if !strings.HasSuffix(first, "#EXT-X-PRELOAD-HINT:TYPE=PART,URI=\"part00001.m4s\"\n") {
		t.Fatalf("the playlist must end with the next part's hint:\n%s", first)
	}
	if _, err := pw.Write(keyframeFragment(90000, 20000)); err != nil {
		t.Fatal(err)
	}
	next := waitPlaylist(t, dir, "URI=\"part00001.m4s\"\n#EXT-X-PRELOAD")
	if !strings.HasSuffix(next, "#EXT-X-PRELOAD-HINT:TYPE=PART,URI=\"part00002.m4s\"\n") {
		t.Fatalf("the hint must move on with the parts:\n%s", next)
	}
	_ = pw.Close()
	<-packErr
}

func TestViewPlaylistRenamesTheHint(t *testing.T) {
	body := "#EXTM3U\n#EXT-X-PART:DURATION=1.001,URI=\"part00004.m4s\"\n#EXT-X-PRELOAD-HINT:TYPE=PART,URI=\"part00005.m4s\"\n"
	got := string(viewPlaylist([]byte(body), "v", nil))
	if !strings.Contains(got, "#EXT-X-PRELOAD-HINT:TYPE=PART,URI=\"part00005.v.m4s\"\n") {
		t.Fatalf("a view's hint must name the view's part:\n%s", got)
	}
}

func TestStripPreloadHint(t *testing.T) {
	body := "#EXTM3U\n#EXT-X-PART:DURATION=1.001,URI=\"part00004.m4s\"\n#EXT-X-PRELOAD-HINT:TYPE=PART,URI=\"part00005.m4s\"\n"
	got := string(StripPreloadHint([]byte(body)))
	if got != "#EXTM3U\n#EXT-X-PART:DURATION=1.001,URI=\"part00004.m4s\"\n" {
		t.Fatalf("got %q", got)
	}
	plain := []byte("#EXTM3U\nseg00000.m4s\n")
	if string(StripPreloadHint(plain)) != string(plain) {
		t.Fatal("a playlist without a hint must stay as it is")
	}
}

func TestPartNumber(t *testing.T) {
	for _, c := range []struct {
		name, own, prev string
		ok              bool
	}{
		{"part00012.m4s", "part00012.m4s", "part00011.m4s", true},
		{"part00012.v.m4s", "part00012.m4s", "part00011.m4s", true},
		{"part00012.a2.m4s", "part00012.m4s", "part00011.m4s", true},
		{"part00000.m4s", "part00000.m4s", "", true},
		{"part100000.m4s", "part100000.m4s", "part99999.m4s", true},
		{"seg00012.m4s", "", "", false},
		{"part12.m4s", "", "", false},
		{"part00012.m4s.tmp", "", "", false},
		{"part00012.V.m4s", "", "", false},
	} {
		_, own, prev, ok := PartNumber(c.name)
		if ok != c.ok || own != c.own || prev != c.prev {
			t.Errorf("%s: got %q %q %v", c.name, own, prev, ok)
		}
	}
}

func TestWaitPartHoldsOnlyTheNextPart(t *testing.T) {
	dir := t.TempDir()
	write := func(name string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("part00003.m4s")
	ctx := context.Background()
	go func() {
		time.Sleep(150 * time.Millisecond)
		write("part00004.m4s")
	}()
	start := time.Now()
	if !WaitPart(ctx, dir, "part00004.v.m4s", 3*time.Second) {
		t.Fatal("the hinted part was written and the wait did not see it")
	}
	if d := time.Since(start); d < 100*time.Millisecond || d > 2*time.Second {
		t.Fatalf("waited %v", d)
	}
	// A part two ahead, or one with no part before it, does not wait.
	start = time.Now()
	if WaitPart(ctx, dir, "part00006.m4s", 3*time.Second) || time.Since(start) > 100*time.Millisecond {
		t.Fatal("only the part after the newest one waits")
	}
	// A part whose file gave way to a pointer counts as written.
	write("part00005.ref")
	if !WaitPart(ctx, dir, "part00005.m4s", time.Second) {
		t.Fatal("a part kept as a pointer is there")
	}
	// The wait ends with the request and at the limit.
	cctx, cancel := context.WithTimeout(ctx, 80*time.Millisecond)
	defer cancel()
	if WaitPart(cctx, dir, "part00006.m4s", 3*time.Second) {
		t.Fatal("nothing wrote part00006")
	}
	start = time.Now()
	if WaitPart(ctx, dir, "part00006.m4s", 120*time.Millisecond) || time.Since(start) > time.Second {
		t.Fatal("the wait must stop at its limit")
	}
}

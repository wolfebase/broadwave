package live

import (
	"strings"
	"sync"
	"testing"
	"time"
)

// A restart's encode reuses part00000 and seg00000. A playlist request that
// read the old encode's playlist and its clock before the restart, and
// stamps after the reset, must not leave the old dates for the new files.
func TestAStampFromBeforeARestartLeavesNoOldDates(t *testing.T) {
	old := "#EXTM3U\n#EXT-X-VERSION:9\n#EXT-X-TARGETDURATION:2\n#EXT-X-MEDIA-SEQUENCE:0\n#EXT-X-MAP:URI=\"init.mp4\"\n" +
		"#EXT-X-PTS:part00000.m4s:900000\n#EXT-X-PART:DURATION=0.334,INDEPENDENT=YES,URI=\"part00000.m4s\"\n#EXT-X-PTS:seg00000.m4s:900000\n#EXTINF:1.001,\nseg00000.m4s\n" +
		"#EXT-X-PTS:part00003.m4s:990090\n#EXT-X-PART:DURATION=0.334,INDEPENDENT=YES,URI=\"part00003.m4s\"\n"
	// The new encode, eight seconds of broadcast later, with the same names.
	fresh := "#EXTM3U\n#EXT-X-VERSION:9\n#EXT-X-TARGETDURATION:2\n#EXT-X-MEDIA-SEQUENCE:0\n#EXT-X-MAP:URI=\"init.mp4\"\n" +
		"#EXT-X-PTS:part00000.m4s:1620000\n#EXT-X-PART:DURATION=0.334,INDEPENDENT=YES,URI=\"part00000.m4s\"\n#EXT-X-PTS:seg00000.m4s:1620000\n#EXTINF:1.001,\nseg00000.m4s\n"
	start := time.Date(2026, 11, 15, 20, 0, 0, 0, time.UTC)
	oldTL := NewTimeline()
	oldTL.now = func() time.Time { return start }
	newTL := NewTimeline()
	newTL.now = func() time.Time { return start.Add(8 * time.Second) }

	var st playlistStamper
	dir := t.TempDir()
	st.stamp(dir, []byte(old), oldTL)
	gen := st.generation()
	// The restart, then the request that read the old playlist before it.
	st.reset()
	st.stampAt(gen, dir, []byte(old), oldTL)
	got := string(st.stamp(dir, []byte(fresh), newTL))
	want := "#EXT-X-PROGRAM-DATE-TIME:" + start.Add(4*time.Second).Format("2006-01-02T15:04:05.000Z")
	if !strings.Contains(got, want) {
		t.Fatalf("the new encode's segment kept a date from before the restart, want %s:\n%s", want, got)
	}
}

// The same under -race: stamps from before the restart keep arriving while
// the new encode's playlist is stamped.
func TestStampsRacingARestart(t *testing.T) {
	src := "#EXTM3U\n#EXT-X-MEDIA-SEQUENCE:0\n#EXT-X-PTS:seg00000.m4s:900000\n#EXTINF:1.001,\nseg00000.m4s\n"
	var st playlistStamper
	dir := t.TempDir()
	oldTL, newTL := NewTimeline(), NewTimeline()
	newTL.now = func() time.Time { return time.Now().Add(time.Hour) }
	gen := st.generation()
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for range 200 {
				st.stampAt(gen, dir, []byte(src), oldTL)
			}
		})
	}
	st.reset()
	want := string(st.stampAt(st.generation(), dir, []byte(src), newTL))
	wg.Wait()
	if got := string(st.stamp(dir, []byte(src), newTL)); got != want {
		t.Fatalf("a stamp from before the restart changed the new dates:\n%s\n---\n%s", want, got)
	}
}

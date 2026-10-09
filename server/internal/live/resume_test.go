package live

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestResumeAtStaysInsideTheFile(t *testing.T) {
	if got := ResumeAt(2400, 3600); got != 2400 {
		t.Fatalf("deep resume %v", got)
	}
	if got := ResumeAt(1.5, 3600); got != 0 {
		t.Fatalf("near the start %v", got)
	}
	if got := ResumeAt(3599, 3600); got != 3598 {
		t.Fatalf("near the end %v", got)
	}
	// A saved spot past a short file (the contract sample is a few seconds
	// and the saved position is not) plays from the beginning.
	if got := ResumeAt(12.5, 3); got != 0 {
		t.Fatalf("past the end %v", got)
	}
	if got := ResumeAt(40, 0); got != 0 {
		t.Fatalf("unknown length %v", got)
	}
	if got := ResumeAt(40, -1); got != 0 {
		t.Fatalf("unprobed length %v", got)
	}
}

func TestInputSeekPrecedesTheFile(t *testing.T) {
	args := PictureArgs(Graph{Input: "show.ts", Start: 2400, VideoCodec: "MPEG2", Encoder: "libx264", Live: false})
	ss, in := -1, -1
	for i, arg := range args {
		if arg == "-ss" && ss < 0 {
			ss = i
		}
		if arg == "-i" && in < 0 {
			in = i
		}
	}
	if ss < 0 || in < 0 || ss > in {
		t.Fatalf("input seek: %s", strings.Join(args, " "))
	}
	if args[ss+1] != "2400.000" {
		t.Fatalf("seek %s", args[ss+1])
	}
	short := strings.Join(PictureArgs(Graph{Input: "show.ts", Start: 1, VideoCodec: "MPEG2", Encoder: "libx264", Live: false}), " ")
	if strings.Contains(short, "-ss") {
		t.Fatalf("a short resume plays from the start: %s", short)
	}
	pipe := strings.Join(PictureArgs(Graph{Input: "pipe:0", Start: 30, VideoCodec: "MPEG2", Encoder: "libx264"}), " ")
	if strings.Contains(pipe, "-ss") {
		t.Fatalf("a pipe cannot seek: %s", pipe)
	}
}

func TestOffsetPlaylistUsesTheRecordingClock(t *testing.T) {
	src := "#EXTM3U\n#EXT-X-VERSION:6\n#EXT-X-TARGETDURATION:2\n#EXT-X-MEDIA-SEQUENCE:0\n#EXT-X-PLAYLIST-TYPE:EVENT\n" +
		"#EXT-X-DISCONTINUITY\n#EXTINF:2.000,\nseg00000.ts\n#EXTINF:2.000,\nseg00001.ts\n"
	if string(OffsetPlaylist([]byte(src), 0)) != src {
		t.Fatal("a play from the start grew gaps")
	}
	got := string(OffsetPlaylist([]byte(src), 4))
	if !strings.Contains(got, "#EXT-X-VERSION:8\n") {
		t.Fatalf("version: %s", got)
	}
	if !strings.Contains(got, "#EXT-X-START:TIME-OFFSET=4.000,PRECISE=YES\n") {
		t.Fatalf("start: %s", got)
	}
	want := "#EXT-X-GAP\n#EXTINF:2.000,\ngap00000.ts\n#EXT-X-GAP\n#EXTINF:2.000,\ngap00001.ts\n"
	if !strings.Contains(got, want) {
		t.Fatalf("gaps: %s", got)
	}
	if at := mediaTime(got, "seg00000.ts"); at < 3.99 || at > 4.01 {
		t.Fatalf("first real segment at %v, want 4", at)
	}
	// The discontinuity ffmpeg writes in front of the first segment now sits
	// between the gap and that segment, which is where a discontinuity belongs.
	if strings.Index(got, "#EXT-X-DISCONTINUITY") < strings.Index(got, "gap00001.ts") {
		t.Fatalf("discontinuity still leads: %s", got)
	}
}

func mediaTime(playlist, name string) float64 {
	var sum float64
	var pending float64
	for _, line := range strings.Split(playlist, "\n") {
		trim := strings.TrimSpace(line)
		if rest, ok := strings.CutPrefix(trim, "#EXTINF:"); ok {
			sec, _ := strconv.ParseFloat(strings.SplitN(rest, ",", 2)[0], 64)
			pending += sec
			continue
		}
		if trim == "" || strings.HasPrefix(trim, "#") {
			continue
		}
		if trim == name {
			return sum
		}
		sum += pending
		pending = 0
	}
	return -1
}

func TestPlayFileSeeksAndReuses(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "show.ts")
	if err := os.WriteFile(path, mpeg2TS(1, true), 0o644); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(dir, "ffmpeg")
	argsPath := filepath.Join(dir, "args")
	script := "#!/bin/sh\ncase \" $* \" in\n*\" -f hls \"*)\nprintf '%s\\n' \"$@\" > " + argsPath + "\ncat > index.m3u8 << 'EOF'\n#EXTM3U\n#EXT-X-TARGETDURATION:2\n#EXTINF:2.000,\nseg00000.ts\nEOF\necho x > seg00000.ts\n;;\nesac\nexit 0\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	h := &Hub{Dir: dir, Encoder: "libx264", FFmpeg: bin}
	if _, err := h.PlayFile(7, path, "mpeg2video", "broadcast", "progressive", 40); err != nil {
		t.Fatal(err)
	}
	args, err := os.ReadFile(argsPath)
	if err != nil {
		t.Fatal(err)
	}
	if !inputSeek(string(args), "40.000") {
		t.Fatalf("args:\n%s", args)
	}
	if got := FileOffset(filepath.Join(dir, "file", "7")); got != 40 {
		t.Fatalf("offset %v", got)
	}
	if err := os.Remove(argsPath); err != nil {
		t.Fatal(err)
	}
	if _, err := h.PlayFile(7, path, "mpeg2video", "broadcast", "progressive", 40); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(argsPath); err == nil {
		t.Fatal("a second resume at the same point started another encode")
	}
	if _, err := h.PlayFile(7, path, "mpeg2video", "broadcast", "progressive", 0); err != nil {
		t.Fatal(err)
	}
	args, err = os.ReadFile(argsPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(args), "-ss") {
		t.Fatalf("start over sought: %s", args)
	}
	if got := FileOffset(filepath.Join(dir, "file", "7")); got != 0 {
		t.Fatalf("start over offset %v", got)
	}
}

func inputSeek(args, want string) bool {
	lines := strings.Split(args, "\n")
	for i, line := range lines {
		if line == "-ss" && i+1 < len(lines) && lines[i+1] == want {
			for _, later := range lines[i+2:] {
				if later == "-i" {
					return true
				}
			}
		}
	}
	return false
}

func TestPlayFileKeepsAFinishedEncode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "show.ts")
	if err := os.WriteFile(path, mpeg2TS(1, true), 0o644); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(dir, "ffmpeg")
	ran := filepath.Join(dir, "ran")
	script := "#!/bin/sh\necho ran > " + ran + "\nexit 1\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	h := &Hub{Dir: dir, Encoder: "libx264", FFmpeg: bin}
	play := filepath.Join(dir, "file", "3")
	if err := os.MkdirAll(play, 0o755); err != nil {
		t.Fatal(err)
	}
	g := h.fileGraphFor(path, "mpeg2video", "broadcast", "progressive")
	g.Live = false
	if err := os.WriteFile(filepath.Join(play, "graph.txt"), []byte(graphStamp(g)), 0o644); err != nil {
		t.Fatal(err)
	}
	body := "#EXTM3U\n#EXT-X-TARGETDURATION:2\n#EXTINF:2.000,\nseg00000.ts\n#EXT-X-ENDLIST\n"
	if err := os.WriteFile(filepath.Join(play, "index.m3u8"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(play, "offset.txt"), []byte("0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := h.PlayFile(3, path, "mpeg2video", "broadcast", "progressive", 40); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(ran); err == nil {
		t.Fatal("a finished encode from the start was started again")
	}
}

package live

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestKeepTracksSplitsAMultiAudioEncode(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not installed")
	}
	ffprobe, err := exec.LookPath("ffprobe")
	if err != nil {
		t.Skip("ffprobe not installed")
	}
	dir := t.TempDir()
	src := filepath.Join(dir, "src.ts")
	gen := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", "testsrc2=size=640x360:rate=30000/1001",
		"-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000",
		"-f", "lavfi", "-i", "sine=frequency=880:sample_rate=48000",
		"-map", "0", "-map", "1", "-map", "2", "-t", "8",
		"-c:v", "libx264", "-g", "30", "-c:a", "ac3", "-ac:a:0", "2", "-ac:a:1", "1",
		"-output_ts_offset", "95000", "-f", "mpegts", src)
	if out, err := gen.CombinedOutput(); err != nil {
		t.Fatalf("source: %v %s", err, out)
	}
	out := filepath.Join(dir, "packed")
	if err := os.MkdirAll(out, 0o755); err != nil {
		t.Fatal(err)
	}
	in, err := os.Open(src)
	if err != nil {
		t.Fatal(err)
	}
	defer in.Close()
	cmd := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "error", "-copyts", "-i", "pipe:0",
		"-map", "0:v:0", "-map", "0:a:0", "-map", "0:a:1", "-c", "copy",
		"-video_track_timescale", "90000", "-muxdelay", "0", "-muxpreload", "0",
		"-f", "mp4", "-movflags", "frag_keyframe+empty_moov+default_base_moof+delay_moov", "pipe:1")
	cmd.Stdin = in
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	packErr := Pack(out, stdout, nil)
	if err := cmd.Wait(); packErr != nil || err != nil {
		t.Fatalf("pack %v wait %v %s", packErr, err, stderr.String())
	}

	init, err := os.ReadFile(filepath.Join(out, "init.mp4"))
	if err != nil {
		t.Fatal(err)
	}
	video, _, ok := videoTrack(init)
	if !ok {
		t.Fatal("no video track")
	}
	var sound []uint32
	for id := range trackScales(init) {
		if id != video {
			sound = append(sound, id)
		}
	}
	slices.Sort(sound)
	if len(sound) != 2 {
		t.Fatalf("want two sound tracks, got %v", sound)
	}
	segs, err := filepath.Glob(filepath.Join(out, "seg*.m4s"))
	if err != nil || len(segs) < 3 {
		t.Fatalf("segments %v %v", segs, err)
	}
	parts, _ := filepath.Glob(filepath.Join(out, "part*.m4s"))
	slices.Sort(segs)

	views := []struct {
		name  string
		ids   []uint32
		kinds string
	}{
		{"v", []uint32{video}, "video"},
		{"a1", []uint32{sound[0]}, "audio:2"},
		{"a2", []uint32{sound[1]}, "audio:1"},
		{"main", []uint32{video, sound[0]}, "audio:2 video"},
	}
	for _, view := range views {
		t.Run(view.name, func(t *testing.T) {
			head, err := keepTracks(init, view.ids)
			if err != nil {
				t.Fatal(err)
			}
			if got := len(trackScales(head)); got != len(view.ids) {
				t.Fatalf("init keeps %d tracks", got)
			}
			whole := append([]byte{}, head...)
			var first, last int64 = -1, -1
			for _, name := range segs {
				seg, err := os.ReadFile(name)
				if err != nil {
					t.Fatal(err)
				}
				cut, err := keepTracksFrag(seg, view.ids)
				if err != nil {
					t.Fatalf("%s: %v", filepath.Base(name), err)
				}
				// The decode times run on from one segment to the next.
				for _, moof := range boxesOf(cut, "moof") {
					for _, traf := range boxesOf(moof, "traf") {
						r, ok := parseRun(traf)
						if !ok {
							t.Fatalf("%s: unreadable traf", filepath.Base(name))
						}
						if !slices.Contains(view.ids, r.id) {
							t.Fatalf("%s: kept track %d", filepath.Base(name), r.id)
						}
						if r.id != view.ids[0] {
							continue
						}
						if last >= 0 && r.decodeTime() != last {
							t.Fatalf("%s: track %d starts at %d, last ended at %d", filepath.Base(name), r.id, r.decodeTime(), last)
						}
						if first < 0 {
							first = r.decodeTime()
						}
						last = r.decodeTime()
						for _, s := range r.samples {
							last += r.sampleDur(s)
						}
					}
				}
				whole = append(whole, cut...)
			}
			if span := float64(last-first) / float64(trackScales(init)[view.ids[0]]); first < 0 || span < 5 {
				t.Fatalf("track %d covers %.2f s", view.ids[0], span)
			}
			for _, name := range parts {
				part, err := os.ReadFile(name)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := keepTracksFrag(part, view.ids); err != nil {
					t.Fatalf("%s: %v", filepath.Base(name), err)
				}
			}
			file := filepath.Join(dir, view.name+".mp4")
			if err := os.WriteFile(file, whole, 0o644); err != nil {
				t.Fatal(err)
			}
			probe, err := exec.Command(ffprobe, "-v", "error", "-show_entries", "stream=codec_type,channels",
				"-of", "csv=p=0", file).Output()
			if err != nil {
				t.Fatal(err)
			}
			var kinds []string
			for _, line := range strings.Fields(string(probe)) {
				kind, channels, _ := strings.Cut(strings.TrimSuffix(line, ","), ",")
				if kind == "audio" {
					kind += ":" + channels
				}
				kinds = append(kinds, kind)
			}
			slices.Sort(kinds)
			if got := strings.Join(kinds, " "); got != view.kinds {
				t.Fatalf("streams %q, want %q", got, view.kinds)
			}
			decode, err := exec.Command(ffmpeg, "-v", "error", "-i", file, "-f", "null", "-").CombinedOutput()
			if err != nil || len(bytes.TrimSpace(decode)) > 0 {
				t.Fatalf("decode errors: %v %s", err, decode)
			}
		})
	}
}

func TestKeepTracksRefusesWhatItCannotCut(t *testing.T) {
	if _, err := keepTracks(putBox("moov", putBox("mvhd", make([]byte, 100))), []uint32{1}); err == nil {
		t.Fatal("kept a track that is not there")
	}
	moof := putBox("moof", append(putBox("mfhd", make([]byte, 8)), putBox("traf", putBox("sbgp", make([]byte, 8)))...))
	if _, err := keepTracksFrag(append(moof, putBox("mdat", nil)...), []uint32{1}); err == nil {
		t.Fatal("cut a traf it cannot read")
	}
	if _, err := keepTracksFrag(moof, []uint32{1}); err == nil {
		t.Fatal("cut a moof with no mdat")
	}
}

func boxesOf(b []byte, kind string) [][]byte {
	var out [][]byte
	for _, x := range boxes(b) {
		if x.kind == kind {
			out = append(out, x.body)
		}
	}
	return out
}

package live

import (
	"bytes"
	"encoding/binary"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestExtrasFollowTheMainSound(t *testing.T) {
	extras := []AudioTrack{
		{PID: 0x102, Codec: "aac", Channels: 2, Measured: true},
		{PID: 0x103, Codec: "ac3", Channels: 6, Measured: true},
	}
	src := Source{VideoCodec: "H264", AudioCodec: "AC3", Progressive: true, Extras: extras}
	cases := []struct {
		name      string
		r         Rendition
		want, not []string
	}{
		{"copy maps each extra and fixes only the aac one", Rendition{Video: "copy", Audio: "copy"},
			[]string{"-map 0:a:0 -map 0:i:258 -map 0:i:259", "-c:a copy -bsf:a:1 aac_adtstoasc", "-max_interleave_delta 1000000"},
			[]string{"-bsf:a aac", "-bsf:a:0", "-bsf:a:2"}},
		{"surround keeps a stereo extra stereo", Rendition{Video: "copy", Audio: "aac6"},
			[]string{"-ac 6 -b:a 384k -ac:a:1 2 -b:a:1 160k", "-max_interleave_delta 1000000"},
			[]string{"-ac:a:2"}},
		{"stereo needs no per-track width", Rendition{Video: "copy", Audio: "aac2"},
			[]string{"-map 0:i:258 -map 0:i:259", "-ac 2 -b:a 160k -max_interleave_delta"},
			[]string{"-ac:a:"}},
		{"no sound maps no extras", Rendition{Video: "copy", Audio: "none"},
			[]string{"-an"},
			[]string{"0:i:258", "-max_interleave_delta"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := strings.Join(RenditionArgs(0, src, c.r, "libx264", ""), " ")
			for _, w := range c.want {
				if !strings.Contains(got, w) {
					t.Errorf("missing %q in %s", w, got)
				}
			}
			for _, n := range c.not {
				if strings.Contains(got, n) {
					t.Errorf("unexpected %q in %s", n, got)
				}
			}
		})
	}
	plain := src
	plain.Extras = nil
	for _, r := range []Rendition{{Video: "copy", Audio: "copy"}, {Video: "copy", Audio: "aac6"}} {
		got := strings.Join(RenditionArgs(0, plain, r, "libx264", ""), " ")
		if strings.Contains(got, "max_interleave") || strings.Contains(got, ":a:1") {
			t.Errorf("no extras changed the args: %s", got)
		}
	}
}

func TestAnEncodeCarriesTheExtras(t *testing.T) {
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
	// A 5.1 main, a stereo second language, and a mono description, on
	// fixed PIDs like a broadcast PMT.
	gen := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", "testsrc2=size=640x360:rate=30000/1001",
		"-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000",
		"-f", "lavfi", "-i", "sine=frequency=660:sample_rate=48000",
		"-f", "lavfi", "-i", "sine=frequency=880:sample_rate=48000",
		"-map", "0", "-map", "1", "-map", "2", "-map", "3", "-t", "6",
		"-c:v", "libx264", "-g", "30", "-c:a", "ac3", "-ac:a:0", "6", "-ac:a:1", "2", "-ac:a:2", "1",
		"-streamid", "0:0x100", "-streamid", "1:0x101", "-streamid", "2:0x102", "-streamid", "3:0x103",
		"-output_ts_offset", "95000", "-f", "mpegts", src)
	if out, err := gen.CombinedOutput(); err != nil {
		t.Fatalf("source: %v %s", err, out)
	}
	source := Source{VideoCodec: "H264", AudioCodec: "AC3", Progressive: true, Extras: []AudioTrack{
		{PID: 0x102, Codec: "ac3", Channels: 2, Measured: true},
		{PID: 0x103, Codec: "ac3", Channels: 1, Measured: true},
	}}
	cases := []struct {
		r        Rendition
		channels []int // main, then each extra
	}{
		{Rendition{Video: "copy", Audio: "copy"}, []int{6, 2, 1}},
		{Rendition{Video: "copy", Audio: "aac6"}, []int{6, 2, 1}},
		{Rendition{Video: "copy", Audio: "aac2"}, []int{2, 2, 2}},
	}
	for _, c := range cases {
		t.Run(c.r.Key(), func(t *testing.T) {
			out := filepath.Join(dir, c.r.Key())
			if err := os.MkdirAll(out, 0o755); err != nil {
				t.Fatal(err)
			}
			in, err := os.Open(src)
			if err != nil {
				t.Fatal(err)
			}
			defer in.Close()
			cmd := exec.Command(ffmpeg, RenditionArgs(0, source, c.r, "libx264", "")...)
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
			if video, _, ok := videoTrack(init); !ok || video != 1 {
				t.Fatalf("video track %d", video)
			}
			ids := []uint32{1, 2}
			for i := range source.Extras {
				ids = append(ids, extraTrackID(i))
			}
			var got []uint32
			for id := range trackScales(init) {
				got = append(got, id)
			}
			slices.Sort(got)
			if !slices.Equal(got, ids) {
				t.Fatalf("tracks %v, want %v", got, ids)
			}
			segs, _ := filepath.Glob(filepath.Join(out, "seg*.m4s"))
			slices.Sort(segs)
			if len(segs) < 3 {
				t.Fatalf("segments %v", segs)
			}
			// Past the first, every segment carries every track.
			for _, name := range segs[1:] {
				seg, err := os.ReadFile(name)
				if err != nil {
					t.Fatal(err)
				}
				have := map[uint32]bool{}
				for _, moof := range boxesOf(seg, "moof") {
					for _, traf := range boxesOf(moof, "traf") {
						if tfhd := child(traf, "tfhd"); len(tfhd) >= 8 {
							have[binary.BigEndian.Uint32(tfhd[4:8])] = true
						}
					}
				}
				if len(have) != len(ids) {
					t.Fatalf("%s carries tracks %v", filepath.Base(name), have)
				}
			}
			// Each sound track, cut on its own, is the width it should be.
			for i, id := range ids[1:] {
				head, err := keepTracks(init, []uint32{id})
				if err != nil {
					t.Fatal(err)
				}
				whole := head
				for _, name := range segs {
					seg, err := os.ReadFile(name)
					if err != nil {
						t.Fatal(err)
					}
					cut, err := keepTracksFrag(seg, []uint32{id})
					if err != nil {
						t.Fatal(err)
					}
					whole = append(whole, cut...)
				}
				file := filepath.Join(out, "track.mp4")
				if err := os.WriteFile(file, whole, 0o644); err != nil {
					t.Fatal(err)
				}
				probe, err := exec.Command(ffprobe, "-v", "error", "-show_entries", "stream=channels",
					"-of", "csv=p=0", file).Output()
				if err != nil {
					t.Fatal(err)
				}
				if got := strings.Trim(string(probe), " \n,"); got != string(rune('0'+c.channels[i])) {
					t.Fatalf("track %d has %s channels, want %d", id, got, c.channels[i])
				}
				decode, err := exec.Command(ffmpeg, "-v", "error", "-i", file, "-f", "null", "-").CombinedOutput()
				if err != nil || len(bytes.TrimSpace(decode)) > 0 {
					t.Fatalf("track %d decode: %v %s", id, err, decode)
				}
			}
		})
	}
}

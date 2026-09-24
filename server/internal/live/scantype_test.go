package live

import (
	"os/exec"
	"testing"
)

func TestScanTypeCapturedHeaders(t *testing.T) {
	prog := mpeg2TS(1, true)
	if order, ok := scanType(prog, 1); !ok || order != "progressive" {
		t.Fatalf("progressive sequence: got %q %v", order, ok)
	}
	lace := mpeg2TS(1, false)
	if order, ok := scanType(lace, 1); !ok || order != "tt" {
		t.Fatalf("interlaced sequence: got %q %v", order, ok)
	}
	// progressive_frame=1 alone is not enough: film sets it in an interlaced sequence.
	film := programTS(1, streamMPEG2, 0x100, []byte{0x00, 0x00, 0x01, 0xB5, 0x80, 0x00, 0x00, 0x00, 0x80})
	if _, ok := scanType(film, 1); ok {
		t.Fatal("progressive picture header must not decide the sequence")
	}
	if order, ok := scanType(h264TS(1, true), 1); !ok || order != "progressive" {
		t.Fatalf("h264 frames: got %q %v", order, ok)
	}
	if order, ok := scanType(filmTS(1), 1); !ok || order != "film" {
		t.Fatalf("soft 3:2: got %q %v", order, ok)
	}
	if order, ok := scanType(h264TS(1, false), 1); !ok || order != "tt" {
		t.Fatalf("h264 fields: got %q %v", order, ok)
	}
	if _, ok := scanType(prog, 99); ok {
		t.Fatal("wrong program should not match")
	}
}

func TestScanTypeFFmpegHeaders(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not on PATH")
	}
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"mpeg2 progressive", []string{"-c:v", "mpeg2video", "-b:v", "2M"}, "progressive"},
		{"mpeg2 interlaced", []string{"-c:v", "mpeg2video", "-b:v", "2M", "-flags", "+ilme+ildct", "-top", "1"}, "tt"},
		{"h264 frames", []string{"-c:v", "libx264", "-pix_fmt", "yuv420p"}, "progressive"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := t.TempDir() + "/s.ts"
			args := []string{"-hide_banner", "-loglevel", "error", "-f", "lavfi", "-i", "testsrc2=size=320x240:rate=30", "-t", "0.4", "-f", "mpegts"}
			args = append(args, tc.args...)
			args = append(args, path)
			if out, err := exec.Command("ffmpeg", args...).CombinedOutput(); err != nil {
				t.Fatalf("ffmpeg: %v %s", err, out)
			}
			raw, err := exec.Command("cat", path).Output()
			if err != nil {
				t.Fatal(err)
			}
			order, ok := scanType(raw, 1)
			if !ok || order != tc.want {
				t.Fatalf("got %q ok=%v, want %s", order, ok, tc.want)
			}
		})
	}
}

func mpeg2TS(program int, progressive bool) []byte {
	ext := []byte{0x00, 0x00, 0x01, 0xB5, 0x10, 0x00}
	if progressive {
		ext[5] = 0x08
	} else {
		// picture_coding_extension, progressive_frame clear (MSB of the 5th payload byte).
		pic := []byte{0x00, 0x00, 0x01, 0xB5, 0x80, 0x00, 0x00, 0x00, 0x00}
		ext = append(ext, pic...)
	}
	return programTS(program, streamMPEG2, 0x100, ext)
}

func filmTS(program int) []byte {
	// Interlaced sequence, then four progressive pictures with repeat_first_field on two of them.
	es := []byte{0x00, 0x00, 0x01, 0xB5, 0x10, 0x00}
	for i := 0; i < 4; i++ {
		pic := []byte{0x00, 0x00, 0x01, 0xB5, 0x80, 0x00, 0x00, 0x00, 0x80}
		if i%2 == 0 {
			pic[7] = 0x02
		}
		es = append(es, pic...)
	}
	return programTS(program, streamMPEG2, 0x100, es)
}

func h264TS(program int, frames bool) []byte {
	// Main profile SPS: frame_mbs_only_flag is the last bit written below.
	sps := []byte{0x4d, 0x00, 0x28, 0xfb, 0x00}
	if frames {
		sps[4] = 0x80
	}
	nal := append([]byte{0x00, 0x00, 0x01, 0x67}, sps...)
	return programTS(program, streamH264, 0x100, nal)
}

func programTS(program, streamType, videoPID int, es []byte) []byte {
	const pmtPID = 0x1000
	pat := psiSection(0x00, append([]byte{
		0x00, 0x01, 0xc1, 0x00, 0x00,
	}, progPID(program, pmtPID)...))
	pmt := psiSection(0x02, pmtBody(program, streamType, videoPID))
	pes := pesPacket(es)
	var out []byte
	out = append(out, tsPacket(0, true, pat)...)
	out = append(out, tsPacket(pmtPID, true, pmt)...)
	out = append(out, tsPacket(videoPID, true, pes)...)
	return out
}

func progPID(program, pid int) []byte {
	return []byte{
		byte(program >> 8), byte(program),
		0xe0 | byte(pid>>8), byte(pid),
	}
}

func pmtBody(program, streamType, videoPID int) []byte {
	body := []byte{
		byte(program >> 8), byte(program),
		0xc1, 0x00, 0x00,
		0xe0, 0x00, // PCR
		0xf0, 0x00, // program info length
		byte(streamType),
		0xe0 | byte(videoPID>>8), byte(videoPID),
		0xf0, 0x00,
	}
	return body
}

func psiSection(tableID byte, body []byte) []byte {
	// length covers body + CRC.
	n := len(body) + 4
	sec := []byte{tableID, 0xb0 | byte(n>>8), byte(n)}
	sec = append(sec, body...)
	sec = append(sec, 0, 0, 0, 0)
	pkt := []byte{0x00} // pointer
	return append(pkt, sec...)
}

func pesPacket(es []byte) []byte {
	pes := []byte{0x00, 0x00, 0x01, 0xe0, 0x00, 0x00, 0x80, 0x00, 0x00}
	return append(pes, es...)
}

func tsPacket(pid int, start bool, payload []byte) []byte {
	pkt := make([]byte, 188)
	pkt[0] = 0x47
	pkt[1] = byte(pid >> 8)
	if start {
		pkt[1] |= 0x40
	}
	pkt[2] = byte(pid)
	pkt[3] = 0x10
	if len(payload) > 184 {
		payload = payload[:184]
	}
	copy(pkt[4:], payload)
	return pkt
}

package live

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestMPEGCRCMatchesTheSpecVector(t *testing.T) {
	// CRC-32/MPEG-2 check value for the ASCII string "123456789".
	if got := mpegCRC([]byte("123456789")); got != 0x0376e6e7 {
		t.Fatalf("crc %08x", got)
	}
}

func TestProgramFilterDropsASiblingWithoutAPicture(t *testing.T) {
	raw := twoProgramTS(1, 0x1000, 0x110, 0x111, 2, 0x1001, 0x210)
	var buf bytes.Buffer
	w := newProgramPipe(&closeBuf{&buf}, 1)
	if _, err := w.Write(raw); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	got := pidsOf(buf.Bytes())
	for _, pid := range []int{0, 0x1000, 0x110, 0x111} {
		if !got[pid] {
			t.Fatalf("missing pid %d in %v", pid, got)
		}
	}
	if got[0x1001] || got[0x210] {
		t.Fatalf("sibling leaked: %v", got)
	}
	var listed []int
	for _, sec := range sections(buf.Bytes(), 0) {
		if len(sec) < 8 || sec[0] != 0 {
			continue
		}
		end := sectionEnd(sec)
		if end+4 > len(sec) {
			t.Fatalf("short pat %d", len(sec))
		}
		crc := mpegCRC(sec[:end])
		have := uint32(sec[end])<<24 | uint32(sec[end+1])<<16 | uint32(sec[end+2])<<8 | uint32(sec[end+3])
		if crc != have {
			t.Fatalf("pat crc %08x want %08x", have, crc)
		}
		for off := 8; off+4 <= end; off += 4 {
			prog := int(sec[off])<<8 | int(sec[off+1])
			if prog != 0 {
				listed = append(listed, prog)
			}
		}
	}
	if len(listed) != 1 || listed[0] != 1 {
		t.Fatalf("pat programs %v", listed)
	}
}

func TestProgramFilterLearnsAcrossWrites(t *testing.T) {
	raw := twoProgramTS(1, 0x1000, 0x110, 0x111, 2, 0x1001, 0x210)
	var buf bytes.Buffer
	w := newProgramPipe(&closeBuf{&buf}, 1)
	if _, err := w.Write(raw[:188]); err != nil {
		t.Fatal(err)
	}
	if buf.Len() != 0 {
		t.Fatal("emitted before the program map")
	}
	if _, err := w.Write(raw[188:]); err != nil {
		t.Fatal(err)
	}
	got := pidsOf(buf.Bytes())
	if !got[0x110] || !got[0x111] || got[0x210] {
		t.Fatalf("pids %v", got)
	}
}

func TestProgramFilterWaitsOutATableInterval(t *testing.T) {
	// More than a megabyte of null packets, then the map. A 256 KiB give-up
	// would pass the sibling through.
	pkt := tsPacket(0x1FFF, false, nil)
	var raw []byte
	for len(raw) < 1<<20+200000 {
		raw = append(raw, pkt...)
	}
	raw = append(raw, twoProgramTS(1, 0x1000, 0x110, 0x111, 2, 0x1001, 0x210)...)
	var buf bytes.Buffer
	w := newProgramPipe(&closeBuf{&buf}, 1)
	for len(raw) > 0 {
		n := 188 * 49
		if n > len(raw) {
			n = len(raw)
		}
		if _, err := w.Write(raw[:n]); err != nil {
			t.Fatal(err)
		}
		raw = raw[n:]
	}
	got := pidsOf(buf.Bytes())
	if !got[0x110] || !got[0x111] || got[0x210] || got[0x1001] {
		t.Fatalf("pids %v", got)
	}
}

func TestProgramFilterPassesThroughWhenTheProgramIsAbsent(t *testing.T) {
	raw := twoProgramTS(1, 0x1000, 0x110, 0x111, 2, 0x1001, 0x210)
	var buf bytes.Buffer
	w := newProgramPipe(&closeBuf{&buf}, 99)
	if _, err := w.Write(raw); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(buf.Bytes(), raw) {
		t.Fatalf("rewrote %d bytes of a multiplex without this program", buf.Len())
	}
}

func TestProgramFilterKeepsAudioSplitAcrossTheProgramMap(t *testing.T) {
	pat := psiPacket(0, psiSection(0x00, append([]byte{0x00, 0x01, 0xc1, 0x00, 0x00}, progPID(1, 0x1000)...)))
	first, second := splitProgramMap(1, 0x1000, 0x110, 0x111)
	video := tsPacket(0x110, true, pesPacket([]byte{0x00, 0x00, 0x01, 0xb3}))
	audio := tsPacket(0x111, true, []byte{0x00, 0x00, 0x01, 0xc0})
	sibling := tsPacket(0x210, true, bytes.Repeat([]byte{0xff}, 20))
	sibling = append(sibling, tsPacket(0x1fff, false, nil)...)
	sibling = append(sibling, tsPacket(0x1fff, false, nil)...)
	var buf bytes.Buffer
	w := newProgramPipe(&closeBuf{&buf}, 1)
	if _, err := w.Write(append(pat, first...)); err != nil {
		t.Fatal(err)
	}
	if buf.Len() != 0 {
		t.Fatal("closed the stream list on the first half of the program map")
	}
	rest := append(append(second, video...), audio...)
	rest = append(rest, sibling...)
	if _, err := w.Write(rest); err != nil {
		t.Fatal(err)
	}
	got := pidsOf(buf.Bytes())
	if !got[0x110] || !got[0x111] || got[0x210] {
		t.Fatalf("pids %v", got)
	}
}

func TestProgramFilterLocksSyncPastAPartialPacket(t *testing.T) {
	raw := append(bytes.Repeat([]byte{0x11}, 40), twoProgramTS(1, 0x1000, 0x110, 0x111, 2, 0x1001, 0x210)...)
	var buf bytes.Buffer
	w := newProgramPipe(&closeBuf{&buf}, 1)
	for len(raw) > 0 {
		n := 100
		if n > len(raw) {
			n = len(raw)
		}
		if _, err := w.Write(raw[:n]); err != nil {
			t.Fatal(err)
		}
		raw = raw[n:]
	}
	got := pidsOf(buf.Bytes())
	if !got[0x110] || !got[0x111] || got[0x210] {
		t.Fatalf("pids %v", got)
	}
}

func splitProgramMap(program, pmtPID, videoPID, audioPID int) (first, second []byte) {
	const infoLen = 166
	body := []byte{
		byte(program >> 8), byte(program),
		0xc1, 0x00, 0x00,
		0xe0 | byte(videoPID>>8), byte(videoPID),
		0xf0 | byte(infoLen>>8), byte(infoLen),
	}
	body = append(body, bytes.Repeat([]byte{0x00}, infoLen)...)
	body = append(body, streamMPEG2, 0xe0|byte(videoPID>>8), byte(videoPID), 0xf0, 0x00)
	body = append(body, streamAC3, 0xe0|byte(audioPID>>8), byte(audioPID), 0xf0, 0x00)
	sec := appendCRC(0x02, body)
	if len(sec) <= 183 {
		panic("program map fits in one packet")
	}
	pay1 := append([]byte{0x00}, sec[:183]...)
	first = tsPacket(pmtPID, true, pay1)
	second = tsPacket(pmtPID, false, sec[183:])
	return first, second
}

func TestProgramFilterPassesASingleStreamThrough(t *testing.T) {
	raw := []byte("not a transport stream")
	var buf bytes.Buffer
	w := newProgramPipe(&closeBuf{&buf}, 0)
	if _, err := w.Write(raw); err != nil {
		t.Fatal(err)
	}
	if buf.String() != string(raw) {
		t.Fatalf("passthrough %q", buf.String())
	}
}

func TestProgramFilterGivesUpWhenTheMapNeverArrives(t *testing.T) {
	raw := bytes.Repeat([]byte{0x47, 0x1f, 0xff, 0x10}, programFilterCap/4+188)
	var buf bytes.Buffer
	w := newProgramPipe(&closeBuf{&buf}, 7)
	if _, err := w.Write(raw); err != nil {
		t.Fatal(err)
	}
	if buf.Len() == 0 {
		t.Fatal("held the whole multiplex")
	}
	if !bytes.Equal(buf.Bytes(), raw) {
		t.Fatalf("fallback wrote %d of %d", buf.Len(), len(raw))
	}
}

func TestFilteredProgramProbesWithoutTheSibling(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip(err)
	}
	ffprobe, err := exec.LookPath("ffprobe")
	if err != nil {
		t.Skip(err)
	}
	dir := t.TempDir()
	src := filepath.Join(dir, "one.ts")
	cmd := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "error", "-y",
		"-f", "lavfi", "-i", "testsrc2=size=320x240:rate=30:duration=1",
		"-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000:duration=1",
		"-c:v", "mpeg2video", "-g", "15", "-b:v", "500k",
		"-c:a", "ac3", "-b:a", "96k",
		"-f", "mpegts", src)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("sample: %v %s", err, out)
	}
	raw, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	program := onlyProgram(t, raw)
	decoy := addSilentVideo(t, raw, program, 99)
	var filtered bytes.Buffer
	w := newProgramPipe(&closeBuf{&filtered}, program)
	if _, err := w.Write(decoy); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	full := probeNotes(t, ffprobe, decoy)
	if !strings.Contains(full, "Could not find codec parameters") && !strings.Contains(full, "unspecified size") {
		t.Fatalf("sibling did not hold the probe:\n%s", full)
	}
	narrow := probeNotes(t, ffprobe, filtered.Bytes())
	if strings.Contains(narrow, "Could not find codec parameters") || strings.Contains(narrow, "unspecified size") {
		t.Fatalf("filtered probe still waited:\n%s", narrow)
	}
	show := probeShow(t, ffprobe, filtered.Bytes())
	if !strings.Contains(show, "video") || !strings.Contains(show, "320") {
		t.Fatalf("picture %q notes %s", show, narrow)
	}
}

type closeBuf struct{ *bytes.Buffer }

func (c closeBuf) Close() error { return nil }

func twoProgramTS(a, aPMT, aVideo, aAudio, b, bPMT, bVideo int) []byte {
	pat := psiPacket(0, psiSection(0x00, append(append([]byte{0x00, 0x01, 0xc1, 0x00, 0x00},
		progPID(a, aPMT)...), progPID(b, bPMT)...)))
	pmtA := psiPacket(aPMT, psiSection(0x02, pmtWithAudio(a, aVideo, aAudio)))
	pmtB := psiPacket(bPMT, psiSection(0x02, pmtBody(b, streamMPEG2, bVideo)))
	video := tsPacket(aVideo, true, pesPacket([]byte{0x00, 0x00, 0x01, 0xb3, 0x50, 0x02, 0xd0}))
	audio := tsPacket(aAudio, true, []byte{0x00, 0x00, 0x01, 0xc0})
	other := tsPacket(bVideo, true, bytes.Repeat([]byte{0xff}, 20))
	var out []byte
	out = append(out, pat...)
	out = append(out, pmtA...)
	out = append(out, pmtB...)
	out = append(out, video...)
	out = append(out, audio...)
	out = append(out, other...)
	// A live stream keeps going; the filter holds its last two packets.
	out = append(out, tsPacket(0x1fff, false, nil)...)
	out = append(out, tsPacket(0x1fff, false, nil)...)
	return out
}

func pmtWithAudio(program, videoPID, audioPID int) []byte {
	body := pmtBody(program, streamMPEG2, videoPID)
	body = append(body, streamAC3, 0xe0|byte(audioPID>>8), byte(audioPID), 0xf0, 0x00)
	return body
}

func psiPacket(pid int, section []byte) []byte {
	// psiSection already includes the pointer byte.
	return tsPacket(pid, true, section)
}

func pidsOf(data []byte) map[int]bool {
	got := map[int]bool{}
	for off := 0; off+188 <= len(data); off += 188 {
		if data[off] != 0x47 {
			continue
		}
		pid := int(data[off+1]&0x1f)<<8 | int(data[off+2])
		got[pid] = true
	}
	return got
}

func onlyProgram(t *testing.T, data []byte) int {
	t.Helper()
	for _, sec := range sections(data, 0) {
		if len(sec) < 12 || sec[0] != 0 {
			continue
		}
		end := sectionEnd(sec)
		for off := 8; off+4 <= end; off += 4 {
			prog := int(sec[off])<<8 | int(sec[off+1])
			if prog != 0 {
				return prog
			}
		}
	}
	t.Fatal("no program")
	return 0
}

func addSilentVideo(t *testing.T, raw []byte, keep, extra int) []byte {
	t.Helper()
	const pmtPID = 0x1f00
	const videoPID = 0x1f10
	var tsID int
	var programs []byte
	for _, sec := range sections(raw, 0) {
		if len(sec) < 8 || sec[0] != 0 {
			continue
		}
		tsID = int(sec[3])<<8 | int(sec[4])
		end := sectionEnd(sec)
		if end > 8 {
			programs = append([]byte(nil), sec[8:end]...)
		}
		break
	}
	if len(programs) == 0 {
		t.Fatal("pat")
	}
	programs = append(programs, progPID(extra, pmtPID)...)
	body := []byte{byte(tsID >> 8), byte(tsID), 0xc1, 0x00, 0x00}
	body = append(body, programs...)
	patSec := appendCRC(0x00, body)
	pat := tsPacket(0, true, append([]byte{0x00}, patSec...))
	pmtSec := appendCRC(0x02, pmtBody(extra, streamMPEG2, videoPID))
	pmt := tsPacket(pmtPID, true, append([]byte{0x00}, pmtSec...))
	junk := tsPacket(videoPID, true, bytes.Repeat([]byte{0x11}, 40))
	var out []byte
	replaced := false
	for off := 0; off+188 <= len(raw); off += 188 {
		pkt := raw[off : off+188]
		pid := int(pkt[1]&0x1f)<<8 | int(pkt[2])
		if pid == 0 && !replaced {
			out = append(out, pat...)
			out = append(out, pmt...)
			for range 30 {
				out = append(out, junk...)
			}
			replaced = true
			continue
		}
		if pid == 0 {
			out = append(out, pat...)
			continue
		}
		out = append(out, pkt...)
	}
	if !replaced {
		t.Fatal("no pat packet")
	}
	if keep == 0 {
		t.Fatal("program")
	}
	return out
}

func appendCRC(tableID byte, body []byte) []byte {
	n := len(body) + 4
	sec := []byte{tableID, 0xb0 | byte(n>>8), byte(n)}
	sec = append(sec, body...)
	crc := mpegCRC(sec)
	return append(sec, byte(crc>>24), byte(crc>>16), byte(crc>>8), byte(crc))
}

func probeNotes(t *testing.T, ffprobe string, data []byte) string {
	t.Helper()
	cmd := exec.Command(ffprobe, "-v", "warning", "-probesize", "8000000", "-analyzeduration", "1000000", "-i", "pipe:0")
	cmd.Stdin = bytes.NewReader(data)
	out, _ := cmd.CombinedOutput()
	return string(out)
}

func probeShow(t *testing.T, ffprobe string, data []byte) string {
	t.Helper()
	cmd := exec.Command(ffprobe, "-v", "error", "-probesize", "8000000", "-analyzeduration", "1000000",
		"-show_entries", "stream=codec_type,width,height", "-of", "csv=p=0", "-i", "pipe:0")
	cmd.Stdin = bytes.NewReader(data)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("ffprobe %v %s", err, out)
	}
	return string(out)
}

// busyMux is a two-program multiplex whose payload bytes look like packet
// headers for the kept video PID, so any packet read at the wrong phase
// passes the PID check and reaches the decoder as garbage.
func busyMux(packets int) (raw []byte, sent map[string]bool) {
	raw = twoProgramTS(1, 0x1000, 0x110, 0x111, 2, 0x1001, 0x210)
	// Periods of 5 and 7 bytes, so a lookalike header does not repeat at
	// the packet size the way a real one does.
	lookalike := bytes.Repeat([]byte{0x47, 0x01, 0x10, 0x10, 0x00}, 37)
	sibLike := bytes.Repeat([]byte{0x47, 0x01, 0x10, 0x10, 0x00, 0x47, 0x02}, 26)
	sent = map[string]bool{}
	for i := range packets {
		var pkt []byte
		switch i % 3 {
		case 0:
			body := append([]byte{byte(i >> 8), byte(i)}, lookalike...)
			pkt = tsPacket(0x110, false, body)
		case 1:
			pkt = tsPacket(0x111, false, []byte{byte(i >> 8), byte(i)})
		default:
			pkt = tsPacket(0x210, false, sibLike)
		}
		sent[string(pkt)] = true
		raw = append(raw, pkt...)
	}
	for off := 0; off+188 <= len(raw) && off < 188*6; off += 188 {
		sent[string(raw[off:off+188])] = true
	}
	return raw, sent
}

func checkPackets(t *testing.T, out []byte, sent map[string]bool) (video int) {
	t.Helper()
	if len(out)%188 != 0 {
		t.Fatalf("output is %d bytes, not whole packets", len(out))
	}
	for off := 0; off < len(out); off += 188 {
		pkt := out[off : off+188]
		pid := int(pkt[1]&0x1f)<<8 | int(pkt[2])
		if pid == 0 {
			continue
		}
		if !sent[string(pkt)] {
			t.Fatalf("packet %d (pid %#x) was never sent: garbage reached the encode", off/188, pid)
		}
		if pid == 0x210 {
			t.Fatalf("sibling packet %d leaked", off/188)
		}
		if pid == 0x110 {
			video++
		}
	}
	return video
}

func TestProgramFilterRecoversFromALostChunk(t *testing.T) {
	raw, sent := busyMux(3000)
	var buf bytes.Buffer
	w := newProgramPipe(&closeBuf{&buf}, 1)
	// A slow encode loses whole reads, and reads are not packet sized.
	for i, off := 0, 0; off < len(raw); i++ {
		n := min(1000, len(raw)-off)
		if i != 200 {
			if _, err := w.Write(raw[off : off+n]); err != nil {
				t.Fatal(err)
			}
		}
		off += n
	}
	video := checkPackets(t, buf.Bytes(), sent)
	// 1000 packets of video; the lost read holds about 5 packets of it.
	if video < 990 {
		t.Fatalf("only %d of 1000 video packets after the gap", video)
	}
}

func TestProgramFilterIgnoresASyncByteInsideAPayload(t *testing.T) {
	raw, sent := busyMux(600)
	// An encode that attaches mid-stream starts inside a video payload full
	// of 0x47 bytes.
	join := raw[188*6+7:]
	var buf bytes.Buffer
	w := newProgramPipe(&closeBuf{&buf}, 1)
	for off := 0; off < len(join); off += 1000 {
		if _, err := w.Write(join[off:min(off+1000, len(join))]); err != nil {
			t.Fatal(err)
		}
	}
	// No table after the join, so the head of the stream carries none; the
	// filter must still not lock onto a payload byte. Feed the tables again.
	if _, err := w.Write(raw); err != nil {
		t.Fatal(err)
	}
	if got := pidsOf(buf.Bytes()); got[0x210] {
		t.Fatalf("passed the whole multiplex through: %v", got)
	}
	if video := checkPackets(t, buf.Bytes(), sent); video < 200 {
		t.Fatalf("only %d video packets", video)
	}
}

func TestProgramFilterFollowsANewProgramMap(t *testing.T) {
	raw := twoProgramTS(1, 0x1000, 0x110, 0x111, 2, 0x1001, 0x210)
	// The station moves its audio to a new PID mid-stream.
	moved := psiPacket(0x1000, psiSection(0x02, pmtWithAudio(1, 0x110, 0x121)))
	moved[4+1+5] = 0xc3 // version 1
	raw = append(raw, moved...)
	raw = append(raw, tsPacket(0x121, true, []byte{0x00, 0x00, 0x01, 0xc0})...)
	raw = append(raw, tsPacket(0x110, false, nil)...)
	raw = append(raw, tsPacket(0x1fff, false, nil)...)
	raw = append(raw, tsPacket(0x1fff, false, nil)...)
	var buf bytes.Buffer
	w := newProgramPipe(&closeBuf{&buf}, 1)
	for off := 0; off < len(raw); off += 188 {
		if _, err := w.Write(raw[off : off+188]); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if got := pidsOf(buf.Bytes()); !got[0x121] {
		t.Fatalf("new audio pid dropped: %v", got)
	}
}

func TestProgramFilterStartsAtASequenceHeader(t *testing.T) {
	raw := twoProgramTS(1, 0x1000, 0x110, 0x111, 2, 0x1001, 0x210)
	// Join mid picture group: a picture without its sequence header first.
	join := append([]byte(nil), raw[:188*2]...)
	join = append(join, tsPacket(0x110, true, pesPacket([]byte{0x00, 0x00, 0x01, 0x00, 0x11}))...)
	join = append(join, tsPacket(0x111, true, []byte{0x00, 0x00, 0x01, 0xc0})...)
	join = append(join, raw[188*2:]...)
	var buf bytes.Buffer
	w := newProgramPipe(&closeBuf{&buf}, 1)
	if _, err := w.Write(join); err != nil {
		t.Fatal(err)
	}
	out := buf.Bytes()
	var pids []int
	for off := 0; off+188 <= len(out); off += 188 {
		pids = append(pids, int(out[off+1]&0x1f)<<8|int(out[off+2]))
	}
	if len(pids) < 3 || pids[0] != 0 || pids[1] != 0x1000 || pids[2] != 0x110 {
		t.Fatalf("encode must start with the tables and then the sequence header: %v", pids)
	}
	if !sequenceStart(tsPayload(out[188*2:188*3]), streamMPEG2) {
		t.Fatal("first video packet is not the sequence header")
	}
}

// An ATSC 3.0 tune is one program with no stored number. An encode that
// joins it mid-group starts on the parameter sets, behind the tables.
func TestTheFirstProgramStartsAtItsParameterSets(t *testing.T) {
	var join []byte
	join = append(join, psiPacket(0, psiSection(0x00, append([]byte{0x00, 0x01, 0xc1, 0x00, 0x00}, progPID(7, 0x100)...)))...)
	join = append(join, psiPacket(0x100, psiSection(0x02, pmtBody(7, streamHEVC, 0x101)))...)
	// A trailing picture (type 1), then a VPS (type 32) at the next random access point.
	join = append(join, tsPacket(0x101, true, pesPacket([]byte{0x00, 0x00, 0x01, 0x02, 0x01}))...)
	join = append(join, tsPacket(0x101, true, pesPacket([]byte{0x00, 0x00, 0x01, 0x40, 0x01}))...)
	join = append(join, tsPacket(0x1fff, false, nil)...)
	join = append(join, tsPacket(0x1fff, false, nil)...)
	var buf bytes.Buffer
	w := newProgramPipe(&closeBuf{&buf}, anyProgram)
	if _, err := w.Write(join); err != nil {
		t.Fatal(err)
	}
	out := buf.Bytes()
	var pids []int
	for off := 0; off+188 <= len(out); off += 188 {
		pids = append(pids, int(out[off+1]&0x1f)<<8|int(out[off+2]))
	}
	if len(pids) < 3 || pids[0] != 0 || pids[1] != 0x100 || pids[2] != 0x101 {
		t.Fatalf("encode must start with the tables and then the parameter sets: %v", pids)
	}
	if !sequenceStart(tsPayload(out[188*2:188*3]), streamHEVC) {
		t.Fatal("first video packet is not the parameter sets")
	}
	if got := patPMT(tsPayload(out[:188])[1:], 7); got != 0x100 {
		t.Fatalf("the table names program 7 at PID %#x, got %#x", 0x100, got)
	}
}

func ptsPES(stream byte, pts int64) []byte {
	return []byte{0x00, 0x00, 0x01, stream, 0x00, 0x00, 0x80, 0x80, 0x05,
		0x21 | byte(pts>>29)&0x0e, byte(pts >> 22), byte(pts>>14) | 1, byte(pts >> 7), byte(pts<<1) | 1}
}

// audioRun is one audio PES per step, each a start packet and one more.
func audioRun(pid int, pts []int64) []byte {
	var out []byte
	cc := byte(1)
	for _, t := range pts {
		for _, pkt := range [][]byte{tsPacket(pid, true, ptsPES(0xc0, t)), tsPacket(pid, false, bytes.Repeat([]byte{0xaa}, 184))} {
			pkt[3] = 0x10 | cc&0x0f
			cc++
			out = append(out, pkt...)
		}
	}
	return out
}

func filterRun(t *testing.T, pts []int64) (kept []int64, ccs []byte) {
	t.Helper()
	return filterRunHook(t, pts, nil)
}

func filterRunHook(t *testing.T, pts []int64, onBreak func()) (kept []int64, ccs []byte) {
	t.Helper()
	head := twoProgramTS(1, 0x1000, 0x110, 0x111, 2, 0x1001, 0x210)
	head = head[:len(head)-2*188]
	var out bytes.Buffer
	w := newProgramPipe(&closeBuf{&out}, 1)
	w.(*programPipe).onBreak = onBreak
	body := audioRun(0x111, pts)
	// The first audio packet in the header run has no timestamp.
	if _, err := w.Write(append(append(head, body...), bytes.Repeat(tsPacket(0x1fff, false, nil), 2)...)); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	data := out.Bytes()
	for off := 0; off+188 <= len(data); off += 188 {
		pkt := data[off : off+188]
		// The header run's own audio packet has no timestamp and no counter to keep.
		if int(pkt[1]&0x1f)<<8|int(pkt[2]) != 0x111 || pkt[3]&0x0f == 0 && len(ccs) == 0 {
			continue
		}
		ccs = append(ccs, pkt[3]&0x0f)
		if ts, ok := pesTime(tsPayload(pkt)); ok && pkt[1]&0x40 != 0 {
			kept = append(kept, ts)
		}
	}
	return kept, ccs
}

func TestProgramFilterDropsAFrameOffItsTimeline(t *testing.T) {
	step := int64(2880)
	base := int64(301_000_000)
	var pts []int64
	for i := range 10 {
		pts = append(pts, base+int64(i)*step)
	}
	pts[5] = base + 9000*90000
	kept, ccs := filterRun(t, pts)
	for _, ts := range kept {
		if ts == pts[5] {
			t.Fatalf("a frame 9000 s off reached ffmpeg: %v", kept)
		}
	}
	if len(kept) != 9 {
		t.Fatalf("kept %d frames, want 9: %v", len(kept), kept)
	}
	// ffmpeg flags a counter gap as corruption and drops the next good frame too.
	for i := 1; i < len(ccs); i++ {
		if ccs[i] != (ccs[i-1]+1)&0x0f {
			t.Fatalf("continuity gap at %d: %v", i, ccs)
		}
	}
}

// An adaptation-only packet repeats the previous counter and does not advance
// it. After a dropped frame the payload counters are rewritten, and this one
// has to move with them. ffmpeg marks the open frame corrupt when it does not.
func TestProgramFilterKeepsContinuityPastAnAdaptationOnlyPacket(t *testing.T) {
	step := int64(2880)
	base := int64(301_000_000)
	pts := []int64{base, base + step, base + 9000*90000, base + 3*step}
	head := twoProgramTS(1, 0x1000, 0x110, 0x111, 2, 0x1001, 0x210)
	head = head[:len(head)-2*188]
	var body []byte
	cc := byte(1)
	for i, ts := range pts {
		for _, pkt := range [][]byte{
			tsPacket(0x111, true, ptsPES(0xc0, ts)),
			tsPacket(0x111, false, bytes.Repeat([]byte{0xaa}, 184)),
		} {
			pkt[3] = 0x10 | cc&0x0f
			cc++
			body = append(body, pkt...)
		}
		// Adaptation field only: the counter repeats the previous packet.
		if i == 0 || i == 2 {
			clock := tsPacket(0x111, false, nil)
			clock[3] = 0x20 | (cc-1)&0x0f
			clock[4] = 183
			body = append(body, clock...)
		}
	}
	var out bytes.Buffer
	w := newProgramPipe(&closeBuf{&out}, 1)
	if _, err := w.Write(append(append(head, body...), bytes.Repeat(tsPacket(0x1fff, false, nil), 2)...)); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	var kept []int64
	var ccs []byte
	var payload []bool
	clocks := 0
	data := out.Bytes()
	for off := 0; off+188 <= len(data); off += 188 {
		pkt := data[off : off+188]
		if int(pkt[1]&0x1f)<<8|int(pkt[2]) != 0x111 {
			continue
		}
		ccs = append(ccs, pkt[3]&0x0f)
		has := pkt[3]&0x10 != 0
		payload = append(payload, has)
		if !has {
			clocks++
		}
		if ts, ok := pesTime(tsPayload(pkt)); ok && pkt[1]&0x40 != 0 {
			kept = append(kept, ts)
		}
	}
	if clocks != 2 {
		t.Fatalf("adaptation-only packets %d, want 2", clocks)
	}
	for _, ts := range kept {
		if ts == pts[2] {
			t.Fatalf("a frame 9000 s off reached ffmpeg: %v", kept)
		}
	}
	if len(kept) != 3 {
		t.Fatalf("kept %d frames, want 3: %v", len(kept), kept)
	}
	// A payload counter is the previous plus one. An adaptation-only counter
	// repeats the previous one. ffmpeg drops the open frame when it does not.
	for i := 1; i < len(ccs); i++ {
		want := ccs[i-1]
		if payload[i] {
			want = (want + 1) & 0x0f
		}
		if ccs[i] != want {
			t.Fatalf("continuity at %d: got %d want %d payload %v ccs %v", i, ccs[i], want, payload, ccs)
		}
	}
}

func TestProgramFilterFollowsARealTimelineBreak(t *testing.T) {
	step := int64(2880)
	var pts []int64
	for i := range 5 {
		pts = append(pts, 100_000+int64(i)*step)
	}
	// A splice: the station's clock moves 100 s and stays there.
	for i := range 6 {
		pts = append(pts, 9_100_000+int64(i)*step)
	}
	kept, _ := filterRun(t, pts)
	if len(kept) != 5+6-(pesAgree-1) {
		t.Fatalf("kept %d frames: %v", len(kept), kept)
	}
	if kept[len(kept)-1] != pts[len(pts)-1] {
		t.Fatalf("the new timeline was not followed: %v", kept)
	}
	// Across the 33-bit wrap is not a jump.
	wrap := []int64{1<<33 - 2*step, 1<<33 - step, 0, step}
	if kept, _ := filterRun(t, wrap); len(kept) != 4 {
		t.Fatalf("wrap dropped frames: %v", kept)
	}
}

// videoRun is one picture PES per step, each opening with a sequence header
// so a new encode can start on any of them.
func videoRun(pid int, pts []int64, cc *byte) []byte {
	var out []byte
	for _, t := range pts {
		pes := append(ptsPES(0xe0, t), 0x00, 0x00, 0x01, 0xb3, 0x50, 0x02, 0xd0)
		for _, pkt := range [][]byte{tsPacket(pid, true, pes), tsPacket(pid, false, bytes.Repeat([]byte{0xaa}, 184))} {
			pkt[3] = 0x10 | *cc&0x0f
			*cc++
			out = append(out, pkt...)
		}
	}
	return out
}

func videoTimes(data []byte, pid int) []int64 {
	var got []int64
	for off := 0; off+188 <= len(data); off += 188 {
		pkt := data[off : off+188]
		if int(pkt[1]&0x1f)<<8|int(pkt[2]) != pid || pkt[1]&0x40 == 0 {
			continue
		}
		if ts, ok := pesTime(tsPayload(pkt)); ok {
			got = append(got, ts)
		}
	}
	return got
}

func TestProgramFilterHandsABackwardsBreakToTheNextEncode(t *testing.T) {
	step := int64(1500)
	run := func(jump int64) (old, next []int64, breaks int) {
		head := twoProgramTS(1, 0x1000, 0x110, 0x111, 2, 0x1001, 0x210)
		tables := head[:3*188]
		var a, b bytes.Buffer
		w := newProgramPipe(&closeBuf{&a}, 1)
		p := w.(*programPipe)
		p.sw = &pipeSwitch{}
		p.onBreak = func() { breaks++ }
		var pts []int64
		for i := range 5 {
			pts = append(pts, 1_000_000+int64(i)*step)
		}
		var after []int64
		for i := range 6 {
			after = append(after, 1_000_000+4*step+jump+int64(i)*step)
		}
		cc := byte(1)
		body := append(videoRun(0x110, pts, &cc), videoRun(0x110, after, &cc)...)
		if _, err := w.Write(append(head[:len(head)-2*188], body...)); err != nil {
			t.Fatal(err)
		}
		if breaks > 0 {
			p.sw.give(&closeBuf{&b})
		}
		more := []int64{after[5] + step, after[5] + 2*step}
		if _, err := w.Write(append(append(tables, videoRun(0x110, more, &cc)...), bytes.Repeat(tsPacket(0x1fff, false, nil), 2)...)); err != nil {
			t.Fatal(err)
		}
		return videoTimes(a.Bytes(), 0x110), videoTimes(b.Bytes(), 0x110), breaks
	}
	// A loop or splice two seconds back: ffmpeg would clamp the copied track.
	old, next, breaks := run(-2 * 90000)
	if breaks != 1 {
		t.Fatalf("breaks %d, want 1", breaks)
	}
	// Nothing from the new timeline reaches the old encode, and the next one
	// starts on its first picture.
	if len(old) != 5 || old[4] != 1_000_000+4*step {
		t.Fatalf("old encode got %v", old)
	}
	if len(next) != 8 || next[0] != 1_000_000+4*step-2*90000 {
		t.Fatalf("next encode got %v", next)
	}
	// Forward breaks are followed; ffmpeg does not clamp those.
	if old, _, breaks := run(100 * 90000); breaks != 0 || len(old) != 13-(pesAgree-1) {
		t.Fatalf("forward: breaks %d kept %v", breaks, old)
	}
	// A reordered picture is not a break.
	if old, _, breaks := run(-5 * step); breaks != 0 || len(old) != 13 {
		t.Fatalf("reorder: breaks %d kept %v", breaks, old)
	}
}

// A splice moves the picture first. Sound that is still on the old clock
// must not become the next encode's audio baseline: that encode copies
// timestamps, and ffmpeg then clamps every later audio packet to the high one.
func TestProgramFilterDropsOldClockAudioAfterAVideoBreak(t *testing.T) {
	step := int64(1500)
	base := int64(1_000_000)
	// Farther back than pesBack, so it is a real splice and not a reorder.
	// The audio lead over the new picture is this plus one step.
	back := int64(pesBack) + 90000
	head := twoProgramTS(1, 0x1000, 0x110, 0x111, 2, 0x1001, 0x210)
	tables := head[:3*188]

	var stream []byte
	var cc byte = 1
	for i := range 5 {
		ts := base + int64(i)*step
		stream = append(stream, videoRun(0x110, []int64{ts}, &cc)...)
		stream = append(stream, audioRun(0x111, []int64{ts})...)
	}
	var fresh []int64
	for i := range 4 {
		v := base + 4*step - back + int64(i)*step
		// Picture has stepped back; this sound is still on the old clock.
		stream = append(stream, videoRun(0x110, []int64{v}, &cc)...)
		stream = append(stream, audioRun(0x111, []int64{base + int64(5+i)*step})...)
	}
	for i := range 4 {
		v := base + 4*step - back + int64(4+i)*step
		fresh = append(fresh, v)
		stream = append(stream, videoRun(0x110, []int64{v}, &cc)...)
		stream = append(stream, audioRun(0x111, []int64{v})...)
	}

	var breaks int
	var old, next bytes.Buffer
	w := newProgramPipe(&closeBuf{&old}, 1)
	p := w.(*programPipe)
	p.sw = &pipeSwitch{}
	p.onBreak = func() { breaks++ }
	if _, err := w.Write(append(head[:len(head)-2*188], stream...)); err != nil {
		t.Fatal(err)
	}
	if breaks != 1 {
		t.Fatalf("breaks %d, want 1", breaks)
	}
	if got := videoTimes(old.Bytes(), 0x110); len(got) != 5 || got[4] != base+4*step {
		t.Fatalf("old encode video %v", got)
	}
	p.sw.give(&closeBuf{&next})
	// The last two packets stay held until more bytes follow. A frame is
	// exactly two packets, and so is a short table run: two nulls push them
	// through. The new pipe also needs the tables, which are not in the tail.
	nulls := bytes.Repeat(tsPacket(0x1fff, false, nil), 2)
	if _, err := w.Write(append(tables, nulls...)); err != nil {
		t.Fatal(err)
	}
	if got := videoTimes(next.Bytes(), 0x110); len(got) != 8 || got[0] != base+4*step-back {
		t.Fatalf("next encode video %v", got)
	}
	got := videoTimes(next.Bytes(), 0x111)
	if len(got) != len(fresh) {
		t.Fatalf("next encode audio %v, want %v", got, fresh)
	}
	for i := range fresh {
		if got[i] != fresh[i] {
			t.Fatalf("next encode audio %v, want %v", got, fresh)
		}
	}
}

func TestProgramFilterDropsOneBadHeaderBack(t *testing.T) {
	pts := []int64{1_000_000, 1_002_880, 1_005_760, 200_000, 1_008_640, 1_011_520}
	count := 0
	if kept, _ := filterRunHook(t, pts, func() { count++ }); count != 0 || len(kept) != 5 {
		t.Fatalf("bad header: breaks %d kept %v", count, kept)
	}
}

// A candidate break keeps every following packet until the timeline settles.
// Nulls never carry a timestamp, so they must not grow that buffer without limit.
func TestPendingBreakStaysWithinTheCap(t *testing.T) {
	head := twoProgramTS(1, 0x1000, 0x110, 0x111, 2, 0x1001, 0x210)
	var cc byte = 1
	body := videoRun(0x110, []int64{1_000_000, 1_001_500, 1_003_000}, &cc)
	// The pipe holds its last two packets, so the jump is followed by nulls
	// that push it through. Those nulls carry no timestamp.
	jump := videoRun(0x110, []int64{1_003_000 - 2*90000}, &cc)
	jump = append(jump, bytes.Repeat(tsPacket(0x1fff, false, nil), 2)...)
	var out bytes.Buffer
	w := newProgramPipe(&closeBuf{&out}, 1)
	p := w.(*programPipe)
	p.sw = &pipeSwitch{}
	p.onBreak = func() {}
	if _, err := w.Write(append(head[:len(head)-2*188], body...)); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write(jump); err != nil {
		t.Fatal(err)
	}
	if !p.catching {
		t.Fatal("expected to be holding a candidate break")
	}
	nulls := bytes.Repeat(tsPacket(0x1fff, false, nil), tailCap/188+100)
	if _, err := w.Write(nulls); err != nil {
		t.Fatal(err)
	}
	if p.catching || len(p.pending) > tailCap {
		t.Fatalf("catching=%v pending=%d cap=%d", p.catching, len(p.pending), tailCap)
	}
}

// The copied track is where one bad header does lasting harm: with -copyts
// ffmpeg clamps every later audio packet to the bad timestamp plus a tick.
func TestOneBadTimestampDoesNotClampTheCopiedAudio(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip(err)
	}
	dir := t.TempDir()
	src := filepath.Join(dir, "one.ts")
	cmd := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "error", "-y",
		"-f", "lavfi", "-i", "testsrc2=size=320x240:rate=30:duration=4",
		"-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000:duration=4",
		"-c:v", "mpeg2video", "-g", "15", "-b:v", "500k",
		"-c:a", "ac3", "-b:a", "96k",
		"-f", "mpegts", src)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("sample: %v %s", err, out)
	}
	raw, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	program := onlyProgram(t, raw)
	// Move the timestamp of one audio frame in the middle 9000 s ahead.
	var starts []int
	for off := 0; off+188 <= len(raw); off += 188 {
		pkt := raw[off : off+188]
		pay := tsPayload(pkt)
		if pkt[1]&0x40 != 0 && len(pay) > 3 && pay[0] == 0 && pay[1] == 0 && pay[2] == 1 && pay[3] == 0xbd {
			starts = append(starts, off)
		}
	}
	if len(starts) < 10 {
		t.Fatalf("only %d audio frames", len(starts))
	}
	pkt := raw[starts[len(starts)/2] : starts[len(starts)/2]+188]
	pay := tsPayload(pkt)
	ts, _ := pesTime(pay)
	bad := ts + 9000*90000
	copy(pay[9:14], ptsPES(0xbd, bad)[9:14])
	var filtered bytes.Buffer
	w := newProgramPipe(&closeBuf{&filtered}, program)
	if _, err := w.Write(raw); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	copyRun := func(in []byte) string {
		args := renditionArgs(program, Source{VideoCodec: "mpeg2video", AudioCodec: "AC3"}, Rendition{Video: "copy", Audio: "copy"}, "libx264", "", "pipe:0")
		c := exec.Command(ffmpeg, args...)
		c.Stdin = bytes.NewReader(in)
		var errs bytes.Buffer
		c.Stderr = &errs
		_ = c.Run()
		return errs.String()
	}
	if got := copyRun(raw); !strings.Contains(got, "Non-monotonic DTS") {
		t.Fatalf("the unfiltered sample should show the clamp:\n%s", got)
	}
	if got := copyRun(filtered.Bytes()); strings.Contains(got, "Non-monotonic DTS") {
		t.Fatalf("the filtered stream still clamps:\n%s", got)
	}
}

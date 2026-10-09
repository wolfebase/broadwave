package live

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// fileGraph builds the library picture. order is the recording's scan type:
// "progressive", "film", a field order, or empty. Progressive 720p keeps its
// size and rate. Soft telecine plays at 24 when the picture mode is broadcast.
// An empty order stays interlaced, which is what a 1080i recording needs.
func (h *Hub) fileGraph(codec, mode, order string) Graph {
	mode = NormalizeMode(mode)
	if order == "film" && mode == "broadcast" {
		mode = "film"
	}
	return Graph{
		VideoCodec: codec, Profile: "transparent", Audio: "stereo",
		Encoder: h.Encoder, Mode: mode, Deint: h.deintFor(mode, codec),
		Progressive: order == "progressive",
	}
}

// recordingOrder prefers the file's own headers. The channel's stored scan is
// only a fallback for a file that has not written a sequence header yet, and a
// stored "film" is the station, not this recording.
func recordingOrder(fileOrder string, fileOK bool, channelOrder string) string {
	if fileOK && fileOrder != "" {
		return fileOrder
	}
	if storedFieldOrder(channelOrder) == "progressive" {
		return "progressive"
	}
	return ""
}

// fileScanBytes covers a GOP. A 720p sequence header can sit a few hundred
// milliseconds into the recording, and soft telecine needs several pictures.
const fileScanBytes = 4 << 20

func fileScanOrder(path string) (string, bool) {
	f, err := os.Open(path)
	if err != nil {
		return "", false
	}
	defer f.Close()
	buf := make([]byte, fileScanBytes)
	n, err := io.ReadFull(f, buf)
	if n < 188 || (err != nil && err != io.ErrUnexpectedEOF && err != io.EOF) {
		return "", false
	}
	return scanType(buf[:n], 0)
}

// fileAC4 reports a recording whose sound is AC-4, from the PMT at its head.
func fileAC4(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	buf := make([]byte, fileScanBytes)
	n, _ := io.ReadFull(f, buf)
	for _, t := range AudioTracks(buf[:n], 0) {
		if t.Codec == "ac4" {
			return true
		}
	}
	return false
}

// fileGraphFor is fileGraph for the recording at path. Its sound is left out
// when it is AC-4 and ffmpeg cannot decode it.
func (h *Hub) fileGraphFor(path, codec, mode, fieldOrder string) Graph {
	scanned, ok := fileScanOrder(path)
	g := h.fileGraph(codec, mode, recordingOrder(scanned, ok, fieldOrder))
	if h.NoAC4 && fileAC4(path) {
		g.Audio = "none"
	}
	return g
}

func graphStamp(g Graph) string {
	scan := "interlaced"
	if g.Progressive {
		scan = "progressive"
	} else if NormalizeMode(g.Mode) == "film" {
		scan = "film"
	}
	parts := []string{NormalizeMode(g.Mode), g.VideoCodec, g.Encoder, g.Deint, g.Profile, scan}
	// A silent playlist is redone once ffmpeg can decode the sound.
	if g.Audio == "none" {
		parts = append(parts, "silent")
	}
	return strings.Join(parts, "|")
}

func playlistFresh(dir, stamp string) bool {
	body, err := os.ReadFile(filepath.Join(dir, "graph.txt"))
	if err != nil || strings.TrimSpace(string(body)) != stamp {
		return false
	}
	info, err := os.Stat(filepath.Join(dir, "index.m3u8"))
	return err == nil && info.Size() > 0
}

// PlayFile transcodes a finished recording into an HLS playlist and returns when the first segment exists.
// at is where playback resumes, in seconds. The encode input-seeks there when that
// spot is far enough in, and the served playlist keeps the recording's clock.
func (h *Hub) PlayFile(id int64, path, videoCodec, mode, fieldOrder string, at float64) (string, error) {
	if at < resumeMin {
		at = 0
	}
	dir := filepath.Join(h.Dir, "file", fmt.Sprintf("%d", id))
	g := h.fileGraphFor(path, videoCodec, mode, fieldOrder)
	g.Live = false
	stamp := graphStamp(g)
	playlist := filepath.Join(dir, "index.m3u8")
	if playlistFresh(dir, stamp) && filePlaylistCovers(dir, at) {
		return fmt.Sprintf("/media/file/%d/index.m3u8", id), nil
	}
	h.stopFileEncode(id)
	_ = os.RemoveAll(dir)
	h.forgetRecordingPlaylist(dir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	g.Input = abs
	g.Start = at
	if err := writeFileOffset(dir, at); err != nil {
		return "", err
	}
	cmd := exec.Command(h.FFmpeg, PictureArgs(g)...)
	cmd.Dir = dir
	cmd.Stderr = os.Stderr
	if err := os.WriteFile(filepath.Join(dir, "graph.txt"), []byte(stamp), 0o644); err != nil {
		return "", err
	}
	if err := cmd.Start(); err != nil {
		return "", err
	}
	done := h.trackFileEncode(id, cmd)
	go func() {
		_ = cmd.Wait()
		h.dropFileEncode(id, cmd)
		close(done)
	}()
	go extractCaptions(h.FFmpeg, abs, filepath.Join(dir, "captions.vtt"))
	return waitPlaylistFile(playlist, id)
}

// PlayFollow transcodes a recording that is still being written.
// at is where playback resumes, in seconds. The pipe cannot be seeked, so the
// bytes before at are dropped and the playlist's clock still starts there.
func (h *Hub) PlayFollow(id int64, path, videoCodec, mode, fieldOrder string, at float64, still func() bool) (string, error) {
	if at < resumeMin {
		at = 0
	}
	dir := filepath.Join(h.Dir, "file", fmt.Sprintf("%d", id))
	g := h.fileGraphFor(path, videoCodec, mode, fieldOrder)
	g.Input = "pipe:0"
	g.Live = false
	stamp := graphStamp(g)
	playlist := filepath.Join(dir, "index.m3u8")
	if playlistFresh(dir, stamp) && filePlaylistCovers(dir, at) {
		return fmt.Sprintf("/media/file/%d/index.m3u8", id), nil
	}
	h.stopFileEncode(id)
	_ = os.RemoveAll(dir)
	h.forgetRecordingPlaylist(dir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if err := writeFileOffset(dir, at); err != nil {
		return "", err
	}
	cmd := exec.Command(h.FFmpeg, PictureArgs(g)...)
	cmd.Dir = dir
	cmd.Stderr = os.Stderr
	if err := os.WriteFile(filepath.Join(dir, "graph.txt"), []byte(stamp), 0o644); err != nil {
		return "", err
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return "", err
	}
	if err := cmd.Start(); err != nil {
		return "", err
	}
	done := h.trackFileEncode(id, cmd)
	go func() {
		followFile(abs, stdin, still, at, func() {
			// The clock never reached the resume. The bytes that follow are
			// the start of the file, so the playlist clock has to say that
			// before the first segment is published.
			_ = writeFileOffset(dir, 0)
		})
		_ = cmd.Wait()
		h.dropFileEncode(id, cmd)
		close(done)
	}()
	return waitPlaylistFile(playlist, id)
}

func extractCaptions(ffmpeg, path, dest string) {
	if ffmpeg == "" || path == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, ffmpeg, "-hide_banner", "-loglevel", "error", "-i", path, "-map", "0:s:0", "-f", "webvtt", dest)
	_ = cmd.Run()
}

// fileEncode is one ffmpeg process writing a recording's playlist.
type fileEncode struct {
	cmd  *exec.Cmd
	done chan struct{}
}

func (h *Hub) trackFileEncode(id int64, cmd *exec.Cmd) chan struct{} {
	done := make(chan struct{})
	h.playMu.Lock()
	if h.fileEnc == nil {
		h.fileEnc = map[int64]*fileEncode{}
	}
	h.fileEnc[id] = &fileEncode{cmd: cmd, done: done}
	if h.plays == nil {
		h.plays = map[int64]struct{}{}
	}
	h.plays[id] = struct{}{}
	h.playMu.Unlock()
	return done
}

func (h *Hub) dropFileEncode(id int64, cmd *exec.Cmd) {
	h.playMu.Lock()
	if cur := h.fileEnc[id]; cur != nil && cur.cmd == cmd {
		delete(h.fileEnc, id)
		delete(h.plays, id)
	}
	h.playMu.Unlock()
}

// stopFileEncode kills the ffmpeg writing this recording and waits until it
// has exited, so the next encode is not replacing files that process still has open.
func (h *Hub) stopFileEncode(id int64) {
	h.playMu.Lock()
	enc := h.fileEnc[id]
	delete(h.fileEnc, id)
	delete(h.plays, id)
	h.playMu.Unlock()
	if enc == nil {
		return
	}
	if enc.cmd != nil && enc.cmd.Process != nil {
		_ = enc.cmd.Process.Kill()
	}
	select {
	case <-enc.done:
	case <-time.After(3 * time.Second):
	}
}

func waitPlaylistFile(playlist string, id int64) (string, error) {
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if info, err := os.Stat(playlist); err == nil && info.Size() > 0 {
			return fmt.Sprintf("/media/file/%d/index.m3u8", id), nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	return "", fmt.Errorf("recording player did not start")
}

// FollowFile copies a growing recording into dst until still is false and no
// new bytes arrive. The copy starts at the first byte. A pipe has no index.
func FollowFile(path string, dst io.WriteCloser, still func() bool) {
	followFile(path, dst, still, 0, nil)
}

// FollowFileAt copies a growing recording, leaving out the first at seconds
// of its clock. A pipe has no index to seek, so those packets are dropped.
// at below the resume minimum copies from the first byte.
func FollowFileAt(path string, dst io.WriteCloser, still func() bool, at float64) {
	followFile(path, dst, still, at, nil)
}

// MediaWritten is how many seconds of a growing recording are on disk.
// The PCR span is the file's own clock. A span far past the time since the
// recording started is a clock reset, and the elapsed time is used instead.
func MediaWritten(path string, started, now time.Time) float64 {
	elapsed := 0.0
	if !started.IsZero() && now.After(started) {
		elapsed = now.Sub(started).Seconds()
	}
	span, ok := mediaSpan(path)
	if !ok {
		return elapsed
	}
	if elapsed > 0 && span > elapsed+30 {
		return elapsed
	}
	return span
}

// mediaSpan is the PCR clock from the first packet that carries one to the
// last, in seconds. The ends of the file are enough: the clock only moves
// forward, and a whole-file scan of an hour-long recording is not.
func mediaSpan(path string) (float64, bool) {
	f, err := os.Open(path)
	if err != nil {
		return 0, false
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || info.Size() < 188 {
		return 0, false
	}
	first, ok := findPCR(f, info.Size(), true)
	if !ok {
		return 0, false
	}
	last, ok := findPCR(f, info.Size(), false)
	if !ok {
		return 0, false
	}
	delta := (last - first) & (1<<33 - 1)
	if delta <= 0 {
		return 0, false
	}
	return float64(delta) / 90000, true
}

func findPCR(f *os.File, size int64, first bool) (int64, bool) {
	const window = 2 << 20
	start := int64(0)
	if !first && size > window {
		start = size - window
	}
	n := int(size - start)
	if n > window {
		n = window
	}
	if _, err := f.Seek(start, io.SeekStart); err != nil {
		return 0, false
	}
	buf := make([]byte, n)
	got, err := io.ReadFull(f, buf)
	if got < 188 && err != nil {
		return 0, false
	}
	buf = buf[:got]
	align := syncAlign(buf)
	if align < 0 {
		return 0, false
	}
	var pcr int64
	var have bool
	for off := align; off+188 <= len(buf); off += 188 {
		v, ok := readPCR(buf[off : off+188])
		if !ok {
			continue
		}
		if first {
			return v, true
		}
		pcr = v
		have = true
	}
	return pcr, have
}

// syncAlign is the first byte of a 188-byte packet. Two sync bytes in a row
// are the alignment; a longer buffer checks one more so a 0x47 in the payload
// is not the start.
func syncAlign(buf []byte) int {
	if len(buf) < 188*2 {
		return -1
	}
	step := 188
	if len(buf) >= 188*3 {
		step = 188 * 2
	}
	limit := len(buf) - step
	if limit > 187 {
		limit = 187
	}
	for i := 0; i <= limit; i++ {
		if buf[i] != 0x47 || buf[i+188] != 0x47 {
			continue
		}
		if step == 188*2 && buf[i+376] != 0x47 {
			continue
		}
		return i
	}
	return -1
}

// followFile copies a growing recording into ffmpeg until the recording has
// stopped and no new bytes arrive. skip is how many seconds of the file to
// leave out, measured from the first PCR. A pipe has no index to seek.
// missed runs when that clock is never reached and the copy starts over at
// the first byte. It runs before any byte is written.
func followFile(path string, dst io.WriteCloser, still func() bool, skip float64, missed func()) {
	defer dst.Close()
	var file *os.File
	deadline := time.Now().Add(15 * time.Second)
	for {
		opened, err := os.Open(path)
		if err == nil {
			file = opened
			break
		}
		if !still() || time.Now().After(deadline) {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	defer file.Close()
	if skip >= resumeMin && !discardPrefix(file, skip, still) {
		if _, err := file.Seek(0, io.SeekStart); err != nil {
			return
		}
		if missed != nil {
			missed()
		}
	}
	buf := make([]byte, 64*1024)
	quiet := 0
	for {
		n, err := file.Read(buf)
		if n > 0 {
			quiet = 0
			if _, werr := dst.Write(buf[:n]); werr != nil {
				return
			}
		}
		if err == io.EOF || n == 0 {
			if !still() {
				quiet++
				if quiet >= 2 {
					return
				}
			}
			time.Sleep(300 * time.Millisecond)
			continue
		}
		if err != nil {
			return
		}
	}
}

// discardPrefix drops packets until the PCR clock reaches skip seconds.
// The file is left at that packet. A step backward, or a packet with the
// discontinuity flag, starts a new base and is not counted: a splice would
// otherwise look like 2^33 ticks and the copy would begin there. False means
// the recording ended before the clock got there.
func discardPrefix(file *os.File, skip float64, still func() bool) bool {
	want := int64(skip*90000 + 0.5)
	if !alignTS(file, still) {
		return false
	}
	buf := make([]byte, 188)
	var base, elapsed int64
	var have bool
	quiet := 0
	for {
		off, err := file.Seek(0, io.SeekCurrent)
		if err != nil {
			return false
		}
		_, err = io.ReadFull(file, buf)
		if err != nil {
			if _, serr := file.Seek(off, io.SeekStart); serr != nil {
				return false
			}
			if !still() {
				quiet++
				if quiet >= 2 {
					return false
				}
			} else {
				quiet = 0
			}
			time.Sleep(300 * time.Millisecond)
			continue
		}
		if buf[0] != 0x47 {
			if _, serr := file.Seek(off, io.SeekStart); serr != nil {
				return false
			}
			if !alignTS(file, still) {
				return false
			}
			continue
		}
		quiet = 0
		pcr, ok := readPCR(buf)
		if !ok {
			continue
		}
		if !have {
			base = pcr
			have = true
		} else if !advancePCR(&base, &elapsed, pcr, buf[5]&0x80 != 0) {
			continue
		}
		if elapsed >= want {
			_, err = file.Seek(off, io.SeekStart)
			return err == nil
		}
	}
}

// pcrMod is the 33-bit PCR range. A step through more than half of it is a
// clock jump, not a second of the show.
const pcrMod = int64(1) << 33

// advancePCR adds a forward PCR step onto elapsed. A discontinuity, or a
// step through more than half the clock, sets a new base and adds nothing:
// one tick backward must not count as a wrap to the end of the show.
// False means this sample carried no forward time.
func advancePCR(base, elapsed *int64, pcr int64, discontinuity bool) bool {
	step := (pcr - *base) & (pcrMod - 1)
	*base = pcr
	if discontinuity || step > pcrMod/2 || step == 0 {
		return false
	}
	*elapsed += step
	return true
}

// alignTS moves the file to the next 188-byte packet. A leading byte that is
// not 0x47 would hide every PCR after it.
func alignTS(file *os.File, still func() bool) bool {
	quiet := 0
	for {
		off, err := file.Seek(0, io.SeekCurrent)
		if err != nil {
			return false
		}
		buf := make([]byte, 188*8)
		n, err := file.Read(buf)
		if n >= 188*2 {
			if align := syncAlign(buf[:n]); align >= 0 {
				_, serr := file.Seek(off+int64(align), io.SeekStart)
				return serr == nil
			}
		}
		if _, serr := file.Seek(off, io.SeekStart); serr != nil {
			return false
		}
		if !still() {
			quiet++
			if quiet >= 2 {
				return false
			}
		} else {
			quiet = 0
		}
		if err != nil && err != io.EOF && n == 0 {
			return false
		}
		time.Sleep(300 * time.Millisecond)
	}
}

func readPCR(pkt []byte) (int64, bool) {
	if len(pkt) < 12 || pkt[0] != 0x47 || pkt[3]&0x20 == 0 || pkt[4] < 7 || pkt[5]&0x10 == 0 {
		return 0, false
	}
	base := int64(pkt[6])<<25 | int64(pkt[7])<<17 | int64(pkt[8])<<9 | int64(pkt[9])<<1 | int64(pkt[10]>>7)
	return base, true
}

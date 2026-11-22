package live

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
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
	head, ok := fileHead(path)
	if !ok {
		return "", false
	}
	return scanType(head, 0)
}

// fileRate is the frame rate a recording's picture header names, empty when
// its head has none.
func fileRate(path string) string {
	head, ok := fileHead(path)
	if !ok {
		return ""
	}
	p, _ := pictureFacts(head, 0)
	return p.FPS
}

func fileHead(path string) ([]byte, bool) {
	f, err := os.Open(path)
	if err != nil {
		return nil, false
	}
	defer f.Close()
	buf := make([]byte, fileScanBytes)
	n, err := io.ReadFull(f, buf)
	if n < 188 || (err != nil && err != io.ErrUnexpectedEOF && err != io.EOF) {
		return nil, false
	}
	return buf[:n], true
}

// headAC4 reports a recording whose sound is AC-4, from the PMT at its head.
func headAC4(head []byte) bool {
	for _, t := range AudioTracks(head, 0) {
		if t.Codec == "ac4" {
			return true
		}
	}
	return false
}

// fileGraphFor is fileGraph for the recording at path. Its sound is left out
// when it is AC-4 and ffmpeg cannot decode it. The head is read once. sound
// is the recording's stored main track, played when the file lists it; 0,
// or a track the file does not list, plays the file's first sound.
func (h *Hub) fileGraphFor(path, codec, mode, fieldOrder string, sound int) Graph {
	head, read := fileHead(path)
	scanned, ok := "", false
	if read {
		scanned, ok = scanType(head, 0)
	}
	g := h.fileGraph(codec, mode, recordingOrder(scanned, ok, fieldOrder))
	p, _ := pictureFacts(head, 0)
	g.Rate = p.FPS
	if h.NoAC4 && headAC4(head) {
		g.Audio = "none"
	}
	if sound > 0 && g.Audio != "none" {
		if i := slices.IndexFunc(AudioTracks(head, 0), func(t AudioTrack) bool { return t.PID == sound }); i >= 0 {
			g.AudioPID = sound
		}
	}
	return g
}

// recordingSound is the main track stored with recording id, 0 when none is.
func (h *Hub) recordingSound(id int64) int {
	if h.Store == nil {
		return 0
	}
	rec, err := h.Store.Recording(context.Background(), id)
	if err != nil {
		return 0
	}
	return rec.SoundPID
}

func graphStamp(g Graph) string {
	scan := "interlaced"
	if g.Progressive {
		scan = "progressive"
	} else if NormalizeMode(g.Mode) == "film" {
		scan = "film"
	}
	parts := []string{NormalizeMode(g.Mode), g.VideoCodec, g.Encoder, g.Deint, g.Profile, scan}
	// Encodes made before the rate was read were at 59.94; a 25 Hz one is redone.
	if FiftyHz(g.Rate) {
		parts = append(parts, "50hz")
	}
	// A copy made at the fixed rate is redone where the encoder spends by need.
	if byNeed(g) {
		parts = append(parts, "need")
	}
	// A silent playlist is redone once ffmpeg can decode the sound.
	if g.Audio == "none" {
		parts = append(parts, "silent")
	}
	// A copy that played the file's first sound is redone on the stored main.
	if g.AudioPID > 0 {
		parts = append(parts, "pid"+strconv.Itoa(g.AudioPID))
	}
	return strings.Join(parts, "|")
}

// mpegtsDelay is what ffmpeg's muxer adds to every timestamp of a segment.
const mpegtsDelay = 1.4

// playStart is how long playback waits for the recording playlist.
// A test shortens it. The encode is stopped when the wait ends.
var playStart = 20 * time.Second

// captionSpan bounds the sidecar caption pass.
var captionSpan = 2 * time.Minute

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
	defer h.lockFileStart(id)()
	if at < resumeMin {
		at = 0
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	g := h.recordingGraph(id, path, videoCodec, mode, fieldOrder)
	return h.playRecording(id, &fileJob{path: abs, graph: g}, at)
}

// PlayFollow transcodes a recording that is still being written.
// at is where playback resumes, in seconds. The pipe cannot be seeked, so the
// bytes before at are dropped and the playlist's clock still starts there.
func (h *Hub) PlayFollow(id int64, path, videoCodec, mode, fieldOrder string, at float64, still func() bool) (string, error) {
	defer h.lockFileStart(id)()
	if at < resumeMin {
		at = 0
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	g := h.recordingGraph(id, path, videoCodec, mode, fieldOrder)
	return h.playRecording(id, &fileJob{path: abs, graph: g, still: still, follow: true}, at)
}

// recordingGraph is the picture for this recording: on the processor when the
// GPU has failed on its copy, so opening it again finds the same copy.
func (h *Hub) recordingGraph(id int64, path, videoCodec, mode, fieldOrder string) Graph {
	g := h.fileGraphFor(path, videoCodec, mode, fieldOrder, h.recordingSound(id))
	g.Live = false
	if h.fileOnSoftware(id) {
		return softwareGraph(g)
	}
	return g
}

// softwareGraph is g on the processor.
func softwareGraph(g Graph) Graph {
	g.Encoder, g.Deint = "libx264", ""
	return g
}

func (h *Hub) fileOnSoftware(id int64) bool {
	h.playMu.Lock()
	defer h.playMu.Unlock()
	return h.fileSoft[id]
}

func (h *Hub) setFileSoft(id int64, soft bool) {
	h.playMu.Lock()
	defer h.playMu.Unlock()
	if !soft {
		delete(h.fileSoft, id)
		return
	}
	if h.fileSoft == nil {
		h.fileSoft = map[int64]bool{}
	}
	h.fileSoft[id] = true
}

// playRecording reuses the copy in the recording's folder when it can show at,
// and otherwise replaces it with an encode from there. The caller holds the
// recording's start lock. A copy with nothing writing it carries on only when
// this run read its clock and found it straight. After a restart nothing here
// knows that, and a new viewer loses nothing when it starts again.
func (h *Hub) playRecording(id int64, job *fileJob, at float64) (string, error) {
	dir := filepath.Join(h.Dir, "file", fmt.Sprintf("%d", id))
	stamp := graphStamp(job.graph)
	playlist := filepath.Join(dir, "index.m3u8")
	url := fmt.Sprintf("/media/file/%d/index.m3u8", id)
	reused := func() (string, error) {
		h.setFillSource(id, dir, fillSource{Path: job.path, Graph: job.graph, Offset: FileOffset(dir)})
		return url, nil
	}
	if playlistFresh(dir, stamp) && playlistSound(dir) && filePlaylistCovers(dir, at) {
		if playlistEnded(playlist) {
			return reused()
		}
		h.playMu.Lock()
		running := h.fileEnc[id] != nil
		prev := h.fileJobs[id]
		h.playMu.Unlock()
		if running {
			return reused()
		}
		if prev != nil && prev.path == job.path && prev.clock.safe(prev.follow) {
			h.carryOn(id, dir, prev)
			return reused()
		}
	}
	h.stopFileEncode(id)
	h.stopFill(id)
	_ = os.RemoveAll(dir)
	h.forgetRecordingPlaylist(dir)
	h.setFillSource(id, dir, fillSource{})
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	// The copy starts on the two-second grid, so what is before it can be filled.
	at = gridStart(at)
	if err := writeFileOffset(dir, at); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(dir, "graph.txt"), []byte(stamp), 0o644); err != nil {
		return "", err
	}
	job.clock = newFileClock(at)
	if !job.follow {
		// A finished file is read from its first byte. Its seek is by timestamp.
		job.clock = newFileClock(0)
	}
	h.keepFileJob(id, job)
	done, cancel, err := h.runFileEncode(id, dir, job, job.graph, followAt{skip: at, least: resumeMin, missed: func() {
		// The clock never reached the resume. The bytes that follow are
		// the start of the file, so the playlist clock has to say that
		// before the first segment is published.
		_ = writeFileOffset(dir, 0)
		job.clock.restartAt(0)
	}}, at)
	if err != nil {
		return "", err
	}
	url, err = waitPlaylistFile(playlist, id, done)
	if err != nil {
		if cancel != nil {
			cancel()
		}
		// A follow copy sees the encode is no longer current and stops.
		h.stopFileEncode(id)
		h.forgetFileJob(id, job)
		h.playMu.Lock()
		closing := h.fileClosed
		h.playMu.Unlock()
		if closing {
			return url, err
		}
		// A stream the GPU can't take gets one try on the processor, as live does.
		// That failing too is not the GPU, and the next press tries it again.
		if gpuEncoder(job.graph.Encoder) {
			slog.Warn(fmt.Sprintf("recording %d did not start on the GPU. Trying the processor.", id))
			h.setFileSoft(id, true)
			// A new job: the failed encode's exit still marks the old one.
			next := &fileJob{path: job.path, graph: softwareGraph(job.graph), follow: job.follow, still: job.still}
			return h.playRecording(id, next, at)
		}
		if h.fileOnSoftware(id) {
			slog.Warn(fmt.Sprintf("recording %d did not start on the processor either, so the GPU was not the cause.", id))
		}
		h.setFileSoft(id, false)
		return url, err
	}
	return reused()
}

// runFileEncode starts one ffmpeg writing the recording's playlist in dir. A
// follow job feeds it the growing file as from says; a finished file seeks to
// at, its clock is read beside it, and so are its captions.
func (h *Hub) runFileEncode(id int64, dir string, job *fileJob, g Graph, from followAt, at float64) (<-chan struct{}, context.CancelFunc, error) {
	if job.follow {
		g.Input = "pipe:0"
	} else {
		g.Input = job.path
		g.Start = at
	}
	before := segmentCount(filepath.Join(dir, "index.m3u8"))
	cmd := exec.Command(h.FFmpeg, PictureArgs(g)...)
	cmd.Dir = dir
	logCommand(cmd, "playback "+filepath.Base(dir))
	var stdin io.WriteCloser
	if job.follow {
		pipe, err := cmd.StdinPipe()
		if err != nil {
			return nil, nil, err
		}
		stdin = pipe
	}
	if err := cmd.Start(); err != nil {
		return nil, nil, err
	}
	// The caption pass is a second ffmpeg. A stop of this encode, a new
	// picture, or shutdown ends it too.
	var capCtx context.Context
	var capCancel context.CancelFunc
	if !job.follow {
		capCtx, capCancel = context.WithTimeout(context.Background(), captionSpan)
	}
	idle := h.FileIdle
	if idle <= 0 {
		idle = fileIdleAfter
	}
	done := h.trackFileEncode(id, cmd, capCancel)
	current := func() bool { return h.fileEncodeCurrent(id, cmd) }
	go h.watchFileRequests(id, cmd, job, done, idle)
	go func() {
		if job.follow {
			var over atomic.Bool
			gone := func() bool { return over.Load() || !current() }
			still := func() bool {
				return !gone() && job.still != nil && job.still()
			}
			missed := from.missed
			if missed == nil {
				missed = func() { over.Store(true) }
			}
			from.missed = missed
			from.clock = job.clock
			followFrom(job.path, stdin, still, gone, from)
		}
		_ = cmd.Wait()
		ended := h.dropFileEncode(id, cmd)
		close(done)
		if ended {
			h.fileEncodeEnded(id, dir, job, at, before)
		}
	}()
	if !job.follow {
		go job.clock.scan(job.path, func() bool { return !current() })
		go func() {
			defer capCancel()
			extractCaptions(capCtx, h.FFmpeg, job.path, filepath.Join(dir, "captions.vtt"))
		}()
	}
	return done, capCancel, nil
}

// carryOn starts an encode after the last whole segment of the copy in dir,
// unless one is running, the copy is complete, or its clock can't be trusted.
// The caller holds the start lock. Numbering and clock continue, so a list that
// already names later segments stays right. A follow job seeks to a mark near
// that clock, so this costs a few seconds of reading and not the whole file.
func (h *Hub) carryOn(id int64, dir string, job *fileJob) {
	h.playMu.Lock()
	running := h.fileEnc[id] != nil
	job.failed = false
	h.playMu.Unlock()
	if running {
		return
	}
	playlist := filepath.Join(dir, "index.m3u8")
	if playlistEnded(playlist) {
		h.forgetFileJob(id, job)
		return
	}
	if !job.clock.safe(job.follow) || !playlistSound(dir) {
		h.markFileJobFailed(job)
		return
	}
	g := job.graph
	at := FileOffset(dir)
	if span := fileSpan(playlist); segmentCount(playlist) > 0 && at+span >= resumeMin {
		span += fileLead(dir)
		at += span
		g.Carry = span
	}
	// ffmpeg seeking this close to the end can find no picture, and still lists a
	// segment. The clock has read the whole file, so there is nothing left to
	// encode. A follow clock stops where its copy stopped, which may not be the end.
	if g.Carry > 0 && (job.still == nil || !job.still()) && job.clock.readTo(job.path) && at >= job.clock.end()-resumeMin {
		closePlaylist(playlist)
		h.forgetFileJob(id, job)
		return
	}
	// A pipe can't seek, so a follow copy starts a little early and drops that.
	want := at
	if job.follow {
		want = max(at-seekPreroll, 0)
		g.Trim = at - want
	}
	mark, ok := job.clock.before(want)
	if job.follow && !ok {
		h.markFileJobFailed(job)
		return
	}
	from := followAt{start: mark.off, skip: want - mark.at}
	if _, _, err := h.runFileEncode(id, dir, job, g, from, at); err != nil {
		slog.Warn(fmt.Sprintf("recording %d: carry on: %v", id, err))
		h.markFileJobFailed(job)
	}
}

// playlistSound reports a list that isn't torn: a stop mid-write can leave one
// cut off. It is complete, or ends on a whole segment that is on disk.
func playlistSound(dir string) bool {
	body, err := os.ReadFile(filepath.Join(dir, "index.m3u8"))
	if err != nil {
		return false
	}
	pl, ok := parseMediaPlaylist(body)
	if !ok || pl.dangling || pl.tail {
		return false
	}
	return pl.ended || len(pl.segs) == 0 || segmentReady(dir, pl.segs[len(pl.segs)-1].uri)
}

// fileLead is how far into its own clock the copy's first picture sits. A file
// whose sound starts first begins its picture late, and the seek and the clock
// that carry on both have to count that, or the join repeats those pictures.
func fileLead(dir string) float64 {
	pts, ok := SegmentPTS(filepath.Join(dir, "seg00000.ts"))
	if !ok {
		return 0
	}
	return max(float64(pts)/90000-mpegtsDelay, 0)
}

// fileEncodeEnded runs when an ffmpeg exited by itself, not by a stop. It closes
// the list only when it knows the file really ended: a clock it read straight
// to within a few seconds of the seek. Anything else waits for someone to press
// play, so a broken encode is not restarted by every player request.
func (h *Hub) fileEncodeEnded(id int64, dir string, job *fileJob, at float64, before int) {
	playlist := filepath.Join(dir, "index.m3u8")
	if playlistEnded(playlist) {
		h.forgetFileJob(id, job)
		return
	}
	if !job.clock.safe(job.follow) {
		h.markFileJobFailed(job)
		return
	}
	if segmentCount(playlist) > before {
		return
	}
	growing := job.still != nil && job.still()
	if before > 0 && !growing && job.clock.readTo(job.path) && at >= job.clock.end()-fileEndMargin {
		closePlaylist(playlist)
		h.forgetFileJob(id, job)
		return
	}
	h.markFileJobFailed(job)
}

func closePlaylist(path string) {
	if f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644); err == nil {
		_, _ = f.WriteString("#EXT-X-ENDLIST\n")
		_ = f.Close()
	}
}

func extractCaptions(ctx context.Context, ffmpeg, path, dest string) {
	if ffmpeg == "" || path == "" || ctx.Err() != nil {
		return
	}
	cmd := exec.CommandContext(ctx, ffmpeg, "-hide_banner", "-loglevel", "error", "-i", path, "-map", "0:s:0", "-f", "webvtt", dest)
	_ = cmd.Run()
}

// fileEncode is one ffmpeg process writing a recording's playlist.
// cancel stops the caption pass that belongs to that process.
type fileEncode struct {
	cmd    *exec.Cmd
	done   chan struct{}
	cancel context.CancelFunc
}

// fileJob is how a recording's copy is encoded, kept so a stopped encode can
// carry on. Its failed and resuming are under playMu.
type fileJob struct {
	path  string
	graph Graph
	// follow feeds ffmpeg the file through a pipe, as it was written. A job that
	// starts that way keeps to it after the recording finishes.
	follow bool
	// still reports a recording being written. A finished recording has none.
	still func() bool
	// clock says whether the encode can stop and carry on.
	clock    *fileClock
	failed   bool
	resuming bool
}

func (h *Hub) trackFileEncode(id int64, cmd *exec.Cmd, cancel context.CancelFunc) chan struct{} {
	done := make(chan struct{})
	h.playMu.Lock()
	if h.fileEnc == nil {
		h.fileEnc = map[int64]*fileEncode{}
	}
	h.fileEnc[id] = &fileEncode{cmd: cmd, done: done, cancel: cancel}
	h.noteFileRequestLocked(id)
	h.playMu.Unlock()
	return done
}

func (h *Hub) lockFileStart(id int64) func() {
	h.playMu.Lock()
	if h.fileStart == nil {
		h.fileStart = map[int64]*sync.Mutex{}
	}
	mu := h.fileStart[id]
	if mu == nil {
		mu = &sync.Mutex{}
		h.fileStart[id] = mu
	}
	h.playMu.Unlock()
	mu.Lock()
	return mu.Unlock
}

// fileEncodeCurrent is false once a newer encode replaced cmd. A follow copy
// waiting at the end of the file never writes, so it would not see the kill.
func (h *Hub) fileEncodeCurrent(id int64, cmd *exec.Cmd) bool {
	h.playMu.Lock()
	defer h.playMu.Unlock()
	cur := h.fileEnc[id]
	return cur != nil && cur.cmd == cmd
}

// dropFileEncode forgets cmd. True means nothing had stopped it: it exited by itself.
func (h *Hub) dropFileEncode(id int64, cmd *exec.Cmd) bool {
	h.playMu.Lock()
	defer h.playMu.Unlock()
	if cur := h.fileEnc[id]; cur != nil && cur.cmd == cmd {
		delete(h.fileEnc, id)
		return true
	}
	return false
}

// stopFileEncodes ends every recording playback. The caller is not holding
// playMu: each stop waits for that ffmpeg to exit.
func (h *Hub) stopFileEncodes() {
	h.playMu.Lock()
	ids := make([]int64, 0, len(h.fileEnc))
	for id := range h.fileEnc {
		ids = append(ids, id)
	}
	h.playMu.Unlock()
	for _, id := range ids {
		h.stopFileEncode(id)
	}
	h.stopFills()
}

// stopFileEncode kills the ffmpeg writing this recording and waits until it
// has exited, so the next encode is not replacing files that process still has open.
func (h *Hub) stopFileEncode(id int64) {
	h.playMu.Lock()
	enc := h.fileEnc[id]
	delete(h.fileEnc, id)
	h.playMu.Unlock()
	if enc == nil {
		return
	}
	if enc.cancel != nil {
		enc.cancel()
	}
	if enc.cmd != nil && enc.cmd.Process != nil {
		_ = enc.cmd.Process.Kill()
	}
	select {
	case <-enc.done:
	case <-time.After(3 * time.Second):
	}
}

// waitPlaylistFile waits for the encode's playlist. exited closes when that
// ffmpeg is gone: a file it cannot read fails at once instead of holding the
// recording's start lock for the whole wait.
func waitPlaylistFile(playlist string, id int64, exited <-chan struct{}) (string, error) {
	appeared := func() bool {
		info, err := os.Stat(playlist)
		return err == nil && info.Size() > 0
	}
	url := fmt.Sprintf("/media/file/%d/index.m3u8", id)
	deadline := time.NewTimer(playStart)
	defer deadline.Stop()
	tick := time.NewTicker(200 * time.Millisecond)
	defer tick.Stop()
	for {
		if appeared() {
			return url, nil
		}
		select {
		case <-exited:
			if appeared() {
				return url, nil
			}
			return "", fmt.Errorf("recording player did not start")
		case <-deadline.C:
			if appeared() {
				return url, nil
			}
			return "", fmt.Errorf("recording player did not start")
		case <-tick.C:
		}
	}
}

// FollowFile copies a growing recording into dst until still is false and no
// new bytes arrive. The copy starts at the first byte. A pipe has no index.
func FollowFile(path string, dst io.WriteCloser, still func() bool) {
	followFile(path, dst, still, nil, 0, nil)
}

// FollowFileAt copies a growing recording, leaving out the first at seconds
// of its clock. A pipe has no index to seek, so those packets are dropped.
// at below the resume minimum copies from the first byte.
func FollowFileAt(path string, dst io.WriteCloser, still func() bool, at float64) {
	followFile(path, dst, still, nil, at, nil)
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
// the first byte. It runs before any byte is written. gone, when set, is true
// once nobody will read dst, and the copy quits without reading the rest.
func followFile(path string, dst io.WriteCloser, still, gone func() bool, skip float64, missed func()) {
	followFrom(path, dst, still, gone, followAt{skip: skip, least: resumeMin, missed: missed})
}

// followAt is where a follow copy starts: byte start, then skip more seconds of
// clock when that is at least least. clock reads everything the copy sends.
type followAt struct {
	start  int64
	skip   float64
	least  float64
	missed func()
	clock  *fileClock
}

func followFrom(path string, dst io.WriteCloser, still, gone func() bool, from followAt) {
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
	if from.start > 0 {
		if _, err := file.Seek(from.start, io.SeekStart); err != nil {
			return
		}
	}
	if from.skip > 0 && from.skip >= from.least && !discardPrefix(file, from.skip, still, gone) {
		// The next encode may already own the playlist folder.
		if gone != nil && gone() {
			return
		}
		if _, err := file.Seek(0, io.SeekStart); err != nil {
			return
		}
		if from.missed != nil {
			from.missed()
		}
		// A carry-on has nothing to start over: there is no more of the show.
		if gone != nil && gone() {
			return
		}
	}
	off, err := file.Seek(0, io.SeekCurrent)
	if err != nil {
		return
	}
	buf := make([]byte, 64*1024)
	quiet := 0
	for {
		n, err := file.Read(buf)
		if n > 0 {
			quiet = 0
			if from.clock != nil {
				from.clock.feed(off, buf[:n])
			}
			off += int64(n)
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
// the recording ended before the clock got there. It reads in large blocks: a
// seek and a read per packet took about 50 s for a 15 GB file on a fast disk.
func discardPrefix(file *os.File, skip float64, still, gone func() bool) bool {
	want := int64(skip*90000 + 0.5)
	if !alignTS(file, still) {
		return false
	}
	off, err := file.Seek(0, io.SeekCurrent)
	if err != nil {
		return false
	}
	buf := make([]byte, 1<<20)
	var base, elapsed int64
	var have bool
	quiet := 0
	for {
		// Every packet already on disk reads at once, so a replaced encode
		// would otherwise scan the whole file before it noticed.
		if gone != nil && gone() {
			return false
		}
		n, err := file.ReadAt(buf, off)
		whole := n - n%188
		if whole == 0 {
			if err != nil && err != io.EOF {
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
		quiet = 0
		for i := 0; i < whole; i += 188 {
			pkt := buf[i : i+188]
			if pkt[0] != 0x47 {
				if _, serr := file.Seek(off+int64(i), io.SeekStart); serr != nil {
					return false
				}
				if !alignTS(file, still) {
					return false
				}
				if off, err = file.Seek(0, io.SeekCurrent); err != nil {
					return false
				}
				whole = -1
				break
			}
			pcr, ok := readPCR(pkt)
			if !ok {
				continue
			}
			if !have {
				base = pcr
				have = true
			} else if !advancePCR(&base, &elapsed, pcr, pkt[5]&0x80 != 0) {
				continue
			}
			if elapsed >= want {
				_, err = file.Seek(off+int64(i), io.SeekStart)
				return err == nil
			}
		}
		if whole > 0 {
			off += int64(whole)
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

package live

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
)

// A mosaic is one encode of two to four channels side by side, with the
// first channel's sound. Apps watch multiview as separate tiles; a mosaic is
// for screens that take one stream: AirPlay, older devices, and Plex or
// Jellyfin through the exports.
const (
	mosaicMin = 2
	mosaicMax = 4
	// mosaicCost is how many pictures of the host's budget a mosaic takes:
	// it decodes every channel in software and encodes one large picture.
	mosaicCost = 2
	mosaicRate = "10M"
	mosaicGOP  = 60
)

// ErrMosaic is a mosaic key or channel list the server cannot build.
var ErrMosaic = errors.New("a mosaic takes 2 to 4 different channels")

// MosaicKey names a mosaic by its channels, the sound channel first.
func MosaicKey(ids []int64) (string, error) {
	if len(ids) < mosaicMin || len(ids) > mosaicMax {
		return "", ErrMosaic
	}
	parts := make([]string, len(ids))
	for i, id := range ids {
		if id <= 0 || slices.Contains(ids[:i], id) {
			return "", ErrMosaic
		}
		parts[i] = strconv.FormatInt(id, 10)
	}
	return strings.Join(parts, "-"), nil
}

// ParseMosaicKey is MosaicKey backwards. It refuses anything MosaicKey would
// not write, so a key is also a safe directory name.
func ParseMosaicKey(key string) ([]int64, error) {
	var ids []int64
	for _, part := range strings.Split(key, "-") {
		id, err := strconv.ParseInt(part, 10, 64)
		if err != nil || strconv.FormatInt(id, 10) != part {
			return nil, ErrMosaic
		}
		ids = append(ids, id)
	}
	if _, err := MosaicKey(ids); err != nil {
		return nil, err
	}
	return ids, nil
}

// MosaicSession is a viewer's handle on a running mosaic.
type MosaicSession struct {
	Key      string  `json:"key"`
	Playlist string  `json:"playlist"`
	Channels []int64 `json:"channelIds"`
	Encoder  string  `json:"encoder"`
	Viewers  int     `json:"viewers"`
}

type mosaic struct {
	key   string
	ids   []int64
	feeds []*feed
	// out is the encode and its packager. Only its process, playlist, and
	// viewer fields are used.
	out  *rendition
	subs []*pipeSub
	// software is set once the GPU encode failed at its start.
	software bool
}

type mosaicInput struct {
	program            int
	input              string
	userAgent, referer string
}

// mosaicArgs decodes every input in software, deinterlaces what is
// interlaced, and stacks the pictures on a 1920x1080 frame at 59.94. Without
// -copyts ffmpeg starts each input near zero, so pictures that arrive
// together show together, and it smooths each broadcast's timestamp breaks
// itself. The output carries the first input's sound.
func mosaicArgs(inputs []mosaicInput, encoder string) []string {
	args := []string{"-hide_banner", "-loglevel", "warning"}
	if vaapiFamily(encoder) {
		args = append(args, "-init_hw_device", "vaapi=va:/dev/dri/renderD128", "-filter_hw_device", "va")
	}
	// Input options apply to the next -i only.
	for _, in := range inputs {
		args = append(args, "-fflags", "+genpts+discardcorrupt")
		probeSize, probeFor := "8000000", "1000000"
		if strings.Contains(in.input, "://") {
			args = urlProtocols(args, in.input)
			args = append(args, "-reconnect", "1", "-reconnect_streamed", "1", "-reconnect_delay_max", "5")
			args = append(args, headerArgs(in.userAgent, in.referer)...)
			probeSize, probeFor = "2000000", "1500000"
		}
		args = append(args, "-probesize", probeSize, "-analyzeduration", probeFor, "-i", in.input)
	}
	pick := func(i int, kind string) string {
		if p := inputs[i].program; p > 0 {
			return fmt.Sprintf("%d:p:%d:%s:0", i, p, kind)
		}
		return fmt.Sprintf("%d:%s:0", i, kind)
	}
	var graph []string
	var stack string
	for i := range inputs {
		graph = append(graph, fmt.Sprintf("[%s]yadif=mode=send_field:deint=interlaced,fps=60000/1001,"+
			"scale=960:540:force_original_aspect_ratio=decrease,pad=960:540:(ow-iw)/2:(oh-ih)/2,setsar=1[t%d]", pick(i, "v"), i))
		stack += fmt.Sprintf("[t%d]", i)
	}
	layout := "0_0|w0_0|0_h0|w0_h0"
	tail := ""
	switch len(inputs) {
	case 2:
		layout = "0_0|w0_0"
		tail = ",pad=1920:1080:0:270"
	case 3:
		layout = "0_0|w0_0|0_h0"
	}
	upload := ",format=yuv420p"
	if vaapiFamily(encoder) {
		upload = ",format=nv12,hwupload"
	}
	graph = append(graph, fmt.Sprintf("%sxstack=inputs=%d:layout=%s:fill=black%s%s[v]", stack, len(inputs), layout, tail, upload))
	args = append(args, "-filter_complex", strings.Join(graph, ";"), "-map", "[v]", "-map", pick(0, "a")+"?")
	args = append(args, videoCodec(encoder, mosaicRate, mosaicGOP)...)
	args = append(args, "-force_key_frames", "expr:gte(t,n_forced)",
		"-af", audioFilter(Rendition{}), "-c:a", "aac", "-ac", "2", "-b:a", "160k")
	return append(args,
		"-video_track_timescale", "90000",
		"-muxdelay", "0", "-muxpreload", "0",
		"-f", "mp4",
		"-movflags", "frag_keyframe+empty_moov+default_base_moof+delay_moov",
		"pipe:1",
	)
}

// WatchMosaic starts or joins the mosaic of these channels. Each channel
// rides its own shared tune, so a channel already on costs no tuner.
func (h *Hub) WatchMosaic(ctx context.Context, ids []int64) (MosaicSession, error) {
	key, err := MosaicKey(ids)
	if err != nil {
		return MosaicSession{}, err
	}
	h.mu.Lock()
	if mo := h.mosaics[key]; mo != nil {
		s := h.joinMosaicLocked(mo)
		h.mu.Unlock()
		return s, nil
	}
	// Checked again after tuning; this one keeps a mosaic that cannot fit
	// from tuning anything.
	if !h.mosaicFitsLocked() {
		h.mu.Unlock()
		return MosaicSession{}, &PictureError{Tiles: h.Host.Tiles}
	}
	h.mu.Unlock()

	var feeds []*feed
	release := func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		for _, f := range feeds {
			f.mosaics--
			h.dropIfUnusedLocked(f)
		}
	}
	for _, id := range ids {
		ch, err := h.Store.SourceChannel(ctx, id)
		if err != nil {
			release()
			return MosaicSession{}, err
		}
		h.mu.Lock()
		_, tuned := h.channels[id]
		busy := streamBusy(ch, h.streamsForDeviceLocked(ch.DeviceID))
		h.mu.Unlock()
		if busy && !tuned {
			release()
			return MosaicSession{}, &StreamLimitError{Limit: ch.StreamLimit}
		}
		var res *http.Response
		if ch.TunerCount == 0 && ch.StreamURL != "" && !hlsStream(ch) {
			if !tuned {
				if res, err = openStream(ch.StreamURL, ch.UserAgent, ch.Referrer); err != nil {
					release()
					return MosaicSession{}, err
				}
			}
		}
		f, err := h.tuneFeed(ctx, ch, res)
		if err != nil {
			h.mu.Unlock()
			release()
			return MosaicSession{}, err
		}
		f.mosaics++
		feeds = append(feeds, f)
		h.mu.Unlock()
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	if mo := h.mosaics[key]; mo != nil {
		// Another viewer started it while these tuned.
		for _, f := range feeds {
			f.mosaics--
			h.dropIfUnusedLocked(f)
		}
		return h.joinMosaicLocked(mo), nil
	}
	if !h.mosaicFitsLocked() {
		for _, f := range feeds {
			f.mosaics--
			h.dropIfUnusedLocked(f)
		}
		return MosaicSession{}, &PictureError{Tiles: h.Host.Tiles}
	}
	mo := &mosaic{key: key, ids: slices.Clone(ids), feeds: feeds}
	if err := h.startMosaicLocked(mo); err != nil {
		for _, f := range feeds {
			f.mosaics--
			h.dropIfUnusedLocked(f)
		}
		return MosaicSession{}, err
	}
	if h.mosaics == nil {
		h.mosaics = map[string]*mosaic{}
	}
	h.mosaics[key] = mo
	slog.Info(fmt.Sprintf("mosaic %s started (%s)", key, mosaicEncoder(h.Encoder, mo.software)))
	return h.joinMosaicLocked(mo), nil
}

// mosaicFitsLocked frees pictures nobody watches, then says whether a
// mosaic fits the host's picture budget.
func (h *Hub) mosaicFitsLocked() bool {
	if h.Host.Tiles <= 0 {
		return true
	}
	if h.transcodesLocked()+mosaicCost > h.Host.Tiles {
		h.releaseIdlePicturesLocked(nil, time.Now(), mosaicCost)
	}
	return h.transcodesLocked()+mosaicCost <= h.Host.Tiles
}

func mosaicEncoder(base string, software bool) string {
	if software || base == "" {
		return "libx264"
	}
	return base
}

func (h *Hub) joinMosaicLocked(mo *mosaic) MosaicSession {
	mo.out.viewers++
	mo.out.seen = time.Now()
	stopTimer(&mo.out.idle)
	h.changed()
	return MosaicSession{
		Key:      mo.key,
		Playlist: fmt.Sprintf("/media/mosaic/%s/index.m3u8", mo.key),
		Channels: slices.Clone(mo.ids),
		Encoder:  mosaicEncoder(h.Encoder, mo.software),
		Viewers:  mo.out.viewers,
	}
}

func (h *Hub) mosaicDir(key string) string {
	return filepath.Join(h.Dir, "live", "mosaic", key)
}

// startMosaicLocked starts the encode and its packager in an empty directory
// and feeds each input from its channel's tune.
func (h *Hub) startMosaicLocked(mo *mosaic) error {
	dir := h.mosaicDir(mo.key)
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	encoder := mosaicEncoder(h.Encoder, mo.software)
	inputs := make([]mosaicInput, len(mo.feeds))
	var readers, writers []*os.File
	closeAll := func() {
		for _, f := range append(readers, writers...) {
			_ = f.Close()
		}
	}
	for i, f := range mo.feeds {
		inputs[i] = mosaicInput{program: f.program, userAgent: f.channel.UserAgent, referer: f.channel.Referrer}
		if m := muxOf(h, f); m != nil && m.input != "" {
			inputs[i].input = m.input
			writers = append(writers, nil)
			continue
		}
		r, w, err := os.Pipe()
		if err != nil {
			closeAll()
			return err
		}
		inputs[i].input = fmt.Sprintf("pipe:%d", 3+len(readers))
		readers = append(readers, r)
		writers = append(writers, w)
	}
	args := mosaicArgs(inputs, encoder)
	cmd := exec.Command(h.FFmpeg, args...)
	cmd.Dir = dir
	cmd.ExtraFiles = readers
	cmd.Stderr = os.Stderr
	stdout, gate, done, err := packOutput(cmd)
	if err != nil {
		closeAll()
		return err
	}
	if err := cmd.Start(); err != nil {
		_ = stdout.Close()
		closeAll()
		return err
	}
	for _, r := range readers {
		_ = r.Close()
	}
	packIn := startPack(dir, stdout, gate, done, nil)
	NotePID(h.Dir, cmd.Process.Pid)
	now := time.Now()
	out := &rendition{dir: dir, cmd: cmd, seen: now, began: now, args: args, gate: gate, packDone: done, input: packIn, fallback: mo.software}
	if mo.out != nil {
		out.viewers = mo.out.viewers
	}
	mo.out = out
	mo.subs = make([]*pipeSub, len(mo.feeds))
	for i, f := range mo.feeds {
		w := writers[i]
		if w == nil {
			continue
		}
		program := f.program
		if program == 0 && NeedFor(f.channel.VideoCodec, f.channel.AudioCodec, f.channel.ATSC3).ATSC3 {
			program = anyProgram
		}
		mo.subs[i] = h.attachPipe(muxOf(h, f), newProgramPipe(w, program), true)
	}
	go h.watchMosaic(mo, out, cmd, encoder)
	return nil
}

// watchMosaic moves a GPU encode that fails at its start to the CPU, and
// otherwise ends the mosaic when its encode ends. Its viewers start it again.
func (h *Hub) watchMosaic(mo *mosaic, out *rendition, cmd *exec.Cmd, encoder string) {
	pid := cmd.Process.Pid
	started := time.Now()
	err := cmd.Wait()
	ForgetPID(h.Dir, pid)
	h.mu.Lock()
	out.waited.Store(true)
	h.mu.Unlock()
	select {
	case <-out.packDone:
	case <-time.After(2 * time.Second):
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.mosaics[mo.key] != mo || mo.out != out {
		return
	}
	if err != nil && !mo.software && gpuEncoder(encoder) && time.Since(started) <= h.fallbackWindow() {
		h.detachMosaicLocked(mo)
		mo.software = true
		if h.startMosaicLocked(mo) == nil {
			slog.Info(fmt.Sprintf("mosaic %s restarted on libx264 after %s: %v", mo.key, time.Since(started).Round(time.Millisecond), err))
			if mo.out.viewers == 0 {
				h.idleMosaicLocked(mo)
			}
			return
		}
	}
	slog.Error(fmt.Sprintf("mosaic %s ended after %s: %v", mo.key, time.Since(started).Round(time.Millisecond), err))
	h.stopMosaicLocked(mo)
}

func (h *Hub) detachMosaicLocked(mo *mosaic) {
	for i, sub := range mo.subs {
		if sub == nil {
			continue
		}
		if m := muxOf(h, mo.feeds[i]); m != nil {
			m.detach(sub)
		} else {
			sub.stop()
		}
	}
	mo.subs = nil
	out := mo.out
	stopTimer(&out.idle)
	if out.cmd != nil && out.cmd.Process != nil && !out.waited.Load() {
		ForgetPID(h.Dir, out.cmd.Process.Pid)
		_ = out.cmd.Process.Kill()
	}
}

// stopMosaicLocked ends the encode and lets go of every channel it held.
func (h *Hub) stopMosaicLocked(mo *mosaic) {
	if h.mosaics[mo.key] != mo {
		return
	}
	h.detachMosaicLocked(mo)
	delete(h.mosaics, mo.key)
	_ = os.RemoveAll(mo.out.dir)
	for _, f := range mo.feeds {
		f.mosaics--
		h.dropIfUnusedLocked(f)
	}
	h.changed()
}

func (h *Hub) mosaicOut(key string) *rendition {
	h.mu.Lock()
	defer h.mu.Unlock()
	mo := h.mosaics[key]
	if mo == nil {
		return nil
	}
	mo.out.seen = time.Now()
	return mo.out
}

// MosaicPlaylist is the mosaic's live playlist with program date-times. The
// mosaic has its own clock: its picture starts at zero.
func (h *Hub) MosaicPlaylist(key string) ([]byte, error) {
	h.mu.Lock()
	mo := h.mosaics[key]
	var out *rendition
	if mo != nil {
		out = mo.out
		out.seen = time.Now()
		if out.clock == nil {
			out.clock = NewTimeline()
		}
	}
	h.mu.Unlock()
	if out == nil {
		return nil, os.ErrNotExist
	}
	raw, err := readPlaylist(filepath.Join(out.dir, "index.m3u8"))
	if err != nil {
		return nil, err
	}
	return out.stamper.stamp(out.dir, raw, out.clock), nil
}

// WaitMosaic holds a blocking playlist request until the mosaic's playlist
// lists that segment or part.
func (h *Hub) WaitMosaic(key string, msn, part int) {
	if out := h.mosaicOut(key); out != nil {
		out.gate.block(msn, part)
	}
}

// MosaicFile is the path of a file in a running mosaic's directory.
func (h *Hub) MosaicFile(key, name string) (string, bool) {
	out := h.mosaicOut(key)
	if out == nil || name != filepath.Base(name) || strings.HasPrefix(name, ".") {
		return "", false
	}
	return filepath.Join(out.dir, name), true
}

// ReleaseMosaic removes one viewer. The encode runs on for RenditionIdle, so
// a viewer coming back finds it.
func (h *Hub) ReleaseMosaic(key string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	mo := h.mosaics[key]
	if mo == nil || mo.out.viewers == 0 {
		return
	}
	mo.out.viewers--
	h.changed()
	if mo.out.viewers == 0 {
		h.idleMosaicLocked(mo)
	}
}

// idleMosaicLocked stops a mosaic with no viewers after RenditionIdle.
func (h *Hub) idleMosaicLocked(mo *mosaic) {
	stopTimer(&mo.out.idle)
	mo.out.idle = time.AfterFunc(h.RenditionIdle, func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		if h.mosaics[mo.key] == mo && mo.out.viewers == 0 {
			h.stopMosaicLocked(mo)
		}
	})
}

// releaseAbandonedMosaicsLocked ends a mosaic nobody has fetched for maxAge,
// with or without viewers counted.
func (h *Hub) releaseAbandonedMosaicsLocked(now time.Time, maxAge time.Duration) {
	for _, mo := range h.mosaics {
		if now.Sub(mo.out.seen) >= maxAge {
			mo.out.viewers = 0
			h.stopMosaicLocked(mo)
		}
	}
}

// releaseIdleMosaicsLocked frees the pictures of mosaics nobody watches
// until the budget has room for need more.
func (h *Hub) releaseIdleMosaicsLocked(now time.Time, need int) {
	if h.Host.Tiles <= 0 {
		return
	}
	for _, mo := range h.mosaics {
		if h.transcodesLocked()+need <= h.Host.Tiles {
			return
		}
		if freePicture(mo.out, now) {
			mo.out.viewers = 0
			h.stopMosaicLocked(mo)
		}
	}
}

// endMosaicsOnLocked stops every mosaic that reads f, which is about to stop.
// A mosaic on a lost tune would show that picture frozen; its viewers
// start it again.
func (h *Hub) endMosaicsOnLocked(f *feed) {
	if f.mosaics == 0 {
		return
	}
	for _, mo := range h.mosaics {
		if slices.Contains(mo.feeds, f) {
			slog.Info(fmt.Sprintf("mosaic %s ended: %s stopped", mo.key, f.channel.GuideNumber))
			h.stopMosaicLocked(mo)
		}
	}
}

// mosaicPicturesLocked is the share of the picture budget mosaics take.
func (h *Hub) mosaicPicturesLocked() int {
	return len(h.mosaics) * mosaicCost
}

// mosaicWait is how long ExportMosaic waits for the first segment.
const mosaicWait = 20 * time.Second

// ExportMosaic writes the mosaic as MPEG-TS to w until ctx ends, for Plex,
// Jellyfin, and other apps that take one stream per channel. It copies the
// mosaic's own segments, so every export shares the one encode.
func (h *Hub) ExportMosaic(ctx context.Context, ids []int64, w io.Writer) error {
	s, err := h.WatchMosaic(ctx, ids)
	if err != nil {
		return err
	}
	defer h.ReleaseMosaic(s.Key)
	out := h.mosaicOut(s.Key)
	if out == nil {
		return os.ErrNotExist
	}
	playlist := filepath.Join(out.dir, "index.m3u8")
	deadline := time.Now().Add(mosaicWait)
	for {
		if raw, err := os.ReadFile(playlist); err == nil && strings.Contains(string(raw), "#EXTINF") {
			break
		}
		if time.Now().After(deadline) {
			return errors.New("the mosaic did not start")
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(200 * time.Millisecond):
		}
	}
	keep, stop := context.WithCancel(ctx)
	defer stop()
	go func() {
		t := time.NewTicker(5 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-keep.Done():
				return
			case <-t.C:
				h.mosaicOut(s.Key)
			}
		}
	}()
	cmd := exec.CommandContext(ctx, h.FFmpeg, "-hide_banner", "-loglevel", "error",
		"-live_start_index", "-2", "-i", playlist, "-map", "0", "-c", "copy", "-f", "mpegts", "pipe:1")
	cmd.Stdout = w
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil && ctx.Err() == nil {
		return err
	}
	return nil
}

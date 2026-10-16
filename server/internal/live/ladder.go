package live

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
)

// ladderAudioBits is the AAC stereo each ladder rung carries, and the
// audio-only variant's whole rate. The video BANDWIDTH adds the same
// headroom the other masters use, on top of this.
const ladderAudioBits = 160_000

// ladderFilmKeys is one keyframe expression for every film rung. pullup
// drops the frame that carried the source keyframe, so "source" would let
// each encoder pick its own. Two seconds matches a 48-frame group at 24 fps.
const ladderFilmKeys = "expr:gte(t,n_forced*2)"

// ladderGroup is the one ffmpeg a ladder's rungs share. The primary
// rendition holds the process; the others only pack an output.
// anchor keeps the first playlist from dating the primary, so the other
// rungs, fetched at the same time, can follow that clock.
type ladderGroup struct {
	rungs  []*rendition
	anchor sync.Mutex
}

// ladderAnchorHook runs after a rung has looked for a sibling clock and
// before the shared anchor. Tests hold every rung there. Nil otherwise.
var ladderAnchorHook func()

// LadderVideos is the picture rungs a host can encode, tallest first.
// A host with no measured height keeps all three. 480p stays even when the
// host is shorter than that: the player can still drop to it, and to audio.
func LadderVideos(host Host) []string {
	switch {
	case host.Height <= 0 || host.Height >= 1080:
		return []string{"1080", "720", "480"}
	case host.Height >= 720:
		return []string{"720", "480"}
	default:
		return []string{"480"}
	}
}

// LadderReason is the session's explanation of the rungs it started.
func LadderReason(videos []string) string {
	names := make([]string, len(videos))
	for i, video := range videos {
		names[i] = video + "p"
	}
	switch len(names) {
	case 0:
		return "Adaptive: audio only"
	case 1:
		return "Adaptive: " + names[0] + ", and audio only"
	case 2:
		return "Adaptive: " + names[0] + " and " + names[1] + ", and audio only"
	default:
		return "Adaptive: " + strings.Join(names[:len(names)-1], ", ") + ", and " + names[len(names)-1] + ", and audio only"
	}
}

// ladderClient is a browser or an Apple player that should switch rungs
// itself. Tiles, a second language, described video, even volume, and silent
// audio stay on the one rendition they asked for.
func ladderClient(caps Caps, prefs Prefs, r Rendition) bool {
	switch caps.Platform {
	case "web", "ios", "ipados", "tvos", "macos", "visionos":
	default:
		return false
	}
	switch prefs.Quality {
	case "tile", "360", "focus":
		return false
	}
	if r.Audio == "none" || prefs.Audio == "none" || r.Track != "" || r.Even || prefs.Even {
		return false
	}
	switch prefs.Track {
	case "language", "described":
		return false
	}
	return true
}

// ApplyLadder turns a decided rendition into the tallest rung of a ladder
// when the setting is on and this client can use it. Off leaves the decision
// alone, and so does a client the ladder does not cover.
func ApplyLadder(on bool, caps Caps, prefs Prefs, host Host, d Decision) Decision {
	if !on || !ladderClient(caps, prefs, d.Rendition) {
		return d
	}
	videos := LadderVideos(host)
	if len(videos) == 0 {
		return d
	}
	r := d.Rendition
	r.Video = videos[0]
	r.Audio = "aac2"
	r.Codec = ""
	r.Track = ""
	r.Even = false
	r.FullRate = false
	r.ABR = true
	r.Mode = NormalizeMode(r.Mode)
	d.Rendition = r.normalized()
	d.Reason = LadderReason(videos)
	return d
}

// ladderRung is one picture size of the ladder, in the same mode as the watch.
func ladderRung(mode, video string) Rendition {
	return Rendition{Video: video, Audio: "aac2", Mode: NormalizeMode(mode), ABR: true}.normalized()
}

// ladderEncoder forces H.264. HEVC and an empty or unknown encoder become
// libx264. The hardware H.264 encoders stay, and they encode only.
func ladderEncoder(base string) string {
	switch base {
	case "h264_videotoolbox", "h264_nvenc", "h264_qsv", "h264_vaapi":
		return base
	default:
		return "libx264"
	}
}

// LadderArgs is one ffmpeg for every video rung: one software decode, one
// deinterlace or film pullup, then a split. Each output is an fMP4 on
// pipe:3 and up. stdout is unused.
func LadderArgs(program int, src Source, videos []string, mode, encoder, input string) []string {
	if len(videos) == 0 {
		videos = []string{"1080", "720", "480"}
	}
	mode = NormalizeMode(mode)
	if src.Film && mode == "broadcast" {
		mode = "film"
	}
	enc := ladderEncoder(encoder)
	if input == "" {
		input = "pipe:0"
	}
	args := ladderInputArgs(program, src, enc, input)
	args = append(args, "-filter_complex", ladderGraph(program, src, videos, mode, enc))
	_, field, _, key, keyint := ladderTiming(src, mode)
	for i, video := range videos {
		g := Graph{Profile: renditionProfile(video), Encoder: enc}
		_, _, rate := outputSize(g, field)
		args = append(args, "-map", fmt.Sprintf("[o%d]", i), "-map", fmt.Sprintf("[a%d]", i))
		args = append(args, videoCodec(enc, rate, keyint)...)
		args = append(args, ladderIDR(enc)...)
		if src.HD {
			args = append(args, "-colorspace", "bt709", "-color_primaries", "bt709", "-color_trc", "bt709")
		}
		args = append(args, "-force_key_frames", key, "-c:a", "aac", "-ac", "2", "-b:a", "160k")
		if codecName(src.AudioCodec) == "ac4" {
			args = append(args, "-ar", "48000")
		}
		args = append(args,
			"-video_track_timescale", "90000",
			"-muxdelay", "0", "-muxpreload", "0",
			"-f", "mp4",
			"-movflags", "frag_keyframe+empty_moov+default_base_moof+delay_moov",
			fmt.Sprintf("pipe:%d", 3+i),
		)
	}
	return args
}

// ladderTiming is the shared frame rate, whether the picture is field-doubled,
// the keyframe expression, and the -g ceiling. 480p uses the same clock as
// 1080p; a half-rate rung would close its segments on different frames.
func ladderTiming(src Source, mode string) (fps string, field bool, gop int, key string, keyint int) {
	interlaced := fieldDoubled(src.VideoCodec, mode, src.Progressive, src.Lace)
	field = interlaced && mode != "film"
	key = openingKeyframes
	keyint = sourceKeyint
	gop = 120
	if mode == "film" {
		fps = "24000/1001"
		gop = 48
		keyint = 2 * gop
		key = ladderFilmKeys
		return fps, false, gop, key, keyint
	}
	if field {
		fps = "60000/1001"
		gop = 120
	}
	return fps, field, gop, key, keyint
}

// ladderIDR makes every forced keyframe an IDR. NVIDIA and Quick Sync
// otherwise force a plain I-frame, and a player that changes rung there
// cannot decode until the next IDR. Quick Sync's own I-frames would land on
// different pictures at each size.
func ladderIDR(encoder string) []string {
	switch encoder {
	case "h264_nvenc":
		return []string{"-forced-idr", "1"}
	case "h264_qsv":
		return []string{"-forced_idr", "1", "-adaptive_i", "0"}
	}
	return nil
}

func ladderInputArgs(program int, src Source, encoder, input string) []string {
	args := []string{"-hide_banner", "-loglevel", "warning", "-fflags", "+genpts+discardcorrupt", "-copyts"}
	if encoder == "h264_vaapi" {
		// The decode stays in software. Each scale branch uploads its own frames.
		args = append(args, "-init_hw_device", "vaapi=va:/dev/dri/renderD128", "-filter_hw_device", "va")
	}
	if strings.Contains(input, "://") {
		args = urlProtocols(args, input)
		args = append(args, "-reconnect", "1", "-reconnect_streamed", "1", "-reconnect_delay_max", "5")
	}
	args = append(args, headerArgs(src.UserAgent, src.Referrer)...)
	probeSize, probeFor := "8000000", "1000000"
	if program > 0 && input == "pipe:0" {
		probeFor = "500000"
	}
	if strings.Contains(input, "://") {
		probeSize, probeFor = "2000000", "1500000"
	}
	args = append(args, "-probesize", probeSize, "-analyzeduration", probeFor)
	args = append(append(args, captionProbe(src.AudioCodec)...), "-i", input)
	return args
}

func ladderPads(prefix string, n int) string {
	var b strings.Builder
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, "[%s%d]", prefix, i)
	}
	return b.String()
}

func ladderGraph(program int, src Source, videos []string, mode, encoder string) string {
	n := len(videos)
	videoIn, audioIn := "[0:v:0]", "[0:a:0]"
	if program > 0 {
		videoIn = fmt.Sprintf("[0:p:%d:v:0]", program)
		audioIn = fmt.Sprintf("[0:p:%d:a:0]", program)
	}
	if src.AudioPID > 0 {
		audioIn = fmt.Sprintf("[0:i:%d]", src.AudioPID)
	}
	fps, _, _, _, _ := ladderTiming(src, mode)
	var pre []string
	if src.HD {
		pre = append(pre, "setparams=colorspace=bt709:color_primaries=bt709:color_trc=bt709")
	}
	switch {
	case mode == "film":
		pre = append(pre, "pullup", "fps="+fps)
	case fps != "":
		pre = append(pre, "bwdif=mode=send_field:parity=auto:deint=interlaced", "fps="+fps)
	}
	var chains []string
	if n == 1 {
		head := videoIn
		if len(pre) > 0 {
			head += strings.Join(pre, ",") + ","
		}
		chains = append(chains, head+ladderScale(videos[0], encoder)+"[o0]")
		chains = append(chains, audioIn+"aresample=async=1000:min_hard_comp=1000000,aformat=channel_layouts=stereo[a0]")
		return strings.Join(chains, ";")
	}
	head := videoIn
	if len(pre) > 0 {
		head += strings.Join(pre, ",") + ","
	}
	chains = append(chains, fmt.Sprintf("%ssplit=%d%s", head, n, ladderPads("v", n)))
	for i, video := range videos {
		chains = append(chains, fmt.Sprintf("[v%d]%s[o%d]", i, ladderScale(video, encoder), i))
	}
	chains = append(chains, fmt.Sprintf("%saresample=async=1000:min_hard_comp=1000000,aformat=channel_layouts=stereo,asplit=%d%s", audioIn, n, ladderPads("a", n)))
	return strings.Join(chains, ";")
}

func ladderScale(video, encoder string) string {
	g := Graph{Profile: renditionProfile(video)}
	w, h, _ := outputSize(g, true)
	// The declared size is the cap. min() leaves a smaller source alone,
	// so a 720p broadcast is not blown up to 1080. The crop forces an even
	// width and height: 848 wide against 16:9 is 477 tall, and libx264
	// rejects that (yuv420p needs both sides divisible by 2).
	scale := fmt.Sprintf("scale='min(%d,iw)':'min(%d,ih)':force_original_aspect_ratio=decrease,crop=trunc(iw/2)*2:trunc(ih/2)*2,setsar=1", w, h)
	switch encoder {
	case "h264_vaapi":
		return scale + ",format=nv12,hwupload"
	case "h264_qsv":
		return scale + ",format=nv12"
	default:
		return scale + ",format=yuv420p"
	}
}

// ladderVariant is one line of ladder.m3u8. An empty video codec or size
// falls back to the rung's target. Audio-only sets audioOnly.
type ladderVariant struct {
	spec      Rendition
	uri       string
	video     trackCodec
	audioOnly bool
}

func ladderFallbackCodec(video string) string {
	if video == "1080" {
		return "avc1.640028"
	}
	return "avc1.64001F"
}

func ladderMaster(variants []ladderVariant, audioURI, audioCodec, frameRate string, captions bool) []byte {
	if audioCodec == "" {
		audioCodec = "mp4a.40.2"
	}
	var b strings.Builder
	b.WriteString("#EXTM3U\n#EXT-X-VERSION:6\n#EXT-X-INDEPENDENT-SEGMENTS\n")
	if captions {
		b.WriteString(captionMedia)
	}
	for _, v := range variants {
		if v.audioOnly {
			b.WriteString(fmt.Sprintf("#EXT-X-STREAM-INF:BANDWIDTH=%d,AVERAGE-BANDWIDTH=%d,CODECS=%q\n%s\n", ladderAudioBits, ladderAudioBits, audioCodec, audioURI))
			continue
		}
		codec := v.video.codec
		if codec == "" {
			codec = ladderFallbackCodec(v.spec.Video)
		}
		w, h := v.video.width, v.video.height
		if w == 0 || h == 0 {
			w, h, _ = outputSize(Graph{Profile: renditionProfile(v.spec.Video)}, true)
		}
		inf := []string{
			streamRates(v.spec, ladderAudioBits),
			fmt.Sprintf("CODECS=%q", codec+","+audioCodec),
			fmt.Sprintf("RESOLUTION=%dx%d", w, h),
		}
		if frameRate != "" {
			inf = append(inf, "FRAME-RATE="+frameRate)
		}
		if captions {
			inf = append(inf, `SUBTITLES="cc"`)
		}
		inf = append(inf, "CLOSED-CAPTIONS=NONE")
		b.WriteString("#EXT-X-STREAM-INF:" + strings.Join(inf, ",") + "\n" + v.uri + "\n")
	}
	return []byte(b.String())
}

func ladderFrameRate(src Source, mode string) string {
	mode = NormalizeMode(mode)
	if src.Film && mode == "broadcast" {
		mode = "film"
	}
	fps, _, _, _, _ := ladderTiming(src, mode)
	switch fps {
	case "24000/1001":
		return "23.976"
	case "60000/1001":
		return "59.94"
	default:
		return ""
	}
}

func ladderURI(serving, spec Rendition) string {
	if spec.Key() == serving.Key() {
		return "index.m3u8"
	}
	return "../" + spec.Key() + "/index.m3u8"
}

// ladderPlaylistFromDisk reads each rung's init when it exists and lists the
// ladder. The primary init has to be there; a sibling that is not ready yet
// is listed at its target size.
func ladderPlaylistFromDisk(serving Rendition, specs []Rendition, dirs []string, src Source, captions bool) ([]byte, error) {
	if len(specs) == 0 || len(dirs) != len(specs) {
		return nil, os.ErrNotExist
	}
	audio := "mp4a.40.2"
	audioURI := "audio-2.m3u8"
	if serving.Key() != specs[0].Key() {
		audioURI = "../" + specs[0].Key() + "/audio-2.m3u8"
	}
	variants := make([]ladderVariant, 0, len(specs)+1)
	for i, spec := range specs {
		v := ladderVariant{spec: spec, uri: ladderURI(serving, spec)}
		init, err := os.ReadFile(filepath.Join(dirs[i], "init.mp4"))
		if err != nil {
			if i == 0 {
				return nil, err
			}
		} else {
			codecs := initCodecs(init)
			if c, ok := codecs[1]; ok {
				v.video = c
			}
			if i == 0 {
				if c, ok := codecs[2]; ok && c.codec != "" {
					audio = c.codec
				}
			}
		}
		variants = append(variants, v)
	}
	variants = append(variants, ladderVariant{audioOnly: true})
	return ladderMaster(variants, audioURI, audio, ladderFrameRate(src, specs[0].Mode), captions), nil
}

func (f *feed) ladderPrimary(mode string) *rendition {
	mode = NormalizeMode(mode)
	for _, r := range f.renditions {
		if r == nil || r.ladderChild || r.group == nil || !r.spec.ABR {
			continue
		}
		if NormalizeMode(r.spec.Mode) == mode {
			return r
		}
	}
	return nil
}

func ladderPrimaryOf(r *rendition) *rendition {
	if r == nil || r.group == nil {
		return nil
	}
	for _, mate := range r.group.rungs {
		if mate != nil && !mate.ladderChild {
			return mate
		}
	}
	return nil
}

// establishLadderAnchor dates the primary from its playlist before any rung
// stamps itself. A player asks for every rung at once. Each first stamp
// would otherwise read the wall clock on its own, and the same frame would
// carry a different program date on each rung.
func establishLadderAnchor(primary *rendition) {
	if primary == nil || primary.group == nil || primary.clock == nil || primary.dir == "" {
		return
	}
	primary.group.anchor.Lock()
	defer primary.group.anchor.Unlock()
	if primary.clock.hasDate() {
		return
	}
	raw, err := readPlaylist(filepath.Join(primary.dir, "index.m3u8"))
	if err != nil {
		return
	}
	_ = primary.stamper.stamp(primary.dir, raw, primary.clock)
}

// noteLadderSeen keeps the primary's idle clock fresh when a player fetches
// a shorter rung or the master. The children have no viewers of their own.
func noteLadderSeen(r *rendition) {
	if r == nil {
		return
	}
	now := time.Now()
	r.seen = now
	if primary := ladderPrimaryOf(r); primary != nil {
		primary.seen = now
	}
}

func (h *Hub) ladderCommandLocked(f *feed, want Rendition, encoder string) (string, []string) {
	input := "pipe:0"
	if m := muxOf(h, f); m != nil && m.input != "" {
		input = m.input
	}
	if encoder == "" {
		encoder = h.Encoder
	}
	src := f.sourceFor(want)
	src.Extras = nil
	return input, LadderArgs(f.program, src, LadderVideos(h.Host), want.Mode, encoder, input)
}

// ensureLadderLocked starts the ladder, or returns the one already running
// for this picture mode. A request for a shorter rung still returns the
// primary: the player switches from the master, and the viewers sit on one key.
func (h *Hub) ensureLadderLocked(f *feed, want Rendition, frame time.Time) (*rendition, error) {
	want = want.normalized()
	if primary := f.ladderPrimary(want.Mode); primary != nil {
		return primary, nil
	}
	if h.Host.Tiles > 0 && h.transcodesLocked() >= h.Host.Tiles {
		h.releaseIdleTranscodesLocked(f, time.Now())
	}
	if h.Host.Tiles > 0 && h.transcodesLocked() >= h.Host.Tiles {
		return nil, &PictureError{Tiles: h.Host.Tiles}
	}
	videos := LadderVideos(h.Host)
	specs := make([]Rendition, len(videos))
	for i, video := range videos {
		specs[i] = ladderRung(want.Mode, video)
	}
	return h.startLadderLocked(f, specs, frame)
}

func (h *Hub) startLadderLocked(f *feed, specs []Rendition, frame time.Time) (*rendition, error) {
	if len(specs) == 0 {
		return nil, errors.New("ladder has no rungs")
	}
	_, args := h.ladderCommandLocked(f, specs[0], "")
	dirs := make([]string, len(specs))
	for i, spec := range specs {
		dirs[i] = filepath.Join(h.Dir, "live", fmt.Sprintf("%d", f.channel.ID), spec.Key())
		if err := os.RemoveAll(dirs[i]); err != nil {
			h.removeLadderDirs(dirs[:i])
			return nil, err
		}
		if err := os.MkdirAll(dirs[i], 0o755); err != nil {
			h.removeLadderDirs(dirs[:i+1])
			return nil, err
		}
	}
	cmd, stdin, pipes, err := h.openLadder(args, len(specs), dirs[0])
	if err != nil {
		h.removeLadderDirs(dirs)
		return nil, err
	}
	if stdin != nil {
		h.startCaptionsLocked(f)
	}
	NotePID(h.Dir, cmd.Process.Pid)
	now := time.Now()
	group := &ladderGroup{}
	var primary *rendition
	for i, spec := range specs {
		gate := newPlaylistGate()
		done := make(chan struct{})
		h.seedLong(f, gate)
		packIn := startPack(dirs[i], pipes[i], gate, done, captionLine(f), false)
		r := &rendition{
			spec: spec, dir: dirs[i], seen: now, began: now, args: args,
			gate: gate, packDone: done, input: packIn, group: group, ladderChild: i > 0,
		}
		group.rungs = append(group.rungs, r)
		f.renditions[spec.Key()] = r
		if i == 0 {
			primary = r
			primary.cmd = cmd
			primary.stdin = stdin
		}
	}
	if stdin != nil {
		primary.sub = h.attachRenditionLocked(f, primary, h.renditionPipe(f, primary, stdin), frame, now)
	}
	go h.watchLadder(f, primary, cmd, ladderEncoder(h.Encoder), false)
	return primary, nil
}

func (h *Hub) removeLadderDirs(dirs []string) {
	for _, dir := range dirs {
		if dir != "" {
			_ = os.RemoveAll(dir)
		}
	}
}

// openLadder starts ffmpeg with one extra pipe per rung. The write ends stay
// open until startPack closes them, which has to be after Start: closing
// first races the child.
func (h *Hub) openLadder(args []string, n int, dir string) (*exec.Cmd, io.WriteCloser, []*packPipe, error) {
	cmd := exec.Command(h.FFmpeg, args...)
	cmd.Dir = dir
	cmd.Stdout = io.Discard
	pipes, err := ladderOutputs(cmd, n)
	if err != nil {
		return nil, nil, nil, err
	}
	var stdin io.WriteCloser
	if usesPipe(args) {
		stdin, err = cmd.StdinPipe()
		if err != nil {
			closeLadderPipes(pipes)
			return nil, nil, nil, err
		}
	}
	logCommand(cmd, "ladder")
	if err := cmd.Start(); err != nil {
		closeLadderPipes(pipes)
		if stdin != nil {
			_ = stdin.Close()
		}
		return nil, nil, nil, err
	}
	return cmd, stdin, pipes, nil
}

func ladderOutputs(cmd *exec.Cmd, n int) ([]*packPipe, error) {
	pipes := make([]*packPipe, n)
	for i := 0; i < n; i++ {
		r, w, err := os.Pipe()
		if err != nil {
			closeLadderPipes(pipes)
			return nil, err
		}
		cmd.ExtraFiles = append(cmd.ExtraFiles, w)
		pipes[i] = &packPipe{File: r, w: w}
	}
	return pipes, nil
}

func closeLadderPipes(pipes []*packPipe) {
	for _, p := range pipes {
		if p != nil {
			_ = p.Close()
		}
	}
}

// stopLadderLocked ends the one process and every rung directory. A child
// key redirects here so an idle sweep cannot delete one output out from
// under the encode.
func (h *Hub) stopLadderLocked(f *feed, primary *rendition) {
	if primary == nil {
		return
	}
	stopTimer(&primary.idle)
	if m := muxOf(h, f); m != nil {
		m.detach(primary.sub)
	} else if primary.sub != nil {
		primary.sub.stop()
	}
	if primary.cmd != nil && primary.cmd.Process != nil && !primary.waited.Load() {
		ForgetPID(h.Dir, primary.cmd.Process.Pid)
		if primary.stdin != nil {
			_ = primary.stdin.Close()
		}
		_ = primary.cmd.Process.Kill()
	}
	rungs := []*rendition{primary}
	if primary.group != nil && len(primary.group.rungs) > 0 {
		rungs = primary.group.rungs
	}
	for _, mate := range rungs {
		if mate == nil {
			continue
		}
		stopTimer(&mate.idle)
		delete(f.renditions, mate.spec.Key())
		_ = os.RemoveAll(mate.dir)
	}
	if len(f.renditions) == 0 {
		h.stopCaptionsLocked(f)
	}
	h.changed()
}

func waitLadderPacks(rungs []*rendition) {
	deadline := time.Now().Add(2 * time.Second)
	for _, r := range rungs {
		if r == nil || r.packDone == nil {
			continue
		}
		wait := time.Until(deadline)
		if wait <= 0 {
			return
		}
		select {
		case <-r.packDone:
		case <-time.After(wait):
			return
		}
	}
}

// restartLadderLocked wipes every rung and starts the same group again.
// software forces libx264. The rendition values stay, so the viewers do too.
func (h *Hub) restartLadderLocked(f *feed, primary *rendition, software bool) bool {
	if primary == nil || primary.group == nil {
		return false
	}
	rungs := primary.group.rungs
	old := primary.cmd
	live := old != nil && old.Process != nil && !primary.waited.Load()
	if m := muxOf(h, f); m != nil {
		m.detach(primary.sub)
	} else if primary.sub != nil {
		primary.sub.stop()
	}
	if primary.sub == nil && primary.stdin != nil {
		_ = primary.stdin.Close()
	}
	if live {
		_ = old.Process.Kill()
	}
	primary.sub = nil
	primary.stdin = nil
	primary.filter = nil
	for _, r := range rungs {
		if r == nil {
			continue
		}
		r.stamper.reset()
		r.clock = nil
		r.ringFrom = time.Time{}
		r.dated = false
	}
	waitLadderPacks(rungs)
	dirs := make([]string, len(rungs))
	for i, r := range rungs {
		dirs[i] = r.dir
		if err := os.RemoveAll(r.dir); err != nil {
			return false
		}
		if err := os.MkdirAll(r.dir, 0o755); err != nil {
			return false
		}
	}
	encoder := ladderEncoder(h.Encoder)
	args := primary.args
	if software || len(args) == 0 {
		encoder = "libx264"
		_, args = h.ladderCommandLocked(f, primary.spec, encoder)
		primary.fallback = true
	}
	cmd, stdin, pipes, err := h.openLadder(args, len(rungs), primary.dir)
	if err != nil {
		h.removeLadderDirs(dirs)
		return false
	}
	if stdin != nil {
		h.startCaptionsLocked(f)
	}
	NotePID(h.Dir, cmd.Process.Pid)
	for i, r := range rungs {
		gate := newPlaylistGate()
		done := make(chan struct{})
		h.seedLong(f, gate)
		r.input = startPack(r.dir, pipes[i], gate, done, captionLine(f), false)
		r.gate = gate
		r.packDone = done
		r.args = args
	}
	primary.cmd = cmd
	primary.stdin = stdin
	primary.restarted = true
	primary.waited.Store(false)
	if stdin != nil {
		primary.sub = h.attachPipeLocked(muxOf(h, f), h.renditionPipe(f, primary, stdin))
	}
	go h.watchLadder(f, primary, cmd, encoder, false)
	return true
}

// unfreezeLadderRungLocked restarts the shared encode when one rung's
// playlist stops while the primary's is still moving. The child has no
// process of its own, so the watchdog has to kill the primary's.
func (h *Hub) unfreezeLadderRungLocked(f *feed, r *rendition, now, fed time.Time, limit time.Duration) {
	p := ladderPrimaryOf(r)
	if p == nil || p.cmd == nil || p.cmd.Process == nil || p.waited.Load() || now.Sub(p.unfroze) < unfreezeGap {
		return
	}
	if r.gate == nil || !r.gate.stuck(now, fed, limit) {
		return
	}
	p.unfroze = now
	p.restarted = false
	slog.Warn(fmt.Sprintf("rendition %s on %s wrote nothing for %s while the tuner sent; starting the ladder again", r.spec.Key(), f.channel.GuideNumber, now.Sub(time.Unix(0, r.gate.moved.Load())).Round(time.Second)))
	_ = p.cmd.Process.Kill()
}

// watchLadder restarts the whole group when the one process dies. A child
// has no process of its own.
func (h *Hub) watchLadder(f *feed, primary *rendition, cmd *exec.Cmd, encoder string, respawn bool) {
	if cmd == nil || cmd.Process == nil || primary == nil {
		return
	}
	pid := cmd.Process.Pid
	started := time.Now()
	err := cmd.Wait()
	h.mu.Lock()
	replaced := primary.cmd != cmd
	rungs := []*rendition(nil)
	if primary.group != nil {
		rungs = primary.group.rungs
	}
	if !replaced {
		primary.waited.Store(true)
	}
	since := primary.respawned
	h.mu.Unlock()
	ForgetPID(h.Dir, pid)
	if replaced {
		slog.Info(fmt.Sprintf("rendition %s on %s: the ladder before the break ended after %s", primary.spec.Key(), f.channel.GuideNumber, time.Since(since).Round(10*time.Millisecond)))
		return
	}
	waitLadderPacks(rungs)
	h.mu.Lock()
	defer h.mu.Unlock()
	if f.renditions[primary.spec.Key()] != primary || primary.cmd != cmd {
		return
	}
	early := err != nil && !respawn && time.Since(started) <= h.fallbackWindow()
	canFall := !primary.fallback && gpuEncoder(encoder)
	if err != nil && !primary.restarted && h.restartLadderLocked(f, primary, early && canFall) {
		slog.Info(fmt.Sprintf("ladder %s on %s restarted after %s", primary.spec.Key(), f.channel.GuideNumber, time.Since(started).Round(time.Millisecond)))
		return
	}
	primary.cmd = nil
	primary.stdin = nil
	slog.Error(fmt.Sprintf("ladder %s on %s released after %s: %v", primary.spec.Key(), f.channel.GuideNumber, time.Since(started).Round(time.Millisecond), err))
	h.stopLadderLocked(f, primary)
	h.dropIfUnusedLocked(f)
}

// followLadderBreak starts the same multi-output command again after a
// backwards timestamp and feeds each rung's packager. One stdout encode
// cannot carry the other rungs.
func (h *Hub) followLadderBreak(f *feed, primary *rendition, p *programPipe) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if primary == nil || primary.group == nil || f.renditions[primary.spec.Key()] != primary || primary.filter != p || primary.cmd == nil {
		return
	}
	if wait := respawnGap - time.Since(primary.respawned); wait > 0 {
		time.AfterFunc(wait, func() { h.followLadderBreak(f, primary, p) })
		return
	}
	rungs := primary.group.rungs
	fail := func(err error) {
		slog.Error(fmt.Sprintf("ladder %s on %s: start again: %v", primary.spec.Key(), f.channel.GuideNumber, err))
		if !h.restartLadderLocked(f, primary, false) {
			h.stopLadderLocked(f, primary)
			h.dropIfUnusedLocked(f)
		}
	}
	for _, r := range rungs {
		if r == nil || r.input == nil {
			fail(errors.New("a rung's packager has stopped"))
			return
		}
	}
	cmd := exec.Command(h.FFmpeg, primary.args...)
	cmd.Dir = primary.dir
	cmd.Stdout = io.Discard
	pipes, err := ladderOutputs(cmd, len(rungs))
	if err != nil {
		fail(err)
		return
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		closeLadderPipes(pipes)
		fail(err)
		return
	}
	logCommand(cmd, "ladder")
	if err := cmd.Start(); err != nil {
		closeLadderPipes(pipes)
		_ = stdin.Close()
		fail(err)
		return
	}
	for i, r := range rungs {
		if !r.input.follow(pipes[i]) {
			_ = stdin.Close()
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
			closeLadderPipes(pipes)
			fail(errors.New("the packager has stopped"))
			return
		}
	}
	NotePID(h.Dir, cmd.Process.Pid)
	if p.sw != nil {
		p.sw.give(stdin)
	}
	old := primary.cmd
	time.AfterFunc(10*time.Second, func() {
		if old != nil && old.Process != nil {
			_ = old.Process.Kill()
		}
	})
	primary.cmd = cmd
	primary.stdin = stdin
	primary.waited.Store(false)
	primary.respawned = time.Now()
	slog.Info(fmt.Sprintf("ladder %s on %s started again after a timestamp break", primary.spec.Key(), f.channel.GuideNumber))
	go h.watchLadder(f, primary, cmd, ladderEncoder(h.Encoder), true)
}

// rebuildLaddersLocked restarts a ladder whose graph changed, once for the
// group. Rebuilding each rung on its own would start a second encode per
// size and point the shorter URLs at the primary.
func (h *Hub) rebuildLaddersLocked(f *feed) {
	type kept struct {
		spec    Rendition
		viewers int
		seen    time.Time
		guess   bool
	}
	seen := map[*ladderGroup]bool{}
	var list []kept
	for _, r := range f.renditions {
		if r == nil || !r.spec.ABR || r.ladderChild || r.group == nil || seen[r.group] {
			continue
		}
		seen[r.group] = true
		_, next := h.ladderCommandLocked(f, r.spec, "")
		if slices.Equal(r.args, next) {
			continue
		}
		list = append(list, kept{r.spec, r.viewers, r.seen, r.guess})
		h.stopLadderLocked(f, r)
	}
	for _, k := range list {
		r, err := h.ensureLadderLocked(f, k.spec, time.Time{})
		if err != nil {
			slog.Error(fmt.Sprintf("rebuild ladder %s on %s: %v", k.spec.Key(), f.channel.GuideNumber, err))
			continue
		}
		r.viewers, r.seen, r.guess = k.viewers, k.seen, k.guess
		if r.viewers == 0 {
			h.idleLocked(f.channel.ID, r.spec.Key(), r)
		} else {
			stopTimer(&r.idle)
		}
	}
	// A failed start has already removed the ladder. Nothing else is watching,
	// so the tuner goes with it. A start that worked still has its renditions.
	if len(list) > 0 {
		h.dropIfUnusedLocked(f)
	}
}

// LadderPlaylist is ladder.m3u8. It waits briefly for the primary init the
// way the audio master waits for its header.
func (h *Hub) LadderPlaylist(ctx context.Context, channelID int64, key string) ([]byte, error) {
	deadline := time.Now().Add(masterWait)
	for {
		body, gate, err := h.ladderOnce(channelID, key)
		if !errors.Is(err, os.ErrNotExist) || gate == nil || ctx.Err() != nil || !time.Now().Before(deadline) {
			return body, err
		}
		started := time.Now()
		gate.wait(0, -1, min(250*time.Millisecond, time.Until(deadline)))
		if time.Since(started) < 50*time.Millisecond {
			time.Sleep(100 * time.Millisecond)
		}
	}
}

func (h *Hub) ladderOnce(channelID int64, key string) ([]byte, *playlistGate, error) {
	h.mu.Lock()
	f := h.channels[channelID]
	if f == nil {
		h.mu.Unlock()
		return nil, nil, os.ErrNotExist
	}
	r := f.renditions[key]
	if r == nil || r.group == nil {
		h.mu.Unlock()
		return nil, nil, os.ErrNotExist
	}
	primary := r
	if p := ladderPrimaryOf(r); p != nil {
		primary = p
	}
	specs := make([]Rendition, 0, len(primary.group.rungs))
	dirs := make([]string, 0, len(primary.group.rungs))
	for _, mate := range primary.group.rungs {
		if mate == nil {
			continue
		}
		specs = append(specs, mate.spec)
		dirs = append(dirs, mate.dir)
	}
	src := f.source
	captions := f.captions != nil
	gate := primary.gate
	h.mu.Unlock()
	body, err := ladderPlaylistFromDisk(r.spec, specs, dirs, src, captions)
	return body, gate, err
}

package live

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"broadwave/internal/fetchguard"
	"broadwave/internal/store"
)

type probeReport struct {
	Programs []struct {
		ProgramNum int           `json:"program_num"`
		Streams    []probeStream `json:"streams"`
	} `json:"programs"`
	Streams []probeStream `json:"streams"`
}

type probeStream struct {
	CodecType  string `json:"codec_type"`
	CodecName  string `json:"codec_name"`
	FieldOrder string `json:"field_order"`
}

const probeEntries = "program=program_num:program_stream=codec_type,codec_name,field_order:stream=codec_type,codec_name,field_order"

// codecsFrom reads one program's first video and audio codec from ffprobe JSON.
func codecsFrom(raw []byte, program int) (video, audio string) {
	var rep probeReport
	if json.Unmarshal(raw, &rep) != nil {
		return "", ""
	}
	pick := func(streams []probeStream) {
		for _, s := range streams {
			switch {
			case s.CodecType == "video" && video == "":
				video = s.CodecName
			case s.CodecType == "audio" && audio == "":
				audio = s.CodecName
			}
		}
	}
	for _, p := range rep.Programs {
		if program == 0 || p.ProgramNum == program {
			pick(p.Streams)
			break
		}
	}
	if program == 0 {
		pick(rep.Streams)
	}
	return video, audio
}

// fieldOrderFrom picks the video field order for one program from ffprobe JSON.
func fieldOrderFrom(raw []byte, program int) string {
	var rep probeReport
	if json.Unmarshal(raw, &rep) != nil {
		return ""
	}
	pick := func(streams []probeStream) string {
		for _, s := range streams {
			if s.CodecType == "video" && s.FieldOrder != "" && s.FieldOrder != "unknown" {
				return s.FieldOrder
			}
		}
		return ""
	}
	for _, p := range rep.Programs {
		if program == 0 || p.ProgramNum == program {
			if fo := pick(p.Streams); fo != "" {
				return fo
			}
		}
	}
	if program == 0 {
		return pick(rep.Streams)
	}
	return ""
}

// probeFieldOrderLocked learns whether a channel is progressive, which decides
// whether its picture can be delivered untouched. It reads from the shared tune.
func (h *Hub) probeFieldOrderLocked(m *mux, f *feed) {
	tool := FFProbePath(h.FFmpeg)
	if tool == "" || h.Store == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	cmd := exec.CommandContext(ctx, tool, "-v", "error", "-probesize", "4000000", "-analyzeduration", "3000000",
		"-show_entries", probeEntries,
		"-of", "json", "-i", "pipe:0")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return
	}
	var out strings.Builder
	cmd.Stdout = &out
	if err := cmd.Start(); err != nil {
		cancel()
		return
	}
	sub := h.attachPipeLocked(m, stdin)
	f.probing = true
	channelID, program := f.channel.ID, f.program
	go func() {
		defer cancel()
		_ = cmd.Wait()
		order := fieldOrderFrom([]byte(out.String()), program)
		video, audio := codecsFrom([]byte(out.String()), program)
		h.mu.Lock()
		defer h.mu.Unlock()
		m.detach(sub)
		f.probing = false
		if h.channels[channelID] == f {
			if len(f.tracks) > 0 {
				audio = ""
			}
			if h.learnCodecsLocked(f, video, audio) {
				h.rebuildRenditionsLocked(f)
			}
		} else {
			ch := f.channel
			go h.saveCodecs(ch, video, audio)
		}
		// The packet scan is the one that can see film. A probe that lands
		// first still corrects a rendition that started on the wrong graph.
		if f.headerOrder != "" {
			if h.channels[channelID] == f {
				h.dropIfUnusedLocked(f)
			}
			return
		}
		if h.channels[channelID] != f {
			stored := storedFieldOrder(order)
			if stored != "" && h.Store != nil {
				_ = h.Store.SetFieldOrder(context.Background(), channelID, stored)
			}
			return
		}
		h.applyProbeLocked(f, order)
		h.dropIfUnusedLocked(f)
	}()
}

// probeInputLocked learns the field order of a URL the rendition reads itself
// (HLS, and any other source that is not a tuner pipe). The packet scan never
// sees those bytes, so an empty order would otherwise stick for the life of
// the channel. Unscanned H.264 is not doubled while this runs.
func (h *Hub) probeInputLocked(m *mux, f *feed) {
	if m == nil || m.input == "" || f == nil || !probeInput(f.channel) {
		return
	}
	tool := FFProbePath(h.FFmpeg)
	if tool == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	args := []string{"-v", "error", "-probesize", "2000000", "-analyzeduration", "1500000"}
	args = urlProtocols(args, m.input)
	args = append(args, headerArgs(f.source.UserAgent, f.source.Referrer)...)
	args = append(args,
		"-show_entries", probeEntries,
		"-of", "json", "-i", m.input)
	cmd := exec.CommandContext(ctx, tool, args...)
	var out strings.Builder
	cmd.Stdout = &out
	if err := cmd.Start(); err != nil {
		cancel()
		return
	}
	f.probing = true
	channelID, program := f.channel.ID, f.program
	input := m.input
	ua, ref := f.source.UserAgent, f.source.Referrer
	scanned := f.channel.FieldOrder != ""
	go func() {
		defer cancel()
		_ = cmd.Wait()
		order := fieldOrderFrom([]byte(out.String()), program)
		video, audio := codecsFrom([]byte(out.String()), program)
		// The HLS demuxer leaves field_order off the playlist. The segment has it.
		// The playlist read often uses its whole timeout, so the segment gets
		// its own. A child of ctx is already cancelled by then.
		if order == "" && !scanned && hlsURL(input) {
			if alt := hlsProbeTarget(input, ua, ref); alt != "" {
				segCtx, segCancel := hlsSegmentProbeContext()
				order = ffprobeFieldOrder(segCtx, tool, alt, ua, ref, 0)
				segCancel()
			}
		}
		h.mu.Lock()
		defer h.mu.Unlock()
		if cur := h.channels[channelID]; cur == f {
			f.probing = false
			if h.learnCodecsLocked(f, video, audio) {
				h.rebuildRenditionsLocked(f)
			}
		} else {
			ch := f.channel
			go h.saveCodecs(ch, video, audio)
		}
		if f.headerOrder != "" {
			return
		}
		if h.channels[channelID] != f {
			stored := storedFieldOrder(order)
			if stored != "" && h.Store != nil {
				_ = h.Store.SetFieldOrder(context.Background(), channelID, stored)
			}
			return
		}
		h.applyProbeLocked(f, order)
		h.dropIfUnusedLocked(f)
	}()
}

// probeInput is a URL input whose field order is not stored, or a link whose
// codecs are not. A link's stream is the only record of its codecs.
func probeInput(ch store.SourceChannel) bool {
	return ch.FieldOrder == "" || linkChannel(ch) && (ch.VideoCodec == "" || ch.AudioCodec == "")
}

// hlsSegmentProbeContext is a fresh 8s budget for the segment read. It is
// not a child of the playlist probe: that context is often already done.
func hlsSegmentProbeContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 8*time.Second)
}

func ffprobeFieldOrder(ctx context.Context, tool, input, userAgent, referrer string, program int) string {
	args := []string{"-v", "error", "-probesize", "2000000", "-analyzeduration", "1500000"}
	args = urlProtocols(args, input)
	args = append(args, headerArgs(userAgent, referrer)...)
	args = append(args,
		"-show_entries", probeEntries,
		"-of", "json", "-i", input)
	out, err := exec.CommandContext(ctx, tool, args...).Output()
	if err != nil {
		return ""
	}
	return fieldOrderFrom(out, program)
}

// hlsProbeTarget is the media file ffprobe can read a field order from.
// A playlist probe omits it. A transport-stream segment has it, and an fMP4
// playlist needs the init segment in front of the first media segment.
func hlsProbeTarget(raw, userAgent, referrer string) string {
	return allowedProbeTarget(hlsProbeTargetDepth(raw, userAgent, referrer, 0))
}

// allowedProbeTarget drops a target ffmpeg would open with a scheme other
// than http or https. A remote playlist can name a file on this machine.
func allowedProbeTarget(raw string) string {
	if raw == "" {
		return ""
	}
	for _, part := range strings.Split(raw, "|") {
		part = strings.TrimPrefix(part, "concat:")
		if inputHasScheme(part) && !strings.Contains(part, "://") {
			return ""
		}
		if strings.Contains(part, "://") && fetchguard.Allowed(part) != nil {
			return ""
		}
	}
	return raw
}

func hlsProbeTargetDepth(raw, userAgent, referrer string, depth int) string {
	if depth > 3 {
		return ""
	}
	text, err := readProbePlaylist(raw, userAgent, referrer)
	if err != nil {
		return ""
	}
	variant, segment, init := playlistEntries(text)
	if variant != "" {
		next := resolveMedia(raw, variant)
		if next == "" || next == raw {
			return ""
		}
		return hlsProbeTargetDepth(next, userAgent, referrer, depth+1)
	}
	if segment == "" {
		return ""
	}
	seg := resolveMedia(raw, segment)
	if init == "" {
		return seg
	}
	initPath := resolveMedia(raw, init)
	if initPath == "" || seg == "" || strings.Contains(initPath, "://") || strings.Contains(seg, "://") {
		return ""
	}
	return "concat:" + initPath + "|" + seg
}

func playlistEntries(text string) (variant, segment, init string) {
	master := strings.Contains(text, "#EXT-X-STREAM-INF")
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(strings.TrimRight(line, "\r"))
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "#EXT-X-MAP:") {
			init = uriAttr(line)
			continue
		}
		if strings.HasPrefix(line, "#") {
			continue
		}
		if master {
			return line, "", ""
		}
		return "", line, init
	}
	return "", "", init
}

func uriAttr(line string) string {
	const key = `URI="`
	i := strings.Index(line, key)
	if i < 0 {
		return ""
	}
	rest := line[i+len(key):]
	j := strings.Index(rest, `"`)
	if j < 0 {
		return ""
	}
	return rest[:j]
}

func resolveMedia(base, ref string) string {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return ""
	}
	if strings.Contains(ref, "://") {
		return ref
	}
	if u, err := url.Parse(base); err == nil && u.Scheme != "" && u.Host != "" {
		r, err := url.Parse(ref)
		if err != nil {
			return ""
		}
		return u.ResolveReference(r).String()
	}
	if filepath.IsAbs(ref) {
		return ref
	}
	return filepath.Join(filepath.Dir(base), filepath.FromSlash(ref))
}

func readProbePlaylist(raw, userAgent, referrer string) (string, error) {
	if strings.Contains(raw, "://") {
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
		defer cancel()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
		if err != nil {
			return "", err
		}
		if userAgent != "" {
			req.Header.Set("User-Agent", fetchguard.OneLine(userAgent))
		}
		if referrer != "" {
			req.Header.Set("Referer", fetchguard.OneLine(referrer))
		}
		res, err := fetchguard.Do(req, 4*time.Second)
		if err != nil {
			return "", err
		}
		defer res.Body.Close()
		if res.StatusCode != http.StatusOK {
			return "", fmt.Errorf("playlist returned %s", res.Status)
		}
		buf, err := io.ReadAll(io.LimitReader(res.Body, 512<<10))
		return string(buf), err
	}
	b, err := os.ReadFile(raw)
	if err != nil {
		return "", err
	}
	if len(b) > 512<<10 {
		b = b[:512<<10]
	}
	return string(b), nil
}

package live

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// recordingTargetFloor is the target duration a recording playlist advertises
// from the first segment. Field-rate pictures run 2.002s, over a target of 2,
// and Apple requires every EXTINF to be at most the target duration. A late
// keyframe makes one segment near 4s. AVPlayer drops the item when a reload
// changes the target (CoreMedia -12642), so the first response is already
// large enough to keep.
const recordingTargetFloor = 4

// PlaylistIssue is one HLS rule AVPlayer enforces on a recording playlist.
// A playlist that still has one of these fails the item instead of playing.
type PlaylistIssue struct {
	Rule   string
	Detail string
}

func (p PlaylistIssue) String() string {
	if p.Detail == "" {
		return p.Rule
	}
	return p.Rule + ": " + p.Detail
}

type mediaSeg struct {
	discont bool
	gap     bool
	pdt     string
	dur     float64
	uri     string
}

type mediaPlaylist struct {
	version     int
	target      int
	sequence    int
	hasSeq      bool
	kind        string
	independent bool
	start       string
	mapURI      string
	mapSeen     bool
	mapChanged  bool
	ended       bool
	tail        bool
	dangling    bool
	segs        []mediaSeg
}

// CheckRecordingPlaylist reports the rules AVPlayer enforces on one recording
// media playlist. dir, when set, is where each listed segment file must
// already exist. Gap segments are synthesized and are not on disk.
func CheckRecordingPlaylist(body []byte, dir string) []PlaylistIssue {
	pl, ok := parseMediaPlaylist(body)
	if !ok {
		return []PlaylistIssue{{Rule: "extm3u", Detail: "playlist must start with #EXTM3U"}}
	}
	return pl.issues(dir)
}

// CheckRecordingUpdate reports a reload that would make AVPlayer fail an item
// it had already accepted. An event playlist may only grow at the end.
func CheckRecordingUpdate(prev, next []byte) []PlaylistIssue {
	a, aok := parseMediaPlaylist(prev)
	b, bok := parseMediaPlaylist(next)
	if !aok || !bok {
		return []PlaylistIssue{{Rule: "extm3u", Detail: "playlist must start with #EXTM3U"}}
	}
	var out []PlaylistIssue
	if a.target != b.target {
		out = append(out, PlaylistIssue{Rule: "target-changed", Detail: fmt.Sprintf("%d to %d", a.target, b.target)})
	}
	if a.sequence != b.sequence {
		out = append(out, PlaylistIssue{Rule: "event", Detail: "media sequence changed"})
	}
	if a.ended && !b.ended {
		out = append(out, PlaylistIssue{Rule: "endlist", Detail: "end list removed"})
	}
	if a.mapSeen != b.mapSeen || a.mapURI != b.mapURI {
		out = append(out, PlaylistIssue{Rule: "map-changed", Detail: b.mapURI})
	}
	if len(b.segs) < len(a.segs) {
		out = append(out, PlaylistIssue{Rule: "event", Detail: "segment removed"})
	}
	n := min(len(a.segs), len(b.segs))
	for i := 0; i < n; i++ {
		if a.segs[i].uri != b.segs[i].uri {
			out = append(out, PlaylistIssue{Rule: "event", Detail: "segment order changed"})
			break
		}
		if math.Abs(a.segs[i].dur-b.segs[i].dur) > 0.001 {
			out = append(out, PlaylistIssue{Rule: "duration-changed", Detail: a.segs[i].uri})
		}
		if a.segs[i].discont != b.segs[i].discont {
			out = append(out, PlaylistIssue{Rule: "discontinuity-changed", Detail: a.segs[i].uri})
		}
	}
	if a.ended && len(b.segs) != len(a.segs) {
		out = append(out, PlaylistIssue{Rule: "event", Detail: "playlist grew after the end list"})
	}
	return out
}

// RepairRecordingPlaylist rewrites a recording playlist so it passes
// CheckRecordingPlaylist. prev is the playlist last served for this encode.
// The served prefix (target, durations, discontinuity tags, map) stays, and
// only complete new segments are appended. dir drops a segment whose file is
// not on disk yet, so a player is not sent a link that 404s.
func RepairRecordingPlaylist(raw, prev []byte, dir string) []byte {
	cur, ok := parseMediaPlaylist(raw)
	if !ok {
		return nil
	}
	var out mediaPlaylist
	var good bool
	if old, ok := parseMediaPlaylist(prev); ok && len(old.segs) > 0 && len(cur.segs) > 0 && old.segs[0].uri == cur.segs[0].uri {
		out, good = mergeRecording(old, cur, dir)
	} else {
		out, good = normalizeRecording(cur, dir, 0)
	}
	if !good {
		return nil
	}
	body := renderMediaPlaylist(out)
	if len(parseIssues(body)) > 0 {
		return nil
	}
	return body
}

// StableRecordingPlaylist is the playlist served for this encode. A reload
// keeps the prefix the player has already accepted.
func (h *Hub) StableRecordingPlaylist(dir string, raw []byte) []byte {
	h.playMu.Lock()
	defer h.playMu.Unlock()
	var prev []byte
	if h.fileList != nil {
		prev = h.fileList[dir]
	}
	use := ""
	if recordingSegmentsExist(dir) {
		use = dir
	}
	next := RepairRecordingPlaylist(raw, prev, use)
	if len(next) == 0 {
		if len(prev) > 0 {
			return append([]byte(nil), prev...)
		}
		return raw
	}
	if h.fileList == nil {
		h.fileList = map[string][]byte{}
	}
	h.fileList[dir] = append([]byte(nil), next...)
	return next
}

func (h *Hub) forgetRecordingPlaylist(dir string) {
	h.playMu.Lock()
	delete(h.fileList, dir)
	h.playMu.Unlock()
}

func parseIssues(body []byte) []PlaylistIssue {
	pl, ok := parseMediaPlaylist(body)
	if !ok {
		return []PlaylistIssue{{Rule: "extm3u"}}
	}
	return pl.issues("")
}

func (pl mediaPlaylist) issues(dir string) []PlaylistIssue {
	var out []PlaylistIssue
	if pl.version < neededVersion(pl) {
		out = append(out, PlaylistIssue{Rule: "version", Detail: fmt.Sprintf("%d", pl.version)})
	}
	if pl.target < 1 {
		out = append(out, PlaylistIssue{Rule: "target", Detail: "missing"})
	}
	if !pl.hasSeq || pl.sequence < 0 {
		out = append(out, PlaylistIssue{Rule: "media-sequence", Detail: "missing"})
	}
	if pl.kind != "EVENT" && pl.kind != "VOD" {
		out = append(out, PlaylistIssue{Rule: "playlist-type", Detail: "event or vod required"})
	}
	if pl.kind == "VOD" && !pl.ended {
		out = append(out, PlaylistIssue{Rule: "playlist-type", Detail: "vod without an end list"})
	}
	if pl.dangling {
		out = append(out, PlaylistIssue{Rule: "extinf", Detail: "tag is not followed by a segment"})
	}
	if pl.tail {
		out = append(out, PlaylistIssue{Rule: "endlist", Detail: "lines follow the end list"})
	}
	if len(pl.segs) == 0 {
		out = append(out, PlaylistIssue{Rule: "extinf", Detail: "no segment"})
	}
	if len(pl.segs) > 0 && pl.segs[0].discont {
		out = append(out, PlaylistIssue{Rule: "discontinuity", Detail: "before the first segment"})
	}
	kind := segmentKinds(pl.segs)
	switch {
	case kind == "mixed":
		out = append(out, PlaylistIssue{Rule: "map", Detail: "mpeg-ts and fmp4 in one playlist"})
	case kind == "ts" && pl.mapSeen:
		out = append(out, PlaylistIssue{Rule: "map", Detail: "mpeg-ts playlist must not carry EXT-X-MAP"})
	case kind == "fmp4" && !pl.mapSeen:
		out = append(out, PlaylistIssue{Rule: "map", Detail: "fmp4 playlist needs EXT-X-MAP"})
	case pl.mapChanged:
		out = append(out, PlaylistIssue{Rule: "map-changed", Detail: "map changed without a discontinuity"})
	}
	seen := map[string]bool{}
	for i, s := range pl.segs {
		if s.uri == "" || s.dur <= 0 {
			out = append(out, PlaylistIssue{Rule: "extinf", Detail: s.uri})
			continue
		}
		if seen[s.uri] {
			out = append(out, PlaylistIssue{Rule: "uri", Detail: "duplicate " + s.uri})
		}
		seen[s.uri] = true
		if pl.target >= 1 && s.dur > float64(pl.target)+1e-3 {
			out = append(out, PlaylistIssue{Rule: "extinf", Detail: fmt.Sprintf("%.3f exceeds target %d", s.dur, pl.target)})
		}
		if s.pdt != "" && !rfc3339Date(s.pdt) {
			out = append(out, PlaylistIssue{Rule: "date", Detail: s.pdt})
		}
		if i > 0 && gapURI(pl.segs[i-1].uri) && !gapURI(s.uri) && !s.discont {
			out = append(out, PlaylistIssue{Rule: "discontinuity", Detail: "missing after a gap"})
		}
		if dir != "" && !gapURI(s.uri) && !segmentReady(dir, s.uri) {
			out = append(out, PlaylistIssue{Rule: "file", Detail: s.uri})
		}
	}
	return out
}

func neededVersion(pl mediaPlaylist) int {
	need := 3
	if pl.independent || pl.mapSeen {
		need = 6
	}
	if segmentKinds(pl.segs) == "fmp4" {
		need = 7
	}
	for _, s := range pl.segs {
		if s.gap || gapURI(s.uri) {
			need = 8
			break
		}
	}
	if pl.version > need {
		return pl.version
	}
	return need
}

func normalizeRecording(pl mediaPlaylist, dir string, hold int) (mediaPlaylist, bool) {
	var segs []mediaSeg
	limit := hold
	for i, s := range pl.segs {
		if s.uri == "" || s.dur <= 0 {
			pl.ended = false
			break
		}
		if i == 0 {
			s.discont = false
		}
		if dir != "" && !gapURI(s.uri) && !segmentReady(dir, s.uri) {
			pl.ended = false
			break
		}
		if limit > 0 && s.dur > float64(limit)+1e-3 {
			pl.ended = false
			break
		}
		s.pdt = canonicalDate(s.pdt)
		segs = append(segs, s)
	}
	if len(segs) == 0 {
		return mediaPlaylist{}, false
	}
	for i := 1; i < len(segs); i++ {
		if gapURI(segs[i-1].uri) && !gapURI(segs[i].uri) {
			segs[i].discont = true
		}
	}
	pl.segs = segs
	if hold > 0 {
		pl.target = hold
	} else {
		pl.target = fitTarget(segs)
	}
	pl.sequence = 0
	pl.hasSeq = true
	pl.kind = "EVENT"
	pl.dangling = false
	pl.tail = false
	pl.mapChanged = false
	if segmentKinds(segs) == "ts" {
		pl.mapSeen = false
		pl.mapURI = ""
	}
	pl.version = neededVersion(pl)
	return pl, true
}

func mergeRecording(prev, cur mediaPlaylist, dir string) (mediaPlaylist, bool) {
	out := prev
	out.ended = false
	out.dangling = false
	out.tail = false
	if cur.start != "" {
		out.start = cur.start
	}
	if cur.independent {
		out.independent = true
	}
	seen := make(map[string]bool, len(prev.segs))
	for _, s := range prev.segs {
		seen[s.uri] = true
	}
	for _, s := range cur.segs {
		if seen[s.uri] {
			continue
		}
		if s.uri == "" || s.dur <= 0 {
			break
		}
		if dir != "" && !gapURI(s.uri) && !segmentReady(dir, s.uri) {
			break
		}
		if s.dur > float64(out.target)+1e-3 {
			break
		}
		s.pdt = canonicalDate(s.pdt)
		if n := len(out.segs); n > 0 && gapURI(out.segs[n-1].uri) && !gapURI(s.uri) {
			s.discont = true
		}
		if len(out.segs) == 0 {
			s.discont = false
		}
		out.segs = append(out.segs, s)
		seen[s.uri] = true
	}
	if cur.ended {
		have := make(map[string]bool, len(out.segs))
		for _, s := range out.segs {
			have[s.uri] = true
		}
		all := true
		for _, s := range cur.segs {
			if !have[s.uri] {
				all = false
				break
			}
		}
		out.ended = all
	}
	out.version = neededVersion(out)
	return out, len(out.segs) > 0
}

func fitTarget(segs []mediaSeg) int {
	t := recordingTargetFloor
	for _, s := range segs {
		n := int(math.Ceil(s.dur - 1e-9))
		if n > t {
			t = n
		}
	}
	return t
}

func renderMediaPlaylist(pl mediaPlaylist) []byte {
	var b strings.Builder
	b.WriteString("#EXTM3U\n")
	fmt.Fprintf(&b, "#EXT-X-VERSION:%d\n", pl.version)
	fmt.Fprintf(&b, "#EXT-X-TARGETDURATION:%d\n", pl.target)
	fmt.Fprintf(&b, "#EXT-X-MEDIA-SEQUENCE:%d\n", pl.sequence)
	if pl.kind != "" {
		fmt.Fprintf(&b, "#EXT-X-PLAYLIST-TYPE:%s\n", pl.kind)
	}
	if pl.independent {
		b.WriteString("#EXT-X-INDEPENDENT-SEGMENTS\n")
	}
	if pl.start != "" {
		b.WriteString(pl.start)
		b.WriteByte('\n')
	}
	if pl.mapSeen && pl.mapURI != "" {
		fmt.Fprintf(&b, "#EXT-X-MAP:URI=%q\n", pl.mapURI)
	}
	for _, s := range pl.segs {
		if s.discont {
			b.WriteString("#EXT-X-DISCONTINUITY\n")
		}
		if s.pdt != "" {
			b.WriteString("#EXT-X-PROGRAM-DATE-TIME:")
			b.WriteString(s.pdt)
			b.WriteByte('\n')
		}
		if s.gap || gapURI(s.uri) {
			b.WriteString("#EXT-X-GAP\n")
		}
		fmt.Fprintf(&b, "#EXTINF:%s,\n%s\n", strconv.FormatFloat(s.dur, 'f', 3, 64), s.uri)
	}
	if pl.ended {
		b.WriteString("#EXT-X-ENDLIST\n")
	}
	return []byte(b.String())
}

func parseMediaPlaylist(body []byte) (mediaPlaylist, bool) {
	text := strings.ReplaceAll(string(body), "\r\n", "\n")
	var pl mediaPlaylist
	var pending mediaSeg
	var have bool
	started := false
	discontSinceMap := true
	for _, line := range strings.Split(text, "\n") {
		trim := strings.TrimSpace(line)
		if trim == "" {
			continue
		}
		if !started {
			if trim != "#EXTM3U" {
				return mediaPlaylist{}, false
			}
			started = true
			continue
		}
		if pl.ended {
			pl.tail = true
			continue
		}
		switch {
		case strings.HasPrefix(trim, "#EXT-X-VERSION:"):
			pl.version, _ = strconv.Atoi(strings.TrimPrefix(trim, "#EXT-X-VERSION:"))
		case strings.HasPrefix(trim, "#EXT-X-TARGETDURATION:"):
			pl.target, _ = strconv.Atoi(strings.TrimPrefix(trim, "#EXT-X-TARGETDURATION:"))
		case strings.HasPrefix(trim, "#EXT-X-MEDIA-SEQUENCE:"):
			pl.sequence, _ = strconv.Atoi(strings.TrimPrefix(trim, "#EXT-X-MEDIA-SEQUENCE:"))
			pl.hasSeq = true
		case strings.HasPrefix(trim, "#EXT-X-PLAYLIST-TYPE:"):
			pl.kind = strings.TrimPrefix(trim, "#EXT-X-PLAYLIST-TYPE:")
		case trim == "#EXT-X-INDEPENDENT-SEGMENTS":
			pl.independent = true
		case strings.HasPrefix(trim, "#EXT-X-START:"):
			pl.start = trim
		case strings.HasPrefix(trim, "#EXT-X-MAP:"):
			if pl.mapSeen && !discontSinceMap {
				pl.mapChanged = true
			}
			pl.mapSeen = true
			pl.mapURI = quotedURI(trim)
			discontSinceMap = false
		case trim == "#EXT-X-DISCONTINUITY":
			pending.discont = true
			discontSinceMap = true
			have = true
		case trim == "#EXT-X-GAP":
			pending.gap = true
			have = true
		case strings.HasPrefix(trim, "#EXT-X-PROGRAM-DATE-TIME:"):
			pending.pdt = strings.TrimPrefix(trim, "#EXT-X-PROGRAM-DATE-TIME:")
			have = true
		case strings.HasPrefix(trim, "#EXTINF:"):
			rest := strings.TrimPrefix(trim, "#EXTINF:")
			sec, err := strconv.ParseFloat(strings.SplitN(rest, ",", 2)[0], 64)
			if err == nil {
				pending.dur = sec
			}
			have = true
		case trim == "#EXT-X-ENDLIST":
			if have {
				pl.dangling = true
			}
			pl.ended = true
		case strings.HasPrefix(trim, "#"):
		default:
			pending.uri = trim
			pl.segs = append(pl.segs, pending)
			pending = mediaSeg{}
			have = false
		}
	}
	if !started {
		return mediaPlaylist{}, false
	}
	if have {
		pl.dangling = true
	}
	return pl, true
}

func quotedURI(line string) string {
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

func segmentKinds(segs []mediaSeg) string {
	ts, fmp4 := false, false
	for _, s := range segs {
		switch segmentKind(s.uri) {
		case "ts":
			ts = true
		case "fmp4":
			fmp4 = true
		}
	}
	switch {
	case ts && fmp4:
		return "mixed"
	case fmp4:
		return "fmp4"
	case ts:
		return "ts"
	default:
		return ""
	}
}

func segmentKind(uri string) string {
	ext := strings.ToLower(filepath.Ext(strings.SplitN(uri, "?", 2)[0]))
	switch ext {
	case ".ts":
		return "ts"
	case ".m4s", ".mp4", ".m4v", ".cmfv", ".cmfa":
		return "fmp4"
	default:
		return ""
	}
}

func gapURI(uri string) bool {
	base := filepath.Base(strings.SplitN(uri, "?", 2)[0])
	return strings.HasPrefix(base, "gap") && strings.HasSuffix(strings.ToLower(base), ".ts")
}

func segmentReady(dir, uri string) bool {
	base := filepath.Base(strings.SplitN(uri, "?", 2)[0])
	if base == "." || base == ".." {
		return false
	}
	info, err := os.Stat(filepath.Join(dir, base))
	return err == nil && info.Mode().IsRegular() && info.Size() > 0
}

func recordingSegmentsExist(dir string) bool {
	matches, err := filepath.Glob(filepath.Join(dir, "seg*.ts"))
	if err != nil {
		return false
	}
	for _, name := range matches {
		info, err := os.Stat(name)
		if err == nil && info.Size() > 0 {
			return true
		}
	}
	return false
}

func rfc3339Date(value string) bool {
	_, err := time.Parse(time.RFC3339Nano, value)
	return err == nil
}

func canonicalDate(value string) string {
	if value == "" {
		return ""
	}
	for _, layout := range []string{
		time.RFC3339Nano,
		"2006-01-02T15:04:05.000Z0700",
		"2006-01-02T15:04:05Z0700",
	} {
		if t, err := time.Parse(layout, value); err == nil {
			return t.UTC().Format("2006-01-02T15:04:05.000Z")
		}
	}
	return value
}

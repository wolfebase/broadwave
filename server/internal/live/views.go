package live

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

// A rendition is also served as views: video.m3u8 with the picture alone and
// audio-<id>.m3u8 with one sound track, cut from the same files on request.
// Every view has the sequence numbers, dates, durations, and parts of
// index.m3u8, so a player changes sound without touching the picture. The
// legacy names of an encode that carries other tracks are cut to the picture
// and the main sound: a player that asked for one sound never gets several.

var (
	viewFile = regexp.MustCompile(`^(init|seg\d+|part\d+)\.(v|a\d+)\.(mp4|m4s)$`)
	diskFile = regexp.MustCompile(`^(init\.mp4|seg\d+\.m4s|part\d+\.m4s)$`)
)

// IsViewPlaylist reports whether name is a view's playlist.
func IsViewPlaylist(name string) bool {
	_, ok := playlistTag(name)
	return ok
}

// ViewPlaylist answers video.m3u8 and audio-<id>.m3u8. skip asks for the delta
// form. ok is false for any other name.
func (h *Hub) ViewPlaylist(channelID int64, key, name string, skip bool) (body []byte, ok bool, err error) {
	tag, ok := playlistTag(name)
	if !ok {
		return nil, false, nil
	}
	h.mu.Lock()
	var audio []uint32
	found := false
	if f := h.channels[channelID]; f != nil {
		if r := f.renditions[key]; r != nil {
			found = true
			audio = soundTracks(r)
		}
	}
	h.mu.Unlock()
	if !found {
		return nil, true, os.ErrNotExist
	}
	var siblings []string
	if tag != "v" {
		id, _ := strconv.ParseUint(tag[1:], 10, 32)
		if !slices.Contains(audio, uint32(id)) {
			return nil, true, os.ErrNotExist
		}
		siblings = append(siblings, "video.m3u8")
	}
	for _, id := range audio {
		if "a"+strconv.Itoa(int(id)) != tag {
			siblings = append(siblings, fmt.Sprintf("audio-%d.m3u8", id))
		}
	}
	full, err := h.Playlist(channelID, key)
	if err != nil {
		return nil, true, err
	}
	// An MPEG-TS playlist has one file per segment and nothing to cut.
	if !bytes.Contains(full, []byte("#EXT-X-MAP:")) {
		return nil, true, os.ErrNotExist
	}
	src := full
	if skip {
		src = DeltaPlaylist(full)
	}
	return viewPlaylist(src, tag, renditionReports(full, siblings)), true, nil
}

// ViewMedia answers a view's init, segment, and part names, and the legacy
// names of an encode that carries other sound tracks. ok is false when the
// file on disk is served as it is.
func (h *Hub) ViewMedia(channelID int64, key, name string) (body []byte, ok bool, err error) {
	file, tag := name, ""
	if m := viewFile.FindStringSubmatch(name); m != nil {
		tag = m[2]
		file = m[1] + "." + m[3]
		if (m[1] == "init") != (m[3] == "mp4") {
			return nil, true, os.ErrNotExist
		}
	}
	if !diskFile.MatchString(file) {
		return nil, false, nil
	}
	// Right after an encode restarts without its extras, the old init can
	// still be on disk for a moment. The cut follows the init, not r.extras.
	h.mu.Lock()
	var dir string
	var extras bool
	if f := h.channels[channelID]; f != nil {
		if r := f.renditions[key]; r != nil {
			dir, extras = r.dir, len(r.extras) > 0
		}
	}
	h.mu.Unlock()
	if tag == "" && !extras {
		return nil, false, nil
	}
	if dir == "" {
		return nil, true, os.ErrNotExist
	}
	init, err := h.cuts.file(filepath.Join(dir, "init.mp4"), "raw", func(b []byte) ([]byte, error) { return b, nil })
	if err != nil {
		return nil, true, err
	}
	ids, ok := viewTracks(init, tag)
	if !ok {
		return nil, true, os.ErrNotExist
	}
	cut := func(raw []byte) ([]byte, error) { return keepTracksFrag(raw, ids) }
	if file == "init.mp4" {
		cut = func(raw []byte) ([]byte, error) { return keepTracks(raw, ids) }
	}
	body, err = h.cuts.file(filepath.Join(dir, file), tag, cut)
	return body, true, err
}

// cutBudget bounds the bytes cutCache holds: a few segments of every view
// of a couple of full-size encodes.
const cutBudget = 64 << 20

type cutKey struct {
	path, tag string
	size      int64
	mod       time.Time
}

// cutCache is a small least-recently-used store of cut segments and parts.
// A restart rewrites a file under the same name, so size and time are in the key.
// Screens that a blocking reload releases together ask for the same cut at
// once; one of them reads and cuts, the others wait for it.
type cutCache struct {
	mu      sync.Mutex
	order   []cutKey
	cuts    map[cutKey][]byte
	pending map[cutKey]chan struct{}
	bytes   int
	misses  int
}

// file is the cut of path for tag, made at most once while it is cached.
func (c *cutCache) file(path, tag string, cut func([]byte) ([]byte, error)) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		if raw, ok := PartFromSegment(path); ok {
			return cut(raw)
		}
		return nil, err
	}
	k := cutKey{path: path, tag: tag, size: info.Size(), mod: info.ModTime()}
	for {
		c.mu.Lock()
		if body, ok := c.cuts[k]; ok {
			i := slices.Index(c.order, k)
			c.order = append(slices.Delete(c.order, i, i+1), k)
			c.mu.Unlock()
			return body, nil
		}
		wait, busy := c.pending[k]
		if !busy {
			if c.pending == nil {
				c.pending = map[cutKey]chan struct{}{}
			}
			c.pending[k] = make(chan struct{})
			c.misses++
			c.mu.Unlock()
			break
		}
		c.mu.Unlock()
		<-wait
	}
	raw, err := os.ReadFile(path)
	var body []byte
	if err == nil {
		body, err = cut(raw)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	close(c.pending[k])
	delete(c.pending, k)
	if err != nil {
		return nil, err
	}
	c.putLocked(k, body)
	return body, nil
}

func (c *cutCache) putLocked(k cutKey, body []byte) {
	if c.cuts == nil {
		c.cuts = map[cutKey][]byte{}
	}
	if _, ok := c.cuts[k]; ok || len(body) > cutBudget {
		return
	}
	c.cuts[k] = body
	c.order = append(c.order, k)
	c.bytes += len(body)
	for c.bytes > cutBudget {
		old := c.order[0]
		c.order = c.order[1:]
		c.bytes -= len(c.cuts[old])
		delete(c.cuts, old)
	}
}

// soundTracks are the fMP4 track ids of a rendition's sound: the main mix,
// then each extra.
func soundTracks(r *rendition) []uint32 {
	if r.spec.normalized().Audio == "none" {
		return nil
	}
	ids := []uint32{2}
	for i := range r.extras {
		ids = append(ids, extraTrackID(i))
	}
	return ids
}

func playlistTag(name string) (string, bool) {
	if name == "video.m3u8" {
		return "v", true
	}
	raw, ok := strings.CutPrefix(name, "audio-")
	if !ok {
		return "", false
	}
	raw, ok = strings.CutSuffix(raw, ".m3u8")
	if n, err := strconv.ParseUint(raw, 10, 32); !ok || err != nil || n < 2 || strconv.FormatUint(n, 10) != raw {
		return "", false
	}
	return "a" + raw, true
}

// viewTracks is the track list a view keeps. The legacy view (empty tag) is
// the picture and the lowest sound track, which is the main mix.
func viewTracks(init []byte, tag string) ([]uint32, bool) {
	video, _, ok := videoTrack(init)
	if !ok {
		return nil, false
	}
	var sound []uint32
	for id := range trackScales(init) {
		if id != video {
			sound = append(sound, id)
		}
	}
	slices.Sort(sound)
	switch {
	case tag == "v":
		return []uint32{video}, true
	case tag == "":
		if len(sound) == 0 {
			return []uint32{video}, true
		}
		return []uint32{video, sound[0]}, true
	}
	id, err := strconv.ParseUint(tag[1:], 10, 32)
	if err != nil || !slices.Contains(sound, uint32(id)) {
		return nil, false
	}
	return []uint32{uint32(id)}, true
}

// viewPlaylist renames a stamped playlist's media to a view's and adds the
// sibling views' reports.
func viewPlaylist(body []byte, tag string, reports []string) []byte {
	var b strings.Builder
	for line := range strings.SplitSeq(strings.TrimRight(string(body), "\n"), "\n") {
		switch {
		case strings.HasPrefix(line, "#EXT-X-MAP:"):
			line = strings.Replace(line, `"init.mp4"`, `"init.`+tag+`.mp4"`, 1)
		case strings.HasPrefix(line, "#EXT-X-PART:"):
			line = strings.Replace(line, `.m4s"`, `.`+tag+`.m4s"`, 1)
		case line != "" && !strings.HasPrefix(line, "#"):
			if base, ok := strings.CutSuffix(line, ".m4s"); ok {
				line = base + "." + tag + ".m4s"
			}
		}
		b.WriteString(line + "\n")
	}
	for _, r := range reports {
		b.WriteString(r + "\n")
	}
	return []byte(b.String())
}

// renditionReports points a player at the sibling views' newest media. The
// views are the same playlist, so the numbers are this one's.
func renditionReports(body []byte, siblings []string) []string {
	if len(siblings) == 0 {
		return nil
	}
	// A closed segment lists its parts before it, so the newest part may be
	// the last segment's own; a report must name it when there is one.
	origin, segments, parts, closedParts := -1, 0, 0, 0
	for line := range strings.SplitSeq(string(body), "\n") {
		switch {
		case strings.HasPrefix(line, "#EXT-X-MEDIA-SEQUENCE:"):
			origin, _ = strconv.Atoi(strings.TrimPrefix(line, "#EXT-X-MEDIA-SEQUENCE:"))
		case strings.HasPrefix(line, "#EXT-X-PART:"):
			parts++
		case line != "" && !strings.HasPrefix(line, "#"):
			segments++
			closedParts, parts = parts, 0
		}
	}
	if origin < 0 || segments+parts == 0 {
		return nil
	}
	last := fmt.Sprintf("LAST-MSN=%d", origin+segments-1)
	switch {
	case parts > 0:
		last = fmt.Sprintf("LAST-MSN=%d,LAST-PART=%d", origin+segments, parts-1)
	case closedParts > 0:
		last = fmt.Sprintf("LAST-MSN=%d,LAST-PART=%d", origin+segments-1, closedParts-1)
	}
	out := make([]string, len(siblings))
	for i, s := range siblings {
		out[i] = fmt.Sprintf(`#EXT-X-RENDITION-REPORT:URI="%s",%s`, s, last)
	}
	return out
}

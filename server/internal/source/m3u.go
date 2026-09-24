package source

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"waveguide/internal/hdhr"
	"waveguide/internal/store"
)

// Entry is one channel from an M3U playlist or a single stream link.
type Entry struct {
	Number        string
	Name          string
	URL           string
	ID            string
	Logo          string
	Group         string
	GuideURL      string
	Station       string
	Shift         string
	Catchup       string
	CatchupSource string
	CatchupDays   string
	GuideTitle    string
	GuideText     string
	GuideArt      string
	GuideTags     string
	GuideGenres   string
	Video         string
	Audio         string
	UserAgent     string
	Referrer      string
	DRM           bool
}

// ParseM3U reads an extended M3U playlist. Lines that are not channels are ignored.
func ParseM3U(r io.Reader) []Entry {
	var out []Entry
	var pending Entry
	var have bool
	var guideURL string
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	n := 1
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "#EXTM3U") {
			guideURL = attr(line, "url-tvg")
			if guideURL == "" {
				guideURL = attr(line, "x-tvg-url")
			}
			continue
		}
		if strings.HasPrefix(line, "#EXTINF:") {
			name := line
			if comma := strings.LastIndex(line, ","); comma >= 0 {
				name = strings.TrimSpace(line[comma+1:])
			}
			if quoted := attr(line, "tvg-name"); quoted != "" && name == "" {
				name = quoted
			}
			number := attr(line, "tvg-chno")
			if number == "" {
				number = attr(line, "channel-number")
			}
			id := attr(line, "tvg-id")
			if id == "" {
				id = attr(line, "channel-id")
			}
			pending = Entry{
				Number: number, Name: name, ID: id,
				Logo: attr(line, "tvg-logo"), Group: attr(line, "group-title"),
				Station: attr(line, "tvc-guide-stationid"),
				Shift:   attr(line, "tvg-shift"), Catchup: attr(line, "catchup"),
				CatchupSource: attr(line, "catchup-source"), CatchupDays: attr(line, "catchup-days"),
				GuideTitle: attr(line, "tvc-guide-title"), GuideText: attr(line, "tvc-guide-description"),
				GuideArt: attr(line, "tvc-guide-art"), GuideTags: attr(line, "tvc-guide-tags"),
				GuideGenres: attr(line, "tvc-guide-genres"),
				Video:       attr(line, "tvc-stream-vcodec"), Audio: attr(line, "tvc-stream-acodec"),
				DRM: drmFlag(attr(line, "drm")),
			}
			have = true
			continue
		}
		if strings.HasPrefix(line, "#EXTVLCOPT:") || strings.HasPrefix(line, "#KODIPROP:") {
			if have {
				applyStreamOpt(&pending, line)
			}
			continue
		}
		if strings.HasPrefix(line, "#") {
			continue
		}
		entry := pending
		if !have {
			entry = Entry{}
		}
		if entry.Name == "" {
			entry.Name = fmt.Sprintf("Stream %d", n)
		}
		if entry.Number == "" {
			entry.Number = fmt.Sprintf("%d", 800+n)
		}
		entry.URL = line
		entry.GuideURL = guideURL
		out = append(out, entry)
		pending = Entry{}
		have = false
		n++
	}
	return out
}

// FilterGroups keeps channels in the named groups. A name prefixed with - is excluded.
// An empty list leaves the playlist alone.
func FilterGroups(entries []Entry, groups string) []Entry {
	groups = strings.TrimSpace(groups)
	if groups == "" {
		return entries
	}
	var include, exclude []string
	for _, part := range strings.FieldsFunc(groups, func(r rune) bool { return r == ',' || r == ';' }) {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if strings.HasPrefix(part, "-") {
			exclude = append(exclude, strings.ToLower(strings.TrimPrefix(part, "-")))
			continue
		}
		include = append(include, strings.ToLower(part))
	}
	var out []Entry
	for _, entry := range entries {
		name := strings.ToLower(strings.TrimSpace(entry.Group))
		if listed(exclude, name) {
			continue
		}
		if len(include) > 0 && !listed(include, name) {
			continue
		}
		out = append(out, entry)
	}
	return out
}

func listed(names []string, group string) bool {
	for _, name := range names {
		if name == group {
			return true
		}
	}
	return false
}

// GroupNames lists the groups in a playlist, in first-seen order.
func GroupNames(entries []Entry) []string {
	seen := map[string]bool{}
	var out []string
	for _, entry := range entries {
		name := strings.TrimSpace(entry.Group)
		if name == "" || seen[strings.ToLower(name)] {
			continue
		}
		seen[strings.ToLower(name)] = true
		out = append(out, name)
	}
	return out
}

// BigPlaylistMessage asks the user to name groups before a long playlist is added.
// An empty string means the playlist can be added as it is.
func BigPlaylistMessage(entries []Entry) string {
	if len(entries) <= 300 {
		return ""
	}
	names := GroupNames(entries)
	if len(names) > 8 {
		names = append(names[:8], fmt.Sprintf("%d more", len(names)-8))
	}
	if len(names) == 0 {
		return fmt.Sprintf("This playlist has %d channels.", len(entries))
	}
	return fmt.Sprintf("This playlist has %d channels. Keep groups: %s.", len(entries), strings.Join(names, ", "))
}

// PlaylistPick is the list a person chooses from before a long playlist is added.
func PlaylistPick(entries []Entry, message string) map[string]any {
	groups := GroupNames(entries)
	out := map[string]any{"pick": true, "count": len(entries), "message": message, "groups": groups}
	if len(groups) > 0 {
		return out
	}
	channels := make([]map[string]string, 0, len(entries))
	for _, entry := range entries {
		channels = append(channels, map[string]string{"name": entry.Name, "id": entry.ID, "number": entry.Number})
	}
	out["channels"] = channels
	return out
}

// FilterKeep leaves the channels whose name or tvg-id is in the comma-separated list.
// An empty list keeps every channel.
func FilterKeep(entries []Entry, keep string) []Entry {
	want := map[string]bool{}
	for _, part := range strings.FieldsFunc(keep, func(r rune) bool { return r == ',' || r == ';' }) {
		part = strings.TrimSpace(part)
		if part != "" {
			want[strings.ToLower(part)] = true
		}
	}
	if len(want) == 0 {
		return entries
	}
	out := make([]Entry, 0, len(want))
	for _, entry := range entries {
		if want[strings.ToLower(entry.Name)] || (entry.ID != "" && want[strings.ToLower(entry.ID)]) {
			out = append(out, entry)
		}
	}
	return out
}

// GuideFromPlaylist uses the URL the user typed, or the one written on the playlist.
func GuideFromPlaylist(explicit string, entries []Entry) string {
	if strings.TrimSpace(explicit) != "" {
		return strings.TrimSpace(explicit)
	}
	for _, entry := range entries {
		if entry.GuideURL != "" {
			return entry.GuideURL
		}
	}
	return ""
}

func applyStreamOpt(e *Entry, line string) {
	_, rest, ok := strings.Cut(line, ":")
	if !ok {
		return
	}
	key, val, ok := strings.Cut(rest, "=")
	if !ok {
		return
	}
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "http-user-agent":
		e.UserAgent = strings.TrimSpace(val)
	case "http-referrer", "http-referer":
		e.Referrer = strings.TrimSpace(val)
	case "inputstream.adaptive.license_type":
		kind := strings.ToLower(strings.TrimSpace(val))
		if strings.Contains(kind, "widevine") || strings.Contains(kind, "playready") {
			e.DRM = true
		}
	}
}

func drmFlag(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes":
		return true
	default:
		return false
	}
}

func attr(line, key string) string {
	needle := key + `="`
	i := strings.Index(line, needle)
	if i < 0 {
		return ""
	}
	rest := line[i+len(needle):]
	end := strings.Index(rest, `"`)
	if end < 0 {
		return ""
	}
	return rest[:end]
}

// Install puts playlist entries on the guide as a tuner-free source.
func Install(ctx context.Context, st *store.Store, id int64, name, kind string, entries []Entry) error {
	if len(entries) == 0 {
		return fmt.Errorf("the playlist has no channels")
	}
	devID := fmt.Sprintf("src-%d", id)
	channels := make([]hdhr.Channel, 0, len(entries))
	for _, entry := range entries {
		key := entry.ID
		if key == "" {
			key = entry.Station
		}
		video, audio := entry.Video, entry.Audio
		if video == "" {
			video = "H264"
		}
		if audio == "" {
			audio = "AAC"
		}
		channels = append(channels, hdhr.Channel{
			GuideNumber: entry.Number,
			GuideName:   entry.Name,
			StreamURL:   entry.URL,
			VideoCodec:  video,
			AudioCodec:  audio,
			GuideKey:    key,
			ArtURL:      entry.Logo,
			UserAgent:   entry.UserAgent,
			Referrer:    entry.Referrer,
		})
	}
	label := name
	if label == "" {
		label = kind
	}
	return st.UpsertDevice(ctx, hdhr.Device{
		DeviceID: devID, FriendlyName: label, ModelNumber: kind, FirmwareName: "waveguide",
		BaseURL: "source", TunerCount: 0,
	}, channels)
}

// Renumber replaces channel numbers with a sequence starting at start.
// A start of 0 keeps the numbers from the playlist.
func Renumber(entries []Entry, start int) []Entry {
	if start <= 0 {
		return entries
	}
	out := make([]Entry, len(entries))
	for i, entry := range entries {
		entry.Number = strconv.Itoa(start + i)
		out[i] = entry
	}
	return out
}

// ReadPlaylist loads a playlist from an http address or a file on this machine.
// A gzip file is unpacked.
func ReadPlaylist(ctx context.Context, raw string) ([]byte, error) {
	raw = strings.TrimSpace(raw)
	var body []byte
	var err error
	if strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://") {
		body, err = FetchText(ctx, raw)
	} else {
		body, err = os.ReadFile(raw)
	}
	if err != nil {
		return nil, err
	}
	return UnpackPlaylist(body)
}

// UnpackPlaylist returns playlist text, unpacking gzip when the bytes are compressed.
func UnpackPlaylist(body []byte) ([]byte, error) {
	if len(body) >= 2 && body[0] == 0x1f && body[1] == 0x8b {
		gz, err := gzip.NewReader(bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		defer gz.Close()
		return io.ReadAll(io.LimitReader(gz, 32<<20))
	}
	return body, nil
}

// FetchText downloads a playlist or guide document.
func FetchText(ctx context.Context, rawURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Waveguide/0.1")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("source returned %s", res.Status)
	}
	return io.ReadAll(io.LimitReader(res.Body, 32<<20))
}

// ProbeFormat reads the start of a stream and returns "hls" or "mpegts".
// An empty string means the check did not recognize it.
func ProbeFormat(ctx context.Context, rawURL string) string {
	rawURL = strings.TrimSpace(rawURL)
	if strings.Contains(strings.ToLower(rawURL), ".m3u8") {
		return "hls"
	}
	ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return ""
	}
	req.Header.Set("User-Agent", "Waveguide/0.1")
	res, err := (&http.Client{Timeout: 4 * time.Second}).Do(req)
	if err != nil {
		return ""
	}
	defer res.Body.Close()
	buf := make([]byte, 512)
	n, _ := io.ReadFull(res.Body, buf)
	buf = buf[:n]
	if bytes.HasPrefix(buf, []byte("#EXTM3U")) {
		return "hls"
	}
	if len(buf) > 0 && buf[0] == 0x47 {
		return "mpegts"
	}
	return ""
}

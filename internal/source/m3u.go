package source

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"ota-viewer/internal/hdhr"
	"ota-viewer/internal/store"
)

// Entry is one channel from an M3U playlist or a single stream link.
type Entry struct {
	Number string
	Name   string
	URL    string
}

// ParseM3U reads an extended M3U playlist. Lines that are not channels are ignored.
func ParseM3U(r io.Reader) []Entry {
	var out []Entry
	var pending string
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	n := 1
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#EXTM3U") {
			continue
		}
		if strings.HasPrefix(line, "#EXTINF:") {
			name := line
			if comma := strings.LastIndex(line, ","); comma >= 0 {
				name = strings.TrimSpace(line[comma+1:])
			}
			pending = name
			continue
		}
		if strings.HasPrefix(line, "#") {
			continue
		}
		name := pending
		if name == "" {
			name = fmt.Sprintf("Stream %d", n)
		}
		out = append(out, Entry{Number: fmt.Sprintf("%d", 800+n), Name: name, URL: line})
		pending = ""
		n++
	}
	return out
}

// Install puts playlist entries on the guide as a tuner-free source.
func Install(ctx context.Context, st *store.Store, id int64, name, kind string, entries []Entry) error {
	if len(entries) == 0 {
		return fmt.Errorf("the playlist has no channels")
	}
	devID := fmt.Sprintf("src-%d", id)
	channels := make([]hdhr.Channel, 0, len(entries))
	for _, entry := range entries {
		channels = append(channels, hdhr.Channel{
			GuideNumber: entry.Number,
			GuideName:   entry.Name,
			StreamURL:   entry.URL,
			VideoCodec:  "H264",
			AudioCodec:  "AAC",
		})
	}
	label := name
	if label == "" {
		label = kind
	}
	return st.UpsertDevice(ctx, hdhr.Device{
		DeviceID: devID, FriendlyName: label, ModelNumber: kind, FirmwareName: "ota-viewer",
		BaseURL: "source", TunerCount: 0,
	}, channels)
}

// FetchText downloads a playlist or guide document.
func FetchText(ctx context.Context, rawURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "OTAViewer/0.1")
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

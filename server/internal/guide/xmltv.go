package guide

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"broadwave/internal/hdhr"
	"broadwave/internal/store"
)

type tv struct {
	XMLName    xml.Name    `xml:"tv"`
	Channels   []channel   `xml:"channel"`
	Programmes []programme `xml:"programme"`
}

type iconEl struct {
	Src    string `xml:"src,attr"`
	Width  string `xml:"width,attr"`
	Height string `xml:"height,attr"`
}

type channel struct {
	ID    string   `xml:"id,attr"`
	Names []string `xml:"display-name"`
	Icons []iconEl `xml:"icon"`
}

type episodeNum struct {
	System string `xml:"system,attr"`
	Value  string `xml:",chardata"`
}

type programme struct {
	Start   string       `xml:"start,attr"`
	Stop    string       `xml:"stop,attr"`
	Channel string       `xml:"channel,attr"`
	Title   string       `xml:"title"`
	Sub     string       `xml:"sub-title"`
	Desc    string       `xml:"desc"`
	Cats    []string     `xml:"category"`
	EpNums  []episodeNum `xml:"episode-num"`
	Icons   []iconEl     `xml:"icon"`
	Date    string       `xml:"date"`
	Series  string       `xml:"series-id"`
	Rating  *struct {
		Value string `xml:"value"`
	} `xml:"rating"`
	Credits *struct {
		Actors []string `xml:"actor"`
	} `xml:"credits"`
	New      *struct{} `xml:"new"`
	Shown    *struct{} `xml:"previously-shown"`
	Live     *struct{} `xml:"live"`
	Premiere *struct{} `xml:"premiere"`
	Finale   *struct{} `xml:"finale"`
}

// PullURL reads an XMLTV document from an http address the user supplied.
func PullURL(ctx context.Context, rawURL string) ([]byte, error) {
	rawURL = strings.TrimSpace(rawURL)
	if !strings.HasPrefix(rawURL, "https://") && !strings.HasPrefix(rawURL, "http://") {
		return nil, fmt.Errorf("the guide address needs to start with http")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Broadwave/0.1")
	req.Header.Set("Accept-Encoding", "gzip")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("guide returned %s", res.Status)
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, 32<<20))
	if err != nil {
		return nil, err
	}
	return inflateGuide(body, res.Header.Get("Content-Encoding"), rawURL)
}

func inflateGuide(body []byte, encoding, rawURL string) ([]byte, error) {
	gzipped := strings.EqualFold(encoding, "gzip") || strings.HasSuffix(strings.ToLower(rawURL), ".gz")
	if !gzipped && len(body) >= 2 && body[0] == 0x1f && body[1] == 0x8b {
		gzipped = true
	}
	if xzCompressed(body, encoding, rawURL) {
		return inflateXZ(body)
	}
	if !gzipped {
		return body, nil
	}
	gz, err := gzip.NewReader(bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	defer gz.Close()
	return io.ReadAll(io.LimitReader(gz, 32<<20))
}

func xzCompressed(body []byte, encoding, rawURL string) bool {
	if strings.EqualFold(encoding, "xz") || strings.HasSuffix(strings.ToLower(rawURL), ".xz") {
		return true
	}
	return len(body) >= 6 && body[0] == 0xfd && body[1] == 0x37 && body[2] == 0x7a && body[3] == 0x58 && body[4] == 0x5a && body[5] == 0x00
}

func inflateXZ(body []byte) ([]byte, error) {
	cmd := exec.Command("xz", "-dc")
	cmd.Stdin = bytes.NewReader(body)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("this guide is xz compressed and xz is not available")
	}
	if len(out) > 32<<20 {
		out = out[:32<<20]
	}
	return out, nil
}

// Pull asks SiliconDust for XMLTV. DeviceAuth is read for this call and not returned.
func Pull(ctx context.Context, client *hdhr.Client, baseURL string) ([]byte, error) {
	if client == nil {
		client = &hdhr.Client{}
	}
	auth, err := client.DeviceAuth(ctx, baseURL)
	if err != nil {
		return nil, err
	}
	if auth == "" {
		return nil, fmt.Errorf("tuner did not offer guide access")
	}
	u := "https://api.hdhomerun.com/api/xmltv?DeviceAuth=" + url.QueryEscape(auth)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Broadwave/0.1")
	req.Header.Set("Accept-Encoding", "gzip")
	var res *http.Response
	for attempt := 0; attempt < 2; attempt++ {
		res, err = http.DefaultClient.Do(req)
		if err != nil {
			return nil, err
		}
		if res.StatusCode == http.StatusOK {
			break
		}
		res.Body.Close()
		if attempt == 1 {
			return nil, fmt.Errorf("guide returned %s", res.Status)
		}
		req, err = http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", "Broadwave/0.1")
		req.Header.Set("Accept-Encoding", "gzip")
	}
	defer res.Body.Close()
	var reader io.Reader = res.Body
	if res.Header.Get("Content-Encoding") == "gzip" {
		gz, err := gzip.NewReader(res.Body)
		if err != nil {
			return nil, err
		}
		defer gz.Close()
		reader = gz
	}
	body, err := io.ReadAll(io.LimitReader(reader, 32<<20))
	if err != nil {
		return nil, err
	}
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("guide returned %s", res.Status)
	}
	return body, nil
}

func Parse(data []byte, channels []store.Channel) ([]store.Airing, map[int64]string, error) {
	var doc tv
	if err := xml.Unmarshal(data, &doc); err != nil {
		return nil, nil, err
	}
	xmlToChannel := assign(doc.Channels, channels)
	art := map[int64]string{}
	for _, ch := range doc.Channels {
		id := xmlToChannel[ch.ID]
		if id == 0 || art[id] != "" {
			continue
		}
		if src := iconURL(ch.Icons); src != "" {
			art[id] = src
		}
	}
	var out []store.Airing
	for _, p := range doc.Programmes {
		channelID := xmlToChannel[p.Channel]
		if channelID == 0 {
			continue
		}
		start, err1 := parseWhen(p.Start)
		end, err2 := parseWhen(p.Stop)
		if err1 != nil || err2 != nil || !end.After(start) {
			continue
		}
		programID, label, season, episode := episodeDetail(p.EpNums)
		imageURL, imageW, imageH := PickIcon(p.Icons)
		out = append(out, store.Airing{
			ChannelID: channelID, Title: strings.TrimSpace(p.Title), Subtitle: strings.TrimSpace(p.Sub),
			Description: strings.TrimSpace(p.Desc), Category: joinCats(p.Cats), ProgramID: programID,
			ImageURL: imageURL, ImageWidth: imageW, ImageHeight: imageH, Season: season, Episode: episode, EpisodeLabel: label,
			OriginalAir: originalAir(p.Date), SeriesID: strings.TrimSpace(p.Series),
			New: p.New != nil && p.Shown == nil, Live: p.Live != nil, Premiere: p.Premiere != nil, Finale: p.Finale != nil,
			Rating: ratingValue(p.Rating), Cast: castList(p.Credits), Start: start, End: end,
		})
	}
	return out, art, nil
}

func iconURL(icons []iconEl) string {
	src, _, _ := PickIcon(icons)
	return src
}

func cleanIcon(src string) string {
	src = strings.TrimSpace(src)
	if len(src) > 500 {
		return ""
	}
	if strings.HasPrefix(src, "https://") || strings.HasPrefix(src, "http://") {
		return src
	}
	return ""
}

func joinCats(cats []string) string {
	var parts []string
	for _, cat := range cats {
		cat = strings.TrimSpace(cat)
		if cat != "" {
			parts = append(parts, cat)
		}
	}
	return strings.Join(parts, ", ")
}

func episodeDetail(nums []episodeNum) (programID, label string, season, episode int) {
	for _, num := range nums {
		value := strings.TrimSpace(num.Value)
		if value == "" {
			continue
		}
		switch strings.ToLower(num.System) {
		case "dd_progid", "":
			if programID == "" {
				programID = value
			}
		case "onscreen":
			label = value
		case "xmltv_ns":
			season, episode = xmltvNS(value)
		}
	}
	return programID, label, season, episode
}

// xmltvNS is zero-based season.episode.part. Empty parts stay unknown.
func xmltvNS(value string) (int, int) {
	parts := strings.Split(value, ".")
	read := func(i int) int {
		if i >= len(parts) {
			return 0
		}
		n, err := strconv.Atoi(strings.TrimSpace(parts[i]))
		if err != nil || n < 0 {
			return 0
		}
		return n + 1
	}
	return read(0), read(1)
}

func originalAir(value string) string {
	value = strings.TrimSpace(value)
	digits := 0
	for _, r := range value {
		if r < '0' || r > '9' {
			break
		}
		digits++
	}
	if digits >= 8 {
		return value[:4] + "-" + value[4:6] + "-" + value[6:8]
	}
	if digits >= 4 {
		return value[:4]
	}
	return ""
}

func ratingValue(r *struct {
	Value string `xml:"value"`
}) string {
	if r == nil {
		return ""
	}
	return strings.TrimSpace(r.Value)
}

func castList(c *struct {
	Actors []string `xml:"actor"`
}) string {
	if c == nil {
		return ""
	}
	var names []string
	for _, name := range c.Actors {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		names = append(names, name)
		if len(names) == 8 {
			break
		}
	}
	return strings.Join(names, ", ")
}

func parseWhen(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if len(value) >= 20 && value[14] == ' ' {
		return time.Parse("20060102150405 -0700", value)
	}
	return time.Parse("20060102150405", value)
}

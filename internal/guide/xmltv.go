package guide

import (
	"compress/gzip"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"ota-viewer/internal/hdhr"
	"ota-viewer/internal/store"
)

type tv struct {
	XMLName    xml.Name    `xml:"tv"`
	Channels   []channel   `xml:"channel"`
	Programmes []programme `xml:"programme"`
}

type channel struct {
	ID    string   `xml:"id,attr"`
	Names []string `xml:"display-name"`
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
	New     *struct{}    `xml:"new"`
	Shown   *struct{}    `xml:"previously-shown"`
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
	req.Header.Set("User-Agent", "OTAViewer/0.1")
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
		req.Header.Set("User-Agent", "OTAViewer/0.1")
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

func Parse(data []byte, channels []store.Channel) ([]store.Airing, error) {
	var doc tv
	if err := xml.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	byGuide := map[string]int64{}
	byName := map[string]int64{}
	for _, ch := range channels {
		byGuide[ch.GuideNumber] = ch.ID
		byName[strings.ToLower(ch.GuideName)] = ch.ID
		byName[strings.ToLower(ch.DisplayName)] = ch.ID
	}
	xmlToChannel := map[string]int64{}
	for _, ch := range doc.Channels {
		if id, ok := byGuide[ch.ID]; ok {
			xmlToChannel[ch.ID] = id
			continue
		}
		for _, name := range ch.Names {
			if id, ok := byGuide[strings.TrimSpace(name)]; ok {
				xmlToChannel[ch.ID] = id
				break
			}
			if id, ok := byName[strings.ToLower(strings.TrimSpace(name))]; ok {
				xmlToChannel[ch.ID] = id
				break
			}
		}
	}
	var out []store.Airing
	for _, p := range doc.Programmes {
		channelID := xmlToChannel[p.Channel]
		if channelID == 0 {
			if id, ok := byGuide[p.Channel]; ok {
				channelID = id
			}
		}
		if channelID == 0 {
			continue
		}
		start, err1 := parseWhen(p.Start)
		end, err2 := parseWhen(p.Stop)
		if err1 != nil || err2 != nil || !end.After(start) {
			continue
		}
		out = append(out, store.Airing{
			ChannelID: channelID, Title: strings.TrimSpace(p.Title), Subtitle: strings.TrimSpace(p.Sub),
			Description: strings.TrimSpace(p.Desc), Category: joinCats(p.Cats), ProgramID: programID(p.EpNums),
			New: p.New != nil && p.Shown == nil, Start: start, End: end,
		})
	}
	return out, nil
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

func programID(nums []episodeNum) string {
	fallback := ""
	for _, num := range nums {
		value := strings.TrimSpace(num.Value)
		if value == "" {
			continue
		}
		if strings.EqualFold(num.System, "dd_progid") || num.System == "" {
			return value
		}
		if fallback == "" {
			fallback = value
		}
	}
	return fallback
}

func parseWhen(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if len(value) >= 20 && value[14] == ' ' {
		return time.Parse("20060102150405 -0700", value)
	}
	return time.Parse("20060102150405", value)
}

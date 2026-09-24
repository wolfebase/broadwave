package hdhr

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Device is the public discover.json document.
// DeviceAuth is deliberately not mapped, so it is dropped on decode.
type Device struct {
	FriendlyName     string `json:"friendlyName"`
	ModelNumber      string `json:"modelNumber"`
	FirmwareName     string `json:"firmwareName"`
	FirmwareVersion  string `json:"firmwareVersion"`
	UpgradeAvailable string `json:"upgradeAvailable,omitempty"`
	DeviceID         string `json:"deviceId"`
	BaseURL          string `json:"baseUrl"`
	LineupURL        string `json:"lineupUrl"`
	TunerCount       int    `json:"tunerCount"`
}

type deviceJSON struct {
	FriendlyName     string `json:"FriendlyName"`
	ModelNumber      string `json:"ModelNumber"`
	FirmwareName     string `json:"FirmwareName"`
	FirmwareVersion  string `json:"FirmwareVersion"`
	UpgradeAvailable string `json:"UpgradeAvailable"`
	DeviceID         string `json:"DeviceID"`
	BaseURL          string `json:"BaseURL"`
	LineupURL        string `json:"LineupURL"`
	TunerCount       int    `json:"TunerCount"`
}

// Channel is one lineup row. The tuner stream URL stays on the server.
type Channel struct {
	GuideNumber string `json:"guideNumber"`
	GuideName   string `json:"guideName"`
	VideoCodec  string `json:"videoCodec,omitempty"`
	AudioCodec  string `json:"audioCodec,omitempty"`
	HD          bool   `json:"hd"`
	Favorite    bool   `json:"favorite"`
	Protected   bool   `json:"protected,omitempty"`
	StreamURL   string `json:"-"`
	GuideKey    string `json:"-"`
	ArtURL      string `json:"-"`
	UserAgent   string `json:"-"`
	Referrer    string `json:"-"`
}

type lineupJSON struct {
	GuideNumber string `json:"GuideNumber"`
	GuideName   string `json:"GuideName"`
	URL         string `json:"URL"`
	VideoCodec  string `json:"VideoCodec"`
	AudioCodec  string `json:"AudioCodec"`
	HD          *int   `json:"HD"`
	Favorite    *int   `json:"Favorite"`
	DRM         *int   `json:"DRM"`
	Tags        string `json:"Tags"`
}

// Client fetches HDHomeRun HTTP documents. It never requests the stream port.
type Client struct {
	HTTP *http.Client
}

func (c *Client) httpClient() *http.Client {
	if c != nil && c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: 20 * time.Second}
}

func (c *Client) FetchDevice(ctx context.Context, baseURL string) (Device, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	var raw deviceJSON
	if err := c.getJSON(ctx, baseURL+"/discover.json", &raw); err != nil {
		return Device{}, err
	}
	if raw.DeviceID == "" || raw.BaseURL == "" {
		return Device{}, fmt.Errorf("discover.json from %s is missing a device id or base url", baseURL)
	}
	if raw.LineupURL == "" {
		raw.LineupURL = strings.TrimRight(raw.BaseURL, "/") + "/lineup.json"
	}
	return Device{
		FriendlyName:     raw.FriendlyName,
		ModelNumber:      raw.ModelNumber,
		FirmwareName:     raw.FirmwareName,
		FirmwareVersion:  raw.FirmwareVersion,
		UpgradeAvailable: raw.UpgradeAvailable,
		DeviceID:         strings.ToUpper(raw.DeviceID),
		BaseURL:          strings.TrimRight(raw.BaseURL, "/"),
		LineupURL:        raw.LineupURL,
		TunerCount:       raw.TunerCount,
	}, nil
}

// DeviceAuth reads the short-lived guide credential. Callers must not store or log it.
func (c *Client) DeviceAuth(ctx context.Context, baseURL string) (string, error) {
	var raw struct {
		DeviceAuth string `json:"DeviceAuth"`
	}
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if err := c.getJSON(ctx, baseURL+"/discover.json", &raw); err != nil {
		return "", err
	}
	return raw.DeviceAuth, nil
}

func (c *Client) FetchLineup(ctx context.Context, lineupURL string) ([]Channel, error) {
	var raw []lineupJSON
	if err := c.getJSON(ctx, lineupURL, &raw); err != nil {
		return nil, err
	}
	out := make([]Channel, 0, len(raw))
	for _, item := range raw {
		if item.GuideNumber == "" {
			continue
		}
		out = append(out, Channel{
			GuideNumber: item.GuideNumber,
			GuideName:   item.GuideName,
			VideoCodec:  item.VideoCodec,
			AudioCodec:  item.AudioCodec,
			HD:          item.HD != nil && *item.HD != 0,
			Favorite:    item.Favorite != nil && *item.Favorite != 0,
			Protected:   (item.DRM != nil && *item.DRM != 0) || strings.Contains(strings.ToLower(item.Tags), "drm"),
			StreamURL:   item.URL,
		})
	}
	return out, nil
}

// LibraryFile is one recording on a SCRIBE or SERVIO.
type LibraryFile struct {
	Title    string `json:"title"`
	Episode  string `json:"episode,omitempty"`
	Filename string `json:"filename"`
}

// FetchLibrary reads recorded_files.json. It does not copy the video.
func (c *Client) FetchLibrary(ctx context.Context, baseURL string) ([]LibraryFile, error) {
	var raw []struct {
		Title        string `json:"Title"`
		EpisodeTitle string `json:"EpisodeTitle"`
		Filename     string `json:"Filename"`
	}
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if err := c.getJSON(ctx, baseURL+"/recorded_files.json", &raw); err != nil {
		return nil, err
	}
	out := make([]LibraryFile, 0, len(raw))
	for _, item := range raw {
		if item.Filename == "" && item.Title == "" {
			continue
		}
		out = append(out, LibraryFile{Title: item.Title, Episode: item.EpisodeTitle, Filename: item.Filename})
	}
	return out, nil
}

// StartScan asks the tuner to rescan. It uses a tuner until the scan finishes.
func (c *Client) StartScan(ctx context.Context, baseURL string) error {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/lineup.post?scan=start", nil)
	if err != nil {
		return err
	}
	res, err := c.httpClient().Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 1<<20))
	if res.StatusCode >= 400 {
		return fmt.Errorf("scan returned %s", res.Status)
	}
	return nil
}

// ScanProgress is lineup_status.json. Found grows while Scan is true.
type ScanProgress struct {
	Scan  bool
	Found int
}

func (c *Client) ScanProgress(ctx context.Context, baseURL string) (ScanProgress, error) {
	var raw struct {
		Scan     *int `json:"Scan"`
		Found    int  `json:"Found"`
		Progress int  `json:"Progress"`
		ScanIn   *int `json:"ScanInProgress"`
	}
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if err := c.getJSON(ctx, baseURL+"/lineup_status.json", &raw); err != nil {
		return ScanProgress{}, err
	}
	on := (raw.Scan != nil && *raw.Scan != 0) || (raw.ScanIn != nil && *raw.ScanIn != 0)
	found := raw.Found
	if found == 0 {
		found = raw.Progress
	}
	return ScanProgress{Scan: on, Found: found}, nil
}

func (c *Client) getJSON(ctx context.Context, rawURL string, dest any) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return err
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("unsupported url scheme")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return err
	}
	res, err := c.httpClient().Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, 8<<20))
	if err != nil {
		return err
	}
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("%s returned %s", rawURL, res.Status)
	}
	if err := json.Unmarshal(body, dest); err != nil {
		return fmt.Errorf("%s: %w", rawURL, err)
	}
	return nil
}

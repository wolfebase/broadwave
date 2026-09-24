package source

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// TVHeadend downloads the channel playlist. The guide address carries the login
// so it can be stored masked. Web calls need that account or tvheadend returns 403.
func TVHeadend(ctx context.Context, base, user, pass string) (playlist []byte, playlistURL, guide string, err error) {
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	if base == "" || !strings.Contains(base, "://") {
		return nil, "", "", fmt.Errorf("The server address should start with http:// or https://.")
	}
	playlistURL = withLogin(base+"/playlist/channels", user, pass)
	body, status, err := authedGet(ctx, base+"/playlist/channels", user, pass)
	if err != nil {
		return nil, "", "", err
	}
	if status == http.StatusForbidden || status == http.StatusUnauthorized {
		return nil, "", "", fmt.Errorf("That login was not accepted. Check the username and password.")
	}
	if status != http.StatusOK || !bytesHasM3U(body) {
		return nil, "", "", fmt.Errorf("That server did not return a channel list. Check the address.")
	}
	return body, playlistURL, withLogin(base+"/xmltv/channels", user, pass), nil
}

// ChannelsDVR downloads the playlist Channels serves for other apps.
// format=ts&codec=copy asks for the original broadcast instead of a transcode.
func ChannelsDVR(ctx context.Context, base string) (playlist []byte, playlistURL, guide string, err error) {
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	if base == "" || !strings.Contains(base, "://") {
		return nil, "", "", fmt.Errorf("The server address should start with http:// or https://.")
	}
	playlistURL = base + "/devices/ANY/channels.m3u?format=ts&codec=copy"
	guide = base + "/devices/ANY/guide/xmltv"
	body, status, err := authedGet(ctx, playlistURL, "", "")
	if err != nil {
		return nil, "", "", err
	}
	if status == http.StatusForbidden {
		return nil, "", "", fmt.Errorf("Channels DVR refused the request. The server and Channels need to be on the same network.")
	}
	if status != http.StatusOK || !bytesHasM3U(body) {
		return nil, "", "", fmt.Errorf("That server did not return a channel list. Check the address.")
	}
	return body, playlistURL, guide, nil
}

// emulatorPaths are the playlist and guide each app serves by default.
var emulatorPaths = map[string][2]string{
	"threadfin":   {"/m3u/threadfin.m3u", "/xmltv/threadfin.xml"},
	"xteve":       {"/m3u/xteve.m3u", "/xmltv/xteve.xml"},
	"ersatztv":    {"/iptv/channels.m3u", "/iptv/xmltv.xml"},
	"dispatcharr": {"/output/m3u", "/output/epg"},
}

// EmulatorM3U downloads the playlist for one of those apps.
func EmulatorM3U(ctx context.Context, kind, base string) (playlist []byte, playlistURL, guide string, err error) {
	paths, ok := emulatorPaths[kind]
	if !ok {
		return nil, "", "", fmt.Errorf("That server type is not supported.")
	}
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	if base == "" || !strings.Contains(base, "://") {
		return nil, "", "", fmt.Errorf("The server address should start with http:// or https://.")
	}
	playlistURL = base + paths[0]
	guide = base + paths[1]
	body, status, err := authedGet(ctx, playlistURL, "", "")
	if err != nil {
		return nil, "", "", err
	}
	if status != http.StatusOK || !bytesHasM3U(body) {
		return nil, "", "", fmt.Errorf("That server did not return a channel list. Check the address.")
	}
	return body, playlistURL, guide, nil
}

func withLogin(raw, user, pass string) string {
	if user == "" && pass == "" {
		return raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	u.User = url.UserPassword(user, pass)
	return u.String()
}

func bytesHasM3U(body []byte) bool {
	return strings.Contains(string(body), "#EXTM3U")
}

func authedGet(ctx context.Context, raw, user, pass string) ([]byte, int, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("The server address should start with http:// or https://.")
	}
	if user != "" || pass != "" {
		req.SetBasicAuth(user, pass)
	}
	req.Header.Set("User-Agent", "Broadwave/0.1")
	res, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("Broadwave could not reach that server. Check the address.")
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, 32<<20))
	if err != nil {
		return nil, res.StatusCode, err
	}
	return body, res.StatusCode, nil
}

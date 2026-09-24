package source

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

// Xtream reads live categories and streams from an Xtream Codes server.
// The returned playlist text uses the same attributes as an M3U import.
// The guide address carries the login so it can be stored masked.
func Xtream(ctx context.Context, base, user, pass string) (playlist []byte, guide string, err error) {
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	user = strings.TrimSpace(user)
	pass = strings.TrimSpace(pass)
	if base == "" || user == "" || pass == "" {
		return nil, "", fmt.Errorf("Add the server, username, and password.")
	}
	if _, err := url.Parse(base); err != nil || !strings.Contains(base, "://") {
		return nil, "", fmt.Errorf("The server address should start with http:// or https://.")
	}
	categories, err := xtreamCategories(ctx, base, user, pass)
	if err != nil {
		return nil, "", err
	}
	streams, err := xtreamStreams(ctx, base, user, pass)
	if err != nil {
		return nil, "", err
	}
	if len(streams) == 0 {
		return nil, "", fmt.Errorf("That server returned no live channels. Check the login.")
	}
	var b strings.Builder
	guide = base + "/xmltv.php?username=" + url.QueryEscape(user) + "&password=" + url.QueryEscape(pass)
	b.WriteString("#EXTM3U url-tvg=\"")
	b.WriteString(guide)
	b.WriteString("\"\n")
	for _, stream := range streams {
		id := stream.ID.String()
		if id == "" || stream.Name == "" {
			continue
		}
		group := categories[stream.Category.String()]
		number := stream.Num.String()
		if number == "" || number == "0" {
			number = id
		}
		b.WriteString("#EXTINF:-1")
		if stream.EPG != "" {
			b.WriteString(" tvg-id=\"")
			b.WriteString(escapeAttr(stream.EPG))
			b.WriteString("\"")
		}
		b.WriteString(" tvg-chno=\"")
		b.WriteString(escapeAttr(number))
		b.WriteString("\"")
		if group != "" {
			b.WriteString(" group-title=\"")
			b.WriteString(escapeAttr(group))
			b.WriteString("\"")
		}
		if stream.Icon != "" {
			b.WriteString(" tvg-logo=\"")
			b.WriteString(escapeAttr(stream.Icon))
			b.WriteString("\"")
		}
		b.WriteString(",")
		b.WriteString(stream.Name)
		b.WriteByte('\n')
		b.WriteString(base)
		b.WriteString("/live/")
		b.WriteString(url.PathEscape(user))
		b.WriteByte('/')
		b.WriteString(url.PathEscape(pass))
		b.WriteByte('/')
		b.WriteString(id)
		b.WriteString(".ts\n")
	}
	return []byte(b.String()), guide, nil
}

// XtreamFromStored reads a server address that already has the login in it.
func XtreamFromStored(ctx context.Context, raw string) ([]byte, string, error) {
	u, err := url.Parse(raw)
	if err != nil || u.User == nil {
		return nil, "", fmt.Errorf("This Xtream source has no saved login. Add it again.")
	}
	pass, _ := u.User.Password()
	user := u.User.Username()
	u.User = nil
	u.RawQuery = ""
	u.Path = ""
	return Xtream(ctx, u.String(), user, pass)
}

func escapeAttr(value string) string {
	return strings.NewReplacer(`"`, "", "\n", " ").Replace(value)
}

type xtreamStream struct {
	Name     string      `json:"name"`
	ID       json.Number `json:"stream_id"`
	Icon     string      `json:"stream_icon"`
	EPG      string      `json:"epg_channel_id"`
	Category json.Number `json:"category_id"`
	Num      json.Number `json:"num"`
}

func xtreamCategories(ctx context.Context, base, user, pass string) (map[string]string, error) {
	body, err := xtreamGet(ctx, base, user, pass, "get_live_categories")
	if err != nil {
		return nil, err
	}
	var rows []struct {
		ID   json.Number `json:"category_id"`
		Name string      `json:"category_name"`
	}
	if err := json.Unmarshal(body, &rows); err != nil {
		return nil, fmt.Errorf("That server did not return live categories. Check the address.")
	}
	out := map[string]string{}
	for _, row := range rows {
		out[row.ID.String()] = row.Name
	}
	return out, nil
}

func xtreamStreams(ctx context.Context, base, user, pass string) ([]xtreamStream, error) {
	body, err := xtreamGet(ctx, base, user, pass, "get_live_streams")
	if err != nil {
		return nil, err
	}
	if strings.Contains(string(body), `"auth":0`) || strings.Contains(string(body), `"auth":"0"`) {
		return nil, fmt.Errorf("That login was not accepted. Check the username and password.")
	}
	var rows []xtreamStream
	if err := json.Unmarshal(body, &rows); err != nil {
		return nil, fmt.Errorf("That server did not return live channels. Check the address.")
	}
	return rows, nil
}

func xtreamGet(ctx context.Context, base, user, pass, action string) ([]byte, error) {
	u, err := url.Parse(base + "/player_api.php")
	if err != nil {
		return nil, fmt.Errorf("The server address should start with http:// or https://.")
	}
	q := u.Query()
	q.Set("username", user)
	q.Set("password", pass)
	q.Set("action", action)
	u.RawQuery = q.Encode()
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("The server address should start with http:// or https://.")
	}
	req.Header.Set("User-Agent", "Waveguide/0.1")
	res, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
	if err != nil {
		return nil, fmt.Errorf("Waveguide could not reach that server. Check the address.")
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("The server returned %s. Check the address.", res.Status)
	}
	return io.ReadAll(io.LimitReader(res.Body, 32<<20))
}

package guide

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"

	"broadwave/internal/store"
)

// FillImages adds a poster address to airings that have none.
// An empty key skips the lookup. The key is not written to the log.
func FillImages(ctx context.Context, key string, rows []store.Airing) []store.Airing {
	return fillImages(ctx, http.DefaultClient, "https://api.themoviedb.org/3", key, rows)
}

func fillImages(ctx context.Context, client *http.Client, base, key string, rows []store.Airing) []store.Airing {
	key = strings.TrimSpace(key)
	if key == "" || client == nil || len(rows) == 0 {
		return rows
	}
	found := map[string]string{}
	lookups := 0
	for i := range rows {
		if strings.TrimSpace(rows[i].ImageURL) != "" {
			continue
		}
		title := strings.TrimSpace(rows[i].Title)
		if len(title) < 2 {
			continue
		}
		if poster, ok := found[title]; ok {
			rows[i].ImageURL = poster
			continue
		}
		if lookups >= 20 {
			continue
		}
		lookups++
		poster, err := tmdbPoster(ctx, client, base, key, title)
		if err != nil {
			return rows
		}
		found[title] = poster
		rows[i].ImageURL = poster
	}
	return rows
}

func tmdbPoster(ctx context.Context, client *http.Client, base, key, title string) (string, error) {
	u, err := url.Parse(strings.TrimRight(base, "/") + "/search/multi")
	if err != nil {
		return "", err
	}
	q := u.Query()
	q.Set("query", title)
	q.Set("api_key", key)
	u.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Broadwave/0.1")
	res, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return "", errStatus(res.StatusCode)
	}
	var body struct {
		Results []struct {
			Poster string `json:"poster_path"`
		} `json:"results"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		return "", err
	}
	for _, item := range body.Results {
		if strings.HasPrefix(item.Poster, "/") {
			return "https://image.tmdb.org/t/p/w342" + item.Poster, nil
		}
	}
	return "", nil
}

type statusError int

func (e statusError) Error() string { return "artwork lookup failed" }

func errStatus(code int) error { return statusError(code) }

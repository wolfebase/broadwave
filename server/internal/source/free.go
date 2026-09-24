package source

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// FreeGroup is the guide group for channels that come from the internet.
const FreeGroup = "Streamed from the internet"

// FreeGuide is what to show when no free-channel server is on the network.
const FreeGuide = `Run FastChannels next to Waveguide, then add it here.

services:
  fastchannels:
    image: ghcr.io/kineticman/fastchannels:latest
    ports:
      - "5523:5523"
    volumes:
      - fastchannels:/data
volumes:
  fastchannels:

On Unraid, the container name is FastChannels.`

// Feed is one free-channel playlist a generator is already serving.
type Feed struct {
	Kind     string `json:"kind"`
	Name     string `json:"name"`
	Addr     string `json:"addr"`
	Playlist string `json:"playlist"`
	Guide    string `json:"guide"`
}

type freePort struct {
	port     int
	kind     string
	name     string
	playlist string
	guide    string
}

var freePorts = []freePort{
	{5523, "fastchannels", "FastChannels", "/feeds/default/m3u", "/feeds/default/epg.xml"},
	{7777, "pluto", "Pluto", "/playlist.m3u", "/epg.xml"},
	{8182, "samsung", "Samsung TV Plus", "/playlist.m3u8?regions=us", "/epg.xml"},
}

// FindFree probes hosts for FastChannels, Pluto, and Samsung generators.
// It only reads playlists those servers already publish.
func FindFree(ctx context.Context, hosts []string) []Feed {
	seen := map[string]bool{}
	var list []string
	for _, host := range append([]string{"127.0.0.1"}, hosts...) {
		host = strings.TrimSpace(host)
		if host == "" || seen[host] {
			continue
		}
		seen[host] = true
		list = append(list, host)
	}
	if len(list) == 0 {
		return nil
	}
	client := &http.Client{Timeout: 700 * time.Millisecond}
	jobs := make(chan string)
	out := make(chan Feed)
	var wg sync.WaitGroup
	workers := 32
	if len(list) < workers {
		workers = len(list)
	}
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for host := range jobs {
				if ctx.Err() != nil {
					continue
				}
				for _, port := range freePorts {
					feed, ok := probeFree(ctx, client, host, port)
					if ok {
						out <- feed
					}
				}
			}
		}()
	}
	go func() {
		defer close(jobs)
		for _, host := range list {
			select {
			case <-ctx.Done():
				return
			case jobs <- host:
			}
		}
	}()
	go func() {
		wg.Wait()
		close(out)
	}()
	var found []Feed
	for feed := range out {
		found = append(found, feed)
	}
	return found
}

func probeFree(ctx context.Context, client *http.Client, host string, port freePort) (Feed, bool) {
	base := "http://" + net.JoinHostPort(host, fmt.Sprintf("%d", port.port))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+port.playlist, nil)
	if err != nil {
		return Feed{}, false
	}
	res, err := client.Do(req)
	if err != nil {
		return Feed{}, false
	}
	body, _ := io.ReadAll(io.LimitReader(res.Body, 64))
	res.Body.Close()
	if res.StatusCode >= 400 || !bytesHasM3U(body) {
		if port.kind == "fastchannels" && fastChannelsAPI(ctx, client, base) {
			return Feed{
				Kind: port.kind, Name: port.name, Addr: base,
				Playlist: base + port.playlist, Guide: base + port.guide,
			}, true
		}
		return Feed{}, false
	}
	return Feed{
		Kind: port.kind, Name: port.name, Addr: base,
		Playlist: base + port.playlist, Guide: base + port.guide,
	}, true
}

// FeedByKind rebuilds the playlist URLs for a generator found on addr.
func FeedByKind(kind, addr string) (Feed, error) {
	kind = strings.ToLower(strings.TrimSpace(kind))
	addr = strings.TrimRight(strings.TrimSpace(addr), "/")
	if !strings.Contains(addr, "://") {
		addr = "http://" + addr
	}
	for _, port := range freePorts {
		if port.kind != kind {
			continue
		}
		return Feed{
			Kind: port.kind, Name: port.name, Addr: addr,
			Playlist: addr + port.playlist, Guide: addr + port.guide,
		}, nil
	}
	return Feed{}, fmt.Errorf("that is not a free-channel server")
}

// PrepareFree drops DRM streams and puts the rest in one guide group.
func PrepareFree(entries []Entry) ([]Entry, int) {
	var out []Entry
	skipped := 0
	for _, entry := range entries {
		if entry.DRM || drmURL(entry.URL) || drmGroup(entry.Group) {
			skipped++
			continue
		}
		entry.Group = FreeGroup
		out = append(out, entry)
	}
	return out, skipped
}

func fastChannelsAPI(ctx context.Context, client *http.Client, base string) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/api/feeds", nil)
	if err != nil {
		return false
	}
	res, err := client.Do(req)
	if err != nil {
		return false
	}
	body, _ := io.ReadAll(io.LimitReader(res.Body, 2048))
	res.Body.Close()
	return res.StatusCode == http.StatusOK && strings.Contains(string(body), "/feeds/")
}

func drmURL(raw string) bool {
	u := strings.ToLower(raw)
	return strings.Contains(u, "widevine") || strings.Contains(u, "playready") || strings.HasPrefix(u, "drm:")
}

func drmGroup(group string) bool {
	return strings.EqualFold(strings.TrimSpace(group), "drm")
}

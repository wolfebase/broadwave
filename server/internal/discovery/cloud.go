package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"waveguide/internal/hdhr"
)

// KeepLocalCloud returns cloud devices whose LocalIP answers with the same device id.
// Strangers on the list are dropped. probe is DiscoverHost in production.
func KeepLocalCloud(ctx context.Context, raw []byte, probe func(context.Context, string) (string, bool)) []Found {
	var rows []struct {
		DeviceID string `json:"DeviceID"`
		LocalIP  string `json:"LocalIP"`
		Name     string `json:"FriendlyName"`
	}
	if err := json.Unmarshal(raw, &rows); err != nil || probe == nil {
		return nil
	}
	var out []Found
	for _, row := range rows {
		if row.LocalIP == "" || row.DeviceID == "" {
			continue
		}
		id, ok := probe(ctx, row.LocalIP)
		if !ok || id != row.DeviceID {
			continue
		}
		name := row.Name
		if name == "" {
			name = "HDHomeRun"
		}
		out = append(out, Found{Kind: "hdhomerun", Name: name, Addr: row.LocalIP, ID: row.DeviceID})
	}
	return out
}

// FetchCloud is the unsupported SiliconDust list. Callers must run KeepLocalCloud on it.
func FetchCloud(ctx context.Context) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://ipv4-api.hdhomerun.com/discover", nil)
	if err != nil {
		return nil, err
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("cloud discover returned %s", res.Status)
	}
	return io.ReadAll(io.LimitReader(res.Body, 1<<20))
}

// ConfirmCloud fetches the cloud list and keeps devices that answer locally.
func ConfirmCloud(ctx context.Context) []Found {
	raw, err := FetchCloud(ctx)
	if err != nil {
		return nil
	}
	return KeepLocalCloud(ctx, raw, func(ctx context.Context, host string) (string, bool) {
		reply, err := hdhr.DiscoverHost(host, 700*time.Millisecond)
		if err != nil {
			return "", false
		}
		return reply.DeviceID, reply.DeviceID != ""
	})
}

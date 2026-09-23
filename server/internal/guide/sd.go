package guide

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"waveguide/internal/store"
)

// SchedulesDirect fills guide rows for channels the free SiliconDust feed left empty.
// It runs only when SD_USERNAME, SD_PASSWORD, and SD_LINEUP are set. The password is
// read from the environment and is not stored.
func SchedulesDirect(ctx context.Context, channels []store.Channel) ([]store.Airing, []int64, error) {
	user := strings.TrimSpace(os.Getenv("SD_USERNAME"))
	pass := os.Getenv("SD_PASSWORD")
	lineup := strings.TrimSpace(os.Getenv("SD_LINEUP"))
	if user == "" || pass == "" || lineup == "" {
		return nil, nil, nil
	}
	return fetchSchedules(ctx, http.DefaultClient, "https://json.schedulesdirect.org/20141201", user, pass, lineup, channels)
}

func fetchSchedules(ctx context.Context, client *http.Client, base, user, pass, lineup string, channels []store.Channel) ([]store.Airing, []int64, error) {
	sum := sha1.Sum([]byte(pass))
	token, err := sdPost(ctx, client, base+"/token", "", map[string]string{"username": user, "password": hex.EncodeToString(sum[:])})
	if err != nil {
		return nil, nil, err
	}
	var tok struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(token, &tok); err != nil || tok.Token == "" {
		return nil, nil, fmt.Errorf("schedules direct did not return a token")
	}
	body, err := sdGet(ctx, client, base+"/lineups/"+lineup, tok.Token)
	if err != nil {
		return nil, nil, err
	}
	var doc struct {
		Map []struct {
			StationID string `json:"stationID"`
			Channel   string `json:"channel"`
		} `json:"map"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, nil, err
	}
	byNumber := map[string]int64{}
	for _, ch := range channels {
		byNumber[ch.GuideNumber] = ch.ID
	}
	stationChannel := map[string]int64{}
	var stations []string
	for _, row := range doc.Map {
		id, ok := byNumber[row.Channel]
		if !ok || row.StationID == "" {
			continue
		}
		stationChannel[row.StationID] = id
		stations = append(stations, row.StationID)
	}
	if len(stations) == 0 {
		return nil, nil, nil
	}
	day := time.Now().Format("2006-01-02")
	var req []map[string]any
	for _, id := range stations {
		req = append(req, map[string]any{"stationID": id, "date": []string{day}})
	}
	raw, err := sdPost(ctx, client, base+"/schedules", tok.Token, req)
	if err != nil {
		return nil, nil, err
	}
	var blocks []struct {
		StationID string `json:"stationID"`
		Programs  []struct {
			ProgramID   string `json:"programID"`
			AirDateTime string `json:"airDateTime"`
			Duration    int    `json:"duration"`
		} `json:"programs"`
	}
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return nil, nil, err
	}
	ids := map[string]struct{}{}
	for _, block := range blocks {
		for _, prog := range block.Programs {
			if prog.ProgramID != "" {
				ids[prog.ProgramID] = struct{}{}
			}
		}
	}
	var programIDs []string
	for id := range ids {
		programIDs = append(programIDs, id)
	}
	titles := map[string]string{}
	if len(programIDs) > 0 {
		meta, err := sdPost(ctx, client, base+"/programs", tok.Token, programIDs)
		if err == nil {
			var programs []struct {
				ProgramID string `json:"programID"`
				Titles    []struct {
					Title string `json:"title120"`
				} `json:"titles"`
			}
			if json.Unmarshal(meta, &programs) == nil {
				for _, prog := range programs {
					if len(prog.Titles) > 0 {
						titles[prog.ProgramID] = prog.Titles[0].Title
					}
				}
			}
		}
	}
	var out []store.Airing
	seenChannels := map[int64]bool{}
	for _, block := range blocks {
		channelID := stationChannel[block.StationID]
		if channelID == 0 {
			continue
		}
		for _, prog := range block.Programs {
			start, err := time.Parse(time.RFC3339, prog.AirDateTime)
			if err != nil || prog.Duration <= 0 {
				continue
			}
			title := titles[prog.ProgramID]
			if title == "" {
				title = prog.ProgramID
			}
			out = append(out, store.Airing{
				ChannelID: channelID, Title: title, ProgramID: prog.ProgramID,
				Start: start, End: start.Add(time.Duration(prog.Duration) * time.Second),
			})
			seenChannels[channelID] = true
		}
	}
	var channelIDs []int64
	for id := range seenChannels {
		channelIDs = append(channelIDs, id)
	}
	return out, channelIDs, nil
}

func sdPost(ctx context.Context, client *http.Client, url, token string, body any) ([]byte, error) {
	raw, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("token", token)
	}
	return sdDo(client, req)
}

func sdGet(ctx context.Context, client *http.Client, url, token string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("token", token)
	return sdDo(client, req)
}

func sdDo(client *http.Client, req *http.Request) ([]byte, error) {
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	if res.StatusCode >= 300 {
		return nil, fmt.Errorf("schedules direct returned %s", res.Status)
	}
	return body, nil
}

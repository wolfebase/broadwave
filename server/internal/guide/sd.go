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
	"slices"
	"strings"
	"time"

	"broadwave/internal/fetchguard"
	"broadwave/internal/store"
)

// SchedulesDirect fills guide rows for channels the free SiliconDust feed left empty.
// Empty account fields mean skip. The password is passed in and is not logged.
func SchedulesDirect(ctx context.Context, channels []store.Channel, user, pass, lineup string) ([]store.Airing, []int64, error) {
	user = strings.TrimSpace(user)
	lineup = strings.TrimSpace(lineup)
	if user == "" || pass == "" || lineup == "" {
		return nil, nil, nil
	}
	return fetchSchedules(ctx, fetchguard.Client(20*time.Second), "https://json.schedulesdirect.org/20141201", user, pass, lineup, channels)
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
	dates := make([]string, 14)
	for i := range dates {
		dates[i] = time.Now().AddDate(0, 0, i).Format("2006-01-02")
	}
	type program struct {
		ProgramID   string `json:"programID"`
		AirDateTime string `json:"airDateTime"`
		Duration    int    `json:"duration"`
	}
	type block struct {
		StationID string    `json:"stationID"`
		Programs  []program `json:"programs"`
	}
	// Fourteen days of a whole lineup is far past one response's cap, so the
	// stations are asked for a few at a time. An account that refuses the
	// longer request gets one day.
	var blocks []block
	days := dates
	for part := range slices.Chunk(stations, sdStationsPerRequest) {
		var req []map[string]any
		for _, id := range part {
			req = append(req, map[string]any{"stationID": id, "date": days})
		}
		raw, err := sdPost(ctx, client, base+"/schedules", tok.Token, req)
		if err != nil && len(days) > 1 {
			days = dates[:1]
			for i := range req {
				req[i]["date"] = days
			}
			raw, err = sdPost(ctx, client, base+"/schedules", tok.Token, req)
		}
		if err != nil {
			return nil, nil, err
		}
		var got []block
		if err := json.Unmarshal(raw, &got); err != nil {
			return nil, nil, err
		}
		blocks = append(blocks, got...)
	}
	ids := map[string]struct{}{}
	var programIDs []string
	for _, block := range blocks {
		for _, prog := range block.Programs {
			if _, ok := ids[prog.ProgramID]; prog.ProgramID != "" && !ok {
				ids[prog.ProgramID] = struct{}{}
				programIDs = append(programIDs, prog.ProgramID)
			}
		}
	}
	// A title missing from a batch that failed falls back to the program id.
	titles := map[string]string{}
	for part := range slices.Chunk(programIDs, sdProgramsPerRequest) {
		meta, err := sdPost(ctx, client, base+"/programs", tok.Token, part)
		if err != nil {
			continue
		}
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
	if res.StatusCode >= 300 {
		return nil, fmt.Errorf("schedules direct returned %s", res.Status)
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, sdMaxResponse+1))
	if err != nil {
		return nil, err
	}
	// A response cut at the cap would fail to parse with a JSON error that
	// names no cause.
	if len(body) > sdMaxResponse {
		return nil, fmt.Errorf("schedules direct sent more than %d MB in one response", sdMaxResponse>>20)
	}
	return body, nil
}

// Schedules Direct takes up to 5,000 stations or programs a request; these
// are smaller so each response stays well under sdMaxResponse. Fourteen days
// of one station is about 150 KB of schedule, and a program's details about
// 2 KB.
const (
	sdStationsPerRequest = 20
	sdProgramsPerRequest = 500
	sdMaxResponse        = 8 << 20
)

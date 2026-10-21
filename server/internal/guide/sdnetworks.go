package guide

import (
	"encoding/json"
	"strconv"
	"strings"

	"broadwave/internal/network"
	"broadwave/internal/store"
)

// SDNetworks reads a Schedules Direct lineup (GET /lineups/{id}) and returns
// the network of each lineup channel it maps, from the station's affiliate
// field, then its name and call sign.
func SDNetworks(body []byte, lineup []store.Channel) map[int64]string {
	var doc struct {
		Map []struct {
			StationID      string `json:"stationID"`
			Channel        string `json:"channel"`
			VirtualChannel string `json:"virtualChannel"`
			AtscMajor      int    `json:"atscMajor"`
			AtscMinor      int    `json:"atscMinor"`
		} `json:"map"`
		Stations []struct {
			StationID string `json:"stationID"`
			Callsign  string `json:"callsign"`
			Name      string `json:"name"`
			Affiliate string `json:"affiliate"`
		} `json:"stations"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil
	}
	byNumber := map[string]int64{}
	for _, ch := range lineup {
		byNumber[ch.GuideNumber] = ch.ID
	}
	station := map[string]int64{}
	for _, row := range doc.Map {
		numbers := []string{row.Channel, row.VirtualChannel}
		if row.AtscMajor > 0 {
			numbers = append(numbers, strconv.Itoa(row.AtscMajor)+"."+strconv.Itoa(row.AtscMinor))
		}
		for _, number := range numbers {
			if id, ok := byNumber[strings.TrimSpace(number)]; ok && row.StationID != "" {
				station[row.StationID] = id
				break
			}
		}
	}
	out := map[int64]string{}
	for _, st := range doc.Stations {
		id := station[st.StationID]
		if id == 0 {
			continue
		}
		// Gracenote writes "NBC" or "NBC Affiliate".
		affiliate := strings.TrimSpace(st.Affiliate)
		for _, suf := range []string{" Affiliate", " affiliate", " AFFILIATE"} {
			affiliate = strings.TrimSuffix(affiliate, suf)
		}
		if net := network.Of([]string{affiliate, st.Name}, st.Callsign); net != "" {
			out[id] = net
		}
	}
	return out
}

package httpapi

import (
	"encoding/json"
	"net/http"
	"testing"

	"broadwave/internal/live"
)

func TestDiagnosticsShowsTheHostBudget(t *testing.T) {
	st := testStore(t)
	host := live.Budget("software", 0.4)
	host.Encoder = "libx264"
	api := &Server{
		Store:   st,
		Hub:     &live.Hub{Store: st, Dir: t.TempDir(), Encoder: "libx264", FFmpeg: "ffmpeg", Host: host},
		Version: "dev",
	}
	res := get(t, api.Handler(), "/api/v1/diagnostics")
	if res.Code != http.StatusOK {
		t.Fatalf("status %d", res.Code)
	}
	var body struct {
		Encoder struct {
			Class    string  `json:"class"`
			Speed    float64 `json:"speed"`
			Height   int     `json:"height"`
			Focus    string  `json:"focus"`
			Tiles    int     `json:"tiles"`
			FullRate bool    `json:"fullRate"`
			Line     string  `json:"line"`
		} `json:"encoder"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	got := body.Encoder
	if got.Class != "software" || got.Speed != 0.4 || got.Height != 540 || got.Focus != "360" || got.Tiles != 1 || got.FullRate {
		t.Fatalf("encoder %+v", got)
	}
	if got.Line != "Software encoder: 1080p60 at 0.4x real time. Picture up to 540p. 360p on the selected tile, 1 tile." {
		t.Fatalf("line %q", got.Line)
	}
}

package live

import (
	"os/exec"
	"strings"
	"sync"
)

type RenditionStatus struct {
	Key     string `json:"key"`
	Viewers int    `json:"viewers"`
}

// FeedStatus is one tuned channel as the relay sees it.
type FeedStatus struct {
	ChannelID   int64             `json:"channelId"`
	GuideNumber string            `json:"guideNumber"`
	Name        string            `json:"name"`
	FrequencyHz int               `json:"frequencyHz"`
	Tuner       int               `json:"tuner"`
	Renditions  []RenditionStatus `json:"renditions"`
	Recording   bool              `json:"recording"`
	Exports     int               `json:"exports"`
	FieldOrder  string            `json:"fieldOrder,omitempty"`
}

// FeedStat is one tuned channel: viewers and the ffmpeg processes on it.
type FeedStat struct {
	ChannelID   int64  `json:"channelId"`
	GuideNumber string `json:"guideNumber"`
	Name        string `json:"name"`
	Viewers     int    `json:"viewers"`
	FFmpeg      int    `json:"ffmpeg"`
	Recording   bool   `json:"recording"`
	Exports     int    `json:"exports"`
}

// FeedStats counts viewers and ffmpeg processes the hub already has.
// It does not tune.
func (h *Hub) FeedStats() []FeedStat {
	if h == nil {
		return []FeedStat{}
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	out := []FeedStat{}
	for _, f := range h.feedsLocked() {
		st := FeedStat{
			ChannelID: f.channel.ID, GuideNumber: f.channel.GuideNumber, Name: f.channel.DisplayName,
			Recording: f.recording != nil, Exports: f.exports,
		}
		for _, r := range f.renditions {
			if r == nil {
				continue
			}
			st.Viewers += r.viewers
			if renditionRunning(r) {
				st.FFmpeg++
			}
		}
		if f.recording != nil && cmdRunning(f.recording.cmd) {
			st.FFmpeg++
		}
		out = append(out, st)
	}
	return out
}

func renditionRunning(r *rendition) bool {
	return r != nil && !r.waited.Load() && cmdRunning(r.cmd)
}

func cmdRunning(cmd *exec.Cmd) bool {
	return cmd != nil && cmd.Process != nil
}

func (h *Hub) Status() []FeedStatus {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := []FeedStatus{}
	for _, f := range h.feedsLocked() {
		st := FeedStatus{
			ChannelID: f.channel.ID, GuideNumber: f.channel.GuideNumber, Name: f.channel.DisplayName,
			FrequencyHz: f.channel.FrequencyHz, Tuner: -1, Recording: f.recording != nil, Exports: f.exports,
			FieldOrder: f.channel.FieldOrder, Renditions: []RenditionStatus{},
		}
		if m := muxOf(h, f); m != nil {
			st.Tuner = m.tuner
		}
		for key, r := range f.renditions {
			st.Renditions = append(st.Renditions, RenditionStatus{Key: key, Viewers: r.viewers})
		}
		out = append(out, st)
	}
	return out
}

var (
	versionOnce sync.Once
	versionText string
)

// FFmpegVersion is the first line of `ffmpeg -version`, read once.
func (h *Hub) FFmpegVersion() string {
	versionOnce.Do(func() {
		out, err := exec.Command(h.FFmpeg, "-hide_banner", "-version").Output()
		if err != nil {
			versionText = "not found"
			return
		}
		versionText, _, _ = strings.Cut(string(out), "\n")
	})
	return versionText
}

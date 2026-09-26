package live

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Host is what startup measured, and the picture sizes that measurement allows.
// Height is the tallest transcode. Focus is the selected tile. Tiles is how
// many of those encodes can run at once. FullRate is field rate on a 540p or
// 360p tile; 720p is always field rate.
type Host struct {
	Class    string
	Encoder  string
	Speed    float64
	Height   int
	Focus    string
	Tiles    int
	FullRate bool
}

// GPUVendor reads the render node's PCI vendor, when the host exposes one.
// The bench and the encode open renderD128, so that node's vendor wins over
// any other node that happens to sort first.
func GPUVendor() string {
	if vendor := readVendor("/sys/class/drm/renderD128/device/vendor"); vendor != "" {
		return vendor
	}
	matches, _ := filepath.Glob("/sys/class/drm/renderD*/device/vendor")
	if len(matches) == 0 {
		matches, _ = filepath.Glob("/sys/class/drm/card*/device/vendor")
	}
	for _, path := range matches {
		if vendor := readVendor(path); vendor != "" {
			return vendor
		}
	}
	return ""
}

func readVendor(path string) string {
	raw, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(raw))
}

// Classify names the encoder family. Vendor is a PCI id such as 0x8086.
// An empty vendor on VAAPI stays "vaapi", because Intel and AMD share that encoder.
func Classify(encoder, vendor string) string {
	switch {
	case strings.Contains(encoder, "nvenc"):
		return "nvidia"
	case strings.Contains(encoder, "videotoolbox"):
		return "apple"
	case strings.Contains(encoder, "qsv"):
		return "intel-qsv"
	case strings.Contains(encoder, "vaapi"):
		switch strings.TrimSpace(vendor) {
		case "0x8086":
			return "intel-vaapi"
		case "0x1002", "0x1022":
			return "amd-vaapi"
		default:
			return "vaapi"
		}
	default:
		return "software"
	}
}

// Budget turns one 1080p60 speed into the rendition and the tile budget.
// Speed is how many times faster than real time the three-second encode ran.
// Zero means the bench did not finish: a GPU keeps a modest budget, and
// software stays on the small picture (a Pi 5 or a J4125-class CPU).
func Budget(class string, speed float64) Host {
	h := Host{Class: class, Speed: speed}
	switch {
	case speed >= 4:
		h.Height, h.Focus, h.Tiles, h.FullRate = 1080, "720", 4, true
	case speed >= 2:
		h.Height, h.Focus, h.Tiles, h.FullRate = 1080, "720", 2, true
	case speed >= 1:
		h.Height, h.Focus, h.Tiles, h.FullRate = 720, "540", 2, true
	case speed >= 0.5:
		h.Height, h.Focus, h.Tiles, h.FullRate = 540, "360", 1, true
	case speed > 0:
		h.Height, h.Focus, h.Tiles, h.FullRate = 540, "360", 1, false
	default:
		if class == "" || class == "software" {
			h.Height, h.Focus, h.Tiles, h.FullRate = 540, "360", 1, false
		} else {
			h.Height, h.Focus, h.Tiles, h.FullRate = 1080, "720", 2, true
		}
	}
	return h
}

// MeasureHost benches the encoder already chosen and stores the budget.
func MeasureHost(ctx context.Context, ffmpeg, encoder string) Host {
	if encoder == "" {
		encoder = "libx264"
	}
	class := Classify(encoder, GPUVendor())
	// The process error is ignored on purpose. A killed ffmpeg reports the
	// signal, not the deadline, so a bench that ran out of time is ctx's error.
	speed, _ := BenchEncoder(ctx, ffmpeg, encoder)
	if speed <= 0 {
		speed = 0
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) && speed <= 0 {
		h := Budget(class, 0.4)
		h.Speed = 0
		h.Encoder = encoder
		return h
	}
	h := Budget(class, speed)
	h.Encoder = encoder
	return h
}

func (h Host) displayName() string {
	switch h.Class {
	case "intel-vaapi", "intel-qsv":
		return "Intel GPU"
	case "amd-vaapi":
		return "AMD GPU"
	case "nvidia":
		return "NVIDIA GPU"
	case "apple":
		return "Apple GPU"
	case "software":
		return "Software"
	default:
		return EncoderName(h.Encoder)
	}
}

// Line is the Diagnostics sentence for this host.
func (h Host) Line() string {
	name := h.displayName()
	var base string
	if h.Speed <= 0 {
		if name == "Software" {
			base = "Software encoder"
		} else {
			base = name + " found"
		}
	} else {
		rate := fmt.Sprintf("%.1fx", h.Speed)
		if h.Speed >= 10 {
			rate = fmt.Sprintf("%.0fx", h.Speed)
		}
		if name == "Software" {
			base = fmt.Sprintf("Software encoder: 1080p60 at %s real time", rate)
		} else {
			base = fmt.Sprintf("%s found: 1080p60 at %s real time", name, rate)
		}
	}
	var b strings.Builder
	b.WriteString(base)
	b.WriteString(".")
	if h.Height > 0 && h.Height < 1080 {
		fmt.Fprintf(&b, " Picture up to %dp.", h.Height)
	}
	if h.Focus != "" && h.Tiles > 0 {
		rate := h.Focus + "p"
		if h.FullRate || h.Focus == "720" {
			rate += "60"
		}
		tiles := "1 tile"
		if h.Tiles != 1 {
			tiles = fmt.Sprintf("%d tiles", h.Tiles)
		}
		fmt.Fprintf(&b, " %s on the selected tile, %s.", rate, tiles)
	}
	return b.String()
}

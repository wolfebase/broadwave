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
// Height is the tallest transcode. Focus is the large tile of a multiview
// (two up, or the big one beside smaller ones; a quad is all 360p). Tiles is how
// many encodes of any size can run at once. FullRate is field rate on a 540p or
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
// Speed is how many times faster than real time BenchEncoder ran. Zero means
// the bench did not finish: a GPU keeps a modest budget, and software stays
// on the small picture (a Pi 5 or a J4125-class CPU).
func Budget(class string, speed float64) Host {
	if class == "" || class == "software" {
		return softwareBudget(class, speed)
	}
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
		h.Height, h.Focus, h.Tiles, h.FullRate = 1080, "720", 2, true
	}
	return h
}

// softwareBudget leaves every layout half again the CPU it needs. Each
// encode decodes and deinterlaces its own 1080i feed. Measured on 1080i
// MPEG-2 with the live settings, at 3 and at 6 cores, one encode at real time
// takes this much of the bench speed: a 360p60 tile 0.79, 540p60 0.90,
// 720p60 1.27. So a quad needs 4.7 and two 720p60 pictures 3.8. One big
// and three (720p60 and three at 540p60) needs 6.0; at 6.2 it ran 1.4x
// and 1.6x, and a broadcast runs about 20% slower than the test picture.
func softwareBudget(class string, speed float64) Host {
	h := Host{Class: class, Speed: speed}
	switch {
	case speed >= 5.5:
		h.Height, h.Focus, h.Tiles, h.FullRate = 1080, "720", 4, true
	case speed >= 3.8:
		h.Height, h.Focus, h.Tiles, h.FullRate = 1080, "720", 2, true
	case speed >= 2.7:
		h.Height, h.Focus, h.Tiles, h.FullRate = 720, "540", 2, true
	case speed >= 1.4:
		h.Height, h.Focus, h.Tiles, h.FullRate = 540, "360", 1, true
	default:
		h.Height, h.Focus, h.Tiles, h.FullRate = 540, "360", 1, false
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
	h := budgetFor(class, encoder, speed)
	h.Encoder = encoder
	return h
}

// budgetFor is Budget for the encoder that will run. One encode on Intel's
// fixed-function encoder is held back by its own pipeline, not by the GPU:
// on a UHD 770 that ran one 1080 encode at 7x, four 1080 encodes and four
// tiles together still each ran at 2.4x or faster. Six pictures fit a quad
// beside two full screens and leave room for another app's transcodes.
func budgetFor(class, encoder string, speed float64) Host {
	h := Budget(class, speed)
	if speed >= 4 && lowPower[encoder] && strings.HasPrefix(class, "intel") {
		h.Tiles = 6
	}
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
		pictures := "one picture at a time"
		if h.Tiles != 1 {
			pictures = fmt.Sprintf("%d pictures at once", h.Tiles)
		}
		fmt.Fprintf(&b, " %s on a large tile, %s.", rate, pictures)
	}
	return b.String()
}

// softwareFirst is the BenchLive speed at which a CPU encode is preferred to
// a GPU. A broadcast runs about 20% slower than the bench, so 2.5x holds two
// live 1080i pictures.
const softwareFirst = 2.5

// PreferSoftware reports whether a CPU that ran BenchLive at speed should
// carry live encodes instead of a GPU that other apps on the server share.
func PreferSoftware(speed float64) bool {
	return speed >= softwareFirst
}

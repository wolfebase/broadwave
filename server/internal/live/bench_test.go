package live

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestParseSpeedUsesTheLastFigure(t *testing.T) {
	log := "frame=30 speed=5.5x\nframe=60 fps=660 speed=11.2x\n"
	if got := ParseSpeed(log); got != 11.2 {
		t.Fatalf("speed %v", got)
	}
	if ParseSpeed("no speed here") != 0 {
		t.Fatal("missing speed should be 0")
	}
}

func TestFormatEncoderLine(t *testing.T) {
	if got := FormatEncoderLine("h264_videotoolbox", 11, true); got != "Apple GPU found: 1080p60 at 11x real time." {
		t.Fatalf("apple: %q", got)
	}
	if got := FormatEncoderLine("libx264", 2.4, true); got != "Software encoder: 1080p60 at 2.4x real time." {
		t.Fatalf("software: %q", got)
	}
	if got := FormatEncoderLine("h264_nvenc", 0, false); got != "NVIDIA GPU found." {
		t.Fatalf("failed: %q", got)
	}
	if got := FormatEncoderLine("libx264", 0, false); got != "Software encoder." {
		t.Fatalf("software failed: %q", got)
	}
	if got := EncoderName("h264_qsv"); got != "Intel GPU" {
		t.Fatalf("qsv: %q", got)
	}
}

func TestBenchEncoderParsesAScript(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ffmpeg")
	script := "#!/bin/sh\necho 'speed=4.0x' >&2\necho 'frame=60 speed=11x' >&2\nexit 0\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	speed, err := BenchEncoder(context.Background(), path, "h264_videotoolbox")
	if err != nil {
		t.Fatal(err)
	}
	if speed != 11 {
		t.Fatalf("speed %v", speed)
	}
}

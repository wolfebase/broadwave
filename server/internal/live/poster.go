package live

import (
	"os"
	"os/exec"
	"path/filepath"
)

// EnsurePoster writes a JPEG still from a recording when one is not already on disk.
func EnsurePoster(ffmpeg, src, dest string) error {
	if _, err := os.Stat(dest); err == nil {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	cmd := exec.Command(ffmpeg,
		"-hide_banner", "-loglevel", "error",
		"-ss", "2",
		"-i", src,
		"-frames:v", "1",
		"-vf", "scale=480:-2",
		dest,
	)
	return cmd.Run()
}

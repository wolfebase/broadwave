package live

import (
	"bytes"
	"log"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"broadwave/internal/logbuf"
)

func TestFFmpegProgressDropsAPassword(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "ffmpeg")
	script := "#!/bin/sh\necho 'open http://user:secretpass@playlist.example/a.ts' >&2\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	prevLog := log.Writer()
	prevSlog := slog.Default()
	t.Cleanup(func() {
		log.SetOutput(prevLog)
		slog.SetDefault(prevSlog)
	})
	var buf bytes.Buffer
	logbuf.Install(&buf)
	cmd := exec.Command(bin)
	logCommand(cmd)
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), "secretpass") || !strings.Contains(buf.String(), "playlist.example") {
		t.Fatalf("log %s", buf.String())
	}
}

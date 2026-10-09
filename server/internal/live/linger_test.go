package live

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// A followBreak whose new encode cannot start falls back to a restart. The
// encode it replaces has to be killed: its watch only returns once it exits,
// and stop signals only the command stored on the rendition.
func TestFollowBreakFailureKillsTheEncodeThatStaysUp(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "ffmpeg")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\nexec sleep 60\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	h, m := testHub(t)
	m.input = "sample.ts"
	h.FFmpeg = bin
	f := addTestFeed(h, m, 1, "4.1")

	spec, ok := ParseRenditionKey("1080.aac2.broadcast")
	if !ok {
		t.Fatal("rendition key")
	}
	dir := filepath.Join(h.Dir, spec.Key())
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	oldCmd := exec.Command("sleep", "30")
	stdin, err := oldCmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := oldCmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		_ = oldCmd.Wait()
		close(done)
	}()
	t.Cleanup(func() {
		if oldCmd.Process != nil {
			_ = oldCmd.Process.Kill()
		}
		<-done
		h.mu.Lock()
		if live := h.channels[1]; live != nil {
			h.stopFeedLocked(live)
		}
		h.mu.Unlock()
	})

	// The subscriber closes this stdin when it is detached. sleep ignores that.
	p := &programPipe{w: stdin, program: 1, sw: &pipeSwitch{}, onBreak: func() {}}
	r := &rendition{
		spec: spec, dir: dir, cmd: oldCmd, stdin: stdin, filter: p,
		input: &packInput{cur: io.NopCloser(bytes.NewReader(nil)), done: true},
		args:  []string{"-hide_banner", "-i", "sample.ts"},
	}
	h.mu.Lock()
	f.renditions[spec.Key()] = r
	r.sub = h.attachPipeLocked(muxOf(h, f), p)
	h.mu.Unlock()

	h.followBreak(f, r, p)
	h.mu.Lock()
	replaced := r.cmd != nil && r.cmd != oldCmd
	h.mu.Unlock()
	if !replaced {
		t.Fatal("followBreak did not leave a replacement encode running")
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("the encode that stayed up was not killed")
	}
}

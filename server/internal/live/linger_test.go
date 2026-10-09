package live

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// A replaced encode is killed if it is still running after lingerKill. Once
// Wait has reaped it, that kill must not run: the pid can belong to another
// process by then.
func TestFollowBreakDoesNotSignalAReapedEncode(t *testing.T) {
	oldLinger := lingerKill
	lingerKill = time.Second
	oldSignal := signalProcess
	var mu sync.Mutex
	var killed []int
	signalProcess = func(p *os.Process) error {
		if p == nil {
			return nil
		}
		mu.Lock()
		killed = append(killed, p.Pid)
		mu.Unlock()
		return nil
	}
	t.Cleanup(func() {
		lingerKill = oldLinger
		signalProcess = oldSignal
	})

	bin := filepath.Join(t.TempDir(), "ffmpeg")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\nexec sleep 60\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	h, m := testHub(t)
	m.input = "sample.ts"
	h.FFmpeg = bin
	f := addTestFeed(h, m, 1, "4.1")
	t.Cleanup(func() {
		h.mu.Lock()
		if live := h.channels[1]; live != nil {
			h.stopFeedLocked(live)
		}
		h.mu.Unlock()
	})

	spec, ok := ParseRenditionKey("1080.aac2.broadcast")
	if !ok {
		t.Fatal("rendition key")
	}
	dir := filepath.Join(h.Dir, spec.Key())
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	oldCmd := exec.Command("cat")
	stdin, err := oldCmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := oldCmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stdin.Close() })
	p := &programPipe{program: 1, sw: &pipeSwitch{}, onBreak: func() {}}
	r := &rendition{
		spec: spec, dir: dir, cmd: oldCmd, stdin: stdin,
		input:  &packInput{cur: io.NopCloser(bytes.NewReader(nil))},
		filter: p,
	}
	h.mu.Lock()
	f.renditions[spec.Key()] = r
	h.mu.Unlock()
	go h.watchRendition(f, r, oldCmd, "libx264", true)

	h.followBreak(f, r, p)
	if r.cmd == nil || r.cmd == oldCmd {
		t.Fatal("followBreak did not start the next encode")
	}
	if _, armed := lingerStops.Load(oldCmd); !armed {
		t.Fatal("the old encode has no kill backstop")
	}
	// The program pipe closes the old encode's input on the next write.
	// Closing it here is that hand-off: cat exits and Wait reaps it.
	if err := stdin.Close(); err != nil {
		t.Fatal(err)
	}
	signaled := func() bool {
		mu.Lock()
		defer mu.Unlock()
		for _, pid := range killed {
			if pid == oldCmd.Process.Pid {
				return true
			}
		}
		return false
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		if signaled() {
			t.Fatalf("signaled reaped encode pid %d", oldCmd.Process.Pid)
		}
		if _, armed := lingerStops.Load(oldCmd); !armed {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("a reaped encode kept its kill backstop")
		}
		time.Sleep(5 * time.Millisecond)
	}
	// Past the backstop's own deadline. One that was not dropped would have fired.
	time.Sleep(lingerKill + 100*time.Millisecond)
	if signaled() {
		t.Fatalf("signaled reaped encode pid %d", oldCmd.Process.Pid)
	}
}

// A failed followBreak that starts a replacement never signals the encode
// it replaced. That encode's watch only returns once the command changes,
// and stop signals only the command stored on the rendition.
func TestFollowBreakFailureKillsTheEncodeThatStaysUp(t *testing.T) {
	oldLinger := lingerKill
	lingerKill = 40 * time.Millisecond
	t.Cleanup(func() { lingerKill = oldLinger })

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
		disarmLingerKill(oldCmd)
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

	// The subscriber closes this stdin when it is detached. sleep ignores
	// that, which is the encode the break backstop is for.
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

// The backstop still kills an encode that ignores a closed input.
func TestLingerKillStopsAnEncodeThatStaysUp(t *testing.T) {
	oldLinger := lingerKill
	lingerKill = 40 * time.Millisecond
	t.Cleanup(func() { lingerKill = oldLinger })

	cmd := exec.Command("sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(done)
	}()
	t.Cleanup(func() {
		disarmLingerKill(cmd)
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		<-done
	})
	armLingerKill(cmd)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("an encode that stayed up was not killed")
	}
}

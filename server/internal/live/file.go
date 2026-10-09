package live

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// fileGraph builds the library picture. order is the recording's scan type:
// "progressive", "film", a field order, or empty. Progressive 720p keeps its
// size and rate. Soft telecine plays at 24 when the picture mode is broadcast.
// An empty order stays interlaced, which is what a 1080i recording needs.
func (h *Hub) fileGraph(codec, mode, order string) Graph {
	mode = NormalizeMode(mode)
	if order == "film" && mode == "broadcast" {
		mode = "film"
	}
	return Graph{
		VideoCodec: codec, Profile: "transparent", Audio: "stereo",
		Encoder: h.Encoder, Mode: mode, Deint: h.deintFor(mode, codec),
		Progressive: order == "progressive",
	}
}

// recordingOrder prefers the file's own headers. The channel's stored scan is
// only a fallback for a file that has not written a sequence header yet, and a
// stored "film" is the station, not this recording.
func recordingOrder(fileOrder string, fileOK bool, channelOrder string) string {
	if fileOK && fileOrder != "" {
		return fileOrder
	}
	if storedFieldOrder(channelOrder) == "progressive" {
		return "progressive"
	}
	return ""
}

// fileScanBytes covers a GOP. A 720p sequence header can sit a few hundred
// milliseconds into the recording, and soft telecine needs several pictures.
const fileScanBytes = 4 << 20

func fileScanOrder(path string) (string, bool) {
	f, err := os.Open(path)
	if err != nil {
		return "", false
	}
	defer f.Close()
	buf := make([]byte, fileScanBytes)
	n, err := io.ReadFull(f, buf)
	if n < 188 || (err != nil && err != io.ErrUnexpectedEOF && err != io.EOF) {
		return "", false
	}
	return scanType(buf[:n], 0)
}

// fileAC4 reports a recording whose sound is AC-4, from the PMT at its head.
func fileAC4(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	buf := make([]byte, fileScanBytes)
	n, _ := io.ReadFull(f, buf)
	for _, t := range AudioTracks(buf[:n], 0) {
		if t.Codec == "ac4" {
			return true
		}
	}
	return false
}

// fileGraphFor is fileGraph for the recording at path. Its sound is left out
// when it is AC-4 and ffmpeg cannot decode it.
func (h *Hub) fileGraphFor(path, codec, mode, fieldOrder string) Graph {
	scanned, ok := fileScanOrder(path)
	g := h.fileGraph(codec, mode, recordingOrder(scanned, ok, fieldOrder))
	if h.NoAC4 && fileAC4(path) {
		g.Audio = "none"
	}
	return g
}

func graphStamp(g Graph) string {
	scan := "interlaced"
	if g.Progressive {
		scan = "progressive"
	} else if NormalizeMode(g.Mode) == "film" {
		scan = "film"
	}
	parts := []string{NormalizeMode(g.Mode), g.VideoCodec, g.Encoder, g.Deint, g.Profile, scan}
	// A silent playlist is redone once ffmpeg can decode the sound.
	if g.Audio == "none" {
		parts = append(parts, "silent")
	}
	return strings.Join(parts, "|")
}

// playStart is how long playback waits for the recording playlist.
// A test shortens it. The encode still has to exit when the wait ends.
var playStart = 20 * time.Second

// captionSpan bounds the sidecar caption pass. The play error path cancels
// it so that process does not keep running after the player has given up.
var captionSpan = 2 * time.Minute

func playlistFresh(dir, stamp string) bool {
	body, err := os.ReadFile(filepath.Join(dir, "graph.txt"))
	if err != nil || strings.TrimSpace(string(body)) != stamp {
		return false
	}
	info, err := os.Stat(filepath.Join(dir, "index.m3u8"))
	return err == nil && info.Size() > 0
}

// PlayFile transcodes a finished recording into an HLS playlist and returns when the first segment exists.
func (h *Hub) PlayFile(id int64, path, videoCodec, mode, fieldOrder string) (string, error) {
	dir := filepath.Join(h.Dir, "file", fmt.Sprintf("%d", id))
	g := h.fileGraphFor(path, videoCodec, mode, fieldOrder)
	g.Live = false
	stamp := graphStamp(g)
	playlist := filepath.Join(dir, "index.m3u8")
	if playlistFresh(dir, stamp) {
		return fmt.Sprintf("/media/file/%d/index.m3u8", id), nil
	}
	_ = os.RemoveAll(dir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	g.Input = abs
	cmd := exec.Command(h.FFmpeg, PictureArgs(g)...)
	cmd.Dir = dir
	cmd.Stderr = os.Stderr
	if err := os.WriteFile(filepath.Join(dir, "graph.txt"), []byte(stamp), 0o644); err != nil {
		return "", err
	}
	if err := cmd.Start(); err != nil {
		return "", err
	}
	capCtx, capCancel := context.WithTimeout(context.Background(), captionSpan)
	go func() {
		defer capCancel()
		extractCaptions(capCtx, h.FFmpeg, abs, filepath.Join(dir, "captions.vtt"))
	}()
	url := fmt.Sprintf("/media/file/%d/index.m3u8", id)
	if waitForPlaylist(playlist) {
		go func() { _ = cmd.Wait() }()
		return url, nil
	}
	// Nothing has Waited on this encode, so Kill still names our process.
	capCancel()
	stopUnwatched(cmd)
	return "", fmt.Errorf("recording player did not start")
}

// PlayFollow transcodes a recording that is still being written. Playback starts at the beginning of the file.
func (h *Hub) PlayFollow(id int64, path, videoCodec, mode, fieldOrder string, still func() bool) (string, error) {
	dir := filepath.Join(h.Dir, "file", fmt.Sprintf("%d", id))
	g := h.fileGraphFor(path, videoCodec, mode, fieldOrder)
	g.Input = "pipe:0"
	g.Live = false
	stamp := graphStamp(g)
	playlist := filepath.Join(dir, "index.m3u8")
	if playlistFresh(dir, stamp) {
		return fmt.Sprintf("/media/file/%d/index.m3u8", id), nil
	}
	h.playMu.Lock()
	if h.plays == nil {
		h.plays = map[int64]struct{}{}
	}
	if _, running := h.plays[id]; running {
		h.playMu.Unlock()
		return waitPlaylistFile(playlist, id)
	}
	h.plays[id] = struct{}{}
	h.playMu.Unlock()

	_ = os.RemoveAll(dir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		h.clearPlay(id)
		return "", err
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		h.clearPlay(id)
		return "", err
	}
	cmd := exec.Command(h.FFmpeg, PictureArgs(g)...)
	cmd.Dir = dir
	cmd.Stderr = os.Stderr
	if err := os.WriteFile(filepath.Join(dir, "graph.txt"), []byte(stamp), 0o644); err != nil {
		h.clearPlay(id)
		return "", err
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		h.clearPlay(id)
		return "", err
	}
	if err := cmd.Start(); err != nil {
		h.clearPlay(id)
		return "", err
	}
	ctx, cancel := context.WithCancel(context.Background())
	hold := &encodeHold{cmd: cmd}
	stopped := make(chan struct{})
	go func() {
		defer cancel()
		followFile(ctx, abs, stdin, still)
		_ = hold.wait()
		h.clearPlay(id)
		close(stopped)
	}()
	if waitForPlaylist(playlist) {
		return fmt.Sprintf("/media/file/%d/index.m3u8", id), nil
	}
	// Kill before the copy goroutine's Wait reaps the pid, then cancel so
	// that Wait can run and release the recording.
	hold.kill()
	cancel()
	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
	}
	return "", fmt.Errorf("recording player did not start")
}

func extractCaptions(ctx context.Context, ffmpeg, path, dest string) {
	if ffmpeg == "" || path == "" || ctx.Err() != nil {
		return
	}
	cmd := exec.CommandContext(ctx, ffmpeg, "-hide_banner", "-loglevel", "error", "-i", path, "-map", "0:s:0", "-f", "webvtt", dest)
	_ = cmd.Run()
}

func (h *Hub) clearPlay(id int64) {
	h.playMu.Lock()
	delete(h.plays, id)
	h.playMu.Unlock()
}

func playlistAppeared(playlist string) bool {
	info, err := os.Stat(playlist)
	return err == nil && info.Size() > 0
}

func waitForPlaylist(playlist string) bool {
	deadline := time.Now().Add(playStart)
	for time.Now().Before(deadline) {
		if playlistAppeared(playlist) {
			return true
		}
		time.Sleep(200 * time.Millisecond)
	}
	return playlistAppeared(playlist)
}

func waitPlaylistFile(playlist string, id int64) (string, error) {
	if waitForPlaylist(playlist) {
		return fmt.Sprintf("/media/file/%d/index.m3u8", id), nil
	}
	return "", fmt.Errorf("recording player did not start")
}

// stopUnwatched kills an encode this goroutine has not Waited on, then reaps it.
// Kill after Wait is unsafe: the pid may already belong to another process.
func stopUnwatched(cmd *exec.Cmd) {
	if cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
	_ = cmd.Wait()
}

// encodeHold is a started encode whose Wait runs on the copy goroutine.
// kill does nothing once that Wait has reaped the pid.
type encodeHold struct {
	cmd    *exec.Cmd
	mu     sync.Mutex
	reaped bool
}

func (e *encodeHold) wait() error {
	err := e.cmd.Wait()
	e.mu.Lock()
	e.reaped = true
	e.mu.Unlock()
	return err
}

func (e *encodeHold) kill() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.reaped || e.cmd == nil || e.cmd.Process == nil {
		return
	}
	_ = e.cmd.Process.Kill()
}

// followFile copies a growing recording into ffmpeg until the recording has
// stopped and no new bytes arrive. A cancelled context ends the copy so the
// caller can reap the encode.
func followFile(ctx context.Context, path string, dst io.WriteCloser, still func() bool) {
	defer dst.Close()
	var file *os.File
	deadline := time.Now().Add(15 * time.Second)
	for {
		if ctx.Err() != nil {
			return
		}
		opened, err := os.Open(path)
		if err == nil {
			file = opened
			break
		}
		if !still() || time.Now().After(deadline) {
			return
		}
		if !pauseFollow(ctx, 200*time.Millisecond) {
			return
		}
	}
	defer file.Close()
	buf := make([]byte, 64*1024)
	quiet := 0
	for {
		if ctx.Err() != nil {
			return
		}
		n, err := file.Read(buf)
		if n > 0 {
			quiet = 0
			if _, werr := dst.Write(buf[:n]); werr != nil {
				return
			}
		}
		if err == io.EOF || n == 0 {
			if !still() {
				quiet++
				if quiet >= 2 {
					return
				}
			}
			if !pauseFollow(ctx, 300*time.Millisecond) {
				return
			}
			continue
		}
		if err != nil {
			return
		}
	}
}

func pauseFollow(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

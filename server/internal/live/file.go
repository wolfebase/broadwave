package live

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func (h *Hub) fileGraph(codec, mode string) Graph {
	return Graph{
		VideoCodec: codec, Profile: "transparent", Audio: "stereo",
		Encoder: h.Encoder, Mode: mode, Deint: h.deintFor(mode, codec),
	}
}

func graphStamp(g Graph) string {
	return strings.Join([]string{NormalizeMode(g.Mode), g.VideoCodec, g.Encoder, g.Deint, g.Profile}, "|")
}

func playlistFresh(dir, stamp string) bool {
	body, err := os.ReadFile(filepath.Join(dir, "graph.txt"))
	if err != nil || strings.TrimSpace(string(body)) != stamp {
		return false
	}
	info, err := os.Stat(filepath.Join(dir, "index.m3u8"))
	return err == nil && info.Size() > 0
}

// PlayFile transcodes a finished recording into an HLS playlist and returns when the first segment exists.
func (h *Hub) PlayFile(id int64, path, videoCodec, mode string) (string, error) {
	dir := filepath.Join(h.Dir, "file", fmt.Sprintf("%d", id))
	g := h.fileGraph(videoCodec, mode)
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
	go func() { _ = cmd.Wait() }()
	go extractCaptions(h.FFmpeg, abs, filepath.Join(dir, "captions.vtt"))
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if info, err := os.Stat(playlist); err == nil && info.Size() > 0 {
			return fmt.Sprintf("/media/file/%d/index.m3u8", id), nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	return "", fmt.Errorf("recording player did not start")
}

// PlayFollow transcodes a recording that is still being written. Playback starts at the beginning of the file.
func (h *Hub) PlayFollow(id int64, path, videoCodec, mode string, still func() bool) (string, error) {
	dir := filepath.Join(h.Dir, "file", fmt.Sprintf("%d", id))
	g := h.fileGraph(videoCodec, mode)
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
	go func() {
		followFile(abs, stdin, still)
		_ = cmd.Wait()
		h.clearPlay(id)
	}()
	return waitPlaylistFile(playlist, id)
}

func extractCaptions(ffmpeg, path, dest string) {
	if ffmpeg == "" || path == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, ffmpeg, "-hide_banner", "-loglevel", "error", "-i", path, "-map", "0:s:0", "-f", "webvtt", dest)
	_ = cmd.Run()
}

func (h *Hub) clearPlay(id int64) {
	h.playMu.Lock()
	delete(h.plays, id)
	h.playMu.Unlock()
}

func waitPlaylistFile(playlist string, id int64) (string, error) {
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if info, err := os.Stat(playlist); err == nil && info.Size() > 0 {
			return fmt.Sprintf("/media/file/%d/index.m3u8", id), nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	return "", fmt.Errorf("recording player did not start")
}

// followFile copies a growing recording into ffmpeg until the recording has stopped and no new bytes arrive.
func followFile(path string, dst io.WriteCloser, still func() bool) {
	defer dst.Close()
	var file *os.File
	deadline := time.Now().Add(15 * time.Second)
	for {
		opened, err := os.Open(path)
		if err == nil {
			file = opened
			break
		}
		if !still() || time.Now().After(deadline) {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	defer file.Close()
	buf := make([]byte, 64*1024)
	quiet := 0
	for {
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
			time.Sleep(300 * time.Millisecond)
			continue
		}
		if err != nil {
			return
		}
	}
}

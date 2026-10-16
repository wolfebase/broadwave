package live

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var preloadHint = []byte("#EXT-X-PRELOAD-HINT:")

// StripPreloadHint drops EXT-X-PRELOAD-HINT lines. The hint makes a player
// ask for a part before it exists and hold the request open; over HTTP/1.1
// that held request takes one of a browser's six connections, and AVPlayer
// plays low latency only over HTTP/2 anyway.
func StripPreloadHint(body []byte) []byte {
	if !bytes.Contains(body, preloadHint) {
		return body
	}
	out := make([]byte, 0, len(body))
	for line := range bytes.SplitAfterSeq(body, []byte("\n")) {
		if !bytes.HasPrefix(line, preloadHint) {
			out = append(out, line...)
		}
	}
	return out
}

var hintedPart = regexp.MustCompile(`^part([0-9]{5,9})((?:\.[a-z0-9]+)?\.m4s)$`)

// PartNumber reads a part's file name, a view's too (part00012.v.m4s): the
// number, the rendition's own file for it, and the name of the part before.
func PartNumber(name string) (n int, own, prev string, ok bool) {
	m := hintedPart.FindStringSubmatch(name)
	if m == nil {
		return 0, "", "", false
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		return 0, "", "", false
	}
	own = partName5(n)
	if n > 0 {
		prev = partName5(n - 1)
	}
	return n, own, prev, true
}

func partName5(n int) string {
	s := strconv.Itoa(n)
	for len(s) < 5 {
		s = "0" + s
	}
	return "part" + s + ".m4s"
}

// partOnDisk reports a part written in dir: its own file, or the pointer
// into its segment that replaces the file once the segment closes.
func partOnDisk(dir, name string) bool {
	if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
		return true
	}
	_, err := os.Stat(filepath.Join(dir, strings.TrimSuffix(name, ".m4s")+".ref"))
	return err == nil
}

// WaitPart holds a request for the part a playlist hinted (EXT-X-PRELOAD-HINT)
// until the packer writes it, for at most limit. Only the part right after
// the newest one waits; any other missing part answers at once. It reports
// whether the part is there.
func WaitPart(ctx context.Context, dir, name string, limit time.Duration) bool {
	_, own, prev, ok := PartNumber(name)
	if !ok {
		return false
	}
	if partOnDisk(dir, own) {
		return true
	}
	if prev == "" || !partOnDisk(dir, prev) {
		return false
	}
	deadline := time.NewTimer(limit)
	defer deadline.Stop()
	tick := time.NewTicker(20 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return false
		case <-deadline.C:
			return partOnDisk(dir, own)
		case <-tick.C:
			if partOnDisk(dir, own) {
				return true
			}
		}
	}
}

package live

import (
	"bytes"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// resumeMin is the closest a resume gets to either end of a recording before
// playback starts at the beginning. Near the start there is nothing to skip.
// Near the end an input seek can land past the last picture and the encode
// writes no segments.
const resumeMin = 2

// ResumeAt is the input seek, in seconds, for a resume at position into a
// recording of duration seconds. Zero starts at the beginning.
func ResumeAt(position, duration float64) float64 {
	if math.IsNaN(position) || math.IsInf(position, 0) || position < resumeMin || duration <= 0 {
		return 0
	}
	if position > duration-resumeMin {
		position = duration - resumeMin
	}
	if position < resumeMin {
		return 0
	}
	return position
}

// ExportResume is where an export of a recording starts, in seconds.
// A finished file seeks to the schedule point, clamped so the seek stays on
// a picture. A growing file can be seeked only through media already on disk:
// past that point ffmpeg finds nothing and the export ends empty. The copy
// starts a moment behind the last byte so a picture is already there.
func ExportResume(offset, written float64, growing bool) float64 {
	if math.IsNaN(offset) || math.IsInf(offset, 0) || offset < resumeMin {
		return 0
	}
	if !growing {
		return ResumeAt(offset, written)
	}
	if math.IsNaN(written) || math.IsInf(written, 0) || written <= resumeMin {
		return 0
	}
	if offset > written-resumeMin {
		offset = written - resumeMin
	}
	if offset < resumeMin {
		return 0
	}
	return offset
}

func writeFileOffset(dir string, at float64) error {
	return os.WriteFile(filepath.Join(dir, "offset.txt"), []byte(strconv.FormatFloat(at, 'f', 3, 64)+"\n"), 0o644)
}

// FileOffset is the input seek this recording's encode started at, in seconds.
// A playlist written before resumes were seeked has none and starts at 0.
func FileOffset(dir string) float64 {
	body, err := os.ReadFile(filepath.Join(dir, "offset.txt"))
	if err != nil {
		return 0
	}
	at, err := strconv.ParseFloat(strings.TrimSpace(string(body)), 64)
	if err != nil || at < 0 || math.IsNaN(at) {
		return 0
	}
	return at
}

// filePlaylistCovers reports whether the encode already in dir can show at.
// A finished encode that started at or before at can: the player seeks inside
// it. One that started later cannot, and playback starts again at at.
func filePlaylistCovers(dir string, at float64) bool {
	from := FileOffset(dir)
	if at < resumeMin {
		at = 0
	}
	if from < resumeMin {
		from = 0
	}
	if from > at+0.05 {
		return false
	}
	path := filepath.Join(dir, "index.m3u8")
	if playlistEnded(path) {
		return true
	}
	if from+fileSpan(path)+0.05 >= at {
		return true
	}
	// Still encoding from this resume. An encode that started earlier has not
	// reached here, so a deep resume would wait out the prefix.
	return at-from <= resumeMin
}

func playlistEnded(path string) bool {
	body, err := os.ReadFile(path)
	return err == nil && bytes.Contains(body, []byte("#EXT-X-ENDLIST"))
}

func fileSpan(path string) float64 {
	body, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	var sum float64
	for _, line := range strings.Split(string(body), "\n") {
		trim := strings.TrimSpace(line)
		rest, ok := strings.CutPrefix(trim, "#EXTINF:")
		if !ok {
			continue
		}
		sec, err := strconv.ParseFloat(strings.SplitN(rest, ",", 2)[0], 64)
		if err == nil && sec > 0 {
			sum += sec
		}
	}
	return sum
}

// OffsetPlaylist puts a resume on the recording's clock. The encode input-seeks,
// so its first segment is that many seconds in. Gap segments cover the part
// that was not encoded, and playback starts at the resume point. The player's
// seek to that time then lands on the first real segment. A zero offset leaves
// the playlist as ffmpeg wrote it.
func OffsetPlaylist(body []byte, offset float64) []byte {
	if offset < 0.05 || !bytes.Contains(body, []byte("#EXTINF:")) {
		return body
	}
	if bytes.Contains(body, []byte("#EXT-X-START:")) && bytes.Contains(body, []byte("gap00000.ts")) {
		return body
	}
	text := strings.ReplaceAll(string(body), "\r\n", "\n")
	lines := strings.Split(text, "\n")
	var head, tail []string
	media := false
	for _, line := range lines {
		if !media && mediaLine(line) {
			media = true
		}
		if media {
			tail = append(tail, line)
		} else {
			head = append(head, line)
		}
	}
	head = withPlaylistVersion(head, 8)
	head = insertStartTag(head, offset)
	var b strings.Builder
	for _, line := range head {
		if line == "" {
			continue
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	writeGaps(&b, offset)
	for _, line := range tail {
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return []byte(b.String())
}

func mediaLine(line string) bool {
	trim := strings.TrimSpace(line)
	switch {
	case trim == "":
		return false
	case strings.HasPrefix(trim, "#EXTINF:"),
		trim == "#EXT-X-DISCONTINUITY",
		strings.HasPrefix(trim, "#EXT-X-MAP:"),
		strings.HasPrefix(trim, "#EXT-X-PROGRAM-DATE-TIME:"),
		strings.HasPrefix(trim, "#EXT-X-GAP"):
		return true
	default:
		return !strings.HasPrefix(trim, "#")
	}
}

func withPlaylistVersion(head []string, min int) []string {
	for i, line := range head {
		trim := strings.TrimSpace(line)
		rest, ok := strings.CutPrefix(trim, "#EXT-X-VERSION:")
		if !ok {
			continue
		}
		n, err := strconv.Atoi(strings.TrimSpace(rest))
		if err != nil || n < min {
			head[i] = "#EXT-X-VERSION:" + strconv.Itoa(min)
		}
		return head
	}
	out := make([]string, 0, len(head)+1)
	inserted := false
	for _, line := range head {
		out = append(out, line)
		if !inserted && strings.TrimSpace(line) == "#EXTM3U" {
			out = append(out, "#EXT-X-VERSION:"+strconv.Itoa(min))
			inserted = true
		}
	}
	if !inserted {
		out = append([]string{"#EXT-X-VERSION:" + strconv.Itoa(min)}, out...)
	}
	return out
}

func insertStartTag(head []string, offset float64) []string {
	tag := fmt.Sprintf("#EXT-X-START:TIME-OFFSET=%.3f,PRECISE=YES", offset)
	for _, line := range head {
		if strings.HasPrefix(strings.TrimSpace(line), "#EXT-X-START:") {
			return head
		}
	}
	for _, marker := range []string{"#EXT-X-PLAYLIST-TYPE:", "#EXT-X-MEDIA-SEQUENCE:", "#EXT-X-TARGETDURATION:"} {
		for i, line := range head {
			if strings.HasPrefix(strings.TrimSpace(line), marker) {
				return insertLine(head, i+1, tag)
			}
		}
	}
	return append(head, tag)
}

func insertLine(lines []string, i int, line string) []string {
	if i >= len(lines) {
		return append(lines, line)
	}
	out := make([]string, 0, len(lines)+1)
	out = append(out, lines[:i]...)
	out = append(out, line)
	out = append(out, lines[i:]...)
	return out
}

// writeGaps covers offset seconds with segments no longer than the encode's
// own, so the target duration does not have to grow to fit one long gap.
func writeGaps(b *strings.Builder, offset float64) {
	const seg = 2.0
	left := offset
	for n := 0; left > 0.0005 && n < 20000; n++ {
		d := seg
		if left < seg {
			d = left
		}
		fmt.Fprintf(b, "#EXT-X-GAP\n#EXTINF:%.3f,\ngap%05d.ts\n", d, n)
		left -= d
	}
}

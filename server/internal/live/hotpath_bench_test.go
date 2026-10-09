package live

import (
	"fmt"
	"io"
	"testing"
	"time"
)

// discardCloser drops bytes. A bytes.Buffer would allocate as it grows, and
// that growth would hide the pipe's own allocations.
type discardCloser struct{}

func (discardCloser) Write(p []byte) (int, error) { return len(p), nil }
func (discardCloser) Close() error                { return nil }

// BenchmarkProgramPipeWrite is the steady state of one subchannel: the map
// is already learned, and each write is one tuner read (188*49 bytes).
func BenchmarkProgramPipeWrite(b *testing.B) {
	var raw []byte
	for len(raw) < 188*49 {
		raw = append(raw, twoProgramTS(1, 0x1000, 0x110, 0x111, 2, 0x1001, 0x210)...)
	}
	raw = raw[:188*(len(raw)/188)]
	w := newProgramPipe(discardCloser{}, 1)
	if _, err := w.Write(raw); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.SetBytes(int64(len(raw)))
	b.ResetTimer()
	for b.Loop() {
		if _, err := w.Write(raw); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkStampPlaylist is a full live playlist stamped again for one
// viewer. The segment times are already known, which is the path after the
// first playlist of an encode.
func BenchmarkStampPlaylist(b *testing.B) {
	const n = 200
	var src []byte
	src = append(src, "#EXTM3U\n#EXT-X-VERSION:9\n#EXT-X-TARGETDURATION:2\n#EXT-X-MEDIA-SEQUENCE:0\n"...)
	base := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	var p playlistStamper
	p.cache = map[string]int64{}
	p.walls = map[string]time.Time{}
	for i := range n {
		name := fmt.Sprintf("seg%05d.m4s", i)
		src = append(src, "#EXTINF:2.0,\n"...)
		src = append(src, name...)
		src = append(src, '\n')
		p.cache[name] = int64(i) * 180000
		p.walls[name] = base.Add(time.Duration(i) * 2 * time.Second)
	}
	tl := NewTimeline()
	tl.now = func() time.Time { return base }
	if len(p.stamp("", src, tl)) == 0 {
		b.Fatal("empty playlist")
	}
	b.ReportAllocs()
	b.SetBytes(int64(len(src)))
	b.ResetTimer()
	for b.Loop() {
		if len(p.stamp("", src, tl)) == 0 {
			b.Fatal("empty playlist")
		}
	}
}

var _ io.WriteCloser = discardCloser{}

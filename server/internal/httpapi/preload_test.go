package httpapi

import (
	"net/http/httptest"
	"strings"
	"testing"
)

// The preload hint goes only to HTTP/2 clients, and only when low-latency
// hints are on: AVPlayer needs it for low latency, and a browser on HTTP/1.1
// would hold a connection per hinted part.
func TestPreloadHintOnlyOverHTTP2(t *testing.T) {
	body := []byte("#EXTM3U\n#EXT-X-PART:DURATION=1.001,URI=\"part00004.m4s\"\n#EXT-X-PRELOAD-HINT:TYPE=PART,URI=\"part00005.m4s\"\n")
	r := httptest.NewRequest("GET", "/media/live/1/720.aac2.broadcast/index.m3u8", nil)
	if strings.Contains(string(preloadFor(r, body)), "PRELOAD-HINT") {
		t.Fatal("HTTP/1.1 got the hint")
	}
	r.Proto, r.ProtoMajor, r.ProtoMinor = "HTTP/2.0", 2, 0
	if strings.Contains(string(preloadFor(r, body)), "PRELOAD-HINT") {
		t.Fatal("HTTP/2 got the hint with low-latency hints off")
	}
	lowLatencyHints = true
	defer func() { lowLatencyHints = false }()
	if !strings.Contains(string(preloadFor(r, body)), "#EXT-X-PRELOAD-HINT:TYPE=PART,URI=\"part00005.m4s\"") {
		t.Fatal("HTTP/2 lost the hint")
	}
}

package live

import (
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// The protocol list must still read the HLS a playlist source sends:
// transport-stream segments behind an AES key, and fMP4 segments.
func TestProtocolListStillReadsHLS(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not on PATH")
	}
	dir := t.TempDir()
	srv := httptest.NewServer(http.FileServer(http.Dir(dir)))
	defer srv.Close()
	if err := os.WriteFile(filepath.Join(dir, "enc.key"), []byte("0123456789abcdef"), 0o644); err != nil {
		t.Fatal(err)
	}
	info := srv.URL + "/enc.key\n" + filepath.Join(dir, "enc.key") + "\n"
	if err := os.WriteFile(filepath.Join(dir, "enc.keyinfo"), []byte(info), 0o644); err != nil {
		t.Fatal(err)
	}
	write := func(args ...string) {
		t.Helper()
		base := []string{"-hide_banner", "-loglevel", "error", "-f", "lavfi", "-i", "testsrc=size=160x120:rate=30", "-t", "4", "-c:v", "libx264", "-f", "hls", "-hls_time", "2", "-hls_list_size", "0"}
		cmd := exec.Command("ffmpeg", append(base, args...)...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Skipf("ffmpeg could not write the sample: %v %s", err, out)
		}
	}
	write("-hls_key_info_file", "enc.keyinfo", "enc.m3u8")
	write("-hls_segment_type", "fmp4", "fmp4.m3u8")
	for _, name := range []string{"enc.m3u8", "fmp4.m3u8"} {
		input := srv.URL + "/" + name
		args := urlProtocols([]string{"-hide_banner", "-loglevel", "error"}, input)
		args = append(args, "-i", input, "-c", "copy", "-f", "null", "-")
		if out, err := exec.Command("ffmpeg", args...).CombinedOutput(); err != nil {
			t.Fatalf("%s: %v %s", name, err, out)
		}
	}
	local := filepath.Join(dir, "fmp4.m3u8")
	args := urlProtocols([]string{"-hide_banner", "-loglevel", "error"}, "file:"+local)
	args = append(args, "-i", "file:"+local, "-f", "null", "-")
	if err := exec.Command("ffmpeg", args...).Run(); err == nil {
		t.Fatal("the protocol list opened a local file")
	}
}

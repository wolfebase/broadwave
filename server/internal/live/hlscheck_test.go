package live

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ffmpeg's event playlist for a field-rate recording. append_list puts a
// discontinuity before the first segment, the date offset has no colon, and
// 2.002s is over a target of 2. AVPlayer rejects that on the first play.
const growingRecordingPlaylist = "#EXTM3U\n" +
	"#EXT-X-VERSION:6\n" +
	"#EXT-X-TARGETDURATION:2\n" +
	"#EXT-X-MEDIA-SEQUENCE:0\n" +
	"#EXT-X-PLAYLIST-TYPE:EVENT\n" +
	"#EXT-X-INDEPENDENT-SEGMENTS\n" +
	"#EXT-X-DISCONTINUITY\n" +
	"#EXTINF:2.002000,\n" +
	"#EXT-X-PROGRAM-DATE-TIME:2026-10-09T15:26:10.119-0500\n" +
	"seg00000.ts\n"

func TestGrowingRecordingPlaylistBreaksAVPlayerRules(t *testing.T) {
	issues := CheckRecordingPlaylist([]byte(growingRecordingPlaylist), "")
	if !hasIssue(issues, "discontinuity") || !hasIssue(issues, "extinf") || !hasIssue(issues, "date") {
		t.Fatalf("issues: %v", issues)
	}
	got := RepairRecordingPlaylist([]byte(growingRecordingPlaylist), nil, "")
	if issues := CheckRecordingPlaylist(got, ""); len(issues) != 0 {
		t.Fatalf("%v\n%s", issues, got)
	}
	text := string(got)
	if strings.Contains(text, "#EXT-X-DISCONTINUITY") {
		t.Fatalf("leading discontinuity stayed:\n%s", text)
	}
	if !strings.Contains(text, "#EXT-X-TARGETDURATION:4\n") {
		t.Fatalf("target: %s", text)
	}
	if !strings.Contains(text, "#EXT-X-PROGRAM-DATE-TIME:2026-10-09T20:26:10.119Z\n#EXTINF:2.002,\nseg00000.ts\n") {
		t.Fatalf("date and duration:\n%s", text)
	}
}

func TestAReloadKeepsTheTargetAndTheDiscontinuityCount(t *testing.T) {
	h := &Hub{}
	dir := t.TempDir()
	first := h.StableRecordingPlaylist(dir, []byte(growingRecordingPlaylist))
	// A rewrite can change the first segment's duration and the target, and
	// a later segment arrives. The player has already accepted the first body.
	second := strings.Replace(growingRecordingPlaylist, "#EXT-X-TARGETDURATION:2\n", "#EXT-X-TARGETDURATION:3\n", 1)
	second = strings.Replace(second, "#EXTINF:2.002000,\n", "#EXTINF:2.400000,\n", 1)
	second += "#EXTINF:2.002000,\n#EXT-X-PROGRAM-DATE-TIME:2026-10-09T15:26:12.121-0500\nseg00001.ts\n"
	next := h.StableRecordingPlaylist(dir, []byte(second))
	if issues := CheckRecordingUpdate(first, next); len(issues) != 0 {
		t.Fatalf("%v\n%s", issues, next)
	}
	if issues := CheckRecordingPlaylist(next, ""); len(issues) != 0 {
		t.Fatalf("%v\n%s", issues, next)
	}
	if strings.Contains(string(next), "2.400") {
		t.Fatalf("first segment duration changed:\n%s", next)
	}
	if !strings.Contains(string(next), "seg00001.ts") {
		t.Fatalf("new segment missing:\n%s", next)
	}
}

func TestRecordingPlaylistMapRules(t *testing.T) {
	withMap := "#EXTM3U\n#EXT-X-VERSION:6\n#EXT-X-TARGETDURATION:4\n#EXT-X-MEDIA-SEQUENCE:0\n#EXT-X-PLAYLIST-TYPE:EVENT\n" +
		"#EXT-X-MAP:URI=\"init.mp4\"\n#EXTINF:2.000,\nseg00000.ts\n"
	if !hasIssue(CheckRecordingPlaylist([]byte(withMap), ""), "map") {
		t.Fatal("mpeg-ts with a map was accepted")
	}
	if got := string(RepairRecordingPlaylist([]byte(withMap), nil, "")); strings.Contains(got, "EXT-X-MAP") {
		t.Fatalf("map stayed:\n%s", got)
	}
	bare := "#EXTM3U\n#EXT-X-VERSION:7\n#EXT-X-TARGETDURATION:4\n#EXT-X-MEDIA-SEQUENCE:0\n#EXT-X-PLAYLIST-TYPE:EVENT\n" +
		"#EXTINF:2.000,\nseg00000.m4s\n"
	if !hasIssue(CheckRecordingPlaylist([]byte(bare), ""), "map") {
		t.Fatal("fmp4 without a map was accepted")
	}
	ok := "#EXTM3U\n#EXT-X-VERSION:7\n#EXT-X-TARGETDURATION:4\n#EXT-X-MEDIA-SEQUENCE:0\n#EXT-X-PLAYLIST-TYPE:EVENT\n" +
		"#EXT-X-MAP:URI=\"init.mp4\"\n#EXTINF:2.000,\nseg00000.m4s\n#EXTINF:2.000,\nseg00001.m4s\n"
	if issues := CheckRecordingPlaylist([]byte(ok), ""); len(issues) != 0 {
		t.Fatal(issues)
	}
	changed := strings.Replace(ok, "seg00001.m4s", "#EXT-X-MAP:URI=\"init2.mp4\"\nseg00001.m4s", 1)
	if !hasIssue(CheckRecordingPlaylist([]byte(changed), ""), "map-changed") {
		t.Fatal("a new map without a discontinuity was accepted")
	}
}

func TestAMissingSegmentIsNotListed(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "seg00000.ts"), []byte("segment"), 0o644); err != nil {
		t.Fatal(err)
	}
	raw := "#EXTM3U\n#EXT-X-VERSION:6\n#EXT-X-TARGETDURATION:4\n#EXT-X-MEDIA-SEQUENCE:0\n#EXT-X-PLAYLIST-TYPE:EVENT\n" +
		"#EXTINF:2.000,\nseg00000.ts\n#EXTINF:2.000,\nseg00001.ts\n#EXT-X-ENDLIST\n"
	if !hasIssue(CheckRecordingPlaylist([]byte(raw), dir), "file") {
		t.Fatal("a missing segment passed")
	}
	got := string(RepairRecordingPlaylist([]byte(raw), nil, dir))
	if strings.Contains(got, "seg00001") || strings.Contains(got, "ENDLIST") {
		t.Fatalf("listed a segment that is not on disk:\n%s", got)
	}
	if issues := CheckRecordingPlaylist([]byte(got), dir); len(issues) != 0 {
		t.Fatalf("%v\n%s", issues, got)
	}
}

func TestGapSegmentsNeedADiscontinuityBeforeThePicture(t *testing.T) {
	raw := "#EXTM3U\n#EXT-X-VERSION:8\n#EXT-X-TARGETDURATION:4\n#EXT-X-MEDIA-SEQUENCE:0\n#EXT-X-PLAYLIST-TYPE:EVENT\n" +
		"#EXT-X-START:TIME-OFFSET=4.000,PRECISE=YES\n" +
		"#EXT-X-GAP\n#EXTINF:2.000,\ngap00000.ts\n#EXT-X-GAP\n#EXTINF:2.000,\ngap00001.ts\n" +
		"#EXTINF:2.000,\nseg00000.ts\n"
	if !hasIssue(CheckRecordingPlaylist([]byte(raw), ""), "discontinuity") {
		t.Fatal("a gap running into the picture was accepted")
	}
	got := string(RepairRecordingPlaylist([]byte(raw), nil, ""))
	if issues := CheckRecordingPlaylist([]byte(got), ""); len(issues) != 0 {
		t.Fatalf("%v\n%s", issues, got)
	}
	if !strings.Contains(got, "gap00001.ts\n#EXT-X-DISCONTINUITY\n") {
		t.Fatalf("discontinuity:\n%s", got)
	}
	if strings.HasPrefix(strings.TrimPrefix(got, "#EXTM3U\n"), "#EXT-X-DISCONTINUITY") {
		t.Fatalf("discontinuity leads:\n%s", got)
	}
}

func hasIssue(issues []PlaylistIssue, rule string) bool {
	for _, issue := range issues {
		if issue.Rule == rule {
			return true
		}
	}
	return false
}

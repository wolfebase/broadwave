package httpapi

import (
	"testing"
	"time"

	"broadwave/internal/realtime"
)

func TestPlaylistEndAddsTheSegmentsAfterTheLastDate(t *testing.T) {
	body := []byte("#EXTM3U\n#EXT-X-TARGETDURATION:2\n#EXT-X-PROGRAM-DATE-TIME:2026-10-03T20:00:00.000Z\n#EXTINF:2.002,\nseg00000.m4s\n" +
		"#EXT-X-PROGRAM-DATE-TIME:2026-10-03T20:00:02.002Z\n#EXTINF:1.001,\nseg00001.m4s\n#EXTINF:0.5,\nseg00002.m4s\n#EXT-X-PART:DURATION=0.5,URI=\"p.m4s\"\n")
	end, ok := playlistEnd(body)
	want := time.Date(2026, 10, 3, 20, 0, 3, 503_000_000, time.UTC)
	if !ok || !end.Equal(want) {
		t.Fatalf("end %v %v, want %v", end, ok, want)
	}
	if _, ok := playlistEnd([]byte("#EXTM3U\n#EXTINF:2,\nseg00000.m4s\n")); ok {
		t.Fatal("a playlist with no date has an end")
	}
}

func TestAWatchNamesARoomOnlyWhenItPlaysThatChannel(t *testing.T) {
	bus := realtime.NewBus()
	bus.Rooms.Join("channel:7", 7, 0)
	s := &Server{Bus: bus}
	if _, ok := s.roomOf(watchBody{ChannelID: 7, Room: "channel:7"}); !ok {
		t.Fatal("the room playing channel 7 was not found")
	}
	if _, ok := s.roomOf(watchBody{ChannelID: 8, Room: "channel:7"}); ok {
		t.Fatal("a room playing another channel was used")
	}
	if _, ok := s.roomOf(watchBody{ChannelID: 7}); ok {
		t.Fatal("a watch that names no room got one")
	}
	room, _ := s.roomOf(watchBody{ChannelID: 7, Room: "channel:7"})
	now := time.Now()
	if back := now.Sub(roomFrame(room, now)); back < 15*time.Second || back > 17*time.Second {
		t.Fatalf("a balanced room plays %v behind, want 16 s", back)
	}
}

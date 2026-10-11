package dvr

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"broadwave/internal/hdhr"
	"broadwave/internal/hdhr/fake"
	"broadwave/internal/live"
	"broadwave/internal/store"
)

func TestAPassRecordingNamesItsPass(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg is not installed")
	}
	srv := &fake.Server{Realtime: true, Channels: []fake.Channel{{Number: "4.1", Name: "KBWV", Freq: 593000000}}}
	base, port, err := srv.Start()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(srv.Close)
	t.Setenv("HDHR_CONTROL_PORT", port)
	ctx := context.Background()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	client := &hdhr.Client{}
	dev, err := client.FetchDevice(ctx, base)
	if err != nil {
		t.Fatal(err)
	}
	lineup, err := client.FetchLineup(ctx, dev.LineupURL)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertDevice(ctx, dev, lineup); err != nil {
		t.Fatal(err)
	}
	if err := st.PutSettings(ctx, map[string]string{"watermarkGB": "0"}); err != nil {
		t.Fatal(err)
	}
	hub := live.New(st, t.TempDir(), "ffmpeg", "libx264")
	t.Cleanup(hub.Shutdown)
	chs, err := st.Channels(ctx, false)
	if err != nil || len(chs) != 1 {
		t.Fatalf("%+v %v", chs, err)
	}
	start := time.Now().Add(30 * time.Second).Truncate(time.Second)
	if err := st.InsertAirings(ctx, []store.Airing{{ChannelID: chs[0].ID, Title: "News", Start: start, End: start.Add(30 * time.Minute)}}); err != nil {
		t.Fatal(err)
	}
	if err := st.AddPass(ctx, "Other", 0, 0, 0); err != nil {
		t.Fatal(err)
	}
	id, err := st.AddSeriesPass(ctx, store.Pass{Title: "news", MatchKind: "contains", Kind: "series"})
	if err != nil {
		t.Fatal(err)
	}
	Tick(ctx, st, hub)
	recs, err := st.Recordings(ctx)
	if err != nil || len(recs) != 1 {
		t.Fatalf("recordings %+v %v", recs, err)
	}
	t.Cleanup(func() { hub.StopRecord(recs[0].ID) })
	if recs[0].PassID != id {
		t.Fatalf("recording names pass %d, want %d", recs[0].PassID, id)
	}
}

func TestAnUnwritableFolderDoesNotTune(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can write a mode 0555 directory")
	}
	srv := &fake.Server{Realtime: true, Channels: []fake.Channel{{Number: "4.1", Name: "KBWV", Freq: 593000000}}}
	base, port, err := srv.Start()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(srv.Close)
	t.Setenv("HDHR_CONTROL_PORT", port)
	ctx := context.Background()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	client := &hdhr.Client{}
	dev, err := client.FetchDevice(ctx, base)
	if err != nil {
		t.Fatal(err)
	}
	lineup, err := client.FetchLineup(ctx, dev.LineupURL)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertDevice(ctx, dev, lineup); err != nil {
		t.Fatal(err)
	}
	if err := st.PutSettings(ctx, map[string]string{"watermarkGB": "0"}); err != nil {
		t.Fatal(err)
	}
	hub := live.New(st, t.TempDir(), "ffmpeg", "libx264")
	t.Cleanup(hub.Shutdown)
	recDir := hub.Recordings()
	if err := os.MkdirAll(recDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(recDir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(recDir, 0o755) })
	if f, err := os.CreateTemp(recDir, ".probe-*"); err == nil {
		name := f.Name()
		_ = f.Close()
		_ = os.Remove(name)
		t.Skip("this user can still write a mode 0555 directory")
	}
	chs, err := st.Channels(ctx, false)
	if err != nil || len(chs) != 1 {
		t.Fatalf("%+v %v", chs, err)
	}
	start := time.Now().Add(30 * time.Second).Truncate(time.Second)
	if err := st.InsertAirings(ctx, []store.Airing{{ChannelID: chs[0].ID, Title: "News", Start: start, End: start.Add(30 * time.Minute)}}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.AddSeriesPass(ctx, store.Pass{Title: "news", MatchKind: "contains", Kind: "series"}); err != nil {
		t.Fatal(err)
	}
	Tick(ctx, st, hub)
	recs, err := st.Recordings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 0 {
		t.Cleanup(func() { hub.StopRecord(recs[0].ID) })
		t.Fatalf("started %d recording(s) in a folder that cannot be written", len(recs))
	}
	for _, path := range srv.Requests() {
		if strings.Contains(path, "/auto/") {
			t.Fatalf("tuned %s before noticing the recordings folder", path)
		}
	}
}

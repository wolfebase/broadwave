package live

import (
	"fmt"
	"testing"
	"time"

	"broadwave/internal/store"
)

func tuner(index int, busy bool) Tuner {
	t := Tuner{Index: index}
	if busy {
		t.Guide = "4.1"
	}
	return t
}

func TestPickTunerFailsOverToTheNextDevice(t *testing.T) {
	devices := []DeviceTuners{
		{Host: "192.168.1.20", Tuners: []Tuner{tuner(0, true), tuner(1, true)}},
		{Host: "192.168.1.30", Tuners: []Tuner{tuner(0, true), tuner(1, false)}},
	}
	host, n, ok := PickTuner(devices, nil, nil, Need{})
	if !ok || host != "192.168.1.30" || n != 1 {
		t.Fatalf("host %s tuner %d ok %v", host, n, ok)
	}
}

func TestHoldBackLeavesATunerForARecording(t *testing.T) {
	tuners := []Tuner{tuner(0, false), tuner(1, false)}
	held := HoldBack(tuners, 1)
	host, n, ok := PickTuner([]DeviceTuners{{Host: "192.168.1.20", Tuners: tuners}}, nil, held, Need{})
	if !ok || host != "192.168.1.20" || n != 0 {
		t.Fatalf("watching should take tuner 0, got %s %d %v", host, n, ok)
	}
	if _, _, ok := PickTuner([]DeviceTuners{{Host: "192.168.1.20", Tuners: []Tuner{tuner(0, true), tuner(1, false)}}}, nil, held, Need{}); ok {
		t.Fatal("the last tuner is held for the recording")
	}
}

func TestPickTunerStaysOnTheFirstDevice(t *testing.T) {
	devices := []DeviceTuners{
		{Host: "192.168.1.20", Tuners: []Tuner{tuner(0, false), tuner(1, true)}},
		{Host: "192.168.1.30", Tuners: []Tuner{tuner(0, false)}},
	}
	host, n, ok := PickTuner(devices, nil, nil, Need{})
	if !ok || host != "192.168.1.20" || n != 0 {
		t.Fatalf("host %s tuner %d ok %v", host, n, ok)
	}
}

func TestATSC3TunersOn4K(t *testing.T) {
	if atsc3Tuners("HDFX-4K") != 2 || atsc3Tuners("HDHR5-4K") != 2 {
		t.Fatalf("4k %d %d", atsc3Tuners("HDFX-4K"), atsc3Tuners("HDHR5-4K"))
	}
	if atsc3Tuners("HDHR5-2US") != 0 || atsc3Tuners("HDFX-4US") != 0 || atsc3Tuners("HDFX-2US") != 0 {
		t.Fatal("a non-4k model was marked 3.0")
	}
	got := make([]Tuner, 8)
	markATSC3(got, atsc3Tuners("HDFX-4K"))
	for i, tuner := range got {
		if tuner.ATSC3 != (i < 2) {
			t.Fatalf("tuner %d atsc3 %v", i, tuner.ATSC3)
		}
	}
}

func TestNeedIgnoresGuideNumber(t *testing.T) {
	numbered := store.SourceChannel{Channel: store.Channel{GuideNumber: "104.1", VideoCodec: "MPEG2", AudioCodec: "AC3"}}
	if NeedFor(numbered.VideoCodec, numbered.AudioCodec, numbered.ATSC3).ATSC3 {
		t.Fatal("guide number is not a capability")
	}
	marked := store.SourceChannel{Channel: store.Channel{GuideNumber: "4.1", VideoCodec: "HEVC", AudioCodec: "AC-4"}}
	if !NeedFor(marked.VideoCodec, marked.AudioCodec, marked.ATSC3).ATSC3 {
		t.Fatal("lineup codecs")
	}
	explicit := store.SourceChannel{Channel: store.Channel{GuideNumber: "4.1", VideoCodec: "MPEG2"}, ATSC3: true}
	if !NeedFor(explicit.VideoCodec, explicit.AudioCodec, explicit.ATSC3).ATSC3 {
		t.Fatal("explicit flag")
	}
}

func TestMoveBudgetIsFiveSeconds(t *testing.T) {
	if New(nil, "", "", "").moveLimit() != 5*time.Second {
		t.Fatalf("budget %s", moveBudget)
	}
	h := New(nil, "", "", "")
	h.MoveBudget = 20 * time.Millisecond
	if h.moveLimit() != 20*time.Millisecond {
		t.Fatal(h.moveLimit())
	}
	h.MoveBudget = 0
	if h.moveLimit() != 5*time.Second {
		t.Fatal("zero budget must stay the production limit")
	}
}

func TestPickTunerKeepsATSC3Tuners(t *testing.T) {
	one := []DeviceTuners{dev("only", 1, 0)}
	wantPick(t, "1/atsc1", one, Need{}, "only", 0, true)
	wantPick(t, "1/atsc3-refused", one, Need{ATSC3: true}, "", 0, false)

	one3 := []DeviceTuners{dev("only3", 1, 1)}
	wantPick(t, "1/only-3.0", one3, Need{ATSC3: true}, "only3", 0, true)
	wantPick(t, "1/fallback", one3, Need{}, "only3", 0, true)

	two := []DeviceTuners{dev("duo", 2, 1)}
	wantPick(t, "2/atsc1", two, Need{}, "duo", 1, true)
	wantPick(t, "2/atsc3", two, Need{ATSC3: true}, "duo", 0, true)
	wantPick(t, "2/fallback", []DeviceTuners{dev("duo", 2, 1, 1)}, Need{}, "duo", 0, true)
	wantPick(t, "2/atsc3-busy", []DeviceTuners{dev("duo", 2, 1, 0)}, Need{ATSC3: true}, "", 0, false)

	flex := []DeviceTuners{dev("flex", 4, 2)}
	wantPick(t, "4/atsc1", flex, Need{}, "flex", 2, true)
	wantPick(t, "4/atsc3", flex, Need{ATSC3: true}, "flex", 0, true)
	wantPick(t, "4/fallback", []DeviceTuners{dev("flex", 4, 2, 2, 3)}, Need{}, "flex", 0, true)
	wantPick(t, "4/atsc3-busy", []DeviceTuners{dev("flex", 4, 2, 0, 1)}, Need{ATSC3: true}, "", 0, false)

	pool := []DeviceTuners{
		dev("duo-a", 2, 0),
		dev("duo-b", 2, 0),
		dev("flex", 4, 2),
	}
	wantPick(t, "8/atsc1", pool, Need{}, "duo-a", 0, true)
	wantPick(t, "8/atsc3", pool, Need{ATSC3: true}, "flex", 0, true)
	busyDUOs := []DeviceTuners{
		dev("duo-a", 2, 0, 0, 1),
		dev("duo-b", 2, 0, 0, 1),
		dev("flex", 4, 2),
	}
	wantPick(t, "8/skip-3.0", busyDUOs, Need{}, "flex", 2, true)
	all10 := []DeviceTuners{
		dev("duo-a", 2, 0, 0, 1),
		dev("duo-b", 2, 0, 0, 1),
		dev("flex", 4, 2, 2, 3),
	}
	wantPick(t, "8/fallback", all10, Need{}, "flex", 0, true)
	flexBusy := []DeviceTuners{
		dev("duo-a", 2, 0),
		dev("duo-b", 2, 0),
		dev("flex", 4, 2, 0, 1),
	}
	wantPick(t, "8/no-1.0-for-3.0", flexBusy, Need{ATSC3: true}, "", 0, false)
	next := []DeviceTuners{
		dev("flex", 4, 2, 2, 3),
		dev("duo-a", 2, 0),
	}
	wantPick(t, "8/next-device", next, Need{}, "duo-a", 0, true)
	wantPick(t, "8/one-device", []DeviceTuners{dev("rack", 8, 2)}, Need{}, "rack", 2, true)
	wantPick(t, "8/one-device-3.0", []DeviceTuners{dev("rack", 8, 2)}, Need{ATSC3: true}, "rack", 0, true)

	singles := make([]DeviceTuners, 8)
	singles[0] = dev("t0", 1, 1)
	for i := 1; i < 8; i++ {
		singles[i] = dev(fmt.Sprintf("t%d", i), 1, 0)
	}
	wantPick(t, "8/singles-atsc1", singles, Need{}, "t1", 0, true)
	wantPick(t, "8/singles-atsc3", singles, Need{ATSC3: true}, "t0", 0, true)
	for i := 1; i < 8; i++ {
		singles[i].Tuners[0].Guide = "4.1"
	}
	wantPick(t, "8/singles-fallback", singles, Need{}, "t0", 0, true)
}

func dev(host string, n, atsc3 int, busy ...int) DeviceTuners {
	held := map[int]bool{}
	for _, i := range busy {
		held[i] = true
	}
	tuners := make([]Tuner, n)
	for i := range tuners {
		tuners[i] = Tuner{Index: i, ATSC3: i < atsc3}
		if held[i] {
			tuners[i].Guide = "9.1"
		}
	}
	return DeviceTuners{Host: host, Tuners: tuners}
}

func TestReservedIndexDoesNotBlockTheNextDevice(t *testing.T) {
	devices := []DeviceTuners{
		{Host: "10.0.0.1", Tuners: []Tuner{{Index: 0}}},
		{Host: "10.0.0.2", Tuners: []Tuner{{Index: 0}}},
	}
	host, n, ok := PickTuner(devices, nil, map[int]bool{0: true}, Need{})
	if !ok || host != "10.0.0.2" || n != 0 {
		t.Fatalf("next device tuner 0 looked reserved: %s %d %v", host, n, ok)
	}
}

func TestUsedTunerIndexStaysOnItsDevice(t *testing.T) {
	h := New(nil, t.TempDir(), "ffmpeg", "libx264")
	h.muxes[1] = &mux{freq: 1, tuner: 0, host: "10.0.0.1"}
	h.muxes[2] = &mux{freq: 2, tuner: 1, host: "10.0.0.1"}
	if used := h.usedTunersLocked("10.0.0.2"); len(used) != 0 {
		t.Fatalf("other host looked busy: %v", used)
	}
	host, n, ok := PickTuner([]DeviceTuners{dev("10.0.0.2", 4, 2)}, h.usedTunersLocked("10.0.0.2"), nil, Need{ATSC3: true})
	if !ok || host != "10.0.0.2" || n != 0 {
		t.Fatalf("idle 3.0 tuner was skipped: %s %d %v", host, n, ok)
	}
}

func TestStreamOriginUsesTheStreamPort(t *testing.T) {
	if got := streamOrigin("http://192.168.1.20"); got != "http://192.168.1.20:5004" {
		t.Fatal(got)
	}
	if got := streamOrigin("http://192.168.1.20:80"); got != "http://192.168.1.20:5004" {
		t.Fatal(got)
	}
	if got := streamOrigin("http://192.168.1.20:5004"); got != "http://192.168.1.20:5004" {
		t.Fatal(got)
	}
	if got := streamOrigin("http://127.0.0.1:54321"); got != "http://127.0.0.1:54321" {
		t.Fatal(got)
	}
	if got := originOf("http://192.168.1.20:5004/auto/v4.1"); got != "http://192.168.1.20:5004" {
		t.Fatal(got)
	}
}

func wantPick(t *testing.T, name string, devices []DeviceTuners, need Need, host string, tuner int, ok bool) {
	t.Helper()
	t.Run(name, func(t *testing.T) {
		gotHost, gotN, gotOK := PickTuner(devices, nil, nil, need)
		if gotOK != ok || (ok && (gotHost != host || gotN != tuner)) {
			t.Fatalf("got %s tuner %d ok %v, want %s tuner %d ok %v", gotHost, gotN, gotOK, host, tuner, ok)
		}
	})
}

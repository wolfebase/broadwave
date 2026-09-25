package fake

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"broadwave/internal/dvr"
	"broadwave/internal/hdhr"
	"broadwave/internal/live"
	"broadwave/internal/store"
)

func TestProfileFleet(t *testing.T) {
	t.Run("default", testDefaultServer)
	for _, name := range ProfileNames() {
		t.Run(name, func(t *testing.T) {
			testProfile(t, name, 0)
		})
	}
	for n := 1; n <= maxTuners; n++ {
		t.Run("tuners-"+strconv.Itoa(n), func(t *testing.T) {
			testProfile(t, ProfileConnectDuo, n)
		})
	}
	t.Run("tuner-cap", func(t *testing.T) {
		srv, base, _ := startProfile(t, ProfileConnectDuo, maxTuners+1)
		dev := fetchDevice(t, base)
		if dev.TunerCount != maxTuners {
			t.Fatalf("tuner count %d", dev.TunerCount)
		}
		if len(srv.tuners) != maxTuners {
			t.Fatalf("slice %d", len(srv.tuners))
		}
	})
	t.Run("unknown-profile", func(t *testing.T) {
		srv := &Server{Profile: "NO-SUCH"}
		if _, _, err := srv.Start(); err == nil {
			t.Fatal("expected an unknown profile to fail")
		}
	})
}

func testDefaultServer(t *testing.T) {
	srv, base, ctrl := startProfile(t, "", 0)
	body := httpGet(t, base+"/discover.json")
	if !bytes.Contains(body, []byte("FAKEHDHR")) || !bytes.Contains(body, []byte("HDHR4-2US")) {
		t.Fatalf("discover %s", body)
	}
	dev := fetchDevice(t, base)
	if dev.ModelNumber != "HDHR4-2US" || dev.TunerCount != 2 || dev.FriendlyName != "Fake HDHomeRun" {
		t.Fatalf("%+v", dev)
	}
	if dev.FirmwareName != "hdhomerun_fake" || dev.FirmwareVersion != "20260101" {
		t.Fatalf("%+v", dev)
	}
	assertAuthDropped(t, body, dev, srv)
	channels := fetchLineup(t, dev.LineupURL)
	if len(channels) != 3 || channels[0].GuideNumber != "4.1" || channels[1].GuideNumber != "4.2" || channels[2].GuideNumber != "5.1" {
		t.Fatalf("%+v", channels)
	}
	model, err := ctrl.Get("/sys/model")
	if err != nil || model != "HDHR4-2US" {
		t.Fatalf("model %q %v", model, err)
	}
	assertNoUpgrade(t, base)
}

type wantProfile struct {
	model    string
	friendly string
	firmware string
	version  string
	tuners   int
	auth     bool
	upgrade  bool
	storage  bool
}

func profileWant(name string) wantProfile {
	switch name {
	case ProfileHDHR3US:
		return wantProfile{"HDHR3-US", "HDHomeRun DUAL", "hdhomerun3_atsc", "20260101", 2, true, true, false}
	case ProfileConnectDuo:
		return wantProfile{"HDHR5-2US", "HDHomeRun CONNECT DUO", "hdhomerun5_atsc", "20260101", 2, true, true, false}
	case ProfileConnectQuatro:
		return wantProfile{"HDHR5-4US", "HDHomeRun CONNECT QUATRO", "hdhomerun5_atsc", "20260101", 4, true, true, false}
	case ProfileFlexDuo:
		return wantProfile{"HDFX-2US", "HDHomeRun FLEX DUO", "hdhomerun_dvr_atsc", "20260101", 2, true, true, false}
	case ProfileFlexQuatro:
		return wantProfile{"HDFX-4US", "HDHomeRun FLEX QUATRO", "hdhomerun_dvr_atsc", "20260101", 4, true, true, false}
	case ProfileFlex4K:
		return wantProfile{"HDFX-4K", "HDHomeRun FLEX 4K", "hdhomerun_dvr_atsc3", "20260101", 4, true, true, false}
	case ProfilePrime:
		return wantProfile{"HDHR3-CC", "HDHomeRun PRIME", "hdhomerun3_cablecard", "20260101", 3, true, true, false}
	case ProfileExtend:
		return wantProfile{"HDTC-2US", "HDHomeRun EXTEND", "hdhomeruntc_atsc", "20260101", 2, true, true, false}
	case ProfileScribe:
		return wantProfile{"HDVR-2US-1TB", "HDHomeRun SCRIBE DUO", "hdhomerun_dvr_atsc", "20260101", 2, true, true, true}
	case ProfileServio:
		return wantProfile{"HHDD-2TB", "HDHomeRun SERVIO", "hdhomerun_dvr", "20260101", 0, true, false, true}
	case ProfileOldFirmware:
		return wantProfile{"HDHR3-US", "HDHomeRun DUAL", "hdhomerun3_atsc", "20140301", 2, false, false, false}
	default:
		return wantProfile{}
	}
}

func testProfile(t *testing.T, name string, override int) {
	want := profileWant(name)
	if want.model == "" {
		t.Fatalf("no expectation for %s", name)
	}
	if override > 0 {
		want.tuners = override
		if want.tuners > maxTuners {
			want.tuners = maxTuners
		}
	}
	srv, base, ctrl := startProfile(t, name, override)
	body := httpGet(t, base+"/discover.json")
	dev := fetchDevice(t, base)
	if dev.ModelNumber != want.model || dev.FriendlyName != want.friendly || dev.TunerCount != want.tuners {
		t.Fatalf("discover %+v", dev)
	}
	if dev.FirmwareName != want.firmware || dev.FirmwareVersion != want.version {
		t.Fatalf("firmware %+v", dev)
	}
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		t.Fatal(err)
	}
	if _, ok := raw["UpgradeAvailable"]; ok != want.upgrade {
		t.Fatalf("upgrade present %v", ok)
	}
	if _, ok := raw["StorageURL"]; ok != want.storage {
		t.Fatalf("storage url present %v body %s", ok, body)
	}
	assertAuthDropped(t, body, dev, srv)
	assertNoUpgrade(t, base)

	model, err := ctrl.Get("/sys/model")
	if err != nil || model != want.model {
		t.Fatalf("model %q %v", model, err)
	}
	channels := fetchLineup(t, dev.LineupURL)
	client := &hdhr.Client{}
	ctx := context.Background()
	if want.tuners == 0 {
		if err := client.StartScan(ctx, base); err == nil {
			t.Fatal("storage device accepted a scan")
		}
	} else {
		if err := client.StartScan(ctx, base); err != nil {
			t.Fatal(err)
		}
		prog, err := client.ScanProgress(ctx, base)
		if err != nil || !prog.Scan {
			t.Fatalf("scan %+v %v", prog, err)
		}
		prog, err = client.ScanProgress(ctx, base)
		if err != nil || prog.Scan || prog.Found != len(channels) {
			t.Fatalf("scan done %+v channels %d %v", prog, len(channels), err)
		}
	}

	clear := firstClear(channels)
	if want.tuners > 0 {
		if clear.GuideNumber == "" {
			t.Fatal("no clear channel")
		}
		if _, err := ctrl.Set("/tuner0/vchannel", clear.GuideNumber); err != nil {
			t.Fatal(err)
		}
		status, err := ctrl.Get("/tuner0/status")
		if err != nil || strings.Contains(status, "ch=none") || !strings.Contains(status, "lock=") {
			t.Fatalf("status %q %v", status, err)
		}
		if name == ProfilePrime && override == 0 && !strings.Contains(status, "lock=qam256") {
			t.Fatal(status)
		}
		info, err := ctrl.Get("/tuner0/streaminfo")
		if err != nil || !strings.Contains(info, clear.GuideNumber) {
			t.Fatalf("info %q %v", info, err)
		}
		if _, err := ctrl.Set("/tuner0/vchannel", "none"); err != nil {
			t.Fatal(err)
		}
		rec := recordStream(t, base+"/auto/v"+clear.GuideNumber)
		if !bytes.Contains(rec, []byte("MPEG2")) {
			t.Fatalf("recording codec %q", rec)
		}
		holdAll(t, ctrl, want.tuners, clear.GuideNumber)
		expectHTTP(t, base+"/auto/v"+clear.GuideNumber, http.StatusServiceUnavailable, "805")
		releaseAll(t, ctrl, want.tuners)
		if _, err := ctrl.Set("/tuner"+strconv.Itoa(want.tuners)+"/vchannel", clear.GuideNumber); err == nil {
			t.Fatal("tuned past the last tuner")
		}
	} else if rows := fetchStatus(t, base); len(rows) != 0 {
		t.Fatalf("status %+v", rows)
	}

	planMultiview(t, want.tuners)
	if want.tuners == 0 {
		files, err := client.FetchLibrary(ctx, base)
		if err != nil || len(files) != 1 || files[0].Title != "News" || files[0].Filename != "news.ts" || files[0].Episode != "At 6" {
			t.Fatalf("library %+v %v", files, err)
		}
	} else {
		planRecording(t, want.tuners)
	}

	if override == 0 {
		switch name {
		case ProfileFlex4K:
			assertFlex4K(t, base, ctrl, channels)
		case ProfilePrime:
			assertPrime(t, base, channels)
		case ProfileExtend:
			assertExtend(t, base, ctrl, dev.ModelNumber, clear.GuideNumber)
		case ProfileConnectDuo:
			rec := recordStream(t, base+"/auto/v"+clear.GuideNumber+"?transcode=mobile")
			if !bytes.Contains(rec, []byte("MPEG2")) || bytes.Contains(rec, []byte("AVC")) {
				t.Fatal("CONNECT honored transcode")
			}
		case ProfileScribe, ProfileServio:
			files, err := client.FetchLibrary(ctx, base)
			if err != nil || len(files) != 1 || files[0].Title != "News" || files[0].Filename != "news.ts" {
				t.Fatalf("library %+v %v", files, err)
			}
		case ProfileOldFirmware:
			if bytes.Contains(body, []byte("DeviceAuth")) || bytes.Contains(httpGet(t, dev.LineupURL), []byte("VideoCodec")) {
				t.Fatal("old firmware grew modern fields")
			}
		}
		if !want.storage {
			expectHTTP(t, base+"/recorded_files.json", http.StatusNotFound, "")
		}
	}
	if want.tuners > 0 {
		failover(t, srv, base, want.tuners, clear.GuideNumber, otherClear(channels, clear.GuideNumber))
	}
	switch name {
	case ProfileFlex4K:
		if hdhr.ModelNote(dev.ModelNumber) == "" {
			t.Fatal("missing flex note")
		}
	case ProfilePrime:
		if !strings.Contains(hdhr.ModelNote(dev.ModelNumber), "Copy protected") {
			t.Fatal(hdhr.ModelNote(dev.ModelNumber))
		}
	case ProfileExtend:
		if hdhr.ExtendQuery(dev.ModelNumber) != "transcode=mobile" {
			t.Fatal(hdhr.ExtendQuery(dev.ModelNumber))
		}
	}
}

func assertFlex4K(t *testing.T, base string, ctrl hdhr.Control, channels []hdhr.Channel) {
	t.Helper()
	lineup := httpGet(t, base+"/lineup.json")
	if !bytes.Contains(lineup, []byte("104.1")) || !bytes.Contains(lineup, []byte("HEVC")) || !bytes.Contains(lineup, []byte("AC-4")) {
		t.Fatalf("lineup %s", lineup)
	}
	var drm bool
	for _, ch := range channels {
		if ch.GuideNumber == "105.1" && ch.Protected {
			drm = true
		}
	}
	if !drm {
		t.Fatal("drm channel not protected")
	}
	if _, err := ctrl.Set("/tuner2/vchannel", "104.1"); err == nil || !strings.Contains(err.Error(), "806") {
		t.Fatalf("atsc1 tuner took a 3.0 channel: %v", err)
	}
	if _, err := ctrl.Set("/tuner0/vchannel", "104.1"); err != nil {
		t.Fatal(err)
	}
	status, err := ctrl.Get("/tuner0/status")
	if err != nil || !strings.Contains(status, "lock=atsc3") {
		t.Fatalf("status %q %v", status, err)
	}
	if _, err := ctrl.Set("/tuner0/vchannel", "none"); err != nil {
		t.Fatal(err)
	}
	rec := recordStream(t, base+"/auto/v104.1")
	if !bytes.Contains(rec, []byte("HEVC")) || !bytes.Contains(rec, []byte("AC-4")) {
		t.Fatalf("fixture %q", rec)
	}
	res := openStream(t, base+"/auto/v104.1")
	defer res.Body.Close()
	_ = readSync(t, res.Body)
	found := -1
	for i, row := range fetchStatus(t, base) {
		if row.VctNumber == "104.1" {
			found = i
		}
	}
	if found < 0 || found >= 2 {
		t.Fatalf("3.0 tune landed on tuner %d", found)
	}
	expectHTTP(t, base+"/tuner2/v104.1", http.StatusServiceUnavailable, "806")
	expectHTTP(t, base+"/auto/v105.1", http.StatusServiceUnavailable, "811")
}

func assertPrime(t *testing.T, base string, channels []hdhr.Channel) {
	t.Helper()
	lineup := httpGet(t, base+"/lineup.json")
	if !bytes.Contains(lineup, []byte("copy-once")) || !bytes.Contains(lineup, []byte("copy-never")) {
		t.Fatalf("lineup %s", lineup)
	}
	var once, never bool
	for _, ch := range channels {
		if ch.GuideNumber == "702" && ch.Protected {
			once = true
		}
		if ch.GuideNumber == "703" && ch.Protected {
			never = true
		}
		if ch.GuideNumber == "4" && ch.Protected {
			t.Fatal("clear cable channel marked protected")
		}
	}
	if !once || !never {
		t.Fatalf("protection once %v never %v", once, never)
	}
	expectHTTP(t, base+"/auto/v702", http.StatusServiceUnavailable, "811")
	expectHTTP(t, base+"/auto/v703", http.StatusServiceUnavailable, "811")
}

func assertExtend(t *testing.T, base string, ctrl hdhr.Control, model, number string) {
	t.Helper()
	if hdhr.ExtendQuery(model) != "transcode=mobile" {
		t.Fatal(hdhr.ExtendQuery(model))
	}
	rec := recordStream(t, base+"/auto/v"+number+"?"+hdhr.ExtendQuery(model))
	if !bytes.Contains(rec, []byte("AVC")) || !bytes.Contains(rec, []byte("AAC")) {
		t.Fatalf("transcode %q", rec)
	}
	expectHTTP(t, base+"/auto/v"+number+"?transcode=nope", http.StatusServiceUnavailable, "802")
	got, err := ctrl.Set("/tuner0/transcode", "mobile")
	if err != nil || got != "mobile" {
		t.Fatalf("transcode %q %v", got, err)
	}
	if _, err := ctrl.Set("/tuner0/transcode", "nope"); err == nil || !strings.Contains(err.Error(), "802") {
		t.Fatal(err)
	}
}

func failover(t *testing.T, srv *Server, base string, tuners int, number, other string) {
	t.Helper()
	waitIdle(t, base)
	if other == "" {
		other = number
	}
	if tuners >= 2 {
		res := openStream(t, base+"/tuner0/v"+number)
		_ = readSync(t, res.Body)
		rows := fetchStatus(t, base)
		if len(rows) != tuners || rows[0].VctNumber != number {
			t.Fatalf("before kill %+v", rows)
		}
		srv.KillTuner(0)
		waitClosed(t, res.Body)
		res.Body.Close()
		rows = fetchStatus(t, base)
		if rows[0].TargetIP != "closed" {
			t.Fatalf("dead tuner %+v", rows[0])
		}
		_, n, ok := live.PickTuner([]live.DeviceTuners{{Host: "device", Tuners: asTuners(rows)}}, nil, nil)
		if !ok || n == 0 {
			t.Fatalf("picker %d %v", n, ok)
		}
		res = openStream(t, base+"/auto/v"+other)
		defer res.Body.Close()
		_ = readSync(t, res.Body)
		rows = fetchStatus(t, base)
		if rows[0].VctNumber != "" || rows[n].VctNumber != other {
			t.Fatalf("failover landed wrong %+v picker %d", rows, n)
		}
		return
	}
	if tuners == 1 {
		res := openStream(t, base+"/tuner0/v"+number)
		_ = readSync(t, res.Body)
		srv.KillTuner(0)
		waitClosed(t, res.Body)
		res.Body.Close()
		_, _, ok := live.PickTuner([]live.DeviceTuners{{Host: "device", Tuners: asTuners(fetchStatus(t, base))}}, nil, nil)
		if ok {
			t.Fatal("picker used the closed tuner")
		}
		expectHTTP(t, base+"/auto/v"+other, http.StatusServiceUnavailable, "805")
	}
}

func planMultiview(t *testing.T, tuners int) {
	t.Helper()
	want := []live.PlanChannel{
		{ID: 1, FrequencyHz: 593000000, Number: "4.1"},
		{ID: 2, FrequencyHz: 533000000, Number: "5.1"},
	}
	plan := live.PlanMultiview(want, tuners, nil, nil)
	switch {
	case tuners == 0:
		if len(plan.Blocked) != 2 || !strings.Contains(plan.Blocked[0].Reason, "No tuner") {
			t.Fatalf("%+v", plan)
		}
	case tuners == 1:
		if len(plan.Playable) != 1 || len(plan.Blocked) != 1 {
			t.Fatalf("%+v", plan)
		}
	default:
		if len(plan.Playable) != 2 || len(plan.Blocked) != 0 || plan.TunersNeeded != 2 {
			t.Fatalf("%+v", plan)
		}
	}
	if tuners < 1 {
		return
	}
	share := live.PlanMultiview([]live.PlanChannel{
		{ID: 1, FrequencyHz: 593000000, Number: "4.1"},
		{ID: 3, FrequencyHz: 593000000, Number: "4.2"},
	}, tuners, nil, nil)
	if len(share.Playable) != 2 || !share.Playable[0].Shared || !share.Playable[1].Shared {
		t.Fatalf("%+v", share)
	}
}

func planRecording(t *testing.T, tuners int) {
	t.Helper()
	from := time.Date(2026, 9, 25, 20, 0, 0, 0, time.UTC)
	to := from.Add(2 * time.Hour)
	passes := []store.Pass{{ID: 1, Title: "News", Kind: "series", Priority: 1}}
	one := []store.Airing{{ID: 1, ChannelID: 1, Title: "News", Start: from, End: from.Add(time.Hour)}}
	planned := dvr.Plan(passes, one, tuners, from, to)
	if len(planned) != 1 || planned[0].Skipped || planned[0].Conflict {
		t.Fatalf("%+v", planned)
	}
	two := []store.Airing{
		{ID: 1, ChannelID: 1, Title: "News", Start: from, End: from.Add(time.Hour)},
		{ID: 2, ChannelID: 2, Title: "News", Start: from, End: from.Add(time.Hour)},
	}
	planned = dvr.Plan(passes, two, tuners, from, to)
	skipped := 0
	for _, item := range planned {
		if item.Skipped {
			skipped++
		}
	}
	if tuners == 1 && skipped != 1 {
		t.Fatalf("one tuner %+v", planned)
	}
	if tuners > 1 && skipped != 0 {
		t.Fatalf("enough tuners %+v", planned)
	}
}

func holdAll(t *testing.T, ctrl hdhr.Control, n int, number string) {
	t.Helper()
	for i := 0; i < n; i++ {
		if _, err := ctrl.Set("/tuner"+strconv.Itoa(i)+"/vchannel", number); err != nil {
			t.Fatalf("tuner %d: %v", i, err)
		}
	}
}

func releaseAll(t *testing.T, ctrl hdhr.Control, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		if _, err := ctrl.Set("/tuner"+strconv.Itoa(i)+"/vchannel", "none"); err != nil {
			t.Fatalf("release %d: %v", i, err)
		}
	}
}

func recordStream(t *testing.T, rawURL string) []byte {
	t.Helper()
	res := openStream(t, rawURL)
	buf := readSync(t, res.Body)
	res.Body.Close()
	if u, err := url.Parse(rawURL); err == nil {
		waitIdle(t, u.Scheme+"://"+u.Host)
	}
	path := filepath.Join(t.TempDir(), "rec.ts")
	if err := os.WriteFile(path, buf, 0o644); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Size() != 188 {
		t.Fatalf("recording %v %v", info, err)
	}
	return buf
}

func firstClear(channels []hdhr.Channel) hdhr.Channel {
	for _, ch := range channels {
		if !ch.Protected {
			return ch
		}
	}
	return hdhr.Channel{}
}

func otherClear(channels []hdhr.Channel, current string) string {
	for _, ch := range channels {
		if !ch.Protected && ch.GuideNumber != current {
			return ch.GuideNumber
		}
	}
	return current
}

type statusRow struct {
	VctNumber string
	TargetIP  string
}

func fetchStatus(t *testing.T, base string) []statusRow {
	t.Helper()
	var rows []statusRow
	if err := json.Unmarshal(httpGet(t, base+"/status.json"), &rows); err != nil {
		t.Fatal(err)
	}
	return rows
}

func asTuners(rows []statusRow) []live.Tuner {
	out := make([]live.Tuner, len(rows))
	for i, row := range rows {
		out[i] = live.Tuner{Index: i, Guide: row.VctNumber, Target: row.TargetIP}
	}
	return out
}

func assertAuthDropped(t *testing.T, discover []byte, dev hdhr.Device, srv *Server) {
	t.Helper()
	var raw map[string]any
	if err := json.Unmarshal(discover, &raw); err != nil {
		t.Fatal(err)
	}
	auth, _ := raw["DeviceAuth"].(string)
	encoded, err := json.Marshal(dev)
	if err != nil {
		t.Fatal(err)
	}
	if auth != "" && bytes.Contains(encoded, []byte(auth)) {
		t.Fatalf("device auth leaked: %s", encoded)
	}
	packet := srv.DiscoveryReply()
	reply, err := hdhr.ParseReply(packet, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	if reply.TunerCount != dev.TunerCount || reply.BaseURL != dev.BaseURL {
		t.Fatalf("datagram %+v device %+v", reply, dev)
	}
	if auth != "" && !bytes.Contains(packet, []byte(auth)) {
		t.Fatal("datagram omitted device auth")
	}
	if auth != "" && (strings.Contains(reply.BaseURL, auth) || strings.Contains(reply.DeviceID, auth)) {
		t.Fatalf("parsed discovery kept device auth: %+v", reply)
	}
	if len(dev.DeviceID) == 8 {
		if _, err := strconv.ParseUint(dev.DeviceID, 16, 32); err == nil && reply.DeviceID != dev.DeviceID {
			t.Fatalf("udp id %s http %s", reply.DeviceID, dev.DeviceID)
		}
	}
}

func assertNoUpgrade(t *testing.T, base string) {
	t.Helper()
	for _, path := range []string{"/upgrade", "/firmware"} {
		res, err := http.Post(base+path, "application/octet-stream", strings.NewReader("not-firmware"))
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(res.Body)
		res.Body.Close()
		if res.StatusCode != http.StatusNotFound {
			t.Fatalf("%s %s %s", path, res.Status, body)
		}
	}
}

func startProfile(t *testing.T, name string, tuners int) (*Server, string, hdhr.Control) {
	t.Helper()
	srv := &Server{Profile: name, TunerCount: tuners}
	base, port, err := srv.Start()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(srv.Close)
	return srv, base, hdhr.Control{Addr: "127.0.0.1:" + port}
}

func fetchDevice(t *testing.T, base string) hdhr.Device {
	t.Helper()
	dev, err := (&hdhr.Client{}).FetchDevice(context.Background(), base)
	if err != nil {
		t.Fatal(err)
	}
	return dev
}

func fetchLineup(t *testing.T, lineupURL string) []hdhr.Channel {
	t.Helper()
	channels, err := (&hdhr.Client{}).FetchLineup(context.Background(), lineupURL)
	if err != nil {
		t.Fatal(err)
	}
	return channels
}

func httpGet(t *testing.T, url string) []byte {
	t.Helper()
	res, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusOK {
		t.Fatalf("%s: %s %s", url, res.Status, body)
	}
	return body
}

func openStream(t *testing.T, url string) *http.Response {
	t.Helper()
	res, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(res.Body)
		res.Body.Close()
		t.Fatalf("%s: %s %s", url, res.Status, body)
	}
	return res
}

func expectHTTP(t *testing.T, url string, code int, header string) {
	t.Helper()
	res, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	if res.StatusCode != code {
		t.Fatalf("%s: %s %s", url, res.Status, body)
	}
	if header != "" && !strings.Contains(res.Header.Get("X-HDHomeRun-Error"), header) {
		t.Fatalf("header %q body %s", res.Header.Get("X-HDHomeRun-Error"), body)
	}
}

func readSync(t *testing.T, r io.Reader) []byte {
	t.Helper()
	buf := make([]byte, 188)
	if _, err := io.ReadFull(r, buf); err != nil {
		t.Fatal(err)
	}
	if buf[0] != 0x47 {
		t.Fatalf("sync %x", buf[0])
	}
	return buf
}

func waitIdle(t *testing.T, base string) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for {
		rows := fetchStatus(t, base)
		busy := false
		for _, row := range rows {
			if row.VctNumber != "" || (row.TargetIP != "" && row.TargetIP != "closed") {
				busy = true
			}
		}
		if !busy {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("tuners stayed busy %+v", rows)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func waitClosed(t *testing.T, body io.Reader) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(io.Discard, body)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("stream stayed open after the tuner was closed")
	}
}

package fake

import (
	"io"
	"net/http"
	"testing"
	"time"

	"broadwave/internal/dvbsi"
)

func readStream(t *testing.T, url string, n int) []byte {
	t.Helper()
	res, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	buf := make([]byte, n)
	got, _ := io.ReadFull(res.Body, buf)
	return buf[:got]
}

// The DVB-T profile's multiplex names its services and lists their shows;
// an ATSC profile sends no DVB tables.
func TestTheDVBProfileSendsSDTAndEIT(t *testing.T) {
	s := &Server{Profile: ProfileDVBT}
	base, _, err := s.Start()
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	g := dvbsi.Parse(readStream(t, base+"/tuner0/ch474000000", 188*400))
	names := map[int]string{}
	actual := map[int]bool{}
	for _, svc := range g.Services {
		names[svc.ID], actual[svc.ID] = svc.Name, svc.Actual
	}
	if names[4161] != "Channel One HD" || names[4162] != "Channel Two" || names[8261] != "News 21" || !actual[4161] || actual[8261] {
		t.Fatalf("services %+v", g.Services)
	}
	shows := map[int]int{}
	now := time.Now()
	for _, ev := range g.Events {
		shows[ev.ServiceID]++
		if ev.End.Before(now.Add(-time.Hour)) || ev.Start.After(now.Add(4*time.Hour)) {
			t.Fatalf("listing far from now: %+v", ev)
		}
	}
	if shows[4161] < 4 || shows[4162] < 4 || shows[8261] != 2 {
		t.Fatalf("listings per service %v", shows)
	}

	atsc := &Server{Profile: ProfileConnectDuo, TunerCount: 2}
	base, _, err = atsc.Start()
	if err != nil {
		t.Fatal(err)
	}
	defer atsc.Close()
	if g := dvbsi.Parse(readStream(t, base+"/tuner0/ch593000000", 188*100)); len(g.Services)+len(g.Events) != 0 {
		t.Fatalf("an ATSC tuner sent %+v", g)
	}
}

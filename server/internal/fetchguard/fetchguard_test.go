package fetchguard

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAllowedKeepsLANAndRefusesMetadata(t *testing.T) {
	ok := []string{
		"http://192.168.0.20:5004/auto/v2.1",
		"http://10.0.0.8/lineup.json",
		"http://127.0.0.1:8477/discover.json",
		"https://203.0.113.9/guide.xml",
		"http://[::1]/status.json",
		"http://tv.local/live.m3u8",
	}
	for _, raw := range ok {
		if err := Allowed(raw); err != nil {
			t.Errorf("Allowed(%s) = %v", raw, err)
		}
	}
	bad := []string{
		"http://169.254.169.254/latest/meta-data",
		"http://169.254.1.1/",
		"http://[fe80::1]/",
		"http://[::ffff:169.254.169.254]/",
		"http://0.0.0.0/",
		"http://[fd00:ec2::254]/",
		"http://100.100.100.200/",
		"http://192.0.0.192/",
		"http://metadata.google.internal/",
		"file:///etc/passwd",
		"gopher://127.0.0.1/",
		"http://2130706433/",
		"http://0x7f000001/",
		"http://127.1/",
		"http://0177.0.0.1/",
		"http://169.0xfea9fe/latest/meta-data",
		"http://169.254.0xa9fe/latest/meta-data",
		"http://169.254.169.0xfe/latest/meta-data",
		"http://169.254.169.0XFE/latest/meta-data",
		"http://169.0xfe.0xa9.0xfe/",
		"",
	}
	for _, raw := range bad {
		if err := Allowed(raw); !errors.Is(err, ErrRefused) {
			t.Errorf("Allowed(%s) = %v, want refused", raw, err)
		}
	}
}

func TestDoRefusesMetadataBeforeDialing(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, "http://169.254.169.254/latest/meta-data", nil)
	if err != nil {
		t.Fatal(err)
	}
	res, err := Do(req, 0)
	if res != nil {
		res.Body.Close()
	}
	if !errors.Is(err, ErrRefused) {
		t.Fatal(err)
	}
	if strings.Contains(err.Error(), "169.254") {
		t.Fatal(err)
	}
}

func TestDoFollowsALoopbackRedirectAndStopsAtMetadata(t *testing.T) {
	good := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "playlist")
	}))
	defer good.Close()
	hop := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, good.URL+"/list.m3u", http.StatusFound)
	}))
	defer hop.Close()
	req, err := http.NewRequest(http.MethodGet, hop.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	res, err := Do(req, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	if string(body) != "playlist" {
		t.Fatalf("body %q", body)
	}

	meta := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://169.254.169.254/latest/meta-data", http.StatusFound)
	}))
	defer meta.Close()
	req, err = http.NewRequest(http.MethodGet, meta.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	res, err = Do(req, 0)
	if res != nil {
		res.Body.Close()
	}
	if !errors.Is(err, ErrRefused) {
		t.Fatal(err)
	}
}

func TestReachableLiteralSkipsLookup(t *testing.T) {
	called := false
	prev := lookupIP
	t.Cleanup(func() { lookupIP = prev })
	lookupIP = func(context.Context, string) ([]net.IP, error) {
		called = true
		return nil, errors.New("lookup")
	}
	if err := Reachable(context.Background(), "http://127.0.0.1/live.m3u8"); err != nil {
		t.Fatal(err)
	}
	if called {
		t.Fatal("looked up a literal address")
	}
}

func TestReachableRefusesLinkLocalAnswer(t *testing.T) {
	prev := lookupIP
	t.Cleanup(func() { lookupIP = prev })
	lookupIP = func(context.Context, string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("169.254.169.254")}, nil
	}
	err := Reachable(context.Background(), "http://tv.example/live.m3u8")
	if !errors.Is(err, ErrRefused) {
		t.Fatal(err)
	}
	if strings.Contains(err.Error(), "tv.example") {
		t.Fatal(err)
	}
}

func TestReachableAllowsPublicAnswer(t *testing.T) {
	prev := lookupIP
	t.Cleanup(func() { lookupIP = prev })
	lookupIP = func(context.Context, string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("192.0.2.10")}, nil
	}
	if err := Reachable(context.Background(), "http://tv.example/live.m3u8"); err != nil {
		t.Fatal(err)
	}
}

func TestReachableLookupErrorHidesHost(t *testing.T) {
	prev := lookupIP
	t.Cleanup(func() { lookupIP = prev })
	lookupIP = func(_ context.Context, host string) ([]net.IP, error) {
		return nil, errors.New("lookup " + host + ": no such host")
	}
	err := Reachable(context.Background(), "http://tv.example/live.m3u8")
	if !errors.Is(err, ErrRefused) {
		t.Fatal(err)
	}
	if strings.Contains(err.Error(), "tv.example") {
		t.Fatal(err)
	}
}

func TestReachableCanceledContext(t *testing.T) {
	prev := lookupIP
	t.Cleanup(func() { lookupIP = prev })
	lookupIP = func(context.Context, string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("192.0.2.10")}, nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := Reachable(ctx, "http://tv.example/live.m3u8")
	if !errors.Is(err, context.Canceled) || errors.Is(err, ErrRefused) {
		t.Fatal(err)
	}
	if err := Reachable(ctx, "http://127.0.0.1/live.m3u8"); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestOneLineStripsHeaderBreaks(t *testing.T) {
	got := OneLine("Agent\r\nX-Evil: 1")
	if strings.Contains(got, "\n") || strings.Contains(got, "\r") {
		t.Fatal(got)
	}
}

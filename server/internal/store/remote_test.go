package store

import (
	"encoding/base64"
	"testing"
)

func TestNormalizeRemoteURL(t *testing.T) {
	for in, want := range map[string]string{
		"":                             "",
		"broadwave.example.net":        "https://broadwave.example.net",
		"https://TV.Example.net/":      "https://tv.example.net",
		"https://tv.example.net:443":   "https://tv.example.net",
		"https://tv.example.net:8443/": "https://tv.example.net:8443",
	} {
		got, err := NormalizeRemoteURL(in)
		if err != nil || got != want {
			t.Fatalf("%q: %q %v", in, got, err)
		}
	}
	for _, bad := range []string{"http://tv.example.net", "https://tv.example.net/watch", "https://u:p@tv.example.net", "tv", "https://192.168.1.2", "https://tv.example.net/?a=1"} {
		if got, err := NormalizeRemoteURL(bad); err == nil {
			t.Fatalf("%q accepted as %q", bad, got)
		}
	}
}

func TestCleanTunnelTokenTakesTheInstallCommand(t *testing.T) {
	token := base64.StdEncoding.EncodeToString([]byte(`{"a":"acct","t":"tid","s":"c2VjcmV0"}`))
	for _, in := range []string{token, "  " + token + "\n", "sudo cloudflared service install " + token, "cloudflared tunnel run --token " + token, "docker run cloudflare/cloudflared:latest tunnel --no-autoupdate run --token " + token} {
		got, err := CleanTunnelToken(in)
		if err != nil || got != token {
			t.Fatalf("%q: %q %v", in, got, err)
		}
	}
	if got, err := CleanTunnelToken(""); err != nil || got != "" {
		t.Fatal("empty should clear")
	}
	for _, bad := range []string{"hello", base64.StdEncoding.EncodeToString([]byte(`{"a":"x"}`)), "eyJub3Rqc29u"} {
		if _, err := CleanTunnelToken(bad); err == nil {
			t.Fatalf("%q accepted", bad)
		}
	}
}

func TestInviteCodesAreTypeable(t *testing.T) {
	for range 200 {
		code, err := inviteCode()
		if err != nil {
			t.Fatal(err)
		}
		formatted := FormatInvite(code)
		if norm, ok := normalizeInvite(formatted); !ok || norm != code {
			t.Fatalf("%q round trip %q", formatted, norm)
		}
	}
	if IsInviteCode("123456") || IsInviteCode("ABCD-EFGH-JKL0") {
		t.Fatal("a six-digit code or a zero passed as an invite")
	}
}

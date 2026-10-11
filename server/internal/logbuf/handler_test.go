package logbuf

import (
	"bytes"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"broadwave/internal/store"
)

func TestWithAttrsDropsSecretsAndRedactsStrings(t *testing.T) {
	var buf bytes.Buffer
	const (
		password      = "ops3-fixture-password"
		token         = "ops3-token-value"
		auth          = "ops3-device-auth-token"
		apiKey        = "ops3-api-key-value"
		authorization = "ops3-auth-header"
		sdPassword    = "ops3-sd-password"
		nested        = "ops3-nested-password"
	)
	rawURL := "http://ops3user:" + password + "@playlist.example/pl.m3u"
	nestedURL := "http://ops3user:" + nested + "@listing.example/xmltv.xml"
	logger := slog.New(redactHandler{next: slog.NewTextHandler(&buf, nil)}).With(
		"password", password,
		"token", token,
		"deviceauth", auth,
		"api_key", apiKey,
		"authorization", authorization,
		"sdpassword", sdPassword,
		"room", "den",
		"url", rawURL,
		"channel", 4,
		slog.Group("source",
			slog.String("password", nested),
			slog.String("guide", nestedURL),
			slog.Int("number", 7),
		),
	)
	logger.Info("watching")
	got := buf.String()
	for _, secret := range []string{password, token, auth, apiKey, authorization, sdPassword, nested} {
		if strings.Contains(got, secret) {
			t.Fatalf("%s leaked in %s", secret, got)
		}
	}
	for _, key := range []string{"password=", "token=", "deviceauth=", "api_key=", "authorization=", "sdpassword="} {
		if strings.Contains(got, key) {
			t.Fatalf("kept %s in %s", key, got)
		}
	}
	if !strings.Contains(got, "room=den") || !strings.Contains(got, "channel=4") || !strings.Contains(got, "source.number=7") {
		t.Fatalf("%s", got)
	}
	if !strings.Contains(got, store.MaskURL(rawURL)) || !strings.Contains(got, store.MaskURL(nestedURL)) {
		t.Fatalf("%s", got)
	}
}

func TestWithGroupPrefixesTheNameAndDropsASecret(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(redactHandler{next: slog.NewTextHandler(&buf, nil)}).WithGroup("tuner")
	const password = "ops3-fixture-password"
	logger.Info("status", "password", password, "room", "den")
	got := buf.String()
	if strings.Contains(got, password) || strings.Contains(got, "password=") {
		t.Fatalf("%s", got)
	}
	if !strings.Contains(got, "tuner.room=den") {
		t.Fatalf("%s", got)
	}
}

func TestWriterReturnsTheDestinationError(t *testing.T) {
	prev := shared
	shared = &ring{max: 8}
	t.Cleanup(func() { shared = prev })
	want := errors.New("full")
	const password = "ops3-fixture-password"
	raw := "guide http://ops3user:" + password + "@playlist.example/pl.m3u\n"
	dst := &errWriter{err: want}
	n, err := (writer{dst: dst}).Write([]byte(raw))
	if n != 0 || err != want {
		t.Fatalf("n=%d err=%v", n, err)
	}
	masked := store.MaskURL("http://ops3user:" + password + "@playlist.example/pl.m3u")
	if strings.Contains(Text(), password) || strings.Contains(dst.buf.String(), password) {
		t.Fatalf("ring %q dst %q", Text(), dst.buf.String())
	}
	if !strings.Contains(Text(), masked) || !strings.Contains(dst.buf.String(), masked) {
		t.Fatalf("ring %q dst %q", Text(), dst.buf.String())
	}
}

func TestWriterSkipsALineThatRedactsToEmpty(t *testing.T) {
	prev := shared
	shared = &ring{max: 8}
	t.Cleanup(func() { shared = prev })
	var dst bytes.Buffer
	const password = "ops3-fixture-password"
	raw := "password=" + password + "\nchannel news\n"
	n, err := (writer{dst: &dst}).Write([]byte(raw))
	if err != nil || n != len(raw) {
		t.Fatalf("n=%d err=%v", n, err)
	}
	if dst.String() != "channel news\n" {
		t.Fatalf("dst %q", dst.String())
	}
	if strings.Contains(Text(), password) || !strings.Contains(Text(), "channel news") {
		t.Fatalf("%q", Text())
	}
	for _, line := range Lines() {
		if strings.TrimSpace(line) == "" {
			t.Fatalf("stored empty line in %q", Text())
		}
	}
}

type errWriter struct {
	err error
	buf bytes.Buffer
}

func (w *errWriter) Write(p []byte) (int, error) {
	_, _ = w.buf.Write(p)
	return 0, w.err
}

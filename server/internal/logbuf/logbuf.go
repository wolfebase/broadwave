// Package logbuf keeps a short tail of process logs for the support bundle.
// Install sends the standard logger and slog to one writer and keeps a redacted
// copy of recent lines. A URL password uses store.MaskURL, a password or token
// assignment is dropped, and tuner DeviceAuth is dropped.
package logbuf

import (
	"context"
	"io"
	"log"
	"log/slog"
	"os"
	"regexp"
	"strings"
	"sync"

	"broadwave/internal/store"
)

const ringLines = 400

type ring struct {
	mu    sync.Mutex
	lines []string
	max   int
}

var shared = &ring{max: ringLines}

var writeMu sync.Mutex

// Install sends the process log and slog to w and keeps a redacted copy of recent lines.
// A nil w uses stderr. Later calls replace the destination.
func Install(w io.Writer) {
	if w == nil {
		w = os.Stderr
	}
	out := writer{dst: w}
	log.SetOutput(out)
	slog.SetDefault(slog.New(redactHandler{next: slog.NewTextHandler(out, nil)}))
}

type writer struct {
	dst io.Writer
}

func (w writer) Write(p []byte) (int, error) {
	writeMu.Lock()
	defer writeMu.Unlock()
	var b strings.Builder
	for _, line := range strings.Split(strings.TrimRight(string(p), "\r\n"), "\n") {
		clean := strings.TrimSpace(Redact(line))
		if clean == "" {
			continue
		}
		shared.add(clean)
		b.WriteString(clean)
		b.WriteByte('\n')
	}
	if b.Len() > 0 && w.dst != nil {
		if _, err := io.WriteString(w.dst, b.String()); err != nil {
			return 0, err
		}
	}
	return len(p), nil
}

func (r *ring) add(line string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.max < 1 {
		r.max = ringLines
	}
	r.lines = append(r.lines, line)
	if len(r.lines) > r.max {
		r.lines = append([]string(nil), r.lines[len(r.lines)-r.max:]...)
	}
}

func (r *ring) text() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return strings.Join(r.lines, "\n")
}

func (r *ring) snapshot() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, len(r.lines))
	copy(out, r.lines)
	return out
}

// Text is the recent log, oldest first, already redacted.
func Text() string { return shared.text() }

// Lines is the recent log, oldest first, already redacted.
func Lines() []string { return shared.snapshot() }

// Tail is the newest n lines. n less than 1 returns every stored line.
func Tail(n int) []string {
	lines := Lines()
	if n > 0 && len(lines) > n {
		lines = append([]string(nil), lines[len(lines)-n:]...)
	}
	if lines == nil {
		return []string{}
	}
	return lines
}

var (
	urlRE          = regexp.MustCompile(`https?://[^\s"'<>]+`)
	deviceAuthRE   = regexp.MustCompile(`(?i)"?DeviceAuth"?\s*[:=]\s*"?[^\s"',}&]+"?`)
	secretAssignRE = regexp.MustCompile(`(?i)\b(?:password|pass|token|secret)\b\s*[:=]\s*"?[^\s"',}&]+"?`)
)

// Redact masks passwords inside URLs and removes tuner DeviceAuth and secret assignments.
func Redact(line string) string {
	line = urlRE.ReplaceAllStringFunc(line, func(raw string) string {
		cut := strings.TrimRight(raw, ".,);\"'")
		return store.MaskURL(cut) + raw[len(cut):]
	})
	line = StripDeviceAuth(line)
	return secretAssignRE.ReplaceAllString(line, "")
}

// StripDeviceAuth removes a tuner DeviceAuth assignment, including the name.
func StripDeviceAuth(s string) string {
	return deviceAuthRE.ReplaceAllString(s, "")
}

type redactHandler struct {
	next slog.Handler
}

func (h redactHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

func (h redactHandler) Handle(ctx context.Context, r slog.Record) error {
	clean := slog.NewRecord(r.Time, r.Level, Redact(r.Message), r.PC)
	r.Attrs(func(a slog.Attr) bool {
		if secretAttr(a.Key) {
			return true
		}
		clean.AddAttrs(scrubAttr(a))
		return true
	})
	return h.next.Handle(ctx, clean)
}

func (h redactHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	kept := make([]slog.Attr, 0, len(attrs))
	for _, a := range attrs {
		if secretAttr(a.Key) {
			continue
		}
		kept = append(kept, scrubAttr(a))
	}
	return redactHandler{next: h.next.WithAttrs(kept)}
}

func (h redactHandler) WithGroup(name string) slog.Handler {
	return redactHandler{next: h.next.WithGroup(name)}
}

func scrubAttr(a slog.Attr) slog.Attr {
	a.Value = a.Value.Resolve()
	if a.Value.Kind() == slog.KindGroup {
		var kept []slog.Attr
		for _, g := range a.Value.Group() {
			if secretAttr(g.Key) {
				continue
			}
			kept = append(kept, scrubAttr(g))
		}
		return slog.Attr{Key: a.Key, Value: slog.GroupValue(kept...)}
	}
	if a.Value.Kind() == slog.KindString {
		return slog.String(a.Key, Redact(a.Value.String()))
	}
	return a
}

func secretAttr(key string) bool {
	switch strings.ToLower(key) {
	case "password", "pass", "token", "secret", "deviceauth", "sdpassword", "tmdbkey", "sportsdbkey", "authorization":
		return true
	default:
		return false
	}
}

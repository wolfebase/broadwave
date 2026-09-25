// Package logbuf keeps a short tail of process logs for the support bundle.
// Install replaces the standard logger output so existing log calls are kept
// without each call site changing. Lines are redacted before they are stored
// or printed: a URL password uses store.MaskURL, and tuner DeviceAuth is dropped.
package logbuf

import (
	"io"
	"log"
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

// Install sends the process log to w and keeps a redacted copy of recent lines.
// A nil w uses stderr. Later calls replace the destination.
func Install(w io.Writer) {
	if w == nil {
		w = os.Stderr
	}
	log.SetOutput(writer{dst: w})
}

type writer struct {
	dst io.Writer
}

func (w writer) Write(p []byte) (int, error) {
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

// Text is the recent log, oldest first, already redacted.
func Text() string { return shared.text() }

var (
	urlRE        = regexp.MustCompile(`https?://[^\s"'<>]+`)
	deviceAuthRE = regexp.MustCompile(`(?i)"?DeviceAuth"?\s*[:=]\s*"?[^\s"',}&]+"?`)
)

// Redact masks passwords inside URLs and removes tuner DeviceAuth assignments.
func Redact(line string) string {
	line = urlRE.ReplaceAllStringFunc(line, func(raw string) string {
		cut := strings.TrimRight(raw, ".,);\"'")
		return store.MaskURL(cut) + raw[len(cut):]
	})
	return StripDeviceAuth(line)
}

// StripDeviceAuth removes a tuner DeviceAuth assignment, including the name.
func StripDeviceAuth(s string) string {
	return deviceAuthRE.ReplaceAllString(s, "")
}

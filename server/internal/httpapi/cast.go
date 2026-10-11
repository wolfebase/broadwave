package httpapi

import (
	_ "embed"
	"net/http"
	"strings"
)

//go:embed castpage/receiver.html
var receiverHTML []byte

// castPage is the Cast receiver. The script it names is a built web asset.
// The page stays open when device sign-in is on: the television has no token yet.
func (s *Server) castPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(receiverHTML)
}

// publishCastAppID returns the id the web player should offer, or empty.
// Google's ids are short alphanumeric strings. Anything else is ignored.
func publishCastAppID(raw string) string {
	id := strings.TrimSpace(raw)
	if len(id) < 4 || len(id) > 32 {
		return ""
	}
	for _, r := range id {
		switch {
		case r >= '0' && r <= '9', r >= 'A' && r <= 'Z', r >= 'a' && r <= 'z':
		default:
			return ""
		}
	}
	return id
}

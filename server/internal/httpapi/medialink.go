package httpapi

import (
	"bufio"
	"bytes"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"broadwave/internal/store"
)

// mediaPrefix is the part of a /media path a link covers: one channel, file,
// or mosaic, so its other playlists and segments come along.
func mediaPrefix(path string) string {
	segs := strings.Split(strings.TrimPrefix(path, "/"), "/")
	if len(segs) >= 4 {
		return "/" + strings.Join(segs[:3], "/") + "/"
	}
	return path
}

func (s *Server) mediaLink(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Path string `json:"path"`
	}
	if err := decodeJSON(r, &body); err != nil {
		httpError(w, "invalid json", http.StatusBadRequest)
		return
	}
	u, err := url.Parse(body.Path)
	if err != nil || u.Scheme != "" || u.Host != "" || !strings.HasPrefix(u.Path, "/media/") || strings.Contains(u.Path, "..") {
		httpError(w, "The path must be under /media/.", http.StatusBadRequest)
		return
	}
	exp := s.now().Add(store.MediaLinkTTL).UTC().Truncate(time.Second)
	dev, ok := clientFrom(r.Context())
	if !ok {
		// The home network without sign-in needs no key, and a link made
		// without a device must not open anything from outside.
		writeJSON(w, http.StatusOK, map[string]any{"url": u.String(), "expiresAt": exp})
		return
	}
	q := u.Query()
	q.Del("mk")
	q.Del("access_token")
	q.Set("mk", s.Store.SignMedia(mediaPrefix(u.Path), dev.ID, exp))
	u.RawQuery = q.Encode()
	writeJSON(w, http.StatusOK, map[string]any{"url": u.String(), "expiresAt": exp})
}

// mediaKey takes an mk value off a /media request. The receiver it was made
// for gets watch, and playlists it reads name every next URL with the same mk.
func (s *Server) mediaKey(r *http.Request) (store.ClientDevice, string, *http.Request, bool) {
	if !strings.HasPrefix(r.URL.Path, "/media/") {
		return store.ClientDevice{}, "", r, false
	}
	q := r.URL.Query()
	mk := q.Get("mk")
	if mk == "" {
		return store.ClientDevice{}, "", r, false
	}
	dev, ok, err := s.Store.MediaDevice(r.Context(), mk, mediaPrefix(r.URL.Path), s.now())
	if err != nil || !ok {
		return store.ClientDevice{}, "", r, false
	}
	dev.Scopes = []string{store.ScopeWatch}
	q.Del("mk")
	clone := r.Clone(r.Context())
	u := *r.URL
	u.RawQuery = q.Encode()
	clone.URL = &u
	clone.RequestURI = u.RequestURI()
	return dev, mk, clone, true
}

// keyedPlaylist holds a playlist until the handler finishes, then adds mk to
// every URL in it. Anything else passes straight through.
type keyedPlaylist struct {
	http.ResponseWriter
	mk      string
	decided bool
	hold    bool
	code    int
	buf     bytes.Buffer
}

func (k *keyedPlaylist) WriteHeader(code int) {
	if k.decided {
		return
	}
	k.decided = true
	ct := strings.ToLower(k.Header().Get("Content-Type"))
	k.hold = strings.Contains(ct, "mpegurl")
	if k.hold {
		k.code = code
		return
	}
	k.ResponseWriter.WriteHeader(code)
}

func (k *keyedPlaylist) Write(b []byte) (int, error) {
	if !k.decided {
		if k.Header().Get("Content-Type") == "" {
			k.Header().Set("Content-Type", http.DetectContentType(b))
		}
		k.WriteHeader(http.StatusOK)
	}
	if k.hold {
		return k.buf.Write(b)
	}
	return k.ResponseWriter.Write(b)
}

func (k *keyedPlaylist) Flush() {
	if k.hold {
		return
	}
	if f, ok := k.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (k *keyedPlaylist) Unwrap() http.ResponseWriter { return k.ResponseWriter }

func (k *keyedPlaylist) finish() {
	if !k.hold {
		return
	}
	body := keyPlaylist(k.buf.Bytes(), k.mk)
	k.Header().Del("Content-Length")
	k.ResponseWriter.WriteHeader(k.code)
	_, _ = k.ResponseWriter.Write(body)
}

var uriAttr = regexp.MustCompile(`URI="([^"]*)"`)

func keyPlaylist(body []byte, mk string) []byte {
	add := func(ref string) string {
		if ref == "" || strings.Contains(ref, "://") || strings.HasPrefix(ref, "data:") {
			return ref
		}
		sep := "?"
		if strings.Contains(ref, "?") {
			sep = "&"
		}
		return ref + sep + "mk=" + url.QueryEscape(mk)
	}
	var out bytes.Buffer
	sc := bufio.NewScanner(bytes.NewReader(body))
	sc.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for sc.Scan() {
		line := sc.Text()
		switch {
		case strings.HasPrefix(line, "#"):
			line = uriAttr.ReplaceAllStringFunc(line, func(m string) string {
				return `URI="` + add(m[5:len(m)-1]) + `"`
			})
		case strings.TrimSpace(line) != "":
			line = add(strings.TrimSpace(line))
		}
		out.WriteString(line)
		out.WriteByte('\n')
	}
	return out.Bytes()
}

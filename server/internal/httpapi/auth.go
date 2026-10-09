package httpapi

import (
	"context"
	"net"
	"net/http"
	"strings"
	"time"

	"broadwave/internal/store"
)

const (
	deviceCookie = "bw_token"
	pairWindow   = 10 * time.Minute
	pairIPLimit  = 10
	pairFailCap  = 30
)

type clientContextKey struct{}

func withClient(ctx context.Context, dev store.ClientDevice) context.Context {
	return context.WithValue(ctx, clientContextKey{}, dev)
}

func clientFrom(ctx context.Context) (store.ClientDevice, bool) {
	dev, ok := ctx.Value(clientContextKey{}).(store.ClientDevice)
	return dev, ok && dev.ID != ""
}

// withDeviceAuth accepts a device token on every request. While device
// sign-in is off, a missing token changes nothing. While it is on, a route
// that is not public requires a token whose scope covers the route.
func (s *Server) withDeviceAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.Store == nil {
			next.ServeHTTP(w, r)
			return
		}
		token, r := presentedToken(r)
		dev, revoked, err := s.Store.ClientByToken(r.Context(), token)
		if err != nil {
			writeError(w, err)
			return
		}
		if revoked {
			clearDeviceCookie(w)
			dev = store.ClientDevice{}
		}
		if dev.ID != "" {
			r = r.WithContext(withClient(r.Context(), dev))
			s.noteClient(r.Context(), dev.ID, s.now())
		}
		on, err := s.deviceAuthOn(r.Context())
		if err != nil {
			writeError(w, err)
			return
		}
		need := requiredScope(r.Method, r.URL.Path)
		if !on || need == "public" {
			next.ServeHTTP(w, r)
			return
		}
		if dev.ID == "" {
			httpError(w, "Sign in from a paired device.", http.StatusUnauthorized)
			return
		}
		if !store.ScopeAllows(dev.Scopes, need) {
			httpError(w, "This device can't do that.", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) deviceAuthOn(ctx context.Context) (bool, error) {
	values, err := s.Store.Settings(ctx)
	if err != nil {
		return false, err
	}
	return values[store.SettingDeviceAuth] == "1", nil
}

func (s *Server) noteClient(ctx context.Context, id string, now time.Time) {
	s.seenMu.Lock()
	if s.seenAt == nil {
		s.seenAt = map[string]time.Time{}
	}
	if prev := s.seenAt[id]; !prev.IsZero() && now.Sub(prev) < time.Minute {
		s.seenMu.Unlock()
		return
	}
	s.seenAt[id] = now
	s.seenMu.Unlock()
	_ = s.Store.TouchClient(ctx, id, now)
}

// presentedToken reads a bearer token, then the cookie, then access_token.
// The query form is removed before the handler sees the URL.
func presentedToken(r *http.Request) (string, *http.Request) {
	token := bearerToken(r)
	if token == "" {
		if c, err := r.Cookie(deviceCookie); err == nil {
			token = strings.TrimSpace(c.Value)
		}
	}
	q := r.URL.Query()
	queryToken := strings.TrimSpace(q.Get("access_token"))
	if queryToken == "" {
		return token, r
	}
	if token == "" {
		token = queryToken
	}
	q.Del("access_token")
	clone := r.Clone(r.Context())
	u := *r.URL
	u.RawQuery = q.Encode()
	clone.URL = &u
	// Clone keeps the original request target, which still has the token.
	clone.RequestURI = u.RequestURI()
	return token, clone
}

func bearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	const prefix = "bearer "
	if len(h) < len(prefix) || !strings.EqualFold(h[:len(prefix)], prefix) {
		return ""
	}
	return strings.TrimSpace(h[len(prefix):])
}

func setDeviceCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     deviceCookie,
		Value:    token,
		Path:     "/",
		MaxAge:   365 * 24 * 3600,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	})
}

func clearDeviceCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     deviceCookie,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	})
}

// requiredScope is the minimum scope for a route while device sign-in is on.
// An API route that is not listed is admin.
func requiredScope(method, path string) string {
	if method == http.MethodOptions {
		return "public"
	}
	if method == http.MethodHead {
		method = http.MethodGet
	}
	if api, ok := stripAPI(path); ok {
		return apiScope(method, api)
	}
	switch {
	case strings.HasPrefix(path, "/media/"):
		return store.ScopeWatch
	case strings.HasPrefix(path, "/export/"):
		return "public"
	case method == http.MethodGet:
		return "public"
	default:
		return store.ScopeAdmin
	}
}

func stripAPI(path string) (string, bool) {
	for _, prefix := range []string{"/api/v1", "/api"} {
		if path == prefix {
			return "/", true
		}
		if strings.HasPrefix(path, prefix+"/") {
			return strings.TrimPrefix(path, prefix), true
		}
	}
	return path, false
}

func apiScope(method, path string) string {
	switch {
	case method == http.MethodGet && (path == "/health" || path == "/server" || path == "/profile" || path == "/clients/me"):
		return "public"
	case method == http.MethodPost && (path == "/pair" || path == "/pair/claim"):
		return "public"
	case method == http.MethodGet && pairPollPath(path):
		return "public"
	}
	if scope, ok := watchScope(method, path); ok {
		return scope
	}
	if scope, ok := recordScope(method, path); ok {
		return scope
	}
	return store.ScopeAdmin
}

func pairPollPath(path string) bool {
	rest, ok := strings.CutPrefix(path, "/pair/")
	return ok && rest != "" && !strings.Contains(rest, "/")
}

func watchScope(method, path string) (string, bool) {
	if method == http.MethodGet {
		switch path {
		case "/clock", "/groups", "/channels", "/frames", "/airings", "/schedule", "/playback",
			"/search", "/sports/scoreboard", "/teams", "/passes", "/recordings", "/virtuals",
			"/virtuals/schedule", "/ws":
			return store.ScopeWatch, true
		}
		if tail, ok := strings.CutPrefix(path, "/channels/"); ok {
			action := ""
			if strings.HasSuffix(tail, "/frame") {
				action = "/frame"
			} else if strings.HasSuffix(tail, "/reception") {
				action = "/reception"
			}
			if action != "" && !strings.Contains(strings.TrimSuffix(tail, action), "/") {
				return store.ScopeWatch, true
			}
		}
		if id, action, ok := recordingAction(path); ok && id != "" {
			switch action {
			case "file", "markers", "again":
				return store.ScopeWatch, true
			}
		}
	}
	if method == http.MethodPost {
		switch path {
		case "/watch", "/multiview/plan", "/mosaic":
			return store.ScopeWatch, true
		}
		if _, action, ok := recordingAction(path); ok && action == "play" {
			return store.ScopeWatch, true
		}
		if rest, ok := strings.CutPrefix(path, "/watch/"); ok {
			if _, action, ok := splitAction(rest); ok && (action == "stop" || action == "warm") {
				return store.ScopeWatch, true
			}
		}
		if rest, ok := strings.CutPrefix(path, "/mosaic/"); ok {
			if _, action, ok := splitAction(rest); ok && action == "stop" {
				return store.ScopeWatch, true
			}
		}
		if rest, ok := strings.CutPrefix(path, "/virtuals/"); ok {
			if _, action, ok := splitAction(rest); ok && action == "play" {
				return store.ScopeWatch, true
			}
		}
	}
	if method == http.MethodPut {
		if _, action, ok := recordingAction(path); ok && (action == "progress" || action == "watched") {
			return store.ScopeWatch, true
		}
	}
	return "", false
}

func recordScope(method, path string) (string, bool) {
	if method == http.MethodGet && path == "/events" {
		return store.ScopeRecord, true
	}
	if method == http.MethodPost {
		switch path {
		case "/recordings", "/schedule/skip", "/schedule/fix", "/passes", "/passes/preview", "/virtuals":
			return store.ScopeRecord, true
		}
		if _, action, ok := recordingAction(path); ok {
			switch action {
			case "stop", "move", "markers", "detect":
				return store.ScopeRecord, true
			}
		}
	}
	if method == http.MethodPut {
		switch path {
		case "/passes/order", "/teams":
			return store.ScopeRecord, true
		}
		if _, action, ok := recordingAction(path); ok && action == "keep" {
			return store.ScopeRecord, true
		}
	}
	if method == http.MethodPatch {
		if rest, ok := strings.CutPrefix(path, "/passes/"); ok && rest != "" && !strings.Contains(rest, "/") {
			return store.ScopeRecord, true
		}
		if rest, ok := strings.CutPrefix(path, "/virtuals/"); ok && rest != "" && !strings.Contains(rest, "/") {
			return store.ScopeRecord, true
		}
	}
	if method == http.MethodDelete {
		switch {
		case oneSegment(path, "/markers/"):
			return store.ScopeRecord, true
		case oneSegment(path, "/teams/"):
			return store.ScopeRecord, true
		case strings.HasPrefix(path, "/passes/") && !strings.Contains(strings.TrimPrefix(path, "/passes/"), "/"):
			return store.ScopeRecord, true
		}
		if _, action, ok := recordingAction(path); ok && action == "" {
			return store.ScopeRecord, true
		}
	}
	return "", false
}

// recordingAction splits /recordings/{id} and /recordings/{id}/{action}.
func recordingAction(path string) (id, action string, ok bool) {
	rest, ok := strings.CutPrefix(path, "/recordings/")
	if !ok || rest == "" || rest == "nfo" {
		return "", "", false
	}
	id, action, _ = strings.Cut(rest, "/")
	if id == "" || strings.Contains(action, "/") {
		return "", "", false
	}
	return id, action, true
}

func oneSegment(path, prefix string) bool {
	rest, ok := strings.CutPrefix(path, prefix)
	return ok && rest != "" && !strings.Contains(rest, "/")
}

func splitAction(rest string) (id, action string, ok bool) {
	id, action, ok = strings.Cut(rest, "/")
	if !ok || id == "" || action == "" || strings.Contains(action, "/") {
		return "", "", false
	}
	return id, action, true
}

// pairAllowed counts a pairing attempt against the per-address cap, and
// refuses the call when the failure cap is already full.
func (s *Server) pairAllowed(r *http.Request) bool {
	now := s.now()
	ip := clientIP(r)
	s.pairMu.Lock()
	defer s.pairMu.Unlock()
	if s.pairHits == nil {
		s.pairHits = map[string][]time.Time{}
	}
	if countRecent(s.pairHits["fail:"+ip], now, pairWindow) >= pairIPLimit {
		return false
	}
	if countRecent(s.pairHits["fail:all"], now, pairWindow) >= pairFailCap {
		return false
	}
	return hitAllowed(s.pairHits, "ip:"+ip, now, pairIPLimit, pairWindow)
}

// pairFailed records a wrong code. The call that crosses the cap is refused
// by the next pairAllowed.
func (s *Server) pairFailed(r *http.Request) {
	now := s.now()
	ip := clientIP(r)
	s.pairMu.Lock()
	defer s.pairMu.Unlock()
	if s.pairHits == nil {
		s.pairHits = map[string][]time.Time{}
	}
	hitAllowed(s.pairHits, "fail:"+ip, now, pairIPLimit+1, pairWindow)
	hitAllowed(s.pairHits, "fail:all", now, pairFailCap+1, pairWindow)
}

func countRecent(at []time.Time, now time.Time, window time.Duration) int {
	cut := now.Add(-window)
	n := 0
	for _, t := range at {
		if t.After(cut) {
			n++
		}
	}
	return n
}

func hitAllowed(hits map[string][]time.Time, key string, now time.Time, limit int, window time.Duration) bool {
	cut := now.Add(-window)
	kept := hits[key][:0]
	for _, at := range hits[key] {
		if at.After(cut) {
			kept = append(kept, at)
		}
	}
	if len(kept) >= limit {
		hits[key] = kept
		return false
	}
	hits[key] = append(kept, now)
	return true
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

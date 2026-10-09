package httpapi

import (
	"errors"
	"net/http"

	"broadwave/internal/store"
)

type heldToken struct {
	token  string
	device store.ClientDevice
}

type pairBody struct {
	Name   string   `json:"name"`
	Kind   string   `json:"kind"`
	Scopes []string `json:"scopes"`
	Code   string   `json:"code"`
}

func (s *Server) startPair(w http.ResponseWriter, r *http.Request) {
	if !s.pairOpen(w, r) {
		return
	}
	var body pairBody
	if err := decodeJSON(r, &body); err != nil {
		httpError(w, "invalid json", http.StatusBadRequest)
		return
	}
	id, code, secret, expires, err := s.Store.StartPairing(r.Context(), "show", body.Name, body.Kind, body.Scopes, s.now())
	if err != nil {
		writePairError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"id": id, "code": code, "pollSecret": secret, "expiresAt": expires,
	})
}

func (s *Server) createPairCode(w http.ResponseWriter, r *http.Request) {
	if !s.pairOpen(w, r) {
		return
	}
	var body pairBody
	if err := decodeJSON(r, &body); err != nil {
		httpError(w, "invalid json", http.StatusBadRequest)
		return
	}
	id, code, _, expires, err := s.Store.StartPairing(r.Context(), "claim", body.Name, body.Kind, body.Scopes, s.now())
	if err != nil {
		writePairError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"id": id, "code": code, "expiresAt": expires,
	})
}

func (s *Server) approvePair(w http.ResponseWriter, r *http.Request) {
	if !s.pairOpen(w, r) {
		return
	}
	var body pairBody
	if err := decodeJSON(r, &body); err != nil {
		httpError(w, "invalid json", http.StatusBadRequest)
		return
	}
	// Hold the token before the approved row is visible. pairMu stays locked
	// across the commit so a poll cannot see "approved" with an empty handoff.
	s.pairMu.Lock()
	pairing, dev, token, err := s.Store.ApproveShow(r.Context(), body.Code, body.Scopes, s.now())
	if err != nil {
		s.pairMu.Unlock()
		s.noteBadCode(r, err)
		writePairError(w, err)
		return
	}
	if s.pairTokens == nil {
		s.pairTokens = map[string]heldToken{}
	}
	s.pairTokens[pairing.ID] = heldToken{token: token, device: dev}
	s.pairMu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"device": dev})
}

func (s *Server) claimPair(w http.ResponseWriter, r *http.Request) {
	if !s.pairOpen(w, r) {
		return
	}
	var body pairBody
	if err := decodeJSON(r, &body); err != nil {
		httpError(w, "invalid json", http.StatusBadRequest)
		return
	}
	_, dev, token, err := s.Store.ClaimCode(r.Context(), body.Code, body.Name, body.Kind, s.now())
	if err != nil {
		s.noteBadCode(r, err)
		writePairError(w, err)
		return
	}
	setDeviceCookie(w, token)
	writeJSON(w, http.StatusCreated, map[string]any{"token": token, "device": dev})
}

func (s *Server) pollPair(w http.ResponseWriter, r *http.Request) {
	// The poll URL carries the secret, and one response carries the token.
	w.Header().Set("Cache-Control", "no-store")
	id := r.PathValue("id")
	pairing, err := s.Store.PollPairing(r.Context(), id, r.URL.Query().Get("secret"), s.now())
	if err != nil && !errors.Is(err, store.ErrPairDenied) {
		writePairError(w, err)
		return
	}
	if errors.Is(err, store.ErrPairDenied) || pairing.State == "denied" {
		writeJSON(w, http.StatusOK, map[string]any{"state": "denied"})
		return
	}
	if pairing.State == "expired" {
		writeJSON(w, http.StatusOK, map[string]any{"state": "expired"})
		return
	}
	body := map[string]any{"state": pairing.State, "expiresAt": pairing.ExpiresAt}
	if pairing.State != "approved" {
		writeJSON(w, http.StatusOK, body)
		return
	}
	// ServeMux runs this GET handler for HEAD. HEAD has no body the device
	// can read, so it must not take the token.
	if r.Method == http.MethodHead {
		writeJSON(w, http.StatusOK, body)
		return
	}
	held, ok := s.peekPairToken(id)
	if !ok {
		// Already handed over, or the process restarted before the handoff.
		// The device row stays until someone revokes it. A second poll must
		// not revoke a device that already received its token.
		if pairing.DeviceID == "" {
			writeJSON(w, http.StatusOK, map[string]any{"state": "expired"})
			return
		}
		dev, _, err := s.deviceByID(r, pairing.DeviceID)
		if err != nil {
			writeError(w, err)
			return
		}
		body["device"] = dev
		writeJSON(w, http.StatusOK, body)
		return
	}
	setDeviceCookie(w, held.token)
	body["token"] = held.token
	body["device"] = held.device
	if err := writeJSONBody(w, http.StatusOK, body); err != nil {
		return
	}
	s.dropPairToken(id)
}

func writeJSONBody(w http.ResponseWriter, status int, v any) error {
	body, err := marshalJSON(v)
	if err != nil {
		http.Error(w, "json", http.StatusInternalServerError)
		return err
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, err = w.Write(body)
	return err
}

func (s *Server) deviceByID(r *http.Request, id string) (store.ClientDevice, bool, error) {
	list, err := s.Store.Clients(r.Context())
	if err != nil {
		return store.ClientDevice{}, false, err
	}
	for _, dev := range list {
		if dev.ID == id {
			return dev, true, nil
		}
	}
	return store.ClientDevice{}, false, nil
}

func (s *Server) listClients(w http.ResponseWriter, r *http.Request) {
	list, err := s.Store.Clients(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	if list == nil {
		list = []store.ClientDevice{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"devices": list})
}

func (s *Server) clientMe(w http.ResponseWriter, r *http.Request) {
	on, err := s.deviceAuthOn(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	auth := "local-open"
	if on {
		auth = "device"
	}
	body := map[string]any{"device": nil, "auth": auth}
	if dev, ok := clientFrom(r.Context()); ok {
		body["device"] = dev
	}
	writeJSON(w, http.StatusOK, body)
}

func (s *Server) revokeClient(w http.ResponseWriter, r *http.Request) {
	if err := s.Store.RevokeClient(r.Context(), r.PathValue("id"), s.now()); err != nil {
		writeError(w, err)
		return
	}
	s.listClients(w, r)
}

func (s *Server) peekPairToken(id string) (heldToken, bool) {
	s.pairMu.Lock()
	defer s.pairMu.Unlock()
	held, ok := s.pairTokens[id]
	return held, ok
}

func (s *Server) dropPairToken(id string) {
	s.pairMu.Lock()
	defer s.pairMu.Unlock()
	delete(s.pairTokens, id)
}

func (s *Server) pairOpen(w http.ResponseWriter, r *http.Request) bool {
	if s.pairAllowed(r) {
		return true
	}
	httpError(w, "Too many pairing tries. Wait a few minutes.", http.StatusTooManyRequests)
	return false
}

func (s *Server) noteBadCode(r *http.Request, err error) {
	if errors.Is(err, store.ErrPairCode) || errors.Is(err, store.ErrPairExpired) || errors.Is(err, store.ErrPairDenied) {
		s.pairFailed(r)
	}
}

func writePairError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrPairCode):
		httpError(w, err.Error(), http.StatusNotFound)
	case errors.Is(err, store.ErrPairExpired):
		httpError(w, err.Error(), http.StatusGone)
	case errors.Is(err, store.ErrPairDenied), errors.Is(err, store.ErrPairFull):
		httpError(w, err.Error(), http.StatusTooManyRequests)
	default:
		var status = http.StatusBadRequest
		if err == nil {
			status = http.StatusInternalServerError
		}
		httpError(w, errString(err), status)
	}
}

func errString(err error) string {
	if err == nil {
		return "invalid json"
	}
	return err.Error()
}

// mintBrowserAdmin creates the admin token returned once when sign-in is turned on.
func (s *Server) mintBrowserAdmin(r *http.Request) (string, error) {
	if dev, ok := clientFrom(r.Context()); ok && store.ScopeAllows(dev.Scopes, store.ScopeAdmin) {
		return "", nil
	}
	_, token, err := s.Store.InsertClient(r.Context(), "This browser", "web", []string{store.ScopeAdmin}, s.now())
	return token, err
}

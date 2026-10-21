package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"broadwave/internal/guide"
)

// guideSourcePull is how long a guide source that was just set up may take
// to load. Two weeks of a large Schedules Direct lineup takes a while.
const guideSourcePull = 90 * time.Second

func (s *Server) getGuideSources(w http.ResponseWriter, r *http.Request) {
	list, err := s.guideSourceList(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"sources": list})
}

// sdAccountFrom is the account a request names, with the saved one filling
// what it leaves out (the settings page never shows the saved password).
func (s *Server) sdAccountFrom(ctx context.Context, user, pass string) (string, string, error) {
	saved, err := s.Store.Settings(ctx)
	if err != nil {
		return "", "", err
	}
	user = strings.TrimSpace(user)
	if user == "" {
		user = strings.TrimSpace(saved["sdUser"])
	}
	if pass == "" && user == strings.TrimSpace(saved["sdUser"]) {
		pass = saved["sdPassword"]
	}
	if user == "" || pass == "" {
		return "", "", errors.New("Enter the Schedules Direct username and password.")
	}
	return user, pass, nil
}

func (s *Server) sdLineups(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Username   string `json:"username"`
		Password   string `json:"password"`
		PostalCode string `json:"postalCode"`
	}
	if err := decodeJSON(r, &body); err != nil {
		httpError(w, "invalid json", http.StatusBadRequest)
		return
	}
	postal, country := guide.SDPostal(body.PostalCode)
	if country == "" {
		apiError(w, http.StatusBadRequest, "postal_code", "Enter a US ZIP code (5 digits) or a Canadian postal code (like K1A 0B1).", nil)
		return
	}
	user, pass, err := s.sdAccountFrom(r.Context(), body.Username, body.Password)
	if err != nil {
		apiError(w, http.StatusBadRequest, "sd_account", err.Error(), nil)
		return
	}
	sd := s.schedulesDirect(user, pass)
	lineups, err := sd.Headends(r.Context(), country, postal)
	if err != nil {
		sdFailed(w, err)
		return
	}
	acct, err := sd.Account(r.Context())
	if err != nil {
		sdFailed(w, err)
		return
	}
	if lineups == nil {
		lineups = []guide.SDLineup{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"country": country, "postalCode": postal, "lineups": lineups, "account": acct})
}

// sdFailed answers with what Schedules Direct said, or that it could not be reached.
func sdFailed(w http.ResponseWriter, err error) {
	var sdErr *guide.SDError
	if errors.As(err, &sdErr) || !systemError(err) {
		apiError(w, http.StatusBadGateway, "sd_failed", plainSourceError(err), nil)
		return
	}
	apiError(w, http.StatusBadGateway, "sd_failed", "Schedules Direct could not be reached. Check the server's internet connection, then try again.", nil)
}

func (s *Server) useSDLineup(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Lineup   string `json:"lineup"`
	}
	if err := decodeJSON(r, &body); err != nil {
		httpError(w, "invalid json", http.StatusBadRequest)
		return
	}
	user, pass, err := s.sdAccountFrom(r.Context(), body.Username, body.Password)
	if err != nil {
		apiError(w, http.StatusBadRequest, "sd_account", err.Error(), nil)
		return
	}
	lineup := strings.TrimSpace(body.Lineup)
	if err := s.schedulesDirect(user, pass).AddLineup(r.Context(), lineup); err != nil {
		sdFailed(w, err)
		return
	}
	values := map[string]string{"sdUser": user, "sdLineup": lineup}
	if body.Password != "" {
		values["sdPassword"] = body.Password
	}
	s.saveAndPull(w, r, sourceSD, values)
}

func (s *Server) useXMLTV(w http.ResponseWriter, r *http.Request) {
	var body struct {
		URL string `json:"url"`
	}
	if err := decodeJSON(r, &body); err != nil {
		httpError(w, "invalid json", http.StatusBadRequest)
		return
	}
	link := strings.TrimSpace(body.URL)
	if !strings.HasPrefix(link, "https://") && !strings.HasPrefix(link, "http://") {
		apiError(w, http.StatusBadRequest, "guide_url", "The XMLTV link needs to start with http:// or https://.", nil)
		return
	}
	s.saveAndPull(w, r, sourceXMLTV, map[string]string{"guideUrl": link})
}

// saveAndPull saves a guide source's settings and pulls it now. A source
// that brings no listings is not kept: its old settings come back and the
// answer says why.
func (s *Server) saveAndPull(w http.ResponseWriter, r *http.Request, source string, values map[string]string) {
	ctx := r.Context()
	before, err := s.Store.Settings(ctx)
	if err != nil {
		writeError(w, err)
		return
	}
	if err := s.Store.PutSettings(ctx, values); err != nil {
		httpError(w, err.Error(), http.StatusBadRequest)
		return
	}
	// The pull goes on when the page that asked for it closes.
	pullCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), guideSourcePull)
	defer cancel()
	started := time.Now().UTC().Add(-time.Second)
	n, pullErr := s.pullGuide(pullCtx, map[string]bool{source: true})
	failure := ""
	if states, err := s.Store.GuideSources(ctx); err == nil {
		if st := states[source]; st.Error != "" && !st.ErrorAt.Before(started) {
			failure = st.Error
		}
	}
	if failure == "" && pullErr != nil && !errors.Is(pullErr, ErrNoTuner) {
		failure = pullErr.Error()
	}
	if failure != "" {
		restore := map[string]string{}
		for key := range values {
			if key != "sdPassword" {
				restore[key] = before[key]
			}
		}
		_ = s.Store.PutSettings(ctx, restore)
		apiError(w, http.StatusBadGateway, "guide_source_failed", plainSourceError(errors.New(failure)), nil)
		return
	}
	listed := s.listedChannels(ctx)
	if listed > 0 {
		s.setFinish("guide", "done", fmt.Sprintf("Listings for %s.", countLine(listed, "channel", "channels")))
	}
	list, _ := s.guideSourceList(ctx)
	writeJSON(w, http.StatusOK, map[string]any{"airings": n, "channels": listed, "sources": list})
}

// plainSourceError words a listing source's failure for the person setting it up.
func plainSourceError(err error) string {
	msg := err.Error()
	switch {
	case errors.Is(err, guide.ErrNoPrograms) || strings.Contains(msg, guide.ErrNoPrograms.Error()):
		return "The XMLTV link gave no programs. Check that it is an XMLTV guide (it starts with <tv>) and that it is not empty."
	case strings.Contains(msg, "guide returned 401"), strings.Contains(msg, "guide returned 403"):
		return "The guide link turned the server away (" + strings.TrimPrefix(msg, "guide returned ") + "). Check the link and any sign-in it needs."
	case strings.Contains(msg, "guide returned 404"):
		return "Nothing is at that guide link (404). Check the link."
	}
	if msg == "" {
		return "The guide did not load."
	}
	msg = strings.ToUpper(msg[:1]) + msg[1:]
	if !strings.HasSuffix(msg, ".") {
		msg += "."
	}
	return msg
}

// manualGuideFailed is a Refresh press where every source pulled failed.
func manualGuideFailed(w http.ResponseWriter, err error) bool {
	var sourceErr *GuideSourceError
	if !errors.As(err, &sourceErr) {
		return false
	}
	apiError(w, http.StatusBadGateway, "guide_failed", plainSourceError(err), nil)
	return true
}

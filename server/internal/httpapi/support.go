package httpapi

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"runtime"
	"sort"
	"strings"
	"time"

	"broadwave/internal/doctor"
	"broadwave/internal/logbuf"
	"broadwave/internal/store"
)

// support is a zip of logs, versions, doctor results, and redacted config.
func (s *Server) support(w http.ResponseWriter, r *http.Request) {
	body, err := s.supportBundle(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="broadwave-support.zip"`)
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(body)
}

type versionInfo struct {
	Version    string `json:"version"`
	APIVersion int    `json:"apiVersion"`
	Go         string `json:"go"`
	OS         string `json:"os"`
	Encoder    string `json:"encoder,omitempty"`
	FFmpeg     string `json:"ffmpeg,omitempty"`
	Checked    string `json:"checked"`
}

type supportConfig struct {
	Server   store.Identity    `json:"server"`
	Settings map[string]string `json:"settings"`
	Sources  []store.Source    `json:"sources"`
	Devices  []store.Device    `json:"devices"`
}

func (s *Server) supportBundle(ctx context.Context) ([]byte, error) {
	secrets, err := s.Store.CredentialValues(ctx)
	if err != nil {
		return nil, err
	}
	settings, err := s.Store.Settings(ctx)
	if err != nil {
		return nil, err
	}
	sources, err := s.Store.Sources(ctx)
	if err != nil {
		return nil, err
	}
	devices, err := s.Store.Devices(ctx)
	if err != nil {
		return nil, err
	}
	id, err := s.Store.Identity(ctx, DefaultServerName())
	if err != nil {
		return nil, err
	}
	raw := append([]string{}, secrets...)
	for _, device := range devices {
		raw = append(raw, device.BaseURL, device.LineupURL)
	}
	pieces := secretPieces(raw)

	version := s.Version
	if version == "" {
		version = "dev"
	}
	info := versionInfo{
		Version:    version,
		APIVersion: apiVersion,
		Go:         runtime.Version(),
		OS:         runtime.GOOS + "/" + runtime.GOARCH,
		Checked:    s.now().UTC().Format(time.RFC3339),
	}
	if s.Hub != nil {
		info.Encoder = s.Hub.Encoder
		if s.Hub.FFmpeg != "" {
			info.FFmpeg = s.Hub.FFmpegVersion()
		}
	}
	notes := s.doctorNotes(devices)
	if len(notes) == 0 {
		notes = []doctor.Note{}
	}
	cfg := supportConfig{
		Server:   id,
		Settings: redactSettings(settings),
		Sources:  redactSources(sources),
		Devices:  redactDevices(devices),
	}
	files, err := supportFiles(info, notes, cfg, logbuf.Text(), pieces, s.now().UTC())
	if err != nil {
		return nil, err
	}
	if !bundleClean(files, pieces) {
		return nil, errors.New("The support bundle could not be built.")
	}
	return files, nil
}

func supportFiles(info versionInfo, notes []doctor.Note, cfg supportConfig, logs string, secrets []string, when time.Time) ([]byte, error) {
	versionText, err := jsonText(info)
	if err != nil {
		return nil, err
	}
	doctorText, err := jsonText(map[string]any{"notes": notes})
	if err != nil {
		return nil, err
	}
	configText, err := jsonText(cfg)
	if err != nil {
		return nil, err
	}
	logs = scrubBody(logs, secrets)
	if strings.TrimSpace(logs) == "" {
		logs = "No log lines yet.\n"
	} else if !strings.HasSuffix(logs, "\n") {
		logs += "\n"
	}
	parts := []struct {
		name, body string
	}{
		{"versions.json", scrubBody(versionText, secrets)},
		{"doctor.json", scrubBody(doctorText, secrets)},
		{"config.json", scrubBody(configText, secrets)},
		{"logs.txt", logs},
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, part := range parts {
		hdr := &zip.FileHeader{Name: part.name, Method: zip.Store, Modified: when}
		w, err := zw.CreateHeader(hdr)
		if err != nil {
			return nil, err
		}
		if _, err := io.WriteString(w, part.body); err != nil {
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func jsonText(v any) (string, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func redactSettings(values map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range values {
		switch k {
		case "sdPassword", "tmdbKey", "sdPasswordSet", "tmdbKeySet":
			continue
		default:
			out[k] = redactURL(v)
		}
	}
	if strings.TrimSpace(values["sdPassword"]) != "" {
		out["sdPasswordSet"] = "1"
	} else {
		out["sdPasswordSet"] = "0"
	}
	if strings.TrimSpace(values["tmdbKey"]) != "" {
		out["tmdbKeySet"] = "1"
	} else {
		out["tmdbKeySet"] = "0"
	}
	return out
}

func redactSources(items []store.Source) []store.Source {
	if items == nil {
		return []store.Source{}
	}
	out := make([]store.Source, len(items))
	for i, item := range items {
		item.URL = redactURL(item.URL)
		item.XMLTV = redactURL(item.XMLTV)
		out[i] = item
	}
	return out
}

func redactDevices(items []store.Device) []store.Device {
	if items == nil {
		return []store.Device{}
	}
	out := make([]store.Device, len(items))
	for i, item := range items {
		item.BaseURL = redactURL(item.BaseURL)
		item.LineupURL = redactURL(item.LineupURL)
		out[i] = item
	}
	return out
}

func redactURL(raw string) string {
	masked := store.MaskURL(raw)
	u, err := url.Parse(masked)
	if err != nil || u.Host == "" {
		return logbuf.StripDeviceAuth(masked)
	}
	q := u.Query()
	stripped := false
	for key := range q {
		if strings.EqualFold(key, "DeviceAuth") {
			q.Del(key)
			stripped = true
		}
	}
	if !stripped {
		return masked
	}
	u.RawQuery = q.Encode()
	return u.String()
}

// Short values are left alone so redaction cannot erase an ordinary setting.
func secretPieces(values []string) []string {
	seen := map[string]bool{}
	var out []string
	add := func(v string) {
		v = strings.TrimSpace(v)
		if len(v) < 8 || seen[v] {
			return
		}
		seen[v] = true
		out = append(out, v)
	}
	for _, raw := range values {
		u, err := url.Parse(raw)
		if err != nil || u.Host == "" {
			add(raw)
			continue
		}
		secretish := false
		if u.User != nil {
			if p, ok := u.User.Password(); ok && p != "" {
				add(p)
				secretish = true
			}
		}
		for key, vals := range u.Query() {
			if !secretKey(key) {
				continue
			}
			for _, v := range vals {
				if v != "" {
					add(v)
					secretish = true
				}
			}
		}
		if secretish {
			add(raw)
		}
	}
	sort.Slice(out, func(i, j int) bool { return len(out[i]) > len(out[j]) })
	return out
}

func secretKey(key string) bool {
	switch strings.ToLower(key) {
	case "password", "pass", "token", "secret", "deviceauth":
		return true
	default:
		return false
	}
}

func scrubBody(text string, secrets []string) string {
	text = logbuf.StripDeviceAuth(text)
	for _, secret := range secrets {
		text = strings.ReplaceAll(text, secret, "••••")
	}
	return text
}

func bundleClean(body []byte, secrets []string) bool {
	if bytes.Contains(body, []byte("DeviceAuth")) {
		return false
	}
	for _, secret := range secrets {
		if secret != "" && bytes.Contains(body, []byte(secret)) {
			return false
		}
	}
	return true
}

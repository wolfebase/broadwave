package live

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"ota-viewer/internal/store"
)

func writeSidecar(rec store.Recording) {
	if rec.Path == "" {
		return
	}
	body, err := json.MarshalIndent(map[string]any{
		"id":          rec.ID,
		"title":       rec.Title,
		"guideNumber": rec.GuideNumber,
		"status":      rec.Status,
		"startedAt":   rec.StartedAt,
		"endedAt":     rec.EndedAt,
	}, "", "  ")
	if err != nil {
		return
	}
	side := strings.TrimSuffix(rec.Path, filepath.Ext(rec.Path)) + ".json"
	_ = os.WriteFile(side, append(body, '\n'), 0o644)
}

func WriteEDL(path string, markers []store.Marker) error {
	if path == "" {
		return nil
	}
	var b strings.Builder
	for _, marker := range markers {
		if marker.End <= marker.Start {
			continue
		}
		fmt.Fprintf(&b, "%.3f %.3f 0\n", marker.Start, marker.End)
	}
	side := strings.TrimSuffix(path, filepath.Ext(path)) + ".edl"
	return os.WriteFile(side, []byte(b.String()), 0o644)
}

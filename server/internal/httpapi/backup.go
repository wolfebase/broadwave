package httpapi

import (
	"errors"
	"net/http"

	"broadwave/internal/backup"
)

func (s *Server) listBackups(w http.ResponseWriter, r *http.Request) {
	items, err := backup.List(s.BackupDir)
	if err != nil {
		writeError(w, err)
		return
	}
	if items == nil {
		items = []backup.Item{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"backups": items})
}

func (s *Server) downloadSavedBackup(w http.ResponseWriter, r *http.Request) {
	path, item, err := backup.Resolve(s.BackupDir, r.PathValue("name"))
	if err != nil {
		if errors.Is(err, backup.ErrNotFound) {
			httpError(w, "That backup is not on this server.", http.StatusNotFound)
			return
		}
		writeError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", `attachment; filename="`+item.Name+`"`)
	http.ServeFile(w, r, path)
}

func (s *Server) restoreSavedBackup(w http.ResponseWriter, r *http.Request) {
	if err := backup.Restore(r.Context(), s.Store, s.BackupDir, r.PathValue("name")); err != nil {
		if errors.Is(err, backup.ErrNotFound) {
			httpError(w, "That backup is not on this server.", http.StatusNotFound)
			return
		}
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

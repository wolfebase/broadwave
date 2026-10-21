package httpapi

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"

	"broadwave/internal/backup"
	"broadwave/internal/dvr"
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
	defer s.applyTunnel(context.WithoutCancel(r.Context()))
	if err := backup.Restore(r.Context(), s.Store, s.BackupDir, r.PathValue("name")); err != nil {
		if errors.Is(err, backup.ErrNotFound) {
			httpError(w, "That backup is not on this server.", http.StatusNotFound)
			return
		}
		writeError(w, err)
		return
	}
	s.afterRestore()
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// afterRestore follows recordings that moved since the backup and adds the
// ones on disk the backup did not have. A large folder takes a while, so it
// runs after the answer.
func (s *Server) afterRestore() {
	if s.Store == nil || s.Hub == nil || s.Hub.Dir == "" {
		return
	}
	go func() {
		if _, err := dvr.ImportFound(context.Background(), s.Store, s.Hub); err != nil {
			slog.Warn(fmt.Sprintf("restore: looking for recordings on disk: %v", err))
		}
	}()
}

// importFound adds recordings in the recordings folder that the catalog lacks.
func (s *Server) importFound(w http.ResponseWriter, r *http.Request) {
	if s.Store == nil || s.Hub == nil || s.Hub.Dir == "" {
		httpError(w, "Recordings are not set up on this server.", http.StatusConflict)
		return
	}
	added, err := dvr.ImportFound(r.Context(), s.Store, s.Hub)
	if errors.Is(err, fs.ErrNotExist) {
		httpError(w, "The recordings folder isn't there. Is the drive mounted?", http.StatusConflict)
		return
	}
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"added": added})
}

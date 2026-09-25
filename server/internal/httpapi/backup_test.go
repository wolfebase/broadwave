package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"broadwave/internal/backup"
)

func TestSavedBackupListRestore(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	if err := st.PutSettings(ctx, map[string]string{"layout": "tv"}); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	item, err := backup.Take(ctx, st, dir, time.Date(2026, 5, 2, 3, 0, 0, 0, time.UTC), backup.KindDaily)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.PutSettings(ctx, map[string]string{"layout": "phone"}); err != nil {
		t.Fatal(err)
	}
	api := &Server{Store: st, BackupDir: dir}
	h := api.Handler()

	rec := get(t, h, "/api/v1/backups")
	var body struct {
		Backups []backup.Item `json:"backups"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Backups) != 1 || body.Backups[0].Name != item.Name || body.Backups[0].Kind != backup.KindDaily {
		t.Fatalf("list %+v", body.Backups)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/backups/"+item.Name, nil)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("download %d %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/octet-stream" {
		t.Fatalf("type %s", ct)
	}
	if !bytes.HasPrefix(rec.Body.Bytes(), []byte("SQLite format 3")) {
		t.Fatal("download is not a sqlite file")
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/backups/version", nil)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("version file %d %s", rec.Code, rec.Body.String())
	}
	req = httptest.NewRequest(http.MethodPost, "/api/v1/backups/broadwave-20260101-030000-daily.db/restore", nil)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("missing restore %d %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/api/v1/backups/"+item.Name+"/restore", nil)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("restore %d %s", rec.Code, rec.Body.String())
	}
	values, err := st.Settings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if values["layout"] != "tv" {
		t.Fatalf("layout after restore: %q", values["layout"])
	}
}

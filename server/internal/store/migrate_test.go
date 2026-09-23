package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

func TestMigrationsAreNumberedAndUnique(t *testing.T) {
	list, err := loadMigrations()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) == 0 || list[0].version != 1 {
		t.Fatalf("first migration should be 0001, got %+v", list)
	}
}

func TestOpenTwiceAppliesOnce(t *testing.T) {
	dir := t.TempDir()
	st, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	st.Close()
	st, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	var n int
	if err := st.db.QueryRow(`SELECT count(*) FROM schema_migrations WHERE version = 1`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("baseline recorded %d times, err %v", n, err)
	}
}

func TestLegacyCatalogUpgrades(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ota-viewer.db")
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path))
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`
CREATE TABLE devices (device_id TEXT PRIMARY KEY, base_url TEXT NOT NULL);
CREATE TABLE channels (id INTEGER PRIMARY KEY, device_id TEXT NOT NULL, guide_number TEXT NOT NULL, guide_name TEXT NOT NULL DEFAULT '',
	stream_url TEXT NOT NULL DEFAULT '', video_codec TEXT NOT NULL DEFAULT '', audio_codec TEXT NOT NULL DEFAULT '', hd INTEGER NOT NULL DEFAULT 0,
	favorite INTEGER NOT NULL DEFAULT 0, enabled INTEGER NOT NULL DEFAULT 1, hidden INTEGER NOT NULL DEFAULT 0, custom_name TEXT NOT NULL DEFAULT '',
	custom_number TEXT NOT NULL DEFAULT '', present INTEGER NOT NULL DEFAULT 1, UNIQUE(device_id, guide_number));
CREATE TABLE passes (id INTEGER PRIMARY KEY, title TEXT NOT NULL, channel_id INTEGER NOT NULL DEFAULT 0, kind TEXT NOT NULL DEFAULT 'series');
INSERT INTO passes (title) VALUES ('Jeopardy!');
`)
	if err != nil {
		t.Fatal(err)
	}
	db.Close()

	st, err := Open(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	passes, err := st.Passes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(passes) != 1 || passes[0].Title != "Jeopardy!" || passes[0].MatchKind != "title" || !passes[0].Commercials {
		t.Fatalf("legacy pass should survive with baseline defaults: %+v", passes)
	}
}

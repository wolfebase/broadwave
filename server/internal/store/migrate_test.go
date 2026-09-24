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
	path := filepath.Join(t.TempDir(), "broadwave.db")
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

func TestTunerBecomesASourceWithoutMovingChannels(t *testing.T) {
	path := filepath.Join(t.TempDir(), "broadwave.db")
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path))
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`
CREATE TABLE devices (
	device_id TEXT PRIMARY KEY, friendly_name TEXT NOT NULL DEFAULT '', base_url TEXT NOT NULL,
	tuner_count INTEGER NOT NULL DEFAULT 0, priority INTEGER NOT NULL DEFAULT 0);
CREATE TABLE channels (
	id INTEGER PRIMARY KEY, device_id TEXT NOT NULL, guide_number TEXT NOT NULL,
	guide_name TEXT NOT NULL DEFAULT '');
CREATE TABLE recordings (id INTEGER PRIMARY KEY, channel_id INTEGER NOT NULL, title TEXT NOT NULL, path TEXT NOT NULL, status TEXT NOT NULL);
CREATE TABLE airings (id INTEGER PRIMARY KEY, channel_id INTEGER NOT NULL, title TEXT NOT NULL, starts_at TEXT NOT NULL DEFAULT '', ends_at TEXT NOT NULL DEFAULT '');
CREATE TABLE passes (id INTEGER PRIMARY KEY, title TEXT NOT NULL, channel_id INTEGER NOT NULL DEFAULT 0);
CREATE TABLE sources (id INTEGER PRIMARY KEY, kind TEXT NOT NULL, name TEXT NOT NULL, url TEXT NOT NULL DEFAULT '', xmltv_url TEXT NOT NULL DEFAULT '', enabled INTEGER NOT NULL DEFAULT 1);
CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY, name TEXT NOT NULL, applied_at TEXT NOT NULL);
INSERT INTO devices (device_id, friendly_name, base_url, tuner_count, priority) VALUES ('10611B4C', 'Living room', 'http://192.168.1.252', 2, 1);
INSERT INTO channels (id, device_id, guide_number, guide_name) VALUES (1, '10611B4C', '4.1', 'WDAF');
INSERT INTO recordings (id, channel_id, title, path, status) VALUES (4, 1, 'Jeopardy!', '/tmp/j.ts', 'complete');
INSERT INTO passes (id, title, channel_id) VALUES (1, 'Jeopardy!', 1);
INSERT INTO sources (id, kind, name, url) VALUES (3, 'm3u', 'Playlist', 'http://example/pl.m3u');
`)
	if err != nil {
		t.Fatal(err)
	}
	for v := 1; v <= 12; v++ {
		if _, err := db.Exec(`INSERT INTO schema_migrations (version, name, applied_at) VALUES (?, 'pre', '2026-09-23T00:00:00Z')`, v); err != nil {
			t.Fatal(err)
		}
	}
	db.Close()

	st, err := Open(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	var channelID, recordingChannel, passChannel int64
	if err := st.db.QueryRow(`SELECT id FROM channels WHERE guide_number = '4.1'`).Scan(&channelID); err != nil {
		t.Fatal(err)
	}
	if err := st.db.QueryRow(`SELECT channel_id FROM recordings WHERE id = 4`).Scan(&recordingChannel); err != nil {
		t.Fatal(err)
	}
	if err := st.db.QueryRow(`SELECT channel_id FROM passes WHERE id = 1`).Scan(&passChannel); err != nil {
		t.Fatal(err)
	}
	if channelID != 1 || recordingChannel != 1 || passChannel != 1 {
		t.Fatalf("ids moved: channel %d recording %d pass %d", channelID, recordingChannel, passChannel)
	}
	var kind, key string
	var tuners int
	if err := st.db.QueryRow(`SELECT kind, stable_key, tuner_count FROM sources WHERE device_id = '10611B4C'`).Scan(&kind, &key, &tuners); err != nil {
		t.Fatal(err)
	}
	if kind != "hdhomerun" || key != "hdhr:10611B4C" || tuners != 2 {
		t.Fatalf("tuner source = %s %s %d", kind, key, tuners)
	}
	var playlistKey string
	if err := st.db.QueryRow(`SELECT stable_key FROM sources WHERE id = 3`).Scan(&playlistKey); err != nil {
		t.Fatal(err)
	}
	if playlistKey != "src:3" {
		t.Fatalf("playlist key = %s", playlistKey)
	}
}

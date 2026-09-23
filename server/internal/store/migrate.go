package store

import (
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"
	"time"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

type migration struct {
	version int
	name    string
	sql     string
}

func loadMigrations() ([]migration, error) {
	entries, err := fs.ReadDir(migrationFiles, "migrations")
	if err != nil {
		return nil, err
	}
	var out []migration
	for _, e := range entries {
		name := e.Name()
		prefix, rest, ok := strings.Cut(name, "_")
		if !ok || !strings.HasSuffix(name, ".sql") {
			return nil, fmt.Errorf("migration %q must be named NNNN_name.sql", name)
		}
		version, err := strconv.Atoi(prefix)
		if err != nil {
			return nil, fmt.Errorf("migration %q: %w", name, err)
		}
		body, err := migrationFiles.ReadFile("migrations/" + name)
		if err != nil {
			return nil, err
		}
		out = append(out, migration{version: version, name: strings.TrimSuffix(rest, ".sql"), sql: string(body)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].version < out[j].version })
	for i := 1; i < len(out); i++ {
		if out[i].version == out[i-1].version {
			return nil, fmt.Errorf("two migrations share version %d", out[i].version)
		}
	}
	return out, nil
}

// Migrate applies pending migrations in order, each in its own transaction.
func Migrate(db *sql.DB) error {
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
	version INTEGER PRIMARY KEY,
	name TEXT NOT NULL,
	applied_at TEXT NOT NULL
)`); err != nil {
		return err
	}
	applied := map[int]bool{}
	rows, err := db.Query(`SELECT version FROM schema_migrations`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			rows.Close()
			return err
		}
		applied[v] = true
	}
	rows.Close()
	if len(applied) == 0 {
		upgradeLegacy(db)
	}
	list, err := loadMigrations()
	if err != nil {
		return err
	}
	for _, m := range list {
		if applied[m.version] {
			continue
		}
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(m.sql); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("migration %04d_%s: %w", m.version, m.name, err)
		}
		if _, err := tx.Exec(`INSERT INTO schema_migrations (version, name, applied_at) VALUES (?, ?, ?)`,
			m.version, m.name, time.Now().UTC().Format(time.RFC3339)); err != nil {
			_ = tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

// upgradeLegacy brings a catalog written before numbered migrations up to the
// 0001 baseline. Those builds added columns on every start, so an older file
// can be missing any of them.
func upgradeLegacy(db *sql.DB) {
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = 'channels'`).Scan(&n); err != nil || n == 0 {
		return
	}
	columns := []struct{ table, def string }{
		{"channels", "frequency_hz INTEGER NOT NULL DEFAULT 0"},
		{"channels", "program_num INTEGER NOT NULL DEFAULT 0"},
		{"passes", "pad_before INTEGER NOT NULL DEFAULT 0"},
		{"passes", "pad_after INTEGER NOT NULL DEFAULT 0"},
		{"passes", "priority INTEGER NOT NULL DEFAULT 0"},
		{"passes", "episodes TEXT NOT NULL DEFAULT 'all'"},
		{"passes", "keep_mode TEXT NOT NULL DEFAULT 'all'"},
		{"passes", "keep_count INTEGER NOT NULL DEFAULT 0"},
		{"passes", "limit_count INTEGER NOT NULL DEFAULT 0"},
		{"passes", "rerecord INTEGER NOT NULL DEFAULT 0"},
		{"passes", "commercials INTEGER NOT NULL DEFAULT 1"},
		{"passes", "time_start TEXT NOT NULL DEFAULT ''"},
		{"passes", "time_end TEXT NOT NULL DEFAULT ''"},
		{"passes", "match_kind TEXT NOT NULL DEFAULT 'title'"},
		{"recordings", "duration_sec REAL NOT NULL DEFAULT 0"},
		{"recordings", "subtitle TEXT NOT NULL DEFAULT ''"},
		{"recordings", "description TEXT NOT NULL DEFAULT ''"},
		{"recordings", "category TEXT NOT NULL DEFAULT ''"},
		{"recordings", "program_id TEXT NOT NULL DEFAULT ''"},
		{"recordings", "watched INTEGER NOT NULL DEFAULT 0"},
		{"airings", "program_id TEXT NOT NULL DEFAULT ''"},
		{"airings", "is_new INTEGER NOT NULL DEFAULT 0"},
		{"virtual_channels", "order_mode TEXT NOT NULL DEFAULT 'custom'"},
		{"virtual_channels", "rule_title TEXT NOT NULL DEFAULT ''"},
	}
	for _, c := range columns {
		_, _ = db.Exec(`ALTER TABLE ` + c.table + ` ADD COLUMN ` + c.def)
	}
}

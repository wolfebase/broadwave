// Package backup keeps copies of the catalog database.
// Recordings on disk are never part of a backup and are never deleted by a restore.
package backup

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"broadwave/internal/store"
)

const (
	KindDaily   = "daily"
	KindWeekly  = "weekly"
	KindVersion = "version"

	keepDaily   = 7
	keepWeekly  = 4
	keepVersion = 8

	// nightlyHour is the quiet hour on the clock passed to the scheduler.
	// That clock carries the server's zone. This package never picks a place.
	nightlyHour = 3

	retryBackup = 15 * time.Minute

	// versionFile is the last version that was snapshotted, beside the copies.
	// A version change does not need a schema migration.
	versionFile = "version"
)

// ErrNotFound is a missing backup or a name that is not one of ours.
var ErrNotFound = errors.New("backup not found")

// Item is one saved catalog copy.
type Item struct {
	Name    string    `json:"name"`
	Kind    string    `json:"kind"`
	TakenAt time.Time `json:"takenAt"`
	Bytes   int64     `json:"bytes"`
}

var backupName = regexp.MustCompile(`^broadwave-([0-9]{8})-([0-9]{6})-(daily|weekly|version)\.db$`)

// Filename is the UTC name for a copy taken at at.
func Filename(at time.Time, kind string) string {
	return at.UTC().Format("broadwave-20060102-150405-") + kind + ".db"
}

// List returns saved copies, newest first. A missing folder is an empty list.
// An empty dir does not read the process working directory.
func List(dir string) ([]Item, error) {
	if dir == "" {
		return []Item{}, nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []Item{}, nil
		}
		return nil, err
	}
	var out []Item
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		item, ok := parseName(entry.Name())
		if !ok {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return nil, err
		}
		item.Bytes = info.Size()
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].TakenAt.Equal(out[j].TakenAt) {
			return out[i].Name > out[j].Name
		}
		return out[i].TakenAt.After(out[j].TakenAt)
	})
	if out == nil {
		out = []Item{}
	}
	return out, nil
}

// Resolve returns the path of one named copy inside dir.
// Names that escape the folder or do not match a backup are ErrNotFound.
func Resolve(dir, name string) (string, Item, error) {
	item, ok := parseName(name)
	if !ok || name != filepath.Base(name) || dir == "" {
		return "", Item{}, ErrNotFound
	}
	cleanDir := filepath.Clean(dir)
	path := filepath.Join(cleanDir, item.Name)
	rel, err := filepath.Rel(cleanDir, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", Item{}, ErrNotFound
	}
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", Item{}, ErrNotFound
		}
		return "", Item{}, err
	}
	if info.IsDir() {
		return "", Item{}, ErrNotFound
	}
	item.Bytes = info.Size()
	return path, item, nil
}

// Prune keeps the newest 7 daily, 4 weekly, and 8 pre-update copies.
func Prune(dir string) error {
	items, err := List(dir)
	if err != nil {
		return err
	}
	drop := append(append(pruneKind(items, KindDaily, keepDaily), pruneKind(items, KindWeekly, keepWeekly)...), pruneKind(items, KindVersion, keepVersion)...)
	for _, item := range drop {
		if err := os.Remove(filepath.Join(dir, item.Name)); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

// Take writes one consistent catalog copy and then prunes.
func Take(ctx context.Context, st *store.Store, dir string, at time.Time, kind string) (Item, error) {
	switch kind {
	case KindDaily, KindWeekly, KindVersion:
	default:
		return Item{}, fmt.Errorf("unknown backup kind %q", kind)
	}
	if st == nil {
		return Item{}, errors.New("catalog is not open")
	}
	if dir == "" {
		return Item{}, errors.New("backup folder is not set")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return Item{}, err
	}
	name := Filename(at, kind)
	path := filepath.Join(dir, name)
	if err := st.BackupTo(ctx, path); err != nil {
		return Item{}, err
	}
	if err := Prune(dir); err != nil {
		return Item{}, err
	}
	_, item, err := Resolve(dir, name)
	return item, err
}

// Nightly saves one daily copy and, once per local week, a weekly copy of it.
// The clock's location decides the day and the week.
func Nightly(ctx context.Context, st *store.Store, dir string, now time.Time) error {
	daily, err := Take(ctx, st, dir, now, KindDaily)
	if err != nil {
		return err
	}
	slog.Info(fmt.Sprintf("backup: saved %s", daily.Name))
	if !weeklyDue(dir, now) {
		return nil
	}
	src := filepath.Join(dir, daily.Name)
	dst := filepath.Join(dir, Filename(now, KindWeekly))
	if err := copyFile(src, dst); err != nil {
		return err
	}
	slog.Info(fmt.Sprintf("backup: saved %s", filepath.Base(dst)))
	return Prune(dir)
}

// Restore replaces catalog rows from a named copy. It does not touch recording files.
func Restore(ctx context.Context, st *store.Store, dir, name string) error {
	if st == nil {
		return errors.New("catalog is not open")
	}
	path, _, err := Resolve(dir, name)
	if err != nil {
		return err
	}
	return st.RestoreFrom(ctx, path)
}

// SnapshotIfVersionChanged copies the catalog before migrations when version
// differs from the version stored beside the backups. A missing catalog records
// the version and copies nothing.
func SnapshotIfVersionChanged(ctx context.Context, configDir, version string, now time.Time) error {
	version = strings.TrimSpace(version)
	if version == "" || strings.ContainsAny(version, "\r\n") {
		version = "dev"
	}
	dir := filepath.Join(configDir, "backups")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	last, err := readVersion(dir)
	if err != nil {
		return err
	}
	if last == version {
		return nil
	}
	dbPath := filepath.Join(configDir, "broadwave.db")
	if _, err := os.Stat(dbPath); err != nil {
		if os.IsNotExist(err) {
			return writeVersion(dir, version)
		}
		return err
	}
	name := Filename(now, KindVersion)
	if err := vacuumFile(ctx, dbPath, filepath.Join(dir, name)); err != nil {
		return err
	}
	if err := Prune(dir); err != nil {
		return err
	}
	slog.Info(fmt.Sprintf("backup: saved %s", name))
	return writeVersion(dir, version)
}

// Scheduler takes the nightly copy. A nil Now uses the machine clock.
type Scheduler struct {
	Store *store.Store
	Dir   string
	Now   func() time.Time
}

// Run backs up when today's copy is due, then waits until the next quiet hour.
func (s *Scheduler) Run(ctx context.Context) {
	if s == nil || s.Store == nil || s.Dir == "" {
		return
	}
	timer := time.NewTimer(s.wait(s.now()))
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		if wait := s.wait(s.now()); wait > 0 {
			timer.Reset(wait)
			continue
		}
		if err := Nightly(ctx, s.Store, s.Dir, s.now()); err != nil {
			slog.Error(fmt.Sprintf("backup: %v", err))
			timer.Reset(retryBackup)
			continue
		}
		wait := s.wait(s.now())
		if wait <= 0 {
			wait = 24 * time.Hour
		}
		timer.Reset(wait)
	}
}

func (s *Scheduler) now() time.Time {
	if s != nil && s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func (s *Scheduler) wait(now time.Time) time.Duration {
	d := nextDaily(now, s.Dir).Sub(now)
	if d < 0 {
		return 0
	}
	return d
}

func nextDaily(now time.Time, dir string) time.Time {
	loc := now.Location()
	items, err := List(dir)
	if err != nil {
		items = nil
	}
	haveToday := false
	for _, item := range items {
		if item.Kind != KindDaily {
			continue
		}
		if sameDate(item.TakenAt.In(loc), now) {
			haveToday = true
			break
		}
	}
	todayAt := time.Date(now.Year(), now.Month(), now.Day(), nightlyHour, 0, 0, 0, loc)
	if haveToday {
		return todayAt.AddDate(0, 0, 1)
	}
	if now.Before(todayAt) {
		return todayAt
	}
	return now
}

func weeklyDue(dir string, now time.Time) bool {
	items, err := List(dir)
	if err != nil {
		return false
	}
	loc := now.Location()
	year, week := now.In(loc).ISOWeek()
	for _, item := range items {
		if item.Kind != KindWeekly {
			continue
		}
		iy, iw := item.TakenAt.In(loc).ISOWeek()
		if iy == year && iw == week {
			return false
		}
	}
	return true
}

func pruneKind(items []Item, kind string, keep int) []Item {
	var group []Item
	for _, item := range items {
		if item.Kind == kind {
			group = append(group, item)
		}
	}
	sort.Slice(group, func(i, j int) bool {
		if group[i].TakenAt.Equal(group[j].TakenAt) {
			return group[i].Name > group[j].Name
		}
		return group[i].TakenAt.After(group[j].TakenAt)
	})
	if len(group) <= keep {
		return nil
	}
	return group[keep:]
}

func parseName(name string) (Item, bool) {
	m := backupName.FindStringSubmatch(name)
	if m == nil {
		return Item{}, false
	}
	at, err := time.ParseInLocation("20060102-150405", m[1]+"-"+m[2], time.UTC)
	if err != nil {
		return Item{}, false
	}
	return Item{Name: name, Kind: m[3], TakenAt: at}, true
}

func sameDate(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}

func readVersion(dir string) (string, error) {
	b, err := os.ReadFile(filepath.Join(dir, versionFile))
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}

func writeVersion(dir, version string) error {
	return os.WriteFile(filepath.Join(dir, versionFile), []byte(version+"\n"), 0o644)
}

func vacuumFile(ctx context.Context, src, dest string) error {
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(src)+"?_pragma=busy_timeout(5000)")
	if err != nil {
		return err
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	escaped := strings.ReplaceAll(dest, "'", "''")
	_, err = db.ExecContext(ctx, fmt.Sprintf(`VACUUM INTO '%s'`, escaped))
	return err
}

func copyFile(src, dst string) error {
	tmp := dst + ".partial"
	_ = os.Remove(tmp)
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	ok := false
	defer func() {
		if !ok {
			_ = os.Remove(tmp)
		}
	}()
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	if err := out.Sync(); err != nil {
		_ = out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp, dst); err != nil {
		return err
	}
	ok = true
	return nil
}

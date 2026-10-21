package store

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// restoreTables are replaced whole by a restore: they hold only the user's choices.
var restoreTables = []string{"settings", "passes", "virtual_channels", "virtual_items", "seen_programs", "sources", "skipped_airings", "source_secrets"}

// channelChoices are the columns of a channel the viewer sets. The rest come
// from the tuner and the guide.
var channelChoices = []string{"favorite", "enabled", "hidden", "custom_name", "custom_number", "guide_key", "twin_choice"}

// RestoreFrom brings back a backup's choices and catalog, all or nothing.
// A backup from an older version fills the columns it has; new ones take
// their defaults.
//
// Settings, passes, virtual channels, sources, and seen programs are
// replaced. Channels are matched by tuner and number, and keep the backup's
// favorites, names, and hidden state; a channel the tuner has not listed yet
// is added as not present. Recordings are matched by file: one the catalog
// already has stays as it is, the rest come back, with their break markers
// and playheads. Paired devices and followed teams are added to the ones
// here, so the device doing the restore stays signed in. Ids the backup
// used are kept where they are free and moved where they are not, and
// everything that points at them follows.
func (s *Store) RestoreFrom(ctx context.Context, path string) error {
	defer s.forgetSettings()
	// ATTACH holds for one connection, and it cannot run inside a transaction.
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	escaped := strings.ReplaceAll(path, "'", "''")
	if _, err := conn.ExecContext(ctx, fmt.Sprintf(`ATTACH DATABASE '%s' AS incoming`, escaped)); err != nil {
		return err
	}
	defer func() { _, _ = conn.ExecContext(context.Background(), `DETACH DATABASE incoming`) }()
	for _, q := range []string{
		`CREATE TEMP TABLE IF NOT EXISTS restore_channels (old INTEGER PRIMARY KEY, new INTEGER NOT NULL)`,
		`CREATE TEMP TABLE IF NOT EXISTS restore_recordings (old INTEGER PRIMARY KEY, new INTEGER NOT NULL, added INTEGER NOT NULL DEFAULT 0)`,
	} {
		if _, err := conn.ExecContext(ctx, q); err != nil {
			return err
		}
	}
	defer func() {
		_, _ = conn.ExecContext(context.Background(), `DROP TABLE IF EXISTS temp.restore_channels`)
		_, _ = conn.ExecContext(context.Background(), `DROP TABLE IF EXISTS temp.restore_recordings`)
	}()
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for _, q := range []string{`DELETE FROM temp.restore_channels`, `DELETE FROM temp.restore_recordings`} {
		if _, err := tx.ExecContext(ctx, q); err != nil {
			return err
		}
	}
	for _, table := range restoreTables {
		if err := replaceTable(ctx, tx, table); err != nil {
			return fmt.Errorf("%s: %w", table, err)
		}
	}
	// Channels point at their tuner, so a tuner this server has not found yet comes first.
	if err := mergeTable(ctx, tx, "devices", false); err != nil {
		return fmt.Errorf("devices: %w", err)
	}
	if err := restoreChannels(ctx, tx); err != nil {
		return fmt.Errorf("channels: %w", err)
	}
	if err := restoreRecordings(ctx, tx); err != nil {
		return fmt.Errorf("recordings: %w", err)
	}
	if err := restorePerRecording(ctx, tx); err != nil {
		return err
	}
	if err := mergeTable(ctx, tx, "client_devices", false); err != nil {
		return fmt.Errorf("client_devices: %w", err)
	}
	if err := mergeTable(ctx, tx, "team_follows", true); err != nil {
		return fmt.Errorf("team_follows: %w", err)
	}
	return tx.Commit()
}

func replaceTable(ctx context.Context, tx *sql.Tx, table string) error {
	cols, err := sharedColumns(ctx, tx, table)
	if err != nil || len(cols) == 0 {
		// No columns: the backup predates this table; keep what is here.
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM `+table); err != nil {
		return err
	}
	list := strings.Join(cols, ", ")
	_, err = tx.ExecContext(ctx, `INSERT INTO `+table+` (`+list+`) SELECT `+list+` FROM incoming.`+table)
	return err
}

// mergeTable adds the backup's rows that are not here. withoutID drops the
// id column, for a table whose rows are told apart by another unique key.
func mergeTable(ctx context.Context, tx *sql.Tx, table string, withoutID bool) error {
	cols, err := sharedColumns(ctx, tx, table)
	if err != nil || len(cols) == 0 {
		return err
	}
	if withoutID {
		cols = slices.DeleteFunc(cols, func(c string) bool { return c == `"id"` })
	}
	list := strings.Join(cols, ", ")
	_, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO `+table+` (`+list+`) SELECT `+list+` FROM incoming.`+table)
	return err
}

// restoreChannels fills restore_channels with each backup channel's id here.
func restoreChannels(ctx context.Context, tx *sql.Tx) error {
	cols, err := sharedColumns(ctx, tx, "channels")
	if err != nil || len(cols) == 0 {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO temp.restore_channels (old, new)
SELECT i.id, c.id FROM incoming.channels i
JOIN main.channels c ON c.device_id = i.device_id AND c.guide_number = i.guide_number`); err != nil {
		return err
	}
	var sets []string
	for _, col := range channelChoices {
		if slices.Contains(cols, `"`+col+`"`) {
			sets = append(sets, col+` = i.`+col)
		}
	}
	if len(sets) > 0 {
		if _, err := tx.ExecContext(ctx, `
UPDATE main.channels SET `+strings.Join(sets, ", ")+`
FROM incoming.channels i
WHERE i.device_id = channels.device_id AND i.guide_number = channels.guide_number`); err != nil {
			return err
		}
	}
	missing, err := idsWhere(ctx, tx, `
SELECT i.id FROM incoming.channels i
WHERE i.id NOT IN (SELECT old FROM temp.restore_channels)
	AND i.device_id IN (SELECT device_id FROM main.devices)`)
	if err != nil {
		return err
	}
	// The tuner lists it again on its next refresh; until then it is not on the guide.
	return insertMoved(ctx, tx, "channels", cols, missing, "restore_channels", map[string]string{`"present"`: "0"})
}

// restoreRecordings fills restore_recordings with each backup recording's id here.
func restoreRecordings(ctx context.Context, tx *sql.Tx) error {
	cols, err := sharedColumns(ctx, tx, "recordings")
	if err != nil || len(cols) == 0 {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO temp.restore_recordings (old, new)
SELECT i.id, MIN(r.id) FROM incoming.recordings i
JOIN main.recordings r ON r.path = i.path AND i.path != ''
GROUP BY i.id`); err != nil {
		return err
	}
	missing, err := idsWhere(ctx, tx, `SELECT id FROM incoming.recordings WHERE id NOT IN (SELECT old FROM temp.restore_recordings)`)
	if err != nil {
		return err
	}
	// A recording that was running when the backup was taken stopped there.
	over := map[string]string{
		`"status"`: `CASE WHEN status = 'recording' THEN 'interrupted' ELSE status END`,
		// A channel the backup has no row for is not pointed at another channel here.
		`"channel_id"`: `CASE WHEN channel_id = 0 THEN 0 ELSE COALESCE((SELECT new FROM temp.restore_channels WHERE old = channel_id), 0) END`,
	}
	if err := insertMoved(ctx, tx, "recordings", cols, missing, "restore_recordings", over); err != nil {
		return err
	}
	for _, id := range missing {
		if _, err := tx.ExecContext(ctx, `UPDATE temp.restore_recordings SET added = 1 WHERE old = ?`, id); err != nil {
			return err
		}
	}
	return nil
}

// restorePerRecording points what the backup kept per recording and channel
// at the ids here. A recording the catalog still had keeps its own markers
// and playhead; one that came back gets the backup's.
func restorePerRecording(ctx context.Context, tx *sql.Tx) error {
	steps := []struct{ table, q string }{
		{"passes", `UPDATE passes SET channel_id = (SELECT new FROM temp.restore_channels WHERE old = passes.channel_id)
			WHERE channel_id IN (SELECT old FROM temp.restore_channels)`},
		{"virtual_items", `UPDATE virtual_items SET recording_id = (SELECT new FROM temp.restore_recordings WHERE old = virtual_items.recording_id)
			WHERE recording_id IN (SELECT old FROM temp.restore_recordings)`},
	}
	for _, step := range steps {
		cols, err := sharedColumns(ctx, tx, step.table)
		if err != nil {
			return fmt.Errorf("%s: %w", step.table, err)
		}
		if len(cols) == 0 {
			continue
		}
		if _, err := tx.ExecContext(ctx, step.q); err != nil {
			return fmt.Errorf("%s: %w", step.table, err)
		}
	}
	for _, table := range []string{"markers", "progress"} {
		cols, err := sharedColumns(ctx, tx, table)
		if err != nil {
			return fmt.Errorf("%s: %w", table, err)
		}
		if len(cols) == 0 {
			continue
		}
		cols = slices.DeleteFunc(cols, func(c string) bool { return c == `"id"` })
		pick := make([]string, len(cols))
		for i, c := range cols {
			pick[i] = "i." + c
			if c == `"recording_id"` {
				pick[i] = "m.new"
			}
		}
		if _, err := tx.ExecContext(ctx, `INSERT OR REPLACE INTO `+table+` (`+strings.Join(cols, ", ")+`)
SELECT `+strings.Join(pick, ", ")+` FROM incoming.`+table+` i JOIN temp.restore_recordings m ON m.old = i.recording_id AND m.added = 1`); err != nil {
			return fmt.Errorf("%s: %w", table, err)
		}
	}
	return nil
}

// insertMoved copies the backup rows with these ids, under the same id when
// it is free here and a new one when it is not, and notes each in mapTable.
// over replaces a column's value with an expression over the backup row.
func insertMoved(ctx context.Context, tx *sql.Tx, table string, cols []string, ids []int64, mapTable string, over map[string]string) error {
	if len(ids) == 0 {
		return nil
	}
	pick := make([]string, len(cols))
	var noID, noIDPick []string
	for i, c := range cols {
		pick[i] = c
		if expr, ok := over[c]; ok {
			pick[i] = expr
		}
		if c != `"id"` {
			noID = append(noID, c)
			noIDPick = append(noIDPick, pick[i])
		}
	}
	same := `INSERT INTO main.` + table + ` (` + strings.Join(cols, ", ") + `) SELECT ` + strings.Join(pick, ", ") + ` FROM incoming.` + table + ` WHERE id = ?`
	moved := `INSERT INTO main.` + table + ` (` + strings.Join(noID, ", ") + `) SELECT ` + strings.Join(noIDPick, ", ") + ` FROM incoming.` + table + ` WHERE id = ?`
	for _, id := range ids {
		var taken int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM main.`+table+` WHERE id = ?`, id).Scan(&taken); err != nil {
			return err
		}
		q := same
		if taken > 0 {
			q = moved
		}
		res, err := tx.ExecContext(ctx, q, id)
		if err != nil {
			return err
		}
		next, err := res.LastInsertId()
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO temp.`+mapTable+` (old, new) VALUES (?, ?)`, id, next); err != nil {
			return err
		}
	}
	return nil
}

func idsWhere(ctx context.Context, tx *sql.Tx, q string) ([]int64, error) {
	rows, err := tx.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// AddFoundRecording adds a finished recording found on disk, with when it ended.
func (s *Store) AddFoundRecording(ctx context.Context, rec Recording) (int64, error) {
	id, err := s.CreateRecording(ctx, rec)
	if err != nil {
		return 0, err
	}
	ended := ""
	if rec.EndedAt != nil {
		ended = rec.EndedAt.UTC().Format(time.RFC3339)
	}
	_, err = s.db.ExecContext(ctx, `UPDATE recordings SET ended_at = ?, partial = ? WHERE id = ?`, ended, boolInt(rec.Partial), id)
	return id, err
}

// HasRecordingsIn reports a recording the catalog keeps inside dir.
func (s *Store) HasRecordingsIn(ctx context.Context, dir string) (bool, error) {
	prefix := strings.TrimRight(dir, `/\`) + string(filepath.Separator)
	escaped := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(prefix)
	var found bool
	err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM recordings WHERE path LIKE ? ESCAPE '\')`, escaped+"%").Scan(&found)
	return found, err
}

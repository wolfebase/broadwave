package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"waveguide/internal/hdhr"

	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB

	// OnEvent is called after an activity event is saved, to push it to clients.
	OnEvent func(Event)
}

type Device struct {
	hdhr.Device
	Priority int    `json:"priority"`
	LastSeen string `json:"lastSeen"`
}

type Channel struct {
	ID            int64  `json:"id"`
	DeviceID      string `json:"deviceId"`
	GuideNumber   string `json:"guideNumber"`
	GuideName     string `json:"guideName"`
	DisplayNumber string `json:"displayNumber"`
	DisplayName   string `json:"displayName"`
	VideoCodec    string `json:"videoCodec,omitempty"`
	AudioCodec    string `json:"audioCodec,omitempty"`
	HD            bool   `json:"hd"`
	Favorite      bool   `json:"favorite"`
	Enabled       bool   `json:"enabled"`
	Hidden        bool   `json:"hidden"`
	Present       bool   `json:"present"`
}

type ChannelPatch struct {
	Favorite     *bool
	Enabled      *bool
	Hidden       *bool
	CustomName   *string
	CustomNumber *string
}

func Open(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, "waveguide.db")
	if err := adoptLegacyCatalog(dir, path); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path)+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)")
	if err != nil {
		return nil, err
	}
	if err := Migrate(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

// legacyCatalog is the database file name from before the rename to Waveguide.
const legacyCatalog = "ota-viewer.db"

// adoptLegacyCatalog renames an existing ota-viewer.db (and its WAL files) to the
// current name so upgrades keep every channel, pass, and recording.
func adoptLegacyCatalog(dir, path string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	old := filepath.Join(dir, legacyCatalog)
	if _, err := os.Stat(old); err != nil {
		return nil
	}
	for _, suffix := range []string{"-wal", "-shm", ""} {
		if err := os.Rename(old+suffix, path+suffix); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("adopt %s: %w", legacyCatalog, err)
		}
	}
	return nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) UpsertDevice(ctx context.Context, dev hdhr.Device, channels []hdhr.Channel) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	now := time.Now().UTC().Format(time.RFC3339)
	_, err = tx.ExecContext(ctx, `
INSERT INTO devices (
	device_id, friendly_name, model_number, firmware_name, firmware_version,
	upgrade_available, base_url, lineup_url, tuner_count, last_seen
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(device_id) DO UPDATE SET
	friendly_name=excluded.friendly_name,
	model_number=excluded.model_number,
	firmware_name=excluded.firmware_name,
	firmware_version=excluded.firmware_version,
	upgrade_available=excluded.upgrade_available,
	base_url=excluded.base_url,
	lineup_url=excluded.lineup_url,
	tuner_count=excluded.tuner_count,
	last_seen=excluded.last_seen
`, dev.DeviceID, dev.FriendlyName, dev.ModelNumber, dev.FirmwareName, dev.FirmwareVersion,
		dev.UpgradeAvailable, dev.BaseURL, dev.LineupURL, dev.TunerCount, now)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE channels SET present=0 WHERE device_id=?`, dev.DeviceID); err != nil {
		return err
	}
	for _, ch := range channels {
		_, err := tx.ExecContext(ctx, `
INSERT INTO channels (
	device_id, guide_number, guide_name, stream_url, video_codec, audio_codec, hd, favorite, present
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, 1)
ON CONFLICT(device_id, guide_number) DO UPDATE SET
	guide_name=excluded.guide_name,
	stream_url=excluded.stream_url,
	video_codec=excluded.video_codec,
	audio_codec=excluded.audio_codec,
	hd=excluded.hd,
	present=1
`, dev.DeviceID, ch.GuideNumber, ch.GuideName, ch.StreamURL, ch.VideoCodec, ch.AudioCodec, boolInt(ch.HD), boolInt(ch.Favorite))
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) Devices(ctx context.Context) ([]Device, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT device_id, friendly_name, model_number, firmware_name, firmware_version,
	upgrade_available, base_url, lineup_url, tuner_count, priority, last_seen
FROM devices ORDER BY priority, friendly_name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Device
	for rows.Next() {
		var d Device
		if err := rows.Scan(&d.DeviceID, &d.FriendlyName, &d.ModelNumber, &d.FirmwareName, &d.FirmwareVersion,
			&d.UpgradeAvailable, &d.BaseURL, &d.LineupURL, &d.TunerCount, &d.Priority, &d.LastSeen); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (s *Store) Channels(ctx context.Context, guideOnly bool) ([]Channel, error) {
	q := `SELECT id, device_id, guide_number, guide_name, custom_number, custom_name,
		video_codec, audio_codec, hd, favorite, enabled, hidden, present FROM channels`
	if guideOnly {
		q += ` WHERE present=1 AND enabled=1 AND hidden=0`
	}
	rows, err := s.db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Channel
	for rows.Next() {
		var ch Channel
		var customNumber, customName string
		var hd, fav, en, hidden, present int
		if err := rows.Scan(&ch.ID, &ch.DeviceID, &ch.GuideNumber, &ch.GuideName, &customNumber, &customName,
			&ch.VideoCodec, &ch.AudioCodec, &hd, &fav, &en, &hidden, &present); err != nil {
			return nil, err
		}
		ch.HD = hd != 0
		ch.Favorite = fav != 0
		ch.Enabled = en != 0
		ch.Hidden = hidden != 0
		ch.Present = present != 0
		ch.DisplayNumber = ch.GuideNumber
		if strings.TrimSpace(customNumber) != "" {
			ch.DisplayNumber = strings.TrimSpace(customNumber)
		}
		ch.DisplayName = ch.GuideName
		if strings.TrimSpace(customName) != "" {
			ch.DisplayName = strings.TrimSpace(customName)
		}
		out = append(out, ch)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool {
		cmp := hdhr.CompareGuide(out[i].DisplayNumber, out[j].DisplayNumber)
		if cmp == 0 {
			return out[i].ID < out[j].ID
		}
		return cmp < 0
	})
	return out, nil
}

func (s *Store) PatchChannel(ctx context.Context, id int64, patch ChannelPatch) (Channel, error) {
	sets := []string{}
	args := []any{}
	if patch.Favorite != nil {
		sets = append(sets, "favorite=?")
		args = append(args, boolInt(*patch.Favorite))
	}
	if patch.Enabled != nil {
		sets = append(sets, "enabled=?")
		args = append(args, boolInt(*patch.Enabled))
	}
	if patch.Hidden != nil {
		sets = append(sets, "hidden=?")
		args = append(args, boolInt(*patch.Hidden))
	}
	if patch.CustomName != nil {
		sets = append(sets, "custom_name=?")
		args = append(args, strings.TrimSpace(*patch.CustomName))
	}
	if patch.CustomNumber != nil {
		sets = append(sets, "custom_number=?")
		args = append(args, strings.TrimSpace(*patch.CustomNumber))
	}
	if len(sets) == 0 {
		return Channel{}, errors.New("no changes")
	}
	args = append(args, id)
	res, err := s.db.ExecContext(ctx, `UPDATE channels SET `+strings.Join(sets, ", ")+` WHERE id=?`, args...)
	if err != nil {
		return Channel{}, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return Channel{}, sql.ErrNoRows
	}
	channels, err := s.Channels(ctx, false)
	if err != nil {
		return Channel{}, err
	}
	for _, ch := range channels {
		if ch.ID == id {
			return ch, nil
		}
	}
	return Channel{}, sql.ErrNoRows
}

func (s *Store) Settings(ctx context.Context) (map[string]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT key, value FROM settings`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		out[k] = v
	}
	return out, rows.Err()
}

func (s *Store) PutSettings(ctx context.Context, values map[string]string) error {
	allowed := map[string]bool{
		"recordingsPath": true,
		"layout":         true,
		"profile":        true,
		"audio":          true,
		"encoder":        true,
		"watermarkGB":    true,
		"pictureMode":    true,
		"autoplay":       true,
		"hdhrEmulate":    true,
		"setupComplete":  true,
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for k, v := range values {
		if !allowed[k] {
			return fmt.Errorf("unknown setting %q", k)
		}
		if k == "layout" && v != "auto" && v != "desktop" && v != "tv" && v != "phone" {
			return fmt.Errorf("layout must be auto, desktop, tv, or phone")
		}
		if k == "pictureMode" && v != "broadcast" && v != "smooth" && v != "film" {
			return fmt.Errorf("pictureMode must be broadcast, smooth, or film")
		}
		if k == "watermarkGB" {
			n, err := strconv.Atoi(strings.TrimSpace(v))
			if err != nil || n < 0 || n > 1000000 {
				return fmt.Errorf("watermarkGB must be a whole number of gigabytes from 0 to 1000000")
			}
		}
		if _, err := tx.ExecContext(ctx, `
INSERT INTO settings (key, value) VALUES (?, ?)
ON CONFLICT(key) DO UPDATE SET value=excluded.value`, k, v); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

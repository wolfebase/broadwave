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

	"broadwave/internal/hdhr"

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
	GuideKey      string `json:"guideKey,omitempty"`
	ArtURL        string `json:"artUrl,omitempty"`
	ArtWidth      int    `json:"artWidth,omitempty"`
	ArtHeight     int    `json:"artHeight,omitempty"`
}

type ChannelPatch struct {
	Favorite     *bool
	Enabled      *bool
	Hidden       *bool
	CustomName   *string
	CustomNumber *string
	GuideKey     *string
}

func Open(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, "broadwave.db")
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
		protect := boolInt(ch.Protected)
		if ch.StreamURL != "" {
			res, err := tx.ExecContext(ctx, `
UPDATE channels SET guide_name=?, video_codec=?, audio_codec=?, hd=?, present=1,
	user_agent=CASE WHEN ?!='' THEN ? ELSE user_agent END,
	referrer=CASE WHEN ?!='' THEN ? ELSE referrer END,
	hidden=CASE WHEN ?=1 THEN 1 ELSE hidden END
WHERE device_id=? AND stream_url=?`,
				ch.GuideName, ch.VideoCodec, ch.AudioCodec, boolInt(ch.HD),
				ch.UserAgent, ch.UserAgent, ch.Referrer, ch.Referrer,
				protect, dev.DeviceID, ch.StreamURL)
			if err != nil {
				return err
			}
			if n, _ := res.RowsAffected(); n > 0 {
				continue
			}
		}
		if ch.GuideKey != "" {
			res, err := tx.ExecContext(ctx, `
UPDATE channels SET guide_number=?, guide_name=?, stream_url=?, video_codec=?, audio_codec=?, hd=?, present=1,
	art_url=CASE WHEN ?!='' THEN ? ELSE art_url END,
	user_agent=CASE WHEN ?!='' THEN ? ELSE user_agent END,
	referrer=CASE WHEN ?!='' THEN ? ELSE referrer END,
	hidden=CASE WHEN ?=1 THEN 1 ELSE hidden END
WHERE device_id=? AND guide_key=?`,
				ch.GuideNumber, ch.GuideName, ch.StreamURL, ch.VideoCodec, ch.AudioCodec, boolInt(ch.HD),
				ch.ArtURL, ch.ArtURL, ch.UserAgent, ch.UserAgent, ch.Referrer, ch.Referrer,
				protect, dev.DeviceID, ch.GuideKey)
			if err != nil {
				return err
			}
			if n, _ := res.RowsAffected(); n > 0 {
				continue
			}
		}
		_, err := tx.ExecContext(ctx, `
INSERT INTO channels (
	device_id, guide_number, guide_name, stream_url, video_codec, audio_codec, hd, favorite, present, hidden, guide_key, art_url, user_agent, referrer
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, 1, ?, ?, ?, ?, ?)
ON CONFLICT(device_id, guide_number) DO UPDATE SET
	guide_name=excluded.guide_name,
	stream_url=excluded.stream_url,
	video_codec=excluded.video_codec,
	audio_codec=excluded.audio_codec,
	hd=excluded.hd,
	present=1,
	hidden=CASE WHEN excluded.hidden=1 THEN 1 ELSE channels.hidden END,
	guide_key=CASE WHEN excluded.guide_key!='' THEN excluded.guide_key ELSE channels.guide_key END,
	art_url=CASE WHEN excluded.art_url!='' THEN excluded.art_url ELSE channels.art_url END,
	user_agent=CASE WHEN excluded.user_agent!='' THEN excluded.user_agent ELSE channels.user_agent END,
	referrer=CASE WHEN excluded.referrer!='' THEN excluded.referrer ELSE channels.referrer END
`, dev.DeviceID, ch.GuideNumber, ch.GuideName, ch.StreamURL, ch.VideoCodec, ch.AudioCodec, boolInt(ch.HD), boolInt(ch.Favorite), protect, ch.GuideKey, ch.ArtURL, ch.UserAgent, ch.Referrer)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

// OtherDevices lists base URLs for other tuners that already have this channel number, lowest priority first.
func (s *Store) OtherDevices(ctx context.Context, guideNumber, exceptDevice string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT d.base_url FROM devices d
JOIN channels c ON c.device_id = d.device_id
WHERE c.guide_number = ? AND c.present = 1 AND c.hidden = 0 AND d.device_id != ?
ORDER BY d.priority, d.device_id`, guideNumber, exceptDevice)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var base string
		if err := rows.Scan(&base); err != nil {
			return nil, err
		}
		if base != "" {
			out = append(out, base)
		}
	}
	return out, rows.Err()
}

// AlternateChannels lists other present channels with this guide number, lowest device priority first.
func (s *Store) AlternateChannels(ctx context.Context, guideNumber string, exceptID int64) ([]int64, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT c.id FROM channels c
JOIN devices d ON d.device_id = c.device_id
WHERE c.guide_number = ? AND c.id != ? AND c.present = 1 AND c.hidden = 0 AND c.stream_url != ''
ORDER BY d.priority, c.id`, guideNumber, exceptID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
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
		video_codec, audio_codec, hd, favorite, enabled, hidden, present, guide_key, art_url, art_width, art_height FROM channels`
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
			&ch.VideoCodec, &ch.AudioCodec, &hd, &fav, &en, &hidden, &present, &ch.GuideKey, &ch.ArtURL, &ch.ArtWidth, &ch.ArtHeight); err != nil {
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
	if patch.GuideKey != nil {
		sets = append(sets, "guide_key=?")
		args = append(args, strings.TrimSpace(*patch.GuideKey))
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
		"hideScores":     true,
		"setupComplete":  true,
		"sdUser":         true,
		"sdPassword":     true,
		"sdLineup":       true,
		"guideUrl":       true,
		"sdPasswordSet":  true,
		"tmdbKey":        true,
		"tmdbKeySet":     true,
	}
	cleaned := map[string]string{}
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
		if k == "hideScores" && v != "0" && v != "1" {
			return fmt.Errorf("hideScores must be 0 or 1")
		}
		if k == "guideUrl" {
			v = strings.TrimSpace(v)
			if v != "" && !strings.HasPrefix(v, "https://") && !strings.HasPrefix(v, "http://") {
				return fmt.Errorf("the guide address needs to start with http")
			}
		}
		if k == "sdPassword" && strings.TrimSpace(v) == "" {
			continue
		}
		if k == "sdPasswordSet" || k == "tmdbKeySet" {
			continue
		}
		if k == "tmdbKey" && strings.TrimSpace(v) == "" {
			continue
		}
		cleaned[k] = v
	}
	return s.writeSettings(ctx, cleaned)
}

func (s *Store) writeSettings(ctx context.Context, values map[string]string) error {
	if len(values) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for k, v := range values {
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

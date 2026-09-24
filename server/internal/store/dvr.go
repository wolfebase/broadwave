package store

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"strings"
	"time"
)

type SourceChannel struct {
	Channel
	StreamURL   string `json:"-"`
	BaseURL     string `json:"-"`
	TunerCount  int    `json:"tunerCount"`
	ModelNumber string `json:"modelNumber,omitempty"`
	FrequencyHz int    `json:"frequencyHz"`
	ProgramNum  int    `json:"programNum"`
	// FieldOrder is what a probe saw: progressive, tt, bb, tb, bt, or empty when unknown.
	FieldOrder string `json:"fieldOrder,omitempty"`
}

type Airing struct {
	ID           int64     `json:"id"`
	ChannelID    int64     `json:"channelId"`
	Title        string    `json:"title"`
	Subtitle     string    `json:"subtitle,omitempty"`
	Description  string    `json:"description,omitempty"`
	Category     string    `json:"category,omitempty"`
	ProgramID    string    `json:"programId,omitempty"`
	New          bool      `json:"new,omitempty"`
	ImageURL     string    `json:"imageUrl,omitempty"`
	ImageWidth   int       `json:"imageWidth,omitempty"`
	ImageHeight  int       `json:"imageHeight,omitempty"`
	Season       int       `json:"season,omitempty"`
	Episode      int       `json:"episode,omitempty"`
	EpisodeLabel string    `json:"episodeLabel,omitempty"`
	OriginalAir  string    `json:"originalAir,omitempty"`
	SeriesID     string    `json:"seriesId,omitempty"`
	Live         bool      `json:"live,omitempty"`
	Premiere     bool      `json:"premiere,omitempty"`
	Finale       bool      `json:"finale,omitempty"`
	Rating       string    `json:"rating,omitempty"`
	Cast         string    `json:"cast,omitempty"`
	GameID       string    `json:"gameId,omitempty"`
	Start        time.Time `json:"start"`
	End          time.Time `json:"end"`
}

type Recording struct {
	ID          int64      `json:"id"`
	ChannelID   int64      `json:"channelId"`
	GuideNumber string     `json:"guideNumber"`
	Title       string     `json:"title"`
	Path        string     `json:"-"`
	Status      string     `json:"status"`
	Error       string     `json:"error,omitempty"`
	StartedAt   time.Time  `json:"startedAt"`
	EndsAt      *time.Time `json:"endsAt,omitempty"`
	EndedAt     *time.Time `json:"endedAt,omitempty"`
	Bytes       int64      `json:"bytes,omitempty"`
	Position    float64    `json:"position,omitempty"`
	Duration    float64    `json:"durationSec,omitempty"`
	Subtitle    string     `json:"subtitle,omitempty"`
	Description string     `json:"description,omitempty"`
	Category    string     `json:"category,omitempty"`
	ProgramID   string     `json:"programId,omitempty"`
	GameID      string     `json:"gameId,omitempty"`
	// Watched is 0 when inferred from the playhead, 1 when marked watched, 2 when marked unwatched.
	Watched int `json:"watched,omitempty"`
}

type Pass struct {
	ID          int64  `json:"id"`
	Title       string `json:"title"`
	ChannelID   int64  `json:"channelId,omitempty"`
	Kind        string `json:"kind"`
	PadBefore   int    `json:"padBefore"`
	PadAfter    int    `json:"padAfter"`
	Priority    int    `json:"priority"`
	Episodes    string `json:"episodes,omitempty"`
	KeepMode    string `json:"keepMode,omitempty"`
	KeepCount   int    `json:"keepCount,omitempty"`
	LimitCount  int    `json:"limitCount,omitempty"`
	Rerecord    bool   `json:"rerecord,omitempty"`
	Commercials bool   `json:"commercials"`
	TimeStart   string `json:"timeStart,omitempty"`
	TimeEnd     string `json:"timeEnd,omitempty"`
	MatchKind   string `json:"matchKind,omitempty"`
}

func (s *Store) SourceChannel(ctx context.Context, id int64) (SourceChannel, error) {
	var ch SourceChannel
	var hd, fav, en, hidden, present int
	var customNumber, customName string
	err := s.db.QueryRowContext(ctx, `
SELECT c.id, c.device_id, c.guide_number, c.guide_name, c.custom_number, c.custom_name,
	c.video_codec, c.audio_codec, c.hd, c.favorite, c.enabled, c.hidden, c.present,
	c.stream_url, c.frequency_hz, c.program_num, c.field_order, d.base_url, d.tuner_count, d.model_number
FROM channels c JOIN devices d ON d.device_id = c.device_id WHERE c.id = ?`, id).Scan(
		&ch.ID, &ch.DeviceID, &ch.GuideNumber, &ch.GuideName, &customNumber, &customName,
		&ch.VideoCodec, &ch.AudioCodec, &hd, &fav, &en, &hidden, &present,
		&ch.StreamURL, &ch.FrequencyHz, &ch.ProgramNum, &ch.FieldOrder, &ch.BaseURL, &ch.TunerCount, &ch.ModelNumber,
	)
	if err != nil {
		return ch, err
	}
	ch.HD, ch.Favorite, ch.Enabled, ch.Hidden, ch.Present = hd != 0, fav != 0, en != 0, hidden != 0, present != 0
	ch.DisplayNumber = ch.GuideNumber
	if strings.TrimSpace(customNumber) != "" {
		ch.DisplayNumber = strings.TrimSpace(customNumber)
	}
	ch.DisplayName = ch.GuideName
	if strings.TrimSpace(customName) != "" {
		ch.DisplayName = strings.TrimSpace(customName)
	}
	return ch, nil
}

func (s *Store) SetChannelArt(ctx context.Context, art map[int64]string) error {
	for id, raw := range art {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		if _, err := s.db.ExecContext(ctx, `UPDATE channels SET art_url = ? WHERE id = ?`, raw, id); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) Artwork(ctx context.Context, kind string, id int64) (rawURL, label string, width, height int, err error) {
	switch kind {
	case "channel":
		err = s.db.QueryRowContext(ctx, `SELECT art_url, guide_name, art_width, art_height FROM channels WHERE id = ?`, id).Scan(&rawURL, &label, &width, &height)
	case "airing":
		err = s.db.QueryRowContext(ctx, `SELECT image_url, category, image_width, image_height FROM airings WHERE id = ?`, id).Scan(&rawURL, &label, &width, &height)
	default:
		err = sql.ErrNoRows
	}
	return rawURL, label, width, height, err
}

// SetArtworkSize records a picture's real pixel size the first time it is measured.
func (s *Store) SetArtworkSize(ctx context.Context, kind string, id int64, width, height int) error {
	if id <= 0 || width <= 0 || height <= 0 {
		return nil
	}
	var q string
	switch kind {
	case "channel":
		q = `UPDATE channels SET art_width = ?, art_height = ? WHERE id = ? AND art_width = 0`
	case "airing":
		q = `UPDATE airings SET image_width = ?, image_height = ? WHERE id = ? AND image_width = 0`
	default:
		return nil
	}
	_, err := s.db.ExecContext(ctx, q, width, height, id)
	return err
}

func (s *Store) RememberProgram(ctx context.Context, deviceID, guide string, freq, program int) error {
	_, err := s.db.ExecContext(ctx, `
UPDATE channels SET frequency_hz = ?, program_num = ?
WHERE device_id = ? AND guide_number = ?`, freq, program, deviceID, guide)
	return err
}

func (s *Store) SetFieldOrder(ctx context.Context, channelID int64, order string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE channels SET field_order = ? WHERE id = ?`, order, channelID)
	return err
}

// FrameTarget is a channel sharing a frequency that is already tuned.
type FrameTarget struct {
	ID         int64
	ProgramNum int
}

func (s *Store) FrameTargets(ctx context.Context, deviceID string, freq int) ([]FrameTarget, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, program_num FROM channels
WHERE device_id = ? AND frequency_hz = ? AND present = 1 AND hidden = 0`, deviceID, freq)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []FrameTarget
	for rows.Next() {
		var t FrameTarget
		if err := rows.Scan(&t.ID, &t.ProgramNum); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	if out == nil {
		out = []FrameTarget{}
	}
	return out, rows.Err()
}

func (s *Store) ReplaceAirings(ctx context.Context, rows []Airing) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `DELETE FROM airings`); err != nil {
		return err
	}
	for _, row := range rows {
		if err := insertAiring(ctx, tx, row); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func insertAiring(ctx context.Context, tx *sql.Tx, row Airing) error {
	bit := func(v bool) int {
		if v {
			return 1
		}
		return 0
	}
	_, err := tx.ExecContext(ctx, `
INSERT INTO airings (channel_id, title, subtitle, description, category, starts_at, ends_at, program_id, is_new, image_url,
	image_width, image_height, season, episode, episode_label, original_air, series_id, is_live, is_premiere, is_finale, rating, cast_list, game_id)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		row.ChannelID, row.Title, row.Subtitle, row.Description, row.Category,
		row.Start.UTC().Format(time.RFC3339), row.End.UTC().Format(time.RFC3339), row.ProgramID, bit(row.New), row.ImageURL,
		row.ImageWidth, row.ImageHeight,
		row.Season, row.Episode, row.EpisodeLabel, row.OriginalAir, row.SeriesID, bit(row.Live), bit(row.Premiere), bit(row.Finale), row.Rating, row.Cast, row.GameID)
	return err
}

func (s *Store) Airings(ctx context.Context, from, to time.Time) ([]Airing, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, channel_id, title, subtitle, description, category, starts_at, ends_at, program_id, is_new, image_url,
	image_width, image_height, season, episode, episode_label, original_air, series_id, is_live, is_premiere, is_finale, rating, cast_list, game_id
FROM airings WHERE ends_at > ? AND starts_at < ? ORDER BY starts_at`,
		from.UTC().Format(time.RFC3339), to.UTC().Format(time.RFC3339))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Airing
	for rows.Next() {
		var row Airing
		var start, end string
		var isNew, isLive, isPremiere, isFinale int
		if err := rows.Scan(&row.ID, &row.ChannelID, &row.Title, &row.Subtitle, &row.Description, &row.Category, &start, &end, &row.ProgramID, &isNew, &row.ImageURL,
			&row.ImageWidth, &row.ImageHeight,
			&row.Season, &row.Episode, &row.EpisodeLabel, &row.OriginalAir, &row.SeriesID, &isLive, &isPremiere, &isFinale, &row.Rating, &row.Cast, &row.GameID); err != nil {
			return nil, err
		}
		row.New = isNew != 0
		row.Live = isLive != 0
		row.Premiere = isPremiere != 0
		row.Finale = isFinale != 0
		row.Start, _ = time.Parse(time.RFC3339, start)
		row.End, _ = time.Parse(time.RFC3339, end)
		out = append(out, row)
	}
	return out, rows.Err()
}

// SetAiringGames writes game ids for listings in the window and clears the rest of that window.
func (s *Store) SetAiringGames(ctx context.Context, from, to time.Time, ids map[int64]string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `UPDATE airings SET game_id = '' WHERE starts_at >= ? AND starts_at < ?`,
		from.UTC().Format(time.RFC3339), to.UTC().Format(time.RFC3339)); err != nil {
		return err
	}
	for id, gameID := range ids {
		if gameID == "" {
			continue
		}
		if _, err := tx.ExecContext(ctx, `UPDATE airings SET game_id = ? WHERE id = ?`, gameID, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// AiringGame is the scoreboard id of the listing on this channel now, if it is a game.
func (s *Store) AiringGame(ctx context.Context, channelID int64, title string, now time.Time) string {
	rows, err := s.Airings(ctx, now.Add(-6*time.Hour), now.Add(15*time.Minute))
	if err != nil {
		return ""
	}
	current := ""
	for _, row := range rows {
		if row.ChannelID != channelID || row.GameID == "" {
			continue
		}
		if title != "" && strings.EqualFold(row.Title, title) && row.End.After(now.Add(-time.Minute)) {
			return row.GameID
		}
		if !row.Start.After(now) && row.End.After(now) {
			current = row.GameID
		}
	}
	return current
}

func (s *Store) CreateRecording(ctx context.Context, rec Recording) (int64, error) {
	ends := ""
	if rec.EndsAt != nil {
		ends = rec.EndsAt.UTC().Format(time.RFC3339)
	}
	res, err := s.db.ExecContext(ctx, `
INSERT INTO recordings (channel_id, guide_number, title, path, status, started_at, ends_at, subtitle, description, category, program_id, watched, game_id)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		rec.ChannelID, rec.GuideNumber, rec.Title, rec.Path, rec.Status,
		rec.StartedAt.UTC().Format(time.RFC3339), ends, rec.Subtitle, rec.Description, rec.Category, rec.ProgramID, rec.Watched, rec.GameID)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) FinishRecording(ctx context.Context, id int64, status, errText string) error {
	_, err := s.db.ExecContext(ctx, `
UPDATE recordings SET status = ?, error = ?, ended_at = ? WHERE id = ?`,
		status, errText, time.Now().UTC().Format(time.RFC3339), id)
	return err
}

func (s *Store) SetRecordingEnd(ctx context.Context, id int64, ends time.Time) error {
	_, err := s.db.ExecContext(ctx, `UPDATE recordings SET ends_at = ? WHERE id = ?`, ends.UTC().Format(time.RFC3339), id)
	return err
}

func (s *Store) Recordings(ctx context.Context) ([]Recording, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, channel_id, guide_number, title, path, status, error, started_at, ends_at, ended_at, duration_sec,
	subtitle, description, category, program_id, watched, game_id
FROM recordings ORDER BY id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Recording
	for rows.Next() {
		var rec Recording
		var start, ends, ended string
		if err := rows.Scan(&rec.ID, &rec.ChannelID, &rec.GuideNumber, &rec.Title, &rec.Path, &rec.Status, &rec.Error, &start, &ends, &ended, &rec.Duration, &rec.Subtitle, &rec.Description, &rec.Category, &rec.ProgramID, &rec.Watched, &rec.GameID); err != nil {
			return nil, err
		}
		rec.StartedAt, _ = time.Parse(time.RFC3339, start)
		if ends != "" {
			t, _ := time.Parse(time.RFC3339, ends)
			rec.EndsAt = &t
		}
		if ended != "" {
			t, _ := time.Parse(time.RFC3339, ended)
			rec.EndedAt = &t
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}

func (s *Store) SetDuration(ctx context.Context, id int64, seconds float64) error {
	_, err := s.db.ExecContext(ctx, `UPDATE recordings SET duration_sec = ? WHERE id = ?`, seconds, id)
	return err
}

func EpisodeKey(programID, title, subtitle string, channelID int64) string {
	if strings.TrimSpace(programID) != "" {
		return "id:" + strings.TrimSpace(programID)
	}
	if strings.TrimSpace(subtitle) == "" {
		return ""
	}
	return fmt.Sprintf("ep:%s|%s|%d", strings.ToLower(strings.TrimSpace(title)), strings.ToLower(strings.TrimSpace(subtitle)), channelID)
}

func (r Recording) Played() bool {
	if r.Watched == 2 {
		return false
	}
	if r.Watched == 1 {
		return true
	}
	if r.Duration < 10 || r.Position < 1 {
		return false
	}
	return r.Position >= r.Duration-15 || (r.Duration > 0 && r.Position/r.Duration >= 0.9)
}

func (s *Store) DeleteRecording(ctx context.Context, id int64) error {
	if rec, err := s.Recording(ctx, id); err == nil {
		_ = s.RememberSeen(ctx, EpisodeKey(rec.ProgramID, rec.Title, rec.Subtitle, rec.ChannelID), true)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for _, query := range []string{
		`DELETE FROM markers WHERE recording_id = ?`,
		`DELETE FROM progress WHERE recording_id = ?`,
		`DELETE FROM virtual_items WHERE recording_id = ?`,
		`DELETE FROM recordings WHERE id = ?`,
	} {
		if _, err := tx.ExecContext(ctx, query, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) Recording(ctx context.Context, id int64) (Recording, error) {
	all, err := s.Recordings(ctx)
	if err != nil {
		return Recording{}, err
	}
	for _, rec := range all {
		if rec.ID == id {
			return rec, nil
		}
	}
	return Recording{}, sql.ErrNoRows
}

func (s *Store) AddPass(ctx context.Context, title string, channelID int64, padBefore, padAfter int) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO passes (title, channel_id, kind, pad_before, pad_after) VALUES (?, ?, 'series', ?, ?)`, title, channelID, padBefore, padAfter)
	return err
}

func (s *Store) UpdatePass(ctx context.Context, id int64, padBefore, padAfter, priority int) error {
	res, err := s.db.ExecContext(ctx, `UPDATE passes SET pad_before = ?, pad_after = ?, priority = ? WHERE id = ?`, padBefore, padAfter, priority, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) Passes(ctx context.Context) ([]Pass, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, title, channel_id, kind, pad_before, pad_after, priority,
	episodes, keep_mode, keep_count, limit_count, rerecord, commercials, time_start, time_end, match_kind
	FROM passes ORDER BY title`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Pass
	for rows.Next() {
		var p Pass
		var rerecord, commercials int
		if err := rows.Scan(&p.ID, &p.Title, &p.ChannelID, &p.Kind, &p.PadBefore, &p.PadAfter, &p.Priority,
			&p.Episodes, &p.KeepMode, &p.KeepCount, &p.LimitCount, &rerecord, &commercials, &p.TimeStart, &p.TimeEnd, &p.MatchKind); err != nil {
			return nil, err
		}
		p.Rerecord = rerecord != 0
		p.Commercials = commercials != 0
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) UpdatePassRules(ctx context.Context, p Pass) error {
	res, err := s.db.ExecContext(ctx, `UPDATE passes SET pad_before = ?, pad_after = ?, priority = ?, episodes = ?, keep_mode = ?,
		keep_count = ?, limit_count = ?, rerecord = ?, commercials = ?, time_start = ?, time_end = ?, match_kind = ?, channel_id = ?
		WHERE id = ?`,
		p.PadBefore, p.PadAfter, p.Priority, blank(p.Episodes, "all"), blank(p.KeepMode, "all"),
		p.KeepCount, p.LimitCount, boolInt(p.Rerecord), boolInt(p.Commercials), p.TimeStart, p.TimeEnd, blank(p.MatchKind, "title"), p.ChannelID, p.ID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) SetWatched(ctx context.Context, id int64, watched int) error {
	if watched < 0 || watched > 2 {
		watched = 0
	}
	res, err := s.db.ExecContext(ctx, `UPDATE recordings SET watched = ? WHERE id = ?`, watched, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) RememberSeen(ctx context.Context, key string, deleted bool) error {
	if key == "" {
		return nil
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO seen_programs (program_key, deleted) VALUES (?, ?)
		ON CONFLICT(program_key) DO UPDATE SET deleted = excluded.deleted`, key, boolInt(deleted))
	return err
}

func (s *Store) SeenDeleted(ctx context.Context) (map[string]bool, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT program_key, deleted FROM seen_programs`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var key string
		var deleted int
		if err := rows.Scan(&key, &deleted); err != nil {
			return nil, err
		}
		out[key] = deleted != 0
	}
	return out, rows.Err()
}

func (s *Store) SkipAiring(ctx context.Context, key, starts string) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO skipped_airings (program_key, starts_at) VALUES (?, ?) ON CONFLICT DO NOTHING`, key, starts)
	return err
}

func (s *Store) Skips(ctx context.Context) (map[string]bool, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT program_key, starts_at FROM skipped_airings`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var key, starts string
		if err := rows.Scan(&key, &starts); err != nil {
			return nil, err
		}
		out[key+"|"+starts] = true
	}
	return out, rows.Err()
}

type Source struct {
	ID           int64  `json:"id"`
	Kind         string `json:"kind"`
	Name         string `json:"name"`
	URL          string `json:"url,omitempty"`
	XMLTV        string `json:"xmltvUrl,omitempty"`
	Enabled      bool   `json:"enabled"`
	StableKey    string `json:"stableKey,omitempty"`
	Priority     int    `json:"priority,omitempty"`
	TunerCount   int    `json:"tunerCount,omitempty"`
	StreamLimit  int    `json:"streamLimit,omitempty"`
	StreamFormat string `json:"streamFormat,omitempty"`
	HasGuide     bool   `json:"hasGuide,omitempty"`
	NeedsTuner   bool   `json:"needsTuner,omitempty"`
	Refresh      string `json:"refresh,omitempty"`
	Health       string `json:"health,omitempty"`
	DeviceID     string `json:"deviceId,omitempty"`
}

// maskURL returns a URL safe to show and the original when it carried a secret.
func maskURL(raw string) (public, secret string) {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return raw, ""
	}
	changed := false
	if u.User != nil {
		if pass, ok := u.User.Password(); ok && pass != "" {
			u.User = url.UserPassword(u.User.Username(), "••••")
			changed = true
		}
	}
	q := u.Query()
	for _, key := range []string{"password", "pass", "token", "secret"} {
		if q.Get(key) != "" {
			q.Set(key, "••••")
			changed = true
		}
	}
	if !changed {
		return raw, ""
	}
	u.RawQuery = q.Encode()
	return u.String(), raw
}

func (s *Store) AddSource(ctx context.Context, kind, name, rawURL, xmltv string) (Source, error) {
	public, secret := maskURL(rawURL)
	publicXML, xmlSecret := maskURL(xmltv)
	res, err := s.db.ExecContext(ctx, `INSERT INTO sources (kind, name, url, xmltv_url, enabled) VALUES (?, ?, ?, ?, 1)`, kind, name, public, publicXML)
	if err != nil {
		return Source{}, err
	}
	id, _ := res.LastInsertId()
	key := fmt.Sprintf("src:%d", id)
	if _, err := s.db.ExecContext(ctx, `UPDATE sources SET stable_key=? WHERE id=? AND stable_key=''`, key, id); err != nil {
		return Source{}, err
	}
	kept := secret
	if kept == "" {
		kept = xmlSecret
	}
	if kept != "" {
		if _, err := s.db.ExecContext(ctx, `INSERT INTO source_secrets (source_id, secret) VALUES (?, ?)`, id, kept); err != nil {
			return Source{}, err
		}
	}
	return Source{ID: id, Kind: kind, Name: name, URL: public, XMLTV: publicXML, Enabled: true, StableKey: key}, nil
}

func (s *Store) Sources(ctx context.Context) ([]Source, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, kind, name, url, xmltv_url, enabled, stable_key, priority, tuner_count, stream_limit, stream_format, has_guide, needs_tuner, refresh, health, device_id FROM sources ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Source
	for rows.Next() {
		var item Source
		var enabled, hasGuide, needsTuner int
		if err := rows.Scan(&item.ID, &item.Kind, &item.Name, &item.URL, &item.XMLTV, &enabled, &item.StableKey, &item.Priority, &item.TunerCount, &item.StreamLimit, &item.StreamFormat, &hasGuide, &needsTuner, &item.Refresh, &item.Health, &item.DeviceID); err != nil {
			return nil, err
		}
		item.Enabled = enabled != 0
		item.HasGuide = hasGuide != 0
		item.NeedsTuner = needsTuner != 0
		out = append(out, item)
	}
	if out == nil {
		out = []Source{}
	}
	return out, rows.Err()
}

func (s *Store) ReplaceAiringsFor(ctx context.Context, channelIDs []int64, rows []Airing) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for _, id := range channelIDs {
		if _, err := tx.ExecContext(ctx, `DELETE FROM airings WHERE channel_id = ?`, id); err != nil {
			return err
		}
	}
	for _, row := range rows {
		if err := insertAiring(ctx, tx, row); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func blank(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

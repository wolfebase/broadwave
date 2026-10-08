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
	StreamURL    string `json:"-"`
	BaseURL      string `json:"-"`
	TunerCount   int    `json:"tunerCount"`
	ModelNumber  string `json:"modelNumber,omitempty"`
	StreamLimit  int    `json:"streamLimit,omitempty"`
	StreamFormat string `json:"streamFormat,omitempty"`
	UserAgent    string `json:"-"`
	Referrer     string `json:"-"`
	FrequencyHz  int    `json:"frequencyHz"`
	ProgramNum   int    `json:"programNum"`
	// FieldOrder is the station scan: progressive, tt, bb, tb, bt, or empty.
	// Film cadence is decided per tune and is not stored.
	FieldOrder string `json:"fieldOrder,omitempty"`
	// AudioTracks is the last tune's measured PMT audio, as JSON.
	AudioTracks string `json:"-"`
	// PictureHeight is the last tune's picture height, 0 before one.
	PictureHeight int `json:"-"`
	// LongGroups is set once a tune has seen the station send groups of
	// pictures longer than a second and a half.
	LongGroups bool `json:"-"`
	// ATSC3 is set when this channel cannot use a 1.0 tuner.
	// The lineup's codecs are the other way a row is marked. A guide number is not.
	ATSC3 bool `json:"-"`
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
	GuideSource  string    `json:"guideSource,omitempty"`
	Start        time.Time `json:"start"`
	End          time.Time `json:"end"`
	// Simulcast is the channel that records this broadcast when it also airs
	// there (an ATSC 1.0 and 3.0 pair). Only RecordingAirings sets it.
	Simulcast int64 `json:"-"`
}

// RecordingHealth is the damage counted in a finished recording file.
// Zeros mean the read finished and the file was clean.
type RecordingHealth struct {
	ContinuityErrors int64 `json:"continuityErrors"`
	TransportErrors  int64 `json:"transportErrors"`
	SyncLosses       int64 `json:"syncLosses"`
	Packets          int64 `json:"packets"`
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
	// PassID is the pass that started the recording; 0 for one started by hand or found in a folder.
	PassID int64 `json:"passId,omitempty"`
	// Watched is 0 when inferred from the playhead, 1 when marked watched, 2 when marked unwatched.
	Watched int `json:"watched,omitempty"`
	// Health is counted from the file after the recording finishes.
	// Nil until then, including when the file disappeared during the read.
	Health *RecordingHealth `json:"health,omitempty"`
	// Missing is set by the list when a finished recording's file is gone.
	Missing bool `json:"missing,omitempty"`
	// Season, Episode, EpisodeLabel, and OriginalAir come from the listing the
	// recording covers, or an SxxEyy file name in a library folder.
	Season       int    `json:"season,omitempty"`
	Episode      int    `json:"episode,omitempty"`
	EpisodeLabel string `json:"episodeLabel,omitempty"`
	OriginalAir  string `json:"originalAir,omitempty"`
	// ProgressAt is when the playhead was last saved.
	ProgressAt *time.Time `json:"progressAt,omitempty"`
	// BreaksScanned is set once the server scanned the file for breaks.
	BreaksScanned bool `json:"-"`
	// IntroStart and IntroEnd are where the intro plays; CreditsStart is where
	// the end titles start, or the show ends. 0 when not found.
	IntroStart   float64 `json:"introStart,omitempty"`
	IntroEnd     float64 `json:"introEnd,omitempty"`
	CreditsStart float64 `json:"creditsStart,omitempty"`
	// Listened is set once the server printed the sound of its ends.
	Listened bool `json:"-"`
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
	// Days are the weekdays (0 Sunday … 6 Saturday) an airing may start on; none is every day.
	Days []int `json:"days,omitempty"`
	// AiringStart pins a kind "once" pass to the airing starting then on ChannelID.
	AiringStart time.Time `json:"airingStart,omitzero"`
}

// SourceChannel is the channel's stream. An encrypted 3.0 channel with a clear
// 1.0 twin is that twin's stream under its own id, with PlaysAs set.
func (s *Store) SourceChannel(ctx context.Context, id int64) (SourceChannel, error) {
	ch, err := s.sourceChannel(ctx, id)
	if err != nil || !ch.Protected {
		return ch, err
	}
	chs, err := s.Channels(ctx, false)
	if err != nil {
		return ch, err
	}
	one, ok := StandIns(chs)[id]
	if !ok {
		return ch, nil
	}
	twin, err := s.sourceChannel(ctx, one)
	if err != nil {
		return ch, nil
	}
	twin.ID = id
	twin.PlaysAs = one
	return twin, nil
}

func (s *Store) sourceChannel(ctx context.Context, id int64) (SourceChannel, error) {
	var ch SourceChannel
	var hd, fav, en, hidden, present, protected, long int
	var customNumber, customName string
	err := s.db.QueryRowContext(ctx, `
SELECT c.id, c.device_id, c.guide_number, c.guide_name, c.custom_number, c.custom_name,
	c.video_codec, c.audio_codec, c.hd, c.favorite, c.enabled, c.hidden, c.present, c.protected,
	c.stream_url, c.frequency_hz, c.program_num, c.field_order, c.audio_tracks, c.picture_height, c.long_groups, c.user_agent, c.referrer,
	d.base_url, d.tuner_count, d.model_number,
	COALESCE((SELECT stream_limit FROM sources WHERE device_id = c.device_id LIMIT 1), 0),
	COALESCE((SELECT stream_format FROM sources WHERE device_id = c.device_id LIMIT 1), '')
FROM channels c JOIN devices d ON d.device_id = c.device_id WHERE c.id = ?`, id).Scan(
		&ch.ID, &ch.DeviceID, &ch.GuideNumber, &ch.GuideName, &customNumber, &customName,
		&ch.VideoCodec, &ch.AudioCodec, &hd, &fav, &en, &hidden, &present, &protected,
		&ch.StreamURL, &ch.FrequencyHz, &ch.ProgramNum, &ch.FieldOrder, &ch.AudioTracks, &ch.PictureHeight, &long, &ch.UserAgent, &ch.Referrer,
		&ch.BaseURL, &ch.TunerCount, &ch.ModelNumber, &ch.StreamLimit, &ch.StreamFormat,
	)
	if err != nil {
		return ch, err
	}
	ch.HD, ch.Favorite, ch.Enabled, ch.Hidden, ch.Present = hd != 0, fav != 0, en != 0, hidden != 0, present != 0
	ch.Protected = protected != 0
	ch.LongGroups = long != 0
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

// SetChannelAudioTracks stores the audio a tune measured, as JSON.
func (s *Store) SetChannelAudioTracks(ctx context.Context, channelID int64, tracks string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE channels SET audio_tracks = ? WHERE id = ?`, tracks, channelID)
	return err
}

// SetChannelPictureHeight stores the picture height a tune read.
func (s *Store) SetChannelPictureHeight(ctx context.Context, channelID int64, height int) error {
	_, err := s.db.ExecContext(ctx, `UPDATE channels SET picture_height = ? WHERE id = ?`, height, channelID)
	return err
}

// SetChannelLongGroups records whether a tune found the station sending long
// groups of pictures.
func (s *Store) SetChannelLongGroups(ctx context.Context, channelID int64, long bool) error {
	_, err := s.db.ExecContext(ctx, `UPDATE channels SET long_groups = ? WHERE id = ?`, long, channelID)
	return err
}

// SetChannelCodecs stores codecs a tune read from the stream. An empty value
// keeps what is stored.
func (s *Store) SetChannelCodecs(ctx context.Context, channelID int64, video, audio string) error {
	if video == "" && audio == "" {
		return nil
	}
	_, err := s.db.ExecContext(ctx, `
UPDATE channels SET
	video_codec = CASE WHEN ? != '' THEN ? ELSE video_codec END,
	audio_codec = CASE WHEN ? != '' THEN ? ELSE audio_codec END
WHERE id = ?`, video, video, audio, audio, channelID)
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
	image_width, image_height, season, episode, episode_label, original_air, series_id, is_live, is_premiere, is_finale, rating, cast_list, game_id, guide_source)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		row.ChannelID, row.Title, row.Subtitle, row.Description, row.Category,
		row.Start.UTC().Format(time.RFC3339), row.End.UTC().Format(time.RFC3339), row.ProgramID, bit(row.New), row.ImageURL,
		row.ImageWidth, row.ImageHeight,
		row.Season, row.Episode, row.EpisodeLabel, row.OriginalAir, row.SeriesID, bit(row.Live), bit(row.Premiere), bit(row.Finale), row.Rating, row.Cast, row.GameID, row.GuideSource)
	return err
}

func (s *Store) Airings(ctx context.Context, from, to time.Time) ([]Airing, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, channel_id, title, subtitle, description, category, starts_at, ends_at, program_id, is_new, image_url,
	image_width, image_height, season, episode, episode_label, original_air, series_id, is_live, is_premiere, is_finale, rating, cast_list, game_id, guide_source
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
			&row.Season, &row.Episode, &row.EpisodeLabel, &row.OriginalAir, &row.SeriesID, &isLive, &isPremiere, &isFinale, &row.Rating, &row.Cast, &row.GameID, &row.GuideSource); err != nil {
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
	if rec.ChannelID != 0 && rec.Season == 0 && rec.Episode == 0 && rec.EpisodeLabel == "" && rec.OriginalAir == "" && !rec.StartedAt.IsZero() {
		to := rec.StartedAt.Add(time.Minute)
		if rec.EndsAt != nil && rec.EndsAt.After(to) {
			to = *rec.EndsAt
		}
		if airings, err := s.Airings(ctx, rec.StartedAt, to); err == nil {
			if best := CoveringAiring(rec, airings); best != nil {
				rec.Season, rec.Episode, rec.EpisodeLabel, rec.OriginalAir = best.Season, best.Episode, best.EpisodeLabel, best.OriginalAir
			}
		}
	}
	res, err := s.db.ExecContext(ctx, `
INSERT INTO recordings (channel_id, guide_number, title, path, status, started_at, ends_at, subtitle, description, category, program_id, watched, game_id, pass_id,
	season, episode, episode_label, original_air)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		rec.ChannelID, rec.GuideNumber, rec.Title, rec.Path, rec.Status,
		rec.StartedAt.UTC().Format(time.RFC3339), ends, rec.Subtitle, rec.Description, rec.Category, rec.ProgramID, rec.Watched, rec.GameID, rec.PassID,
		rec.Season, rec.Episode, rec.EpisodeLabel, rec.OriginalAir)
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

// SetRecordingHealth stores the damage counted in a finished file.
// A row that was deleted while the file was read is left alone.
func (s *Store) SetRecordingHealth(ctx context.Context, id, continuity, transport, syncLoss, packets int64) error {
	_, err := s.db.ExecContext(ctx, `
UPDATE recordings SET continuity_errors = ?, transport_errors = ?, sync_losses = ?, packets = ?
WHERE id = ?`, continuity, transport, syncLoss, packets, id)
	return err
}

func (s *Store) SetRecordingEnd(ctx context.Context, id int64, ends time.Time) error {
	_, err := s.db.ExecContext(ctx, `UPDATE recordings SET ends_at = ? WHERE id = ?`, ends.UTC().Format(time.RFC3339), id)
	return err
}

func (s *Store) Recordings(ctx context.Context) ([]Recording, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT r.id, r.channel_id, r.guide_number, r.title, r.path, r.status, r.error, r.started_at, r.ends_at, r.ended_at, r.duration_sec,
	r.subtitle, r.description, r.category, r.program_id, r.watched, r.game_id,
	r.continuity_errors, r.transport_errors, r.sync_losses, r.packets, r.pass_id,
	r.season, r.episode, r.episode_label, r.original_air, r.breaks_scanned,
	r.intro_start, r.intro_end, r.credits_start, EXISTS (SELECT 1 FROM episode_prints e WHERE e.recording_id = r.id),
	COALESCE(p.position_sec, 0), COALESCE(p.updated_at, '')
FROM recordings r LEFT JOIN progress p ON p.recording_id = r.id
ORDER BY r.id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Recording
	for rows.Next() {
		var rec Recording
		var start, ends, ended, played string
		var continuity, transport, syncLoss, packets sql.NullInt64
		if err := rows.Scan(&rec.ID, &rec.ChannelID, &rec.GuideNumber, &rec.Title, &rec.Path, &rec.Status, &rec.Error, &start, &ends, &ended, &rec.Duration, &rec.Subtitle, &rec.Description, &rec.Category, &rec.ProgramID, &rec.Watched, &rec.GameID, &continuity, &transport, &syncLoss, &packets, &rec.PassID,
			&rec.Season, &rec.Episode, &rec.EpisodeLabel, &rec.OriginalAir, &rec.BreaksScanned,
			&rec.IntroStart, &rec.IntroEnd, &rec.CreditsStart, &rec.Listened, &rec.Position, &played); err != nil {
			return nil, err
		}
		if played != "" && rec.Position > 0 {
			t, _ := time.Parse(time.RFC3339, played)
			rec.ProgressAt = &t
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
		if continuity.Valid {
			rec.Health = &RecordingHealth{
				ContinuityErrors: continuity.Int64,
				TransportErrors:  transport.Int64,
				SyncLosses:       syncLoss.Int64,
				Packets:          packets.Int64,
			}
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}

// CoveringAiring returns the listing on the recording's channel that it covers
// most. A padded recording starts before its show, so the start alone would land
// on the show before it. With program ids on both sides they must match;
// otherwise the titles must. Nil when nothing overlaps.
func CoveringAiring(rec Recording, airings []Airing) *Airing {
	if rec.StartedAt.IsZero() {
		return nil
	}
	end := rec.StartedAt.Add(time.Minute)
	if rec.EndedAt != nil && rec.EndedAt.After(rec.StartedAt) {
		end = *rec.EndedAt
	} else if rec.EndsAt != nil && rec.EndsAt.After(rec.StartedAt) {
		end = *rec.EndsAt
	}
	var best *Airing
	var most time.Duration
	for i := range airings {
		air := &airings[i]
		if rec.ChannelID != 0 && air.ChannelID != 0 && air.ChannelID != rec.ChannelID {
			continue
		}
		if rec.ProgramID != "" && air.ProgramID != "" && air.ProgramID != rec.ProgramID {
			continue
		}
		if (rec.ProgramID == "" || air.ProgramID == "") && rec.Title != "" && !strings.EqualFold(strings.TrimSpace(air.Title), strings.TrimSpace(rec.Title)) {
			continue
		}
		overlap := earlier(end, air.End).Sub(later(rec.StartedAt, air.Start))
		if overlap > most {
			best, most = air, overlap
		}
	}
	return best
}

func earlier(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}

func later(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}

// MarkBreaksScanned records that the server scanned a recording for breaks.
func (s *Store) MarkBreaksScanned(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `UPDATE recordings SET breaks_scanned = 1 WHERE id = ?`, id)
	return err
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
		`DELETE FROM episode_prints WHERE recording_id = ?`,
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

// AddSeriesPass saves a series pass with its rules and returns its id.
func (s *Store) AddSeriesPass(ctx context.Context, p Pass) (int64, error) {
	res, err := s.db.ExecContext(ctx, `INSERT INTO passes (title, channel_id, kind, pad_before, pad_after, priority, episodes, keep_mode,
		keep_count, limit_count, rerecord, commercials, time_start, time_end, match_kind, days)
		VALUES (?, ?, 'series', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.Title, p.ChannelID, p.PadBefore, p.PadAfter, p.Priority, blank(p.Episodes, "all"), blank(p.KeepMode, "all"),
		p.KeepCount, p.LimitCount, boolInt(p.Rerecord), boolInt(p.Commercials), p.TimeStart, p.TimeEnd, blank(p.MatchKind, "title"), DaysMask(p.Days))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// SetPassOrder ranks passes in the given order, first highest. Ranks start at 1
// so a new pass (0) lands below every ordered one.
func (s *Store) SetPassOrder(ctx context.Context, ids []int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for i, id := range ids {
		res, err := tx.ExecContext(ctx, `UPDATE passes SET priority = ? WHERE id = ?`, len(ids)-i, id)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return sql.ErrNoRows
		}
	}
	return tx.Commit()
}

// DaysMask packs weekdays into bits, Sunday lowest; out-of-range days are dropped.
func DaysMask(days []int) int {
	mask := 0
	for _, d := range days {
		if d >= 0 && d <= 6 {
			mask |= 1 << d
		}
	}
	return mask
}

// DaysOf lists the weekdays in a mask, in order.
func DaysOf(mask int) []int {
	var days []int
	for d := 0; d <= 6; d++ {
		if mask&(1<<d) != 0 {
			days = append(days, d)
		}
	}
	return days
}

// AddOncePass records the one airing starting at start on the channel. Asking
// twice for the same airing keeps one pass.
func (s *Store) AddOncePass(ctx context.Context, title string, channelID int64, start time.Time, padBefore, padAfter int) error {
	// Asked for by name, it ranks above every pass there is.
	_, err := s.db.ExecContext(ctx, `INSERT INTO passes (title, channel_id, kind, pad_before, pad_after, airing_start, priority)
		SELECT ?, ?, 'once', ?, ?, ?, (SELECT COALESCE(MAX(priority), 0) + 1 FROM passes)
		WHERE NOT EXISTS (SELECT 1 FROM passes WHERE kind = 'once' AND channel_id = ? AND airing_start = ?)`,
		title, channelID, padBefore, padAfter, start.Unix(), channelID, start.Unix())
	return err
}

// DeleteEndedOncePasses drops once passes whose airing started before cutoff.
func (s *Store) DeleteEndedOncePasses(ctx context.Context, cutoff time.Time) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM passes WHERE kind = 'once' AND airing_start < ?`, cutoff.Unix())
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
	episodes, keep_mode, keep_count, limit_count, rerecord, commercials, time_start, time_end, match_kind, airing_start, days
	FROM passes ORDER BY priority DESC, title COLLATE NOCASE, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Pass
	for rows.Next() {
		var p Pass
		var rerecord, commercials int
		var airingStart int64
		var days int
		if err := rows.Scan(&p.ID, &p.Title, &p.ChannelID, &p.Kind, &p.PadBefore, &p.PadAfter, &p.Priority,
			&p.Episodes, &p.KeepMode, &p.KeepCount, &p.LimitCount, &rerecord, &commercials, &p.TimeStart, &p.TimeEnd, &p.MatchKind, &airingStart, &days); err != nil {
			return nil, err
		}
		if airingStart > 0 {
			p.AiringStart = time.Unix(airingStart, 0).UTC()
		}
		p.Rerecord = rerecord != 0
		p.Commercials = commercials != 0
		p.Days = DaysOf(days)
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) UpdatePassRules(ctx context.Context, p Pass) error {
	res, err := s.db.ExecContext(ctx, `UPDATE passes SET title = ?, pad_before = ?, pad_after = ?, priority = ?, episodes = ?, keep_mode = ?,
		keep_count = ?, limit_count = ?, rerecord = ?, commercials = ?, time_start = ?, time_end = ?, match_kind = ?, channel_id = ?, days = ?
		WHERE id = ?`,
		p.Title, p.PadBefore, p.PadAfter, p.Priority, blank(p.Episodes, "all"), blank(p.KeepMode, "all"),
		p.KeepCount, p.LimitCount, boolInt(p.Rerecord), boolInt(p.Commercials), p.TimeStart, p.TimeEnd, blank(p.MatchKind, "title"), p.ChannelID, DaysMask(p.Days), p.ID)
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
	LastRefresh  string `json:"lastRefresh,omitempty"`
	Health       string `json:"health,omitempty"`
	StreamsInUse int    `json:"streamsInUse,omitempty"`
	DeviceID     string `json:"deviceId,omitempty"`
	Groups       string `json:"groups,omitempty"`
	NumberStart  int    `json:"start,omitempty"`
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

// MaskURL is the form of a URL that is safe to show.
// Passwords use the same encoding stored source URLs do.
func MaskURL(raw string) string {
	public, _ := maskURL(raw)
	return public
}

// CredentialValues lists stored logins so a support bundle can omit them.
// The result must not be written to a response, a log, or the database.
func (s *Store) CredentialValues(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT secret, guide_secret FROM source_secrets`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var secret, guide string
		if err := rows.Scan(&secret, &guide); err != nil {
			return nil, err
		}
		if secret != "" {
			out = append(out, secret)
		}
		if guide != "" {
			out = append(out, guide)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	settings, err := s.Settings(ctx)
	if err != nil {
		return nil, err
	}
	for _, key := range []string{"sdPassword", "tmdbKey", "guideUrl", "sportsdbKey"} {
		if v := strings.TrimSpace(settings[key]); v != "" {
			out = append(out, v)
		}
	}
	if out == nil {
		out = []string{}
	}
	return out, nil
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
	if _, err := s.db.ExecContext(ctx, `UPDATE sources SET stable_key=?, device_id=? WHERE id=? AND stable_key=''`, key, fmt.Sprintf("src-%d", id), id); err != nil {
		return Source{}, err
	}
	if secret != "" || xmlSecret != "" {
		if _, err := s.db.ExecContext(ctx, `INSERT INTO source_secrets (source_id, secret, guide_secret) VALUES (?, ?, ?)`, id, secret, xmlSecret); err != nil {
			return Source{}, err
		}
	}
	return Source{ID: id, Kind: kind, Name: name, URL: public, XMLTV: publicXML, Enabled: true, StableKey: key}, nil
}

func (s *Store) Sources(ctx context.Context) ([]Source, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, kind, name, url, xmltv_url, enabled, stable_key, priority, tuner_count, stream_limit, stream_format, has_guide, needs_tuner, refresh, last_refresh, health, device_id, groups, number_start FROM sources ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Source
	for rows.Next() {
		var item Source
		var enabled, hasGuide, needsTuner int
		if err := rows.Scan(&item.ID, &item.Kind, &item.Name, &item.URL, &item.XMLTV, &enabled, &item.StableKey, &item.Priority, &item.TunerCount, &item.StreamLimit, &item.StreamFormat, &hasGuide, &needsTuner, &item.Refresh, &item.LastRefresh, &item.Health, &item.DeviceID, &item.Groups, &item.NumberStart); err != nil {
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

// RememberPlaylist keeps the filter used at import and the next daily refresh.
func (s *Store) RememberPlaylist(ctx context.Context, id int64, groups string, start int, next time.Time) error {
	_, err := s.db.ExecContext(ctx, `UPDATE sources SET groups=?, number_start=?, refresh=? WHERE id=?`, groups, start, next.UTC().Format(time.RFC3339), id)
	return err
}

// NoteRefresh records when the playlist should be fetched again and the last result.
func (s *Store) NoteRefresh(ctx context.Context, id int64, next time.Time, health string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE sources SET refresh=?, last_refresh=?, health=? WHERE id=?`, next.UTC().Format(time.RFC3339), time.Now().UTC().Format(time.RFC3339), health, id)
	return err
}

// FetchURL returns the address to fetch for a stored playlist or guide URL,
// putting back the login that the public copy masks.
func (s *Store) FetchURL(ctx context.Context, id int64, public string) string {
	var secret, guide string
	if err := s.db.QueryRowContext(ctx, `SELECT secret, guide_secret FROM source_secrets WHERE source_id=?`, id).Scan(&secret, &guide); err != nil {
		return public
	}
	for _, candidate := range []string{secret, guide} {
		if candidate == "" {
			continue
		}
		if masked, _ := maskURL(candidate); masked == public {
			return candidate
		}
	}
	return guideFromLogin(public, secret)
}

// guideFromLogin fills an Xtream guide URL saved before guides kept their own
// secret, using the password from the playlist login.
func guideFromLogin(public, secret string) string {
	login, err := url.Parse(secret)
	if err != nil || login.User == nil {
		return public
	}
	pass, ok := login.User.Password()
	if !ok {
		return public
	}
	u, err := url.Parse(public)
	if err != nil {
		return public
	}
	q := u.Query()
	if q.Get("password") != "••••" || q.Get("username") != login.User.Username() {
		return public
	}
	q.Set("password", pass)
	u.RawQuery = q.Encode()
	return u.String()
}

func (s *Store) SetStreamFormat(ctx context.Context, id int64, format string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE sources SET stream_format=? WHERE id=?`, format, id)
	return err
}

// ReplaceAiringsFor swaps the listings of these channels for rows. Listings
// read from the broadcast stay where rows leave a gap, since they fill slots a
// guide feed does not cover.
func (s *Store) ReplaceAiringsFor(ctx context.Context, channelIDs []int64, rows []Airing) error {
	rows = uniqueAirings(rows)
	type span struct{ from, to time.Time }
	spans := map[int64]span{}
	for _, row := range rows {
		sp, ok := spans[row.ChannelID]
		if !ok || row.Start.Before(sp.from) {
			sp.from = row.Start
		}
		if !ok || row.End.After(sp.to) {
			sp.to = row.End
		}
		spans[row.ChannelID] = sp
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for _, id := range channelIDs {
		if _, err := tx.ExecContext(ctx, `DELETE FROM airings WHERE channel_id = ? AND guide_source <> 'broadcast'`, id); err != nil {
			return err
		}
		sp, ok := spans[id]
		if !ok {
			continue
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM airings WHERE channel_id = ? AND guide_source = 'broadcast' AND starts_at < ? AND ends_at > ?`,
			id, sp.to.UTC().Format(time.RFC3339), sp.from.UTC().Format(time.RFC3339)); err != nil {
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

// uniqueAirings keeps one row per channel and start, the one with the most detail.
func uniqueAirings(rows []Airing) []Airing {
	at := map[[2]int64]int{}
	out := make([]Airing, 0, len(rows))
	for _, row := range rows {
		key := [2]int64{row.ChannelID, row.Start.Unix()}
		if i, ok := at[key]; ok {
			if airingDetail(row) > airingDetail(out[i]) {
				out[i] = row
			}
			continue
		}
		at[key] = len(out)
		out = append(out, row)
	}
	return out
}

func airingDetail(row Airing) int {
	n := 0
	for _, v := range []string{row.Subtitle, row.Description, row.ImageURL, row.SeriesID, row.ProgramID, row.EpisodeLabel} {
		if v != "" {
			n++
		}
	}
	return n
}

// InsertAirings adds listings without removing what is already there.
func (s *Store) InsertAirings(ctx context.Context, rows []Airing) error {
	if len(rows) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for _, row := range rows {
		if err := insertAiring(ctx, tx, row); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// NoteGuideScan records when a frequency was last read for the broadcast guide.
func (s *Store) NoteGuideScan(ctx context.Context, freqHz int, at time.Time) error {
	if freqHz <= 0 {
		return nil
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO guide_scans (frequency_hz, scanned_at) VALUES (?, ?)
ON CONFLICT(frequency_hz) DO UPDATE SET scanned_at=excluded.scanned_at`, freqHz, at.UTC().Format(time.RFC3339))
	return err
}

// GuideScans returns the last time each frequency was scanned.
func (s *Store) GuideScans(ctx context.Context) (map[int]time.Time, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT frequency_hz, scanned_at FROM guide_scans`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int]time.Time{}
	for rows.Next() {
		var freq int
		var raw string
		if err := rows.Scan(&freq, &raw); err != nil {
			return nil, err
		}
		when, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			continue
		}
		out[freq] = when
	}
	return out, rows.Err()
}

func blank(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

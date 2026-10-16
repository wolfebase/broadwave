package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"broadwave/internal/hdhr"

	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
	// key is this server's secret, kept in a file beside the catalog and never
	// in the database, so a catalog backup does not carry it. It keys code
	// hashes and media links.
	key []byte

	// OnEvent is called after an activity event is saved, to push it to clients.
	OnEvent func(Event)

	settingsMu sync.Mutex
	settings   map[string]string
}

type Device struct {
	hdhr.Device
	Priority int    `json:"priority"`
	LastSeen string `json:"lastSeen"`
	// Retired is a tuner the owner took away on purpose. It counts toward no
	// plan and raises no note until it answers again.
	Retired bool `json:"retired,omitempty"`
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
	// Network is ABC, CBS, FOX, or NBC when a guide said so. Empty means the caller fills it from the call sign.
	Network string `json:"network,omitempty"`
	// Protected is a channel the tuner marks DRM or copy protected. It stays hidden.
	Protected bool `json:"protected,omitempty"`
	// Standard is "atsc3" for an ATSC 3.0 broadcast and empty for everything else.
	Standard string `json:"standard,omitempty"`
	// TwinID pairs an ATSC 3.0 channel with the 1.0 channel of the same station.
	TwinID int64 `json:"twinId,omitempty"`
	// TwinChoice is the pair's choice: atsc3, atsc1, or both.
	TwinChoice string `json:"twinChoice,omitempty"`
	// PlaysAs is the clear 1.0 channel that plays and records in place of an
	// encrypted 3.0 one.
	PlaysAs int64 `json:"playsAs,omitempty"`
	// SameAs is the row shown for this channel when another tuner carries it too.
	// This row stays off the guide and still plays.
	SameAs int64 `json:"sameAs,omitempty"`
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
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path)+"?"+dsnOptions)
	if err != nil {
		return nil, err
	}
	if err := Migrate(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	key, err := loadKey(dir)
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	// The planner's statistics, read once with a cap so a large catalog
	// does not hold startup. Close and Optimize keep them current.
	_, _ = db.Exec(`PRAGMA optimize=0x10002`)
	return &Store{db: db, key: key}, nil
}

// dsnOptions applies to every pooled connection.
//
// _txlock=immediate takes the write lock when a transaction begins. A
// deferred transaction that read first and then wrote failed at once with
// SQLITE_BUSY when another connection had written meanwhile, and
// busy_timeout does not cover that upgrade. With the lock taken up front,
// writers queue on busy_timeout instead.
//
// synchronous=NORMAL is safe in WAL: a power cut can lose the last commits
// but never corrupts the file. temp_store and cache_size keep sorts and the
// hot pages of the guide in memory.
const dsnOptions = "_txlock=immediate" +
	"&_pragma=busy_timeout(10000)" +
	"&_pragma=journal_mode(WAL)" +
	"&_pragma=foreign_keys(ON)" +
	"&_pragma=synchronous(NORMAL)" +
	"&_pragma=temp_store(MEMORY)" +
	"&_pragma=cache_size(-16000)"

// Close updates the planner's statistics and closes the catalog.
func (s *Store) Close() error {
	_, _ = s.db.Exec(`PRAGMA optimize`)
	return s.db.Close()
}

// Optimize lets SQLite refresh the statistics of tables whose shape
// changed. It is cheap when nothing did; the server runs it hourly.
func (s *Store) Optimize(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `PRAGMA optimize`)
	return err
}

func (s *Store) UpsertDevice(ctx context.Context, dev hdhr.Device, channels []hdhr.Channel) error {
	return s.upsertDevice(ctx, dev, channels, true)
}

// RefreshDevice is UpsertDevice for a device the catalog already has, as a
// tuner or as a source. One removed while its lineup loaded stays removed.
func (s *Store) RefreshDevice(ctx context.Context, dev hdhr.Device, channels []hdhr.Channel) error {
	return s.upsertDevice(ctx, dev, channels, false)
}

// lineupRow is one row of this device whose column matches value and that
// this refresh has not already given to another lineup entry. column is
// stream_url, guide_key, or guide_number.
func lineupRow(ctx context.Context, tx *sql.Tx, deviceID, column, value string, claimed map[int64]bool) (int64, error) {
	if value == "" {
		return 0, nil
	}
	switch column {
	case "stream_url", "guide_key", "guide_number":
	default:
		return 0, fmt.Errorf("channel match %s", column)
	}
	// Claimed rows are skipped here, not in SQL: a long playlist would pass
	// more ids than SQLite takes as parameters.
	rows, err := tx.QueryContext(ctx, `SELECT id FROM channels WHERE device_id=? AND `+column+`=? ORDER BY id`, deviceID, value)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return 0, err
		}
		if !claimed[id] {
			return id, nil
		}
	}
	return 0, rows.Err()
}

// claimChannel picks the existing row for one lineup entry: the stream, then
// the guide key, then the channel number. Zero means the entry is new.
func claimChannel(ctx context.Context, tx *sql.Tx, deviceID string, ch hdhr.Channel, claimed map[int64]bool) (int64, error) {
	if id, err := lineupRow(ctx, tx, deviceID, "stream_url", ch.StreamURL, claimed); id != 0 || err != nil {
		return id, err
	}
	if id, err := lineupRow(ctx, tx, deviceID, "guide_key", ch.GuideKey, claimed); id != 0 || err != nil {
		return id, err
	}
	return lineupRow(ctx, tx, deviceID, "guide_number", ch.GuideNumber, claimed)
}

// writeChannel copies one lineup entry onto a row. Favorite, custom name, and
// measured audio stay as the viewer left them.
func writeChannel(ctx context.Context, tx *sql.Tx, id int64, ch hdhr.Channel) error {
	protect := boolInt(ch.Protected)
	_, err := tx.ExecContext(ctx, `
UPDATE channels SET guide_number=?, guide_name=?, stream_url=?,
	guide_key=CASE WHEN ?!='' THEN ? ELSE guide_key END,
	art_url=CASE WHEN ?!='' THEN ? ELSE art_url END,
	video_codec=CASE WHEN ?!='' THEN ? ELSE video_codec END,
	audio_codec=CASE WHEN ?!='' THEN ? ELSE audio_codec END,
	hd=?, present=1,
	user_agent=CASE WHEN ?!='' THEN ? ELSE user_agent END,
	referrer=CASE WHEN ?!='' THEN ? ELSE referrer END,
	hidden=CASE WHEN ?=1 THEN 1 ELSE hidden END,
	protected=?
WHERE id=?`,
		ch.GuideNumber, ch.GuideName, ch.StreamURL,
		ch.GuideKey, ch.GuideKey, ch.ArtURL, ch.ArtURL,
		ch.VideoCodec, ch.VideoCodec, ch.AudioCodec, ch.AudioCodec, boolInt(ch.HD),
		ch.UserAgent, ch.UserAgent, ch.Referrer, ch.Referrer,
		protect, protect, id)
	return err
}

func (s *Store) upsertDevice(ctx context.Context, dev hdhr.Device, channels []hdhr.Channel, adopt bool) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if !adopt {
		var known int
		if err := tx.QueryRowContext(ctx, `
SELECT (SELECT COUNT(*) FROM devices WHERE device_id=?) + (SELECT COUNT(*) FROM sources WHERE device_id=?)`,
			dev.DeviceID, dev.DeviceID).Scan(&known); err != nil {
			return err
		}
		if known == 0 {
			return nil
		}
	}
	// The refresh parks and rewrites every row, so the change triggers count
	// it even when the lineup came back the same. Then the count goes back.
	var version int64
	if err := tx.QueryRowContext(ctx, `SELECT version FROM data_versions WHERE name='channels'`).Scan(&version); err != nil {
		return err
	}
	before, err := deviceSnapshot(ctx, tx, dev.DeviceID)
	if err != nil {
		return err
	}

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
	last_seen=excluded.last_seen,
	retired=0
`, dev.DeviceID, dev.FriendlyName, dev.ModelNumber, dev.FirmwareName, dev.FirmwareVersion,
		dev.UpgradeAvailable, dev.BaseURL, dev.LineupURL, dev.TunerCount, now)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE channels SET present=0 WHERE device_id=?`, dev.DeviceID); err != nil {
		return err
	}
	// One lineup entry claims one row. Matching every row with the stream URL
	// kept the old channel number and dropped a second entry that shares it.
	// Numbers are parked before they are written: two channels can swap numbers,
	// and the unique (device, number) constraint would reject the first write.
	type lineupSlot struct {
		id   int64
		ch   hdhr.Channel
		drop bool
	}
	claimed := map[int64]bool{}
	slots := make([]lineupSlot, 0, len(channels))
	byNum := map[string]int{}
	for _, ch := range channels {
		id, err := claimChannel(ctx, tx, dev.DeviceID, ch, claimed)
		if err != nil {
			return err
		}
		if prev, ok := byNum[ch.GuideNumber]; ok && !slots[prev].drop {
			old := &slots[prev]
			if id == 0 {
				id = old.id
			} else if old.id != 0 && old.id != id {
				delete(claimed, old.id)
			}
			old.drop = true
		}
		if id != 0 {
			claimed[id] = true
		}
		byNum[ch.GuideNumber] = len(slots)
		slots = append(slots, lineupSlot{id: id, ch: ch})
	}
	orig := map[int64]string{}
	rows, err := tx.QueryContext(ctx, `SELECT id, guide_number FROM channels WHERE device_id=?`, dev.DeviceID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var id int64
		var number string
		if err := rows.Scan(&id, &number); err != nil {
			rows.Close()
			return err
		}
		orig[id] = number
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	if _, err := tx.ExecContext(ctx, `UPDATE channels SET guide_number = char(1) || id WHERE device_id=?`, dev.DeviceID); err != nil {
		return err
	}
	used := map[string]bool{}
	for i := range slots {
		sl := &slots[i]
		if sl.drop {
			continue
		}
		if sl.id == 0 {
			protect := boolInt(sl.ch.Protected)
			res, err := tx.ExecContext(ctx, `
INSERT INTO channels (
	device_id, guide_number, guide_name, stream_url, video_codec, audio_codec, hd, favorite, present, hidden, protected, guide_key, art_url, user_agent, referrer
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, 1, ?, ?, ?, ?, ?, ?)`,
				dev.DeviceID, sl.ch.GuideNumber, sl.ch.GuideName, sl.ch.StreamURL, sl.ch.VideoCodec, sl.ch.AudioCodec,
				boolInt(sl.ch.HD), boolInt(sl.ch.Favorite), protect, protect, sl.ch.GuideKey, sl.ch.ArtURL, sl.ch.UserAgent, sl.ch.Referrer)
			if err != nil {
				return err
			}
			sl.id, err = res.LastInsertId()
			if err != nil {
				return err
			}
			claimed[sl.id] = true
			used[sl.ch.GuideNumber] = true
			continue
		}
		if err := writeChannel(ctx, tx, sl.id, sl.ch); err != nil {
			return err
		}
		used[sl.ch.GuideNumber] = true
	}
	// A channel that left the lineup keeps its number, so the next refresh
	// still finds the same row when only the address changed.
	// When another channel took that number, it gets a readable one that
	// cannot collide, since the settings list shows it.
	for id, number := range orig {
		if claimed[id] {
			continue
		}
		keep := number
		for n := 2; used[keep]; n++ {
			keep = fmt.Sprintf("%s (%d)", number, n)
		}
		if _, err := tx.ExecContext(ctx, `UPDATE channels SET guide_number=? WHERE id=?`, keep, id); err != nil {
			return err
		}
		used[keep] = true
	}
	after, err := deviceSnapshot(ctx, tx, dev.DeviceID)
	if err != nil {
		return err
	}
	if after == before {
		if _, err := tx.ExecContext(ctx, `UPDATE data_versions SET version=? WHERE name='channels'`, version); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return s.ApplyTwinDefaults(ctx)
}

// deviceSnapshot is what clients see of one device and its channels: the
// columns the data_versions triggers compare.
func deviceSnapshot(ctx context.Context, tx *sql.Tx, deviceID string) (string, error) {
	var b strings.Builder
	row := tx.QueryRowContext(ctx, `
SELECT friendly_name, model_number, firmware_version, base_url, tuner_count, priority, retired
FROM devices WHERE device_id=?`, deviceID)
	var name, model, firmware, base string
	var tuners, priority, retired int
	switch err := row.Scan(&name, &model, &firmware, &base, &tuners, &priority, &retired); {
	case errors.Is(err, sql.ErrNoRows):
	case err != nil:
		return "", err
	default:
		fmt.Fprintf(&b, "%q|%q|%q|%q|%d|%d|%d\n", name, model, firmware, base, tuners, priority, retired)
	}
	rows, err := tx.QueryContext(ctx, `
SELECT id, guide_number, guide_name, stream_url, video_codec, audio_codec, hd, favorite, enabled, hidden,
	custom_name, custom_number, present, guide_key, art_url, art_width, art_height, network, protected, twin_choice
FROM channels WHERE device_id=? ORDER BY id`, deviceID)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return "", err
	}
	values := make([]any, len(cols))
	ptrs := make([]any, len(cols))
	for i := range values {
		ptrs[i] = &values[i]
	}
	for rows.Next() {
		if err := rows.Scan(ptrs...); err != nil {
			return "", err
		}
		for _, v := range values {
			fmt.Fprintf(&b, "%v|", v)
		}
		b.WriteByte('\n')
	}
	return b.String(), rows.Err()
}

// OtherDevices lists base URLs for other tuners that already have this channel number, lowest priority first.
// A row hidden from the lineup still counts: hiding is a display choice, and a
// 1.0 channel with its 3.0 twin shown is stored hidden.
func (s *Store) OtherDevices(ctx context.Context, guideNumber, exceptDevice string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT d.base_url FROM devices d
JOIN channels c ON c.device_id = d.device_id
WHERE c.guide_number = ? AND c.present = 1 AND c.protected = 0 AND d.device_id != ?
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

// ChannelStreamURL is the stored stream for one guide number on one device origin.
func (s *Store) ChannelStreamURL(ctx context.Context, base, guide string) (string, error) {
	var raw string
	err := s.db.QueryRowContext(ctx, `
SELECT c.stream_url FROM channels c
JOIN devices d ON d.device_id = c.device_id
WHERE d.base_url = ? AND c.guide_number = ? AND c.present = 1 AND c.stream_url != ''
LIMIT 1`, base, guide).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return raw, err
}

// AlternateChannels lists other present channels with this guide number, lowest device priority first.
func (s *Store) AlternateChannels(ctx context.Context, guideNumber string, exceptID int64) ([]int64, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT c.id FROM channels c
JOIN devices d ON d.device_id = c.device_id
WHERE c.guide_number = ? AND c.id != ? AND c.present = 1 AND c.protected = 0 AND c.stream_url != ''
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
	upgrade_available, base_url, lineup_url, tuner_count, priority, last_seen, retired
FROM devices ORDER BY priority, friendly_name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Device
	for rows.Next() {
		var d Device
		if err := rows.Scan(&d.DeviceID, &d.FriendlyName, &d.ModelNumber, &d.FirmwareName, &d.FirmwareVersion,
			&d.UpgradeAvailable, &d.BaseURL, &d.LineupURL, &d.TunerCount, &d.Priority, &d.LastSeen, &d.Retired); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// SetDeviceRetired marks a tuner taken away on purpose, or brings it back.
// Its channels and recordings stay; the next time it answers, the mark clears.
func (s *Store) SetDeviceRetired(ctx context.Context, deviceID string, retired bool) error {
	res, err := s.db.ExecContext(ctx, `UPDATE devices SET retired = ? WHERE device_id = ?`, boolInt(retired), deviceID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNoDevice
	}
	return nil
}

// ErrNoDevice means no device has that id.
var ErrNoDevice = errors.New("no device with that id")

// RemoveDevice forgets a tuner or playlist and its channels. A channel another
// device also carries hands over its favorite, its custom name and number, and
// its passes; other passes go. Recordings stay.
func (s *Store) RemoveDevice(ctx context.Context, deviceID string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var found int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM devices WHERE device_id=?`, deviceID).Scan(&found); err != nil {
		return err
	}
	if found == 0 {
		return ErrNoDevice
	}
	type gone struct {
		id                               int64
		number, customName, customNumber string
		favorite                         int
	}
	rows, err := tx.QueryContext(ctx, `
SELECT id, guide_number, custom_name, custom_number, favorite FROM channels WHERE device_id=?`, deviceID)
	if err != nil {
		return err
	}
	var list []gone
	for rows.Next() {
		var g gone
		if err := rows.Scan(&g.id, &g.number, &g.customName, &g.customNumber, &g.favorite); err != nil {
			rows.Close()
			return err
		}
		list = append(list, g)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	var favorites []int64
	for _, g := range list {
		var next int64
		err := tx.QueryRowContext(ctx, `
SELECT c.id FROM channels c JOIN devices d ON d.device_id = c.device_id
WHERE c.guide_number = ? AND c.device_id != ? AND c.present = 1 AND c.protected = 0
ORDER BY d.priority, c.id LIMIT 1`, g.number, deviceID).Scan(&next)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
UPDATE channels SET
	favorite = CASE WHEN ?=1 THEN 1 ELSE favorite END,
	custom_name = CASE WHEN custom_name='' THEN ? ELSE custom_name END,
	custom_number = CASE WHEN custom_number='' THEN ? ELSE custom_number END
WHERE id=?`, g.favorite, g.customName, g.customNumber, next); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE passes SET channel_id=? WHERE channel_id=?`, next, g.id); err != nil {
			return err
		}
		if g.favorite == 1 {
			favorites = append(favorites, next)
		}
	}
	// Channel ids are reused, so nothing may keep pointing at these rows.
	for _, q := range []string{
		`DELETE FROM passes WHERE channel_id IN (SELECT id FROM channels WHERE device_id=?)`,
		`UPDATE recordings SET channel_id=0 WHERE channel_id IN (SELECT id FROM channels WHERE device_id=?)`,
		`DELETE FROM airings WHERE channel_id IN (SELECT id FROM channels WHERE device_id=?)`,
		`DELETE FROM channel_signals WHERE channel_id IN (SELECT id FROM channels WHERE device_id=?)`,
		`DELETE FROM sources WHERE device_id=?`,
		`DELETE FROM devices WHERE device_id=?`,
	} {
		if _, err := tx.ExecContext(ctx, q, deviceID); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	// A favorite moved onto the hidden half of a 1.0/3.0 pair shows on the half the guide keeps.
	for _, id := range favorites {
		if err := s.passFavoriteToShown(ctx, id); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) Channels(ctx context.Context, guideOnly bool) ([]Channel, error) {
	q := `SELECT id, device_id, guide_number, guide_name, custom_number, custom_name,
		video_codec, audio_codec, hd, favorite, enabled, hidden, present, guide_key, art_url, art_width, art_height, network,
		protected, twin_choice FROM channels`
	rows, err := s.db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Channel
	choices := map[int64]string{}
	for rows.Next() {
		var ch Channel
		var customNumber, customName, choice string
		var hd, fav, en, hidden, present, protected int
		if err := rows.Scan(&ch.ID, &ch.DeviceID, &ch.GuideNumber, &ch.GuideName, &customNumber, &customName,
			&ch.VideoCodec, &ch.AudioCodec, &hd, &fav, &en, &hidden, &present, &ch.GuideKey, &ch.ArtURL, &ch.ArtWidth, &ch.ArtHeight, &ch.Network,
			&protected, &choice); err != nil {
			return nil, err
		}
		ch.Protected = protected != 0
		if choice != "" {
			choices[ch.ID] = choice
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
	// Pairs are found on every row, so a hidden twin still names its partner.
	markTwins(out, choices)
	if err := s.markSameAs(ctx, out); err != nil {
		return nil, err
	}
	byID := make(map[int64]Channel, len(out))
	for _, ch := range out {
		byID[ch.ID] = ch
	}
	if guideOnly {
		shown := out[:0]
		for _, ch := range out {
			if ch.Present && ch.Enabled && !ch.Hidden && ch.SameAs == 0 {
				shown = append(shown, ch)
			}
		}
		out = shown
	}
	sort.Slice(out, func(i, j int) bool {
		cmp := hdhr.CompareGuide(guideOrderNumber(out[i], byID), guideOrderNumber(out[j], byID))
		if cmp == 0 {
			cmp = hdhr.CompareGuide(out[i].DisplayNumber, out[j].DisplayNumber)
		}
		if cmp == 0 {
			return out[i].ID < out[j].ID
		}
		return cmp < 0
	})
	return out, nil
}

// guideOrderNumber is where a channel sits in the guide. A shown 3.0 channel
// uses its 1.0 twin's number, so 104.1 stays next to the 4s. A number the
// viewer set is kept. A 3.0 channel with no pair keeps its own number.
func guideOrderNumber(ch Channel, byID map[int64]Channel) string {
	if ch.Standard != TwinATSC3 || ch.Hidden || ch.TwinID == 0 || ch.DisplayNumber != ch.GuideNumber {
		return ch.DisplayNumber
	}
	twin, ok := byID[ch.TwinID]
	if !ok || twin.Standard == TwinATSC3 || twin.DisplayNumber == "" {
		return ch.DisplayNumber
	}
	return twin.DisplayNumber
}

// SetNetworks stores a guide's network decision. Blank values are left alone.
func (s *Store) SetNetworks(ctx context.Context, nets map[int64]string) error {
	for id, net := range nets {
		net = strings.TrimSpace(net)
		if net == "" {
			continue
		}
		if _, err := s.db.ExecContext(ctx, `UPDATE channels SET network = ? WHERE id = ?`, net, id); err != nil {
			return err
		}
	}
	return nil
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
	// One channel on two tuners is one row on the guide, so a change to it is
	// a change to every tuner's row.
	ids, err := s.sameChannel(ctx, id)
	if err != nil {
		return Channel{}, err
	}
	marks := make([]string, len(ids))
	for i, other := range ids {
		marks[i] = "?"
		args = append(args, other)
	}
	res, err := s.db.ExecContext(ctx, `UPDATE channels SET `+strings.Join(sets, ", ")+` WHERE id IN (`+strings.Join(marks, ",")+`)`, args...)
	if err != nil {
		return Channel{}, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return Channel{}, sql.ErrNoRows
	}
	if patch.Favorite != nil && *patch.Favorite {
		if err := s.passFavoriteToShown(ctx, id); err != nil {
			return Channel{}, err
		}
	}
	return s.Channel(ctx, id)
}

// Channel is one channel as Channels lists it.
func (s *Store) Channel(ctx context.Context, id int64) (Channel, error) {
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

// Settings returns a copy of every setting. They are read from the catalog
// once and kept in memory: device sign-in alone asked on every request.
// Every write here drops the copy.
func (s *Store) Settings(ctx context.Context) (map[string]string, error) {
	s.settingsMu.Lock()
	defer s.settingsMu.Unlock()
	if s.settings == nil {
		loaded, err := s.loadSettings(ctx)
		if err != nil {
			return nil, err
		}
		s.settings = loaded
	}
	return maps.Clone(s.settings), nil
}

func (s *Store) loadSettings(ctx context.Context) (map[string]string, error) {
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

// forgetSettings drops the in-memory copy after a write.
func (s *Store) forgetSettings() {
	s.settingsMu.Lock()
	s.settings = nil
	s.settingsMu.Unlock()
}

func (s *Store) PutSettings(ctx context.Context, values map[string]string) error {
	cleaned, err := cleanSettings(values)
	if err != nil {
		return err
	}
	return s.writeSettings(ctx, cleaned)
}

func cleanSettings(values map[string]string) (map[string]string, error) {
	allowed := map[string]bool{
		"layout":              true,
		"profile":             true,
		"audio":               true,
		"encoder":             true,
		"watermarkGB":         true,
		"bufferMinutes":       true,
		"writeNfo":            true,
		"deleteWatchedDays":   true,
		"makeRoom":            true,
		"protectFavorites":    true,
		"folderLayout":        true,
		"gameAlerts":          true,
		"pictureMode":         true,
		"autoplay":            true,
		"hdhrEmulate":         true,
		"deviceAuth":          true,
		"deviceToken":         true,
		"exportMosaics":       true,
		"hideScores":          true,
		"liveScores":          true,
		"checkUpdates":        true,
		"setupComplete":       true,
		"sdUser":              true,
		"sdPassword":          true,
		"sdLineup":            true,
		"guideUrl":            true,
		"sdPasswordSet":       true,
		"tmdbKey":             true,
		"tmdbKeySet":          true,
		"sportsdbKey":         true,
		"sportsdbKeySet":      true,
		"needsSetup":          true,
		"lastGuidePull":       true,
		"nextGuidePull":       true,
		"lastManualGuidePull": true,
		"lastGuideError":      true,
		"lastGuideErrorAt":    true,
		"adaptive":            true,
		SettingRemoteURL:      true,
		SettingTunnelToken:    true,
		"tunnelTokenSet":      true,
	}
	cleaned := map[string]string{}
	for k, v := range values {
		// Older apps still send it. The recordings folder is set where the
		// server starts, and a path typed in an app never moved them.
		if k == "recordingsPath" {
			continue
		}
		if !allowed[k] {
			return nil, fmt.Errorf("unknown setting %q", k)
		}
		if k == "layout" && v != "auto" && v != "desktop" && v != "tv" && v != "phone" {
			return nil, fmt.Errorf("layout must be auto, desktop, tv, or phone")
		}
		if k == "exportMosaics" && (len(v) > 256 || strings.Trim(v, "0123456789,-") != "") {
			return nil, fmt.Errorf("exportMosaics is a list of mosaic keys such as 4-12,1-2-3")
		}
		if k == "pictureMode" && v != "broadcast" && v != "smooth" && v != "film" {
			return nil, fmt.Errorf("pictureMode must be broadcast, smooth, or film")
		}
		if k == "profile" && v != "transparent" && v != "balanced" && v != "saver" {
			return nil, fmt.Errorf("profile must be transparent, balanced, or saver")
		}
		if k == "audio" && v != "stereo" && v != "surround" {
			return nil, fmt.Errorf("audio must be stereo or surround")
		}
		if k == "autoplay" && v != "0" && v != "1" {
			return nil, fmt.Errorf("autoplay must be 0 or 1")
		}
		if k == "hdhrEmulate" && v != "0" && v != "1" {
			return nil, fmt.Errorf("hdhrEmulate must be 0 or 1")
		}
		if k == "setupComplete" && v != "0" && v != "1" {
			return nil, fmt.Errorf("setupComplete must be 0 or 1")
		}
		if k == "watermarkGB" {
			n, err := strconv.Atoi(strings.TrimSpace(v))
			if err != nil || n < 0 || n > 1000000 {
				return nil, fmt.Errorf("watermarkGB must be a whole number of gigabytes from 0 to 1000000")
			}
		}
		if k == "bufferMinutes" && v != "0" && v != "30" && v != "60" && v != "120" && v != "240" {
			return nil, fmt.Errorf("bufferMinutes must be 0, 30, 60, 120, or 240")
		}
		if k == "deleteWatchedDays" {
			n, err := strconv.Atoi(strings.TrimSpace(v))
			if err != nil || n < 0 || n > 3650 {
				return nil, fmt.Errorf("deleteWatchedDays must be a whole number of days from 0 to 3650")
			}
			v = strconv.Itoa(n)
		}
		if k == "makeRoom" && v != "0" && v != "1" {
			return nil, fmt.Errorf("makeRoom must be 0 or 1")
		}
		if k == "protectFavorites" && v != "0" && v != "1" {
			return nil, fmt.Errorf("protectFavorites must be 0 or 1")
		}
		if k == "folderLayout" && v != "shows" && v != "flat" {
			return nil, fmt.Errorf("folderLayout must be shows or flat")
		}
		if k == "gameAlerts" && v != "all" && v != "teams" && v != "off" {
			return nil, fmt.Errorf("gameAlerts must be all, teams, or off")
		}
		if k == "writeNfo" && v != "0" && v != "1" {
			return nil, fmt.Errorf("writeNfo must be 0 or 1")
		}
		if k == "adaptive" && v != "0" && v != "1" {
			return nil, fmt.Errorf("adaptive must be 0 or 1")
		}
		if k == "hideScores" && v != "0" && v != "1" {
			return nil, fmt.Errorf("hideScores must be 0 or 1")
		}
		if k == "liveScores" && v != "0" && v != "1" {
			return nil, fmt.Errorf("liveScores must be 0 or 1")
		}
		if k == "checkUpdates" && v != "0" && v != "1" {
			return nil, fmt.Errorf("checkUpdates must be 0 or 1")
		}
		if k == "guideUrl" {
			v = strings.TrimSpace(v)
			if v != "" && !strings.HasPrefix(v, "https://") && !strings.HasPrefix(v, "http://") {
				return nil, fmt.Errorf("the guide address needs to start with http")
			}
		}
		if k == "sdPassword" && strings.TrimSpace(v) == "" {
			continue
		}
		if k == "sdPasswordSet" || k == "tmdbKeySet" || k == "tunnelTokenSet" || k == "sportsdbKeySet" || k == "deviceToken" ||
			k == "needsSetup" || k == "lastGuidePull" || k == "nextGuidePull" || k == "lastManualGuidePull" ||
			k == "lastGuideError" || k == "lastGuideErrorAt" {
			continue
		}
		if k == "deviceAuth" && v != "0" && v != "1" {
			return nil, fmt.Errorf("deviceAuth must be 0 or 1")
		}
		if k == SettingRemoteURL {
			norm, err := NormalizeRemoteURL(v)
			if err != nil {
				return nil, err
			}
			v = norm
		}
		if k == SettingTunnelToken {
			token, err := CleanTunnelToken(v)
			if err != nil {
				return nil, err
			}
			v = token
		}
		if k == "tmdbKey" && strings.TrimSpace(v) == "" {
			continue
		}
		if k == "sportsdbKey" {
			v = strings.TrimSpace(v)
		}
		cleaned[k] = v
	}
	return cleaned, nil
}

func (s *Store) writeSettings(ctx context.Context, values map[string]string) error {
	if len(values) == 0 {
		return nil
	}
	defer s.forgetSettings()
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

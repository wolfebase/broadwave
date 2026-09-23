CREATE TABLE IF NOT EXISTS profiles (
	id INTEGER PRIMARY KEY,
	name TEXT NOT NULL,
	created_at TEXT NOT NULL
);
INSERT INTO profiles (id, name, created_at)
	SELECT 1, 'Home', strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
	WHERE NOT EXISTS (SELECT 1 FROM profiles WHERE id = 1);

CREATE TABLE IF NOT EXISTS devices (
	device_id TEXT PRIMARY KEY,
	friendly_name TEXT NOT NULL DEFAULT '',
	model_number TEXT NOT NULL DEFAULT '',
	firmware_name TEXT NOT NULL DEFAULT '',
	firmware_version TEXT NOT NULL DEFAULT '',
	upgrade_available TEXT NOT NULL DEFAULT '',
	base_url TEXT NOT NULL,
	lineup_url TEXT NOT NULL DEFAULT '',
	tuner_count INTEGER NOT NULL DEFAULT 0,
	priority INTEGER NOT NULL DEFAULT 0,
	last_seen TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS channels (
	id INTEGER PRIMARY KEY,
	device_id TEXT NOT NULL REFERENCES devices(device_id) ON DELETE CASCADE,
	guide_number TEXT NOT NULL,
	guide_name TEXT NOT NULL DEFAULT '',
	stream_url TEXT NOT NULL DEFAULT '',
	video_codec TEXT NOT NULL DEFAULT '',
	audio_codec TEXT NOT NULL DEFAULT '',
	hd INTEGER NOT NULL DEFAULT 0,
	favorite INTEGER NOT NULL DEFAULT 0,
	enabled INTEGER NOT NULL DEFAULT 1,
	hidden INTEGER NOT NULL DEFAULT 0,
	custom_name TEXT NOT NULL DEFAULT '',
	custom_number TEXT NOT NULL DEFAULT '',
	present INTEGER NOT NULL DEFAULT 1,
	frequency_hz INTEGER NOT NULL DEFAULT 0,
	program_num INTEGER NOT NULL DEFAULT 0,
	UNIQUE(device_id, guide_number)
);

CREATE TABLE IF NOT EXISTS settings (
	key TEXT PRIMARY KEY,
	value TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS airings (
	id INTEGER PRIMARY KEY,
	channel_id INTEGER NOT NULL,
	title TEXT NOT NULL,
	subtitle TEXT NOT NULL DEFAULT '',
	description TEXT NOT NULL DEFAULT '',
	category TEXT NOT NULL DEFAULT '',
	starts_at TEXT NOT NULL,
	ends_at TEXT NOT NULL,
	program_id TEXT NOT NULL DEFAULT '',
	is_new INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS airings_channel_start ON airings(channel_id, starts_at);

CREATE TABLE IF NOT EXISTS recordings (
	id INTEGER PRIMARY KEY,
	channel_id INTEGER NOT NULL,
	guide_number TEXT NOT NULL DEFAULT '',
	title TEXT NOT NULL,
	path TEXT NOT NULL,
	status TEXT NOT NULL,
	error TEXT NOT NULL DEFAULT '',
	started_at TEXT NOT NULL,
	ends_at TEXT NOT NULL DEFAULT '',
	ended_at TEXT NOT NULL DEFAULT '',
	duration_sec REAL NOT NULL DEFAULT 0,
	subtitle TEXT NOT NULL DEFAULT '',
	description TEXT NOT NULL DEFAULT '',
	category TEXT NOT NULL DEFAULT '',
	program_id TEXT NOT NULL DEFAULT '',
	watched INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS passes (
	id INTEGER PRIMARY KEY,
	title TEXT NOT NULL,
	channel_id INTEGER NOT NULL DEFAULT 0,
	kind TEXT NOT NULL DEFAULT 'series',
	pad_before INTEGER NOT NULL DEFAULT 0,
	pad_after INTEGER NOT NULL DEFAULT 0,
	priority INTEGER NOT NULL DEFAULT 0,
	episodes TEXT NOT NULL DEFAULT 'all',
	keep_mode TEXT NOT NULL DEFAULT 'all',
	keep_count INTEGER NOT NULL DEFAULT 0,
	limit_count INTEGER NOT NULL DEFAULT 0,
	rerecord INTEGER NOT NULL DEFAULT 0,
	commercials INTEGER NOT NULL DEFAULT 1,
	time_start TEXT NOT NULL DEFAULT '',
	time_end TEXT NOT NULL DEFAULT '',
	match_kind TEXT NOT NULL DEFAULT 'title'
);

CREATE TABLE IF NOT EXISTS seen_programs (
	program_key TEXT PRIMARY KEY,
	deleted INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS skipped_airings (
	program_key TEXT NOT NULL,
	starts_at TEXT NOT NULL,
	PRIMARY KEY (program_key, starts_at)
);

CREATE TABLE IF NOT EXISTS sources (
	id INTEGER PRIMARY KEY,
	kind TEXT NOT NULL,
	name TEXT NOT NULL,
	url TEXT NOT NULL DEFAULT '',
	xmltv_url TEXT NOT NULL DEFAULT '',
	enabled INTEGER NOT NULL DEFAULT 1
);

CREATE TABLE IF NOT EXISTS markers (
	id INTEGER PRIMARY KEY,
	recording_id INTEGER NOT NULL,
	start_sec REAL NOT NULL,
	end_sec REAL NOT NULL
);

CREATE TABLE IF NOT EXISTS virtual_channels (
	id INTEGER PRIMARY KEY,
	number TEXT NOT NULL,
	name TEXT NOT NULL,
	order_mode TEXT NOT NULL DEFAULT 'custom',
	rule_title TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS virtual_items (
	id INTEGER PRIMARY KEY,
	virtual_id INTEGER NOT NULL,
	recording_id INTEGER NOT NULL,
	position INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS progress (
	recording_id INTEGER PRIMARY KEY,
	position_sec REAL NOT NULL,
	updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS events (
	id INTEGER PRIMARY KEY,
	at TEXT NOT NULL,
	kind TEXT NOT NULL,
	message TEXT NOT NULL
);

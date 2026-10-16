-- Indexes for the reads that scanned whole tables: a window by end time
-- (now and next, channels with listings), markers of one recording, a
-- channel by number, and the few recordings still open. A lineup row by
-- stream already narrows on device_id through UNIQUE(device_id, guide_number).
CREATE INDEX IF NOT EXISTS airings_ends ON airings(ends_at, channel_id);
CREATE INDEX IF NOT EXISTS markers_recording ON markers(recording_id);
CREATE INDEX IF NOT EXISTS channels_number ON channels(guide_number);
CREATE INDEX IF NOT EXISTS recordings_open ON recordings(status) WHERE status IN ('recording', 'interrupted');

-- partial is a recording that holds only part of its airing: the piece
-- before or after a restart, or a retry after a failure. It does not count
-- as having the episode.
ALTER TABLE recordings ADD COLUMN partial INTEGER NOT NULL DEFAULT 0;

-- retired is a tuner the owner took away on purpose. It counts toward no
-- plan and raises no "stopped answering" note. Seeing it again clears it.
ALTER TABLE devices ADD COLUMN retired INTEGER NOT NULL DEFAULT 0;

-- data_versions counts changes to what the apps cache, so a request can be
-- answered from a cached body and a reconnecting app can skip what it has.
-- Triggers keep them, so every writer counts, a restore included. An update
-- that changes nothing a client sees (a tuner's last-seen time, a guide
-- refresh rewriting the same row's link) does not count.
-- A catalog that recorded the baseline without running it (the upgrade
-- fixture) can lack these; the triggers below need them.
CREATE TABLE IF NOT EXISTS settings (
	key TEXT PRIMARY KEY,
	value TEXT NOT NULL
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
CREATE TABLE data_versions (
	name TEXT PRIMARY KEY,
	version INTEGER NOT NULL DEFAULT 0
);
INSERT INTO data_versions (name, version) VALUES
	('guide', 1), ('channels', 1), ('recordings', 1), ('schedule', 1), ('settings', 1);

CREATE TRIGGER data_versions_airings_insert AFTER INSERT ON airings BEGIN
	UPDATE data_versions SET version = version + 1 WHERE name = 'guide';
END;
CREATE TRIGGER data_versions_airings_delete AFTER DELETE ON airings BEGIN
	UPDATE data_versions SET version = version + 1 WHERE name = 'guide';
END;
CREATE TRIGGER data_versions_airings_update AFTER UPDATE ON airings
WHEN (old.channel_id, old.title, old.subtitle, old.description, old.category, old.starts_at, old.ends_at,
	old.program_id, old.is_new, old.image_url, old.image_width, old.image_height, old.season, old.episode,
	old.episode_label, old.original_air, old.series_id, old.is_live, old.is_premiere, old.is_finale,
	old.rating, old.cast_list, old.game_id)
	IS NOT (new.channel_id, new.title, new.subtitle, new.description, new.category, new.starts_at, new.ends_at,
	new.program_id, new.is_new, new.image_url, new.image_width, new.image_height, new.season, new.episode,
	new.episode_label, new.original_air, new.series_id, new.is_live, new.is_premiere, new.is_finale,
	new.rating, new.cast_list, new.game_id)
BEGIN
	UPDATE data_versions SET version = version + 1 WHERE name = 'guide';
END;

CREATE TRIGGER data_versions_channels_insert AFTER INSERT ON channels BEGIN
	UPDATE data_versions SET version = version + 1 WHERE name = 'channels';
END;
CREATE TRIGGER data_versions_channels_delete AFTER DELETE ON channels BEGIN
	UPDATE data_versions SET version = version + 1 WHERE name = 'channels';
END;
CREATE TRIGGER data_versions_channels_update AFTER UPDATE ON channels
WHEN (old.device_id, old.guide_number, old.guide_name, old.stream_url, old.video_codec, old.audio_codec, old.hd,
	old.favorite, old.enabled, old.hidden, old.custom_name, old.custom_number, old.present, old.guide_key,
	old.art_url, old.art_width, old.art_height, old.network, old.protected, old.twin_choice)
	IS NOT (new.device_id, new.guide_number, new.guide_name, new.stream_url, new.video_codec, new.audio_codec, new.hd,
	new.favorite, new.enabled, new.hidden, new.custom_name, new.custom_number, new.present, new.guide_key,
	new.art_url, new.art_width, new.art_height, new.network, new.protected, new.twin_choice)
BEGIN
	UPDATE data_versions SET version = version + 1 WHERE name = 'channels';
END;

CREATE TRIGGER data_versions_devices_insert AFTER INSERT ON devices BEGIN
	UPDATE data_versions SET version = version + 1 WHERE name = 'channels';
END;
CREATE TRIGGER data_versions_devices_delete AFTER DELETE ON devices BEGIN
	UPDATE data_versions SET version = version + 1 WHERE name = 'channels';
END;
CREATE TRIGGER data_versions_devices_update AFTER UPDATE ON devices
WHEN (old.friendly_name, old.model_number, old.firmware_version, old.base_url, old.tuner_count, old.priority, old.retired)
	IS NOT (new.friendly_name, new.model_number, new.firmware_version, new.base_url, new.tuner_count, new.priority, new.retired)
BEGIN
	UPDATE data_versions SET version = version + 1 WHERE name = 'channels';
END;

CREATE TRIGGER data_versions_recordings_insert AFTER INSERT ON recordings BEGIN
	UPDATE data_versions SET version = version + 1 WHERE name = 'recordings';
END;
CREATE TRIGGER data_versions_recordings_delete AFTER DELETE ON recordings BEGIN
	UPDATE data_versions SET version = version + 1 WHERE name = 'recordings';
END;
CREATE TRIGGER data_versions_recordings_update AFTER UPDATE ON recordings BEGIN
	UPDATE data_versions SET version = version + 1 WHERE name = 'recordings';
END;

CREATE TRIGGER data_versions_passes_insert AFTER INSERT ON passes BEGIN
	UPDATE data_versions SET version = version + 1 WHERE name = 'schedule';
END;
CREATE TRIGGER data_versions_passes_delete AFTER DELETE ON passes BEGIN
	UPDATE data_versions SET version = version + 1 WHERE name = 'schedule';
END;
CREATE TRIGGER data_versions_passes_update AFTER UPDATE ON passes BEGIN
	UPDATE data_versions SET version = version + 1 WHERE name = 'schedule';
END;
CREATE TRIGGER data_versions_skips_insert AFTER INSERT ON skipped_airings BEGIN
	UPDATE data_versions SET version = version + 1 WHERE name = 'schedule';
END;
CREATE TRIGGER data_versions_skips_delete AFTER DELETE ON skipped_airings BEGIN
	UPDATE data_versions SET version = version + 1 WHERE name = 'schedule';
END;
CREATE TRIGGER data_versions_seen_insert AFTER INSERT ON seen_programs BEGIN
	UPDATE data_versions SET version = version + 1 WHERE name = 'schedule';
END;
CREATE TRIGGER data_versions_seen_update AFTER UPDATE ON seen_programs
WHEN old.deleted IS NOT new.deleted
BEGIN
	UPDATE data_versions SET version = version + 1 WHERE name = 'schedule';
END;
CREATE TRIGGER data_versions_seen_delete AFTER DELETE ON seen_programs BEGIN
	UPDATE data_versions SET version = version + 1 WHERE name = 'schedule';
END;

CREATE TRIGGER data_versions_settings_insert AFTER INSERT ON settings BEGIN
	UPDATE data_versions SET version = version + 1 WHERE name = 'settings';
END;
CREATE TRIGGER data_versions_settings_update AFTER UPDATE ON settings
WHEN old.value IS NOT new.value
BEGIN
	UPDATE data_versions SET version = version + 1 WHERE name = 'settings';
END;
CREATE TRIGGER data_versions_settings_delete AFTER DELETE ON settings BEGIN
	UPDATE data_versions SET version = version + 1 WHERE name = 'settings';
END;

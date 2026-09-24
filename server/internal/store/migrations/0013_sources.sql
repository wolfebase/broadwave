-- A source is the thing a person adds. Devices stay, so existing channels,
-- recordings, and passes keep their ids. Playlist rows already in `sources`
-- gain a stable key. Each tuner becomes a source keyed by its device id.

ALTER TABLE sources ADD COLUMN stable_key TEXT NOT NULL DEFAULT '';
ALTER TABLE sources ADD COLUMN priority INTEGER NOT NULL DEFAULT 0;
ALTER TABLE sources ADD COLUMN tuner_count INTEGER NOT NULL DEFAULT 0;
ALTER TABLE sources ADD COLUMN stream_limit INTEGER NOT NULL DEFAULT 0;
ALTER TABLE sources ADD COLUMN stream_format TEXT NOT NULL DEFAULT '';
ALTER TABLE sources ADD COLUMN has_guide INTEGER NOT NULL DEFAULT 0;
ALTER TABLE sources ADD COLUMN needs_tuner INTEGER NOT NULL DEFAULT 0;
ALTER TABLE sources ADD COLUMN refresh TEXT NOT NULL DEFAULT '';
ALTER TABLE sources ADD COLUMN health TEXT NOT NULL DEFAULT '';
ALTER TABLE sources ADD COLUMN device_id TEXT NOT NULL DEFAULT '';

CREATE TABLE IF NOT EXISTS source_secrets (
	source_id INTEGER PRIMARY KEY REFERENCES sources(id) ON DELETE CASCADE,
	secret TEXT NOT NULL DEFAULT ''
);

UPDATE sources
SET stable_key = 'src:' || id,
	device_id = CASE WHEN device_id = '' THEN 'src-' || id ELSE device_id END
WHERE stable_key = '';

INSERT INTO sources (
	kind, name, url, enabled, stable_key, priority, tuner_count,
	stream_format, needs_tuner, device_id
)
SELECT
	'hdhomerun',
	CASE WHEN friendly_name = '' THEN device_id ELSE friendly_name END,
	base_url,
	1,
	'hdhr:' || device_id,
	priority,
	tuner_count,
	'ts',
	CASE WHEN tuner_count > 0 THEN 1 ELSE 0 END,
	device_id
FROM devices
WHERE device_id NOT LIKE 'src-%'
	AND NOT EXISTS (
		SELECT 1 FROM sources WHERE sources.device_id = devices.device_id
	);

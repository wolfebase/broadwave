ALTER TABLE airings ADD COLUMN guide_source TEXT NOT NULL DEFAULT '';

CREATE TABLE IF NOT EXISTS guide_scans (
	frequency_hz INTEGER PRIMARY KEY,
	scanned_at TEXT NOT NULL
);

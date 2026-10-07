-- Picture prints of commercials from breaks the scan was sure of, one 64-bit
-- hash per second, little-endian. A spot seen again marks a break.
CREATE TABLE spots (
	id INTEGER PRIMARY KEY,
	prints BLOB NOT NULL,
	recording_id INTEGER NOT NULL DEFAULT 0,
	hits INTEGER NOT NULL DEFAULT 0,
	seen_at TEXT NOT NULL
);
CREATE INDEX spots_seen ON spots (seen_at);

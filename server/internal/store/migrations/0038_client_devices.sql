-- Paired phones, TVs, and browsers. Tuners stay in devices.
-- token_hash is the SHA-256 of the bearer token. The token is not stored.
CREATE TABLE client_devices (
	id TEXT PRIMARY KEY,
	name TEXT NOT NULL,
	kind TEXT NOT NULL,
	scopes TEXT NOT NULL,
	token_hash TEXT NOT NULL UNIQUE,
	created_at TEXT NOT NULL,
	last_seen_at TEXT NOT NULL DEFAULT '',
	revoked_at TEXT NOT NULL DEFAULT ''
);

-- A pairing code waits until it is used or it expires.
-- code_hash and poll_secret_hash are SHA-256. The code and the secret are not stored.
CREATE TABLE pairings (
	id TEXT PRIMARY KEY,
	mode TEXT NOT NULL,
	code_hash TEXT NOT NULL,
	poll_secret_hash TEXT NOT NULL DEFAULT '',
	name TEXT NOT NULL DEFAULT '',
	kind TEXT NOT NULL DEFAULT 'other',
	scopes TEXT NOT NULL,
	created_at TEXT NOT NULL,
	expires_at TEXT NOT NULL,
	attempts INTEGER NOT NULL DEFAULT 0,
	state TEXT NOT NULL DEFAULT 'pending',
	device_id TEXT NOT NULL DEFAULT ''
);

CREATE INDEX pairings_pending ON pairings (state, expires_at);

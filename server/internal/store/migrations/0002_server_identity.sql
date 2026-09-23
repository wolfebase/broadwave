CREATE TABLE IF NOT EXISTS server_identity (
	id INTEGER PRIMARY KEY CHECK (id = 1),
	server_id TEXT NOT NULL,
	name TEXT NOT NULL,
	created_at TEXT NOT NULL
);

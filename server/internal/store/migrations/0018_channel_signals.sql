CREATE TABLE IF NOT EXISTS channel_signals (
	channel_id INTEGER PRIMARY KEY,
	strength INTEGER NOT NULL,
	quality INTEGER NOT NULL,
	symbol INTEGER NOT NULL,
	locked INTEGER NOT NULL,
	checked_at TEXT NOT NULL
);

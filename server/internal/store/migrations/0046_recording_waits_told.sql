-- 0045 was edited before any release, so a database here may or may not have
-- told. An open wait is only a retry's place, and no released build has any.
DROP TABLE recording_waits;
CREATE TABLE recording_waits (
	channel_id INTEGER NOT NULL,
	airing_start INTEGER NOT NULL,
	airing_end INTEGER NOT NULL,
	until_at INTEGER NOT NULL,
	pass_id INTEGER NOT NULL,
	title TEXT NOT NULL DEFAULT '',
	subtitle TEXT NOT NULL DEFAULT '',
	description TEXT NOT NULL DEFAULT '',
	category TEXT NOT NULL DEFAULT '',
	program_id TEXT NOT NULL DEFAULT '',
	game_id TEXT NOT NULL DEFAULT '',
	reason TEXT NOT NULL DEFAULT '',
	failure TEXT NOT NULL DEFAULT '',
	since INTEGER NOT NULL,
	told INTEGER NOT NULL DEFAULT 0,
	PRIMARY KEY (channel_id, airing_start)
);

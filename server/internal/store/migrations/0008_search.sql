CREATE VIRTUAL TABLE IF NOT EXISTS airing_search USING fts5(
	title, subtitle, description, cast_list,
	content='airings', content_rowid='id',
	tokenize='porter unicode61'
);

CREATE VIRTUAL TABLE IF NOT EXISTS recording_search USING fts5(
	title, subtitle, description,
	content='recordings', content_rowid='id',
	tokenize='porter unicode61'
);

INSERT INTO airing_search(airing_search) VALUES('rebuild');
INSERT INTO recording_search(recording_search) VALUES('rebuild');

CREATE TRIGGER IF NOT EXISTS airings_search_insert AFTER INSERT ON airings BEGIN
	INSERT INTO airing_search(rowid, title, subtitle, description, cast_list)
	VALUES (new.id, new.title, new.subtitle, new.description, new.cast_list);
END;

CREATE TRIGGER IF NOT EXISTS airings_search_delete AFTER DELETE ON airings BEGIN
	INSERT INTO airing_search(airing_search, rowid, title, subtitle, description, cast_list)
	VALUES ('delete', old.id, old.title, old.subtitle, old.description, old.cast_list);
END;

CREATE TRIGGER IF NOT EXISTS airings_search_update AFTER UPDATE ON airings BEGIN
	INSERT INTO airing_search(airing_search, rowid, title, subtitle, description, cast_list)
	VALUES ('delete', old.id, old.title, old.subtitle, old.description, old.cast_list);
	INSERT INTO airing_search(rowid, title, subtitle, description, cast_list)
	VALUES (new.id, new.title, new.subtitle, new.description, new.cast_list);
END;

CREATE TRIGGER IF NOT EXISTS recordings_search_insert AFTER INSERT ON recordings BEGIN
	INSERT INTO recording_search(rowid, title, subtitle, description)
	VALUES (new.id, new.title, new.subtitle, new.description);
END;

CREATE TRIGGER IF NOT EXISTS recordings_search_delete AFTER DELETE ON recordings BEGIN
	INSERT INTO recording_search(recording_search, rowid, title, subtitle, description)
	VALUES ('delete', old.id, old.title, old.subtitle, old.description);
END;

CREATE TRIGGER IF NOT EXISTS recordings_search_update AFTER UPDATE ON recordings BEGIN
	INSERT INTO recording_search(recording_search, rowid, title, subtitle, description)
	VALUES ('delete', old.id, old.title, old.subtitle, old.description);
	INSERT INTO recording_search(rowid, title, subtitle, description)
	VALUES (new.id, new.title, new.subtitle, new.description);
END;

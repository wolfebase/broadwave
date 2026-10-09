-- game_id and artwork size are not search columns. The old trigger rewrote
-- the search row on every update, including a scoreboard id for the window.
-- A catalog that recorded the early migrations without this trigger (the
-- upgrade fixture) still has to get past the drop.
DROP TRIGGER IF EXISTS airings_search_update;
CREATE TRIGGER airings_search_update
AFTER UPDATE OF title, subtitle, description, cast_list ON airings BEGIN
	INSERT INTO airing_search(airing_search, rowid, title, subtitle, description, cast_list)
	VALUES ('delete', old.id, old.title, old.subtitle, old.description, old.cast_list);
	INSERT INTO airing_search(rowid, title, subtitle, description, cast_list)
	VALUES (new.id, new.title, new.subtitle, new.description, new.cast_list);
END;

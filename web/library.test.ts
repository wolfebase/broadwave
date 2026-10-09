import assert from "node:assert/strict";
import { test } from "node:test";
import { buildLibrary, continueWatching, episodeTag, filterRecordings, nextEpisode, recordingSubject, watched } from "./src/features/library/model.ts";
import type { Recording } from "./src/types.ts";

function rec(id: number, extra: Partial<Recording> = {}): Recording {
  return { id, channelId: 1, guideNumber: "4.1", title: "Mystery Hour", status: "complete", startedAt: `2026-10-0${id % 9}T19:00:00Z`, ...extra };
}

test("a recording's name keeps the episode apart from the show beside it", () => {
  assert.equal(recordingSubject(rec(1, { title: "Desk", subtitle: "The Quiet Hour", season: 1, episode: 4 })), "S1 E4, Desk, The Quiet Hour");
  assert.equal(recordingSubject(rec(1, { title: "Golf", subtitle: "Final round" })), "Golf, Final round");
  assert.equal(recordingSubject(rec(1, { title: "Golf", subtitle: "golf" })), "Golf");
  assert.equal(recordingSubject(rec(1, { title: "Tennis" })), "Tennis");
  assert.equal(episodeTag(rec(1, { episodeLabel: "Part 2" })), "Part 2");
});

test("continue watching is what was started and not finished, last played first", () => {
  const list = [
    rec(1, { position: 600, durationSec: 3600, progressAt: "2026-10-04T10:00:00Z" }),
    rec(2, { position: 900, durationSec: 3600, progressAt: "2026-10-05T10:00:00Z" }),
    rec(3, { position: 3590, durationSec: 3600 }),
    rec(4, { position: 10, durationSec: 3600 }),
    rec(5, { position: 600, durationSec: 3600, watched: 1 }),
    rec(6, { position: 600, durationSec: 3600, watched: 2, progressAt: "2026-10-03T10:00:00Z" }),
    rec(7, { position: 600, durationSec: 3600, status: "recording" }),
  ];
  assert.deepEqual(continueWatching(list).map((r) => r.id), [2, 1, 6]);
  assert.equal(watched(list[2]), true);
  assert.equal(watched(list[5]), false);
});

test("a show groups by season, newest season first, episodes in order", () => {
  const list = [
    rec(1, { season: 1, episode: 2, subtitle: "B" }),
    rec(2, { season: 2, episode: 1, subtitle: "C" }),
    rec(3, { season: 1, episode: 1, subtitle: "A" }),
    rec(4, { subtitle: "Special" }),
    rec(5, { title: "A Western", category: "Movie", bytes: 9 }),
  ];
  const { shows, movies } = buildLibrary(list, "newest");
  assert.equal(shows.length, 1);
  assert.deepEqual(shows[0].seasons.map((s) => s.season), [2, 1, 0]);
  assert.deepEqual(shows[0].seasons[1].items.map((r) => r.subtitle), ["B", "A"]);
  assert.deepEqual(movies.map((r) => r.id), [5]);
  const oldest = buildLibrary(list, "oldest").shows[0];
  assert.deepEqual(oldest.seasons.map((s) => s.season), [1, 2, 0]);
  assert.deepEqual(oldest.seasons[0].items.map((r) => r.subtitle), ["A", "B"]);
});

test("a show counts its unwatched episodes and the space they take", () => {
  const list = [rec(1, { bytes: 1000, watched: 1 }), rec(2, { bytes: 500 }), rec(3, { title: "mystery hour ", bytes: 250 })];
  const show = buildLibrary(list, "newest").shows;
  assert.equal(show.length, 1);
  assert.equal(show[0].unwatched, 2);
  assert.equal(show[0].bytes, 1750);
});

test("shows sort by size and title", () => {
  const list = [rec(1, { title: "Zed", bytes: 10 }), rec(2, { title: "Alpha", bytes: 5 }), rec(3, { title: "Mid", bytes: 50 })];
  assert.deepEqual(buildLibrary(list, "largest").shows.map((s) => s.title), ["Mid", "Zed", "Alpha"]);
  assert.deepEqual(buildLibrary(list, "title").shows.map((s) => s.title), ["Alpha", "Mid", "Zed"]);
});

test("filters by kind and unwatched", () => {
  const list = [rec(1), rec(2, { category: "Movie" }), rec(3, { category: "Sports" }), rec(4, { gameId: "g1" }), rec(5, { watched: 1 })];
  assert.deepEqual(filterRecordings(list, "movies", false).map((r) => r.id), [2]);
  assert.deepEqual(filterRecordings(list, "sports", false).map((r) => r.id), [3, 4]);
  assert.deepEqual(filterRecordings(list, "shows", true).map((r) => r.id), [1]);
});

test("episode tags", () => {
  assert.equal(episodeTag(rec(1, { season: 2, episode: 5 })), "S2 E5");
  assert.equal(episodeTag(rec(1, { episode: 5 })), "E5");
  assert.equal(episodeTag(rec(1, { episodeLabel: "Part 2" })), "Part 2");
  assert.equal(episodeTag(rec(1)), "");
});

test("the next episode is the next one aired of the same show", () => {
  const list = [
    rec(1, { season: 1, episode: 2 }),
    rec(2, { season: 2, episode: 1 }),
    rec(3, { season: 1, episode: 1 }),
    rec(4, { season: 1, episode: 3, status: "recording" }),
    rec(5, { season: 1, episode: 4, missing: true }),
    rec(6, { title: "Other Show", season: 1, episode: 3 }),
    rec(7, { title: " mystery hour ", season: 1, episode: 5 }),
  ];
  const next = (id: number) => nextEpisode(list, list.find((r) => r.id === id)!)?.id;
  assert.equal(next(3), 1);
  assert.equal(next(1), 7);
  assert.equal(next(7), 2);
  // None after the last one.
  assert.equal(next(2), undefined);
});

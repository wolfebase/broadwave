import assert from "node:assert/strict";
import { test } from "node:test";
import { gameKey, missingTuners } from "./src/features/home/homeRails.ts";

test("a game on two stations is one game", () => {
  assert.equal(gameKey({ title: "NFL: Bears at Bills ", start: "2026-10-16T17:00:00Z" }), gameKey({ title: "nfl: bears at bills", start: "2026-10-16T17:00:00.000Z" }));
  assert.notEqual(gameKey({ title: "College Football", category: "Sport, Football", start: "2026-10-16T17:00:00Z" }), gameKey({ title: "College Football", category: "Sport, Football", start: "2026-10-16T20:00:00Z" }));
  // A series is one card however many episodes air.
  assert.equal(gameKey({ title: "The Game", category: "Series, Comedy drama, Football", start: "2026-10-16T16:00:00Z" }), gameKey({ title: "The Game", category: "Series, Comedy drama, Football", start: "2026-10-16T16:30:00Z" }));
});

test("a tuner gone more than a day is missing; a retired one never is", () => {
  const now = Date.parse("2026-10-16T12:00:00Z");
  const list = missingTuners(
    [
      { deviceId: "A", friendlyName: "Attic DUO", lastSeen: "2026-10-14T12:00:00Z", offline: true },
      { deviceId: "B", friendlyName: "Den", lastSeen: "2026-10-16T08:00:00Z", offline: true },
      { deviceId: "C", friendlyName: "Old", lastSeen: "2026-10-01T12:00:00Z", offline: true, retired: true },
      { deviceId: "D", friendlyName: "Live", lastSeen: "2026-10-01T12:00:00Z", offline: false },
    ],
    now,
  );
  assert.deepEqual(list.map((d) => d.id), ["A"]);
  assert.equal(list[0].name, "Attic DUO");
});

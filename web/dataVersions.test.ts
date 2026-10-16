import assert from "node:assert/strict";
import { test } from "node:test";
import { changedEvents, movedSince, readVersions } from "./src/lib/dataVersions.ts";

test("a hello's versions are read, and an older server's hello has none", () => {
  assert.deepEqual(readVersions({ serverTime: 1, boot: "a", versions: { guide: 3, channels: 1, recordings: 9, schedule: 2, settings: 4 } }), {
    guide: 3,
    channels: 1,
    recordings: 9,
    schedule: 2,
    settings: 4,
  });
  assert.equal(readVersions({ serverTime: 1, boot: "a" }), null);
  assert.equal(readVersions(null), null);
});

test("a reconnect reads again only what moved", () => {
  const before = { guide: 3, channels: 1, recordings: 9, schedule: 2, settings: 4 };
  assert.deepEqual(movedSince(before, before), []);
  assert.deepEqual(movedSince(before, { ...before, guide: 4, recordings: 10 }), ["airings", "recordings"]);
  assert.deepEqual(movedSince({}, { schedule: 1 }), ["passes"]);
});

test("each change event names the list it makes stale", () => {
  assert.equal(changedEvents["guide.changed"], "airings");
  assert.equal(changedEvents["schedule.changed"], "passes");
  assert.equal(changedEvents["live.changed"], undefined);
});

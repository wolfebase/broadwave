import assert from "node:assert/strict";
import { test } from "node:test";
import { syncLiveText, syncPillText } from "./src/features/player/syncStatus.ts";

test("the sync pill says how many screens, and the live text says when it is still catching up", () => {
  assert.equal(syncPillText("locked", false, 1), "Sync on");
  assert.equal(syncPillText("locked", false, 3), "3 screens");
  assert.equal(syncPillText("locked", true, 1), "Together");
  assert.equal(syncPillText("locked", true, 2), "Together · 2");
  assert.equal(syncLiveText("locked", false, 1), "Sync on");
  assert.equal(syncLiveText("locked", false, 3), "Synced with 3 screens");
  assert.equal(syncLiveText("locked", true, 4), "Together · 4");
  assert.equal(syncLiveText("waiting", false, 1), "Waiting to sync");
  assert.equal(syncLiveText("syncing", false, 1), "Syncing");
  assert.equal(syncLiveText("syncing", false, 2), "Syncing 2 screens");
  assert.equal(syncLiveText("off", false, 1), "");
  assert.equal(syncLiveText("off", false, 1, true), "Sync off");
});

test("the pill never says synced while sync is waiting, catching up, or off", () => {
  for (const members of [1, 2, 3]) {
    assert.equal(syncPillText("waiting", false, members), "Waiting");
    assert.equal(syncPillText("syncing", false, members), "Syncing");
    assert.equal(syncPillText("off", false, members), "Sync off");
    for (const state of ["waiting", "syncing", "off"] as const) assert.doesNotMatch(syncPillText(state, false, members), /Synced/);
  }
});

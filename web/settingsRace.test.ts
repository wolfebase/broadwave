import assert from "node:assert/strict";
import { test } from "node:test";
import { beginSave, commitSave, followUpRead, LatestReads } from "./src/lib/latest.ts";
import { settingTextCommit, settingTextDraft, shownSettingText } from "./src/features/settings/field.ts";

const disk = { path: "/recordings", freeBytes: 5, totalBytes: 9, watermarkGB: 10 };

test("letters still being typed stay in the box, and they are not saved on each letter", () => {
  assert.equal(shownSettingText("a", "ab"), "ab");
  assert.equal(shownSettingText("", "https://"), "https://");
  assert.equal(shownSettingText("old", null), "old");
  assert.equal(shownSettingText("ab", "ab"), "ab");
  assert.equal(settingTextDraft("a", "ab"), "ab");
  assert.equal(settingTextDraft("ab", "ab"), null);
  assert.equal(settingTextDraft("old", null), null);
  assert.equal(settingTextCommit("a", "a"), null);
  assert.equal(settingTextCommit("a", "ab"), "ab");
  assert.equal(settingTextCommit("https://old", ""), "");
});

test("a settings GET that started while the save was in flight does not replace it", () => {
  const reads = new LatestReads();
  const seq = { n: 0 };
  const before = reads.start(["settings", "storage", "server"]);
  const mine = beginSave(seq);
  reads.start(["settings", "storage", "server"]);
  assert.equal(before("settings"), false);
  const during = reads.start(["settings", "storage", "server"]);
  const committed = commitSave(seq, mine, { sdUser: "ab" }, reads, ["settings", "storage", "server"]);
  assert.ok(committed);
  assert.equal(committed.saved.sdUser, "ab");
  assert.equal(during("settings"), false);
  assert.equal(committed.still("settings"), true);
  assert.equal(committed.still("storage"), true);
  assert.equal(committed.still("server"), true);
});

test("an earlier save writes nothing after a later one, and a failed read does not clear storage", () => {
  const reads = new LatestReads();
  const seq = { n: 0 };
  const first = beginSave(seq);
  reads.start(["settings", "storage", "server"]);
  const second = beginSave(seq);
  reads.start(["settings", "storage", "server"]);
  assert.equal(commitSave(seq, first, { sdUser: "a" }, reads, ["settings", "storage", "server"]), null);
  const committed = commitSave(seq, second, { sdUser: "ab" }, reads, ["settings", "storage", "server"]);
  assert.ok(committed);
  assert.equal(committed.saved.sdUser, "ab");
  // A late first response must not bump the clock after the second save committed.
  assert.equal(commitSave(seq, first, { sdUser: "a" }, reads, ["settings", "storage", "server"]), null);
  assert.equal(committed.still("settings"), true);
  assert.equal(followUpRead(seq, first, true, disk), null);
  assert.equal(followUpRead(seq, second, true, null), null);
  assert.equal(followUpRead(seq, second, false, disk), null);
  assert.deepEqual(followUpRead(seq, second, committed.still("storage"), disk), disk);
  const info = { update: { version: "1" } };
  assert.equal(followUpRead(seq, first, true, info), null);
  assert.deepEqual(followUpRead(seq, second, committed.still("server"), info), info);
  assert.equal(commitSave(seq, second, null, reads, ["settings", "storage", "server"]), null);
  assert.equal(committed.still("storage"), true);
  assert.equal(committed.still("settings"), true);
});

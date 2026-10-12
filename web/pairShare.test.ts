import assert from "node:assert/strict";
import { test } from "node:test";
import { searchFieldValue, noteAfterRecord } from "./src/features/search/field.ts";
import { settingTextCommit, settingTextDraft, shownSettingText } from "./src/features/settings/field.ts";
import { canEditSignIn, formatPairCode, kindLabel, normalizePairCode, pairPollStep, scopeLabel, scopesForAccess } from "./src/features/settings/pairCode.ts";
import { bracketHost, shareRows, tunerAddress } from "./src/features/settings/tunerShare.ts";

test("a pair code keeps six digits and labels the device", () => {
  assert.equal(normalizePairCode("123 456"), "123456");
  assert.equal(normalizePairCode("12-34"), null);
  assert.equal(formatPairCode("123456"), "123 456");
  assert.equal(formatPairCode("12345"), "12345");
  assert.equal(scopeLabel(["admin", "nope"]), "Admin");
  assert.equal(scopeLabel([]), "Watch");
  assert.equal(kindLabel("tv"), "TV");
  assert.equal(kindLabel("tablet"), "Device");
  assert.deepEqual(scopesForAccess("record"), ["watch", "record"]);
  assert.equal(scopesForAccess("guest"), null);
  assert.equal(canEditSignIn(null), true);
  assert.equal(canEditSignIn({ auth: "device", device: { scopes: ["watch"] } }), false);
  assert.equal(canEditSignIn({ auth: "device", device: { scopes: ["admin"] } }), true);
});

test("an approved poll without a token waits once, then misses", () => {
  assert.deepEqual(pairPollStep("approved", "tok", false), { action: "ready", missed: false });
  assert.deepEqual(pairPollStep("approved", "", false), { action: "wait", missed: true });
  assert.deepEqual(pairPollStep("approved", undefined, true), { action: "miss", missed: true });
  assert.equal(pairPollStep("expired", undefined, true).action, "expired");
  assert.equal(pairPollStep("denied", undefined, false).action, "denied");
  assert.deepEqual(pairPollStep("pending", undefined, true), { action: "wait", missed: false });
});

test("share rows stay on this server, and an IPv6 host keeps its brackets", () => {
  assert.equal(bracketHost("2001:db8::1"), "[2001:db8::1]");
  assert.equal(bracketHost("[2001:db8::1]"), "[2001:db8::1]");
  assert.equal(tunerAddress("http://127.0.0.1:8477/settings"), "127.0.0.1:8478");
  const rows = shareRows("http://127.0.0.1:8477/settings", false, true);
  assert.equal(rows.length, 12);
  assert.equal(rows[0].hint, "Turn on Act as an HDHomeRun.");
  assert.equal(rows[1].value, "http://127.0.0.1:8477/export/lineup.m3u");
  const playlistOnly = shareRows("http://127.0.0.1:8477/", true, false);
  assert.equal(playlistOnly.length, 8);
  assert.equal(playlistOnly.some((row) => row.label === "HDHomeRun"), false);
});

test("search and settings keep letters that are still being typed", () => {
  assert.equal(searchFieldValue("news", "new", false), "new");
  assert.equal(searchFieldValue("news", "new", true), "news");
  assert.equal(noteAfterRecord("news", "news", "City News", ""), "Recording every City News.");
  assert.equal(noteAfterRecord("news", "sports", "City News", "kept"), "kept");
  assert.equal(shownSettingText("Den", "De"), "De");
  assert.equal(shownSettingText("Den", "Den"), "Den");
  assert.equal(settingTextDraft("Den", "Den"), null);
  const draft = "Den TV";
  assert.equal(settingTextDraft("Den", draft), draft);
  assert.equal(settingTextCommit("Den", "Den"), null);
  assert.equal(settingTextCommit("Den", "Den TV"), "Den TV");
});

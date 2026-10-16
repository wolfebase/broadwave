import assert from "node:assert/strict";
import { test } from "node:test";
import { lineupRows, matchesLineup, shortTuner, switchPatch } from "./src/features/settings/lineupModel.ts";
import { signalWord, tunerHost } from "./src/features/settings/tunerHealth.ts";
import type { Channel, Device } from "./src/types.ts";

const flex = { deviceId: "F", friendlyName: "HDHomeRun FLEX 4K" } as Device;
const duo = { deviceId: "D", friendlyName: "HDHomeRun CONNECT DUO", offline: true } as Device;

function ch(over: Partial<Channel>): Channel {
  return { id: 1, deviceId: "F", displayNumber: "4.1", guideNumber: "4.1", displayName: "WDAF", guideName: "WDAF", enabled: true, hidden: false, present: true, ...over } as Channel;
}

test("a duplicate names its own tuner and the one the guide uses, and hides by default", () => {
  const rows = lineupRows([ch({ id: 1 }), ch({ id: 2, deviceId: "D", sameAs: 1 })], [flex, duo]);
  assert.equal(rows[0].state.kind, "switch");
  assert.equal(rows[0].extra, false);
  assert.deepEqual(rows[1].state, { kind: "duplicate", of: "4.1", tuner: "FLEX 4K" });
  assert.equal(rows[1].tuner, "CONNECT DUO");
  assert.equal(rows[1].extra, true);
});

test("an off-air channel is extra and has no switch", () => {
  const [row] = lineupRows([ch({ present: false })], [flex]);
  assert.equal(row.state.kind, "offAir");
  assert.equal(row.extra, true);
});

test("one switch: off hides the channel, on shows and enables it", () => {
  assert.deepEqual(switchPatch(false), { hidden: true });
  assert.deepEqual(switchPatch(true), { enabled: true, hidden: false });
  const [hidden] = lineupRows([ch({ hidden: true })], [flex]);
  assert.deepEqual(hidden.state, { kind: "switch", on: false });
});

test("the 1.0 half of a pair follows its 3.0 twin's choice", () => {
  const rows = lineupRows([ch({ id: 1, twinId: 2, twinChoice: "atsc3" }), ch({ id: 2, displayNumber: "104.1", standard: "atsc3", twinId: 1, twinChoice: "atsc3" })], [flex]);
  assert.deepEqual(rows[0].state, { kind: "shownAs", number: "104.1" });
  assert.deepEqual(rows[1].state, { kind: "pair", choice: "atsc3", twin: "4.1" });
});

test("a number search matches from the start; words match names", () => {
  assert.equal(matchesLineup(ch({ displayNumber: "4.1" }), "4"), true);
  assert.equal(matchesLineup(ch({ displayNumber: "14.1", guideNumber: "14.1" }), "4"), false);
  assert.equal(matchesLineup(ch({ displayName: "KCTV" }), "kc"), true);
  assert.equal(matchesLineup(ch({}), "  "), true);
});

test("tuner names drop the HDHomeRun prefix", () => {
  assert.equal(shortTuner(flex), "FLEX 4K");
  assert.equal(shortTuner(undefined), "");
});

test("signal reads as one plain word, led by picture quality", () => {
  assert.equal(signalWord(undefined, undefined), "");
  assert.equal(signalWord(95, 96), "tuners.signalStrong");
  assert.equal(signalWord(100, 80), "tuners.signalGood");
  assert.equal(signalWord(90, 60), "tuners.signalWeak");
  assert.equal(signalWord(90, 30), "tuners.signalPoor");
});

test("a tuner's host drops the scheme and port 80", () => {
  assert.equal(tunerHost("http://192.168.1.252:80/"), "192.168.1.252");
  assert.equal(tunerHost("http://10.0.0.2:5004"), "10.0.0.2:5004");
  assert.equal(tunerHost(undefined), "");
});

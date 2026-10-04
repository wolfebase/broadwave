import assert from "node:assert/strict";
import { test } from "node:test";
import { deviceScans, showFirmware, tunerLine } from "./src/features/settings/deviceCard.ts";

test("a discovered tuner scans, and a playlist does not", () => {
  assert.equal(deviceScans(undefined), true);
  assert.equal(deviceScans("hdhomerun"), true);
  assert.equal(deviceScans("hdhr-compatible"), true);
  for (const kind of ["m3u", "xtream", "link", "folder", "tvheadend", "channels", "free"]) {
    assert.equal(deviceScans(kind), false, kind);
  }
});

test("firmware is shown only when the device names a version", () => {
  assert.equal(showFirmware("20260101"), true);
  assert.equal(showFirmware(undefined), false);
  assert.equal(showFirmware(""), false);
  assert.equal(showFirmware("  "), false);
});

test("a tuner that is off is counted, not dropped", () => {
  assert.equal(tunerLine(0, 4, 4), "All 4 tuners are free.");
  assert.equal(tunerLine(0, 4, undefined), "All 4 tuners are free.");
  assert.equal(tunerLine(0, 4, 6), "All 4 tuners are free. 2 more don't answer.");
  assert.equal(tunerLine(1, 4, 5), "1 of 4 tuners are in use. 1 more doesn't answer.");
  assert.equal(tunerLine(0, 0, 2), "None of the 2 tuners answer. Check that they are on.");
  assert.equal(tunerLine(0, 0, undefined), "Tuner status is not available yet.");
});

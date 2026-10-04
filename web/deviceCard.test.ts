import assert from "node:assert/strict";
import { test } from "node:test";
import { deviceScans, showFirmware } from "./src/features/settings/deviceCard.ts";

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

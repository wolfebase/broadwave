import assert from "node:assert/strict";
import { test } from "node:test";
import { sameZone, zoneOffset } from "./src/features/setup/zone.ts";

const summer = new Date(Date.UTC(2026, 6, 1, 12));

test("zone offsets come from the zone's rules", () => {
  assert.equal(zoneOffset("UTC", summer), 0);
  assert.equal(zoneOffset("America/Denver", summer), -360);
  assert.equal(zoneOffset("Asia/Kolkata", summer), 330);
  assert.equal(zoneOffset("Not/AZone", summer), null);
});

test("server and browser zones are compared by their rules", () => {
  assert.ok(sameZone("America/Toronto", -14400, "America/Toronto", summer));
  assert.ok(sameZone("America/Detroit", -14400, "America/New_York", summer), "two names, one set of rules");
  assert.ok(!sameZone("America/Phoenix", -25200, "America/Denver", new Date(Date.UTC(2026, 0, 15))), "same offset in winter, not in summer");
  assert.ok(!sameZone("UTC", 0, "America/Vancouver", summer), "a container left on UTC");
});

test("a server with only an abbreviation is judged by its offset now", () => {
  assert.ok(sameZone("MDT", -21600, "America/Denver", summer));
  assert.ok(!sameZone("EDT", -14400, "America/Denver", summer));
});

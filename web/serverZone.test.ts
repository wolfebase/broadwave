import assert from "node:assert/strict";
import { test } from "node:test";
import { zoneProblem, zoneProblemKey } from "./src/lib/serverZone.ts";

test("a server on the browser's offset is fine", () => {
  assert.equal(zoneProblem({ timeZone: "America/Denver", timeZoneValid: true, utcOffsetMinutes: -360 }, "America/Boise", -360), null);
});

test("an older server that says nothing shows nothing", () => {
  assert.equal(zoneProblem({}, "America/Denver", -360), null);
  assert.equal(zoneProblem(null, "America/Denver", -360), null);
});

test("a mistyped TZ is named and the browser's zone suggested", () => {
  const p = zoneProblem({ timeZone: "America/Denvr", timeZoneValid: false, utcOffsetMinutes: 0 }, "America/Denver", -360);
  assert.deepEqual(p, { kind: "invalid", zone: "America/Denvr", suggest: "America/Denver" });
  assert.equal(zoneProblemKey(p!), "invalid:America/Denvr");
});

test("a server with no TZ runs on UTC", () => {
  assert.deepEqual(zoneProblem({ timeZone: "", timeZoneValid: true, utcOffsetMinutes: 0 }, "America/Chicago", -300), { kind: "unset", suggest: "America/Chicago" });
  // A browser on UTC can't tell.
  assert.equal(zoneProblem({ timeZone: "", timeZoneValid: true, utcOffsetMinutes: 0 }, "UTC", 0), null);
});

test("a different offset says how far off", () => {
  const p = zoneProblem({ timeZone: "America/New_York", timeZoneValid: true, utcOffsetMinutes: -240 }, "America/Los_Angeles", -420);
  assert.deepEqual(p, { kind: "differs", zone: "America/New_York", hours: 3, suggest: "America/Los_Angeles" });
  const half = zoneProblem({ timeZone: "America/Halifax", timeZoneValid: true, utcOffsetMinutes: -180 }, "America/St_Johns", -150);
  assert.equal(half?.kind === "differs" ? half.hours : 0, 0.5);
});

import assert from "node:assert/strict";
import { test } from "node:test";
import { pickLayout, type LayoutSignals } from "./src/lib/layoutPick.ts";

const base: LayoutSignals = { forced: "", remembered: "", narrow: false, wideCoarse: false, touchPoints: 0, userAgent: "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 Version/18.0 Safari/605.1.15" };
const ipad = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.0 Mobile/15E148 Safari/604.1";

test("an iPad in landscape gets the desktop layout, not the TV one", () => {
  assert.equal(pickLayout({ ...base, userAgent: ipad, wideCoarse: true, touchPoints: 5 }), "desktop");
  assert.equal(pickLayout({ ...base, userAgent: ipad, narrow: true, touchPoints: 5 }), "phone");
  // An iPad that names itself is a tablet even when it reports no touch points.
  assert.equal(pickLayout({ ...base, userAgent: "Mozilla/5.0 (iPad; CPU OS 12_2 like Mac OS X) AppleWebKit/605.1.15 Mobile/15E148 Safari/604.1", wideCoarse: true }), "desktop");
});

test("a wide screen with no fine pointer and no touch is a TV", () => {
  assert.equal(pickLayout({ ...base, wideCoarse: true }), "tv");
  assert.equal(pickLayout({ ...base }), "desktop");
});

test("TV browsers that name themselves get the TV layout at any size", () => {
  for (const ua of [
    "Mozilla/5.0 (SMART-TV; LINUX; Tizen 6.5) AppleWebKit/537.36 (KHTML, like Gecko) 85.0.4183.93/6.5 TV Safari/537.36",
    "Mozilla/5.0 (Web0S; Linux/SmartTV) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/94.0.4606.128 Safari/537.36 WebAppManager",
    "Mozilla/5.0 (Linux; Android 12; BRAVIA 4K VH2 Build/STT1.211025.001.Z4) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120 Mobile Safari/537.36",
    "Mozilla/5.0 (Linux; Android 9; AFTMM Build/PS7633) AppleWebKit/537.36 (KHTML, like Gecko) Silk/112 like Chrome/112 Safari/537.36",
  ]) {
    assert.equal(pickLayout({ ...base, userAgent: ua, narrow: true }), "tv", ua);
  }
});

test("an explicit choice wins, and a remembered one is used next time", () => {
  assert.equal(pickLayout({ ...base, userAgent: ipad, touchPoints: 5, wideCoarse: true, forced: "tv" }), "tv");
  assert.equal(pickLayout({ ...base, remembered: "tv" }), "tv");
  assert.equal(pickLayout({ ...base, wideCoarse: true, remembered: "desktop" }), "desktop");
  assert.equal(pickLayout({ ...base, forced: "phone", remembered: "tv" }), "phone");
  assert.equal(pickLayout({ ...base, forced: "nonsense" }), "desktop");
});

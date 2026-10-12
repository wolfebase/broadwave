import assert from "node:assert/strict";
import { test } from "node:test";
import { currentTitle, formatBytes, formatClockPoint } from "./src/lib/format.ts";
import type { Airing } from "./src/types.ts";

function airing(partial: Partial<Airing> & Pick<Airing, "channelId" | "start" | "end" | "title">): Airing {
  return { id: 1, ...partial };
}

test("a clock point stays at zero and pads the seconds", () => {
  assert.equal(formatClockPoint(0), "0:00");
  assert.equal(formatClockPoint(-4), "0:00");
  assert.equal(formatClockPoint(61.9), "1:01");
  assert.equal(formatClockPoint(3600), "60:00");
});

test("a byte count uses the largest unit that fits", () => {
  assert.equal(formatBytes(0), "0 B");
  assert.equal(formatBytes(999), "999 B");
  assert.equal(formatBytes(1000), "1 KB");
  assert.equal(formatBytes(1_500_000), "1.5 MB");
  assert.equal(formatBytes(2_000_000_000), "2.0 GB");
  assert.equal(formatBytes(3_000_000_000_000), "3.0 TB");
});

test("the title on now is the airing that contains this moment", () => {
  const now = Date.now();
  const list = [
    airing({ channelId: 4, title: "Earlier", start: new Date(now - 3_600_000).toISOString(), end: new Date(now - 60_000).toISOString() }),
    airing({ channelId: 4, title: "City News", start: new Date(now - 60_000).toISOString(), end: new Date(now + 60_000).toISOString() }),
    airing({ channelId: 7, title: "Other", start: new Date(now - 60_000).toISOString(), end: new Date(now + 60_000).toISOString() }),
    airing({ channelId: 4, title: "Next", start: new Date(now + 60_000).toISOString(), end: new Date(now + 3_600_000).toISOString() }),
  ];
  assert.equal(currentTitle(list, 4), "City News");
  assert.equal(currentTitle(list, 7), "Other");
  assert.equal(currentTitle(list, 9), "");
  assert.equal(currentTitle([], 4), "");
});

test("an airing that ends at this exact moment is already over", () => {
  const now = Date.now();
  const list = [
    airing({ channelId: 4, title: "Just ended", start: new Date(now - 60_000).toISOString(), end: new Date(now).toISOString() }),
  ];
  assert.equal(currentTitle(list, 4), "");
});

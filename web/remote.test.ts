import assert from "node:assert/strict";
import { test } from "node:test";
import { channelNumberContinues, firstBelow, nearest, typedChannel, type Box } from "./src/lib/remote.ts";

function box(id: number, left: number, top: number, width = 40, height = 20): Box {
  return { id, left, top, right: left + width, bottom: top + height };
}

test("arrows walk to the nearest control in that direction", () => {
  const tabs = [box(1, 100, 10, 80, 30), box(2, 200, 10, 80, 30), box(3, 300, 10, 80, 30)];
  const watch = box(4, 120, 200, 100, 40);
  const card = box(5, 110, 320, 120, 80);
  const items = [...tabs, watch, card];
  assert.equal(nearest(tabs[1], items, "right"), 3);
  assert.equal(nearest(tabs[1], items, "left"), 1);
  assert.equal(nearest(tabs[1], items, "down"), 4);
  assert.equal(nearest(watch, items, "down"), 5);
  assert.equal(nearest(card, items, "up"), 4);
  assert.equal(nearest(tabs[0], items, "left"), null);
});

test("51 is channel 5.1 after the pause, and a lone 5 tunes at once", () => {
  const channels = [{ displayNumber: "4.1" }, { displayNumber: "4.2" }, { displayNumber: "5.1" }, { displayNumber: "5.2" }];
  assert.equal(typedChannel(channels, "5", false), null);
  assert.equal(typedChannel(channels, "51", false), null);
  assert.equal(typedChannel(channels, "51", true)?.displayNumber, "5.1");
  assert.equal(typedChannel(channels, "5.1", false)?.displayNumber, "5.1");
  assert.equal(typedChannel(channels, "42", false), null);
  assert.equal(typedChannel(channels, "42", true)?.displayNumber, "4.2");
  const lone = channels.filter((channel) => channel.displayNumber !== "5.2");
  assert.equal(typedChannel(lone, "5", false)?.displayNumber, "5.1");
  assert.equal(channelNumberContinues(channels, "51"), true);
  assert.equal(channelNumberContinues(channels, "99"), false);
});

test("a wide row below the tabs wins over a nearer control off to the side", () => {
  const tab = box(1, 400, 10, 80, 30);
  const side = box(2, 40, 80, 80, 30);
  const grid = box(3, 0, 160, 800, 400);
  assert.equal(nearest(tab, [tab, side, grid], "down"), 3);
});

test("a wide button does not skip the nearer control it covers", () => {
  const from = box(1, 140, 1100, 1640, 40);
  const near = box(2, 232, 1177, 285, 40);
  const far = box(3, 800, 1600, 200, 40);
  assert.equal(nearest(from, [from, near, far], "down"), 2);
});

test("a tab still reaches a page that does not line up under it", () => {
  const tab = box(1, 760, 10, 80, 30);
  const button = box(2, 40, 120, 140, 40);
  assert.equal(nearest(tab, [tab, button], "down"), null);
  assert.equal(nearest(tab, [tab, button], "down", true), 2);
  assert.equal(firstBelow(tab, [tab, button]), 2);
});

test("down from a tab enters the first row under it, at the control nearest across", () => {
  const tab = box(1, 760, 10, 80, 30);
  const now = box(2, 40, 90, 60, 40);
  const chip = box(3, 740, 92, 90, 36);
  const search = box(4, 1300, 88, 300, 44);
  const card = box(5, 760, 300, 200, 120);
  assert.equal(firstBelow(tab, [tab, now, chip, search, card]), 3);
  assert.equal(firstBelow(box(6, 1400, 10, 80, 30), [now, chip, search, card]), 4);
});

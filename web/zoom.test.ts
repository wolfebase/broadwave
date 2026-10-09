import assert from "node:assert/strict";
import { test } from "node:test";
import { createZoomBus, zoomFromStorage } from "./src/picture.ts";

test("a same-tab zoom write notifies the player that already mounted", () => {
  const bus = createZoomBus();
  bus.hydrate("fit");
  let seen = bus.get();
  const stop = bus.subscribe(() => {
    seen = bus.get();
  });
  bus.set("fill");
  assert.equal(seen, "fill");
  assert.equal(zoomFromStorage("zoom"), "zoom");
  assert.equal(zoomFromStorage("nope"), "fit");
  stop();
  bus.set("zoom");
  assert.equal(seen, "fill");
});

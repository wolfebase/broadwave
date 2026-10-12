import assert from "node:assert/strict";
import { test } from "node:test";
import { readLiveDelay, saveLiveDelay } from "./src/lib/events.ts";

test("this screen's live delay is balanced until the viewer picks another", () => {
  const store = new Map<string, string>();
  Object.assign(globalThis, {
    localStorage: {
      getItem: (key: string) => store.get(key) ?? null,
      setItem: (key: string, value: string) => store.set(key, value),
      removeItem: (key: string) => store.delete(key),
    },
  });
  assert.equal(readLiveDelay(), "balanced");
  store.set("broadwave-live-delay", "nope");
  assert.equal(readLiveDelay(), "balanced");
  saveLiveDelay("lowest");
  assert.equal(readLiveDelay(), "lowest");
  saveLiveDelay("stable");
  assert.equal(store.get("broadwave-live-delay"), "stable");
  assert.equal(readLiveDelay(), "stable");
});

test("a blocked store still plays at the balanced delay", () => {
  Object.assign(globalThis, {
    localStorage: {
      getItem: () => {
        throw new Error("blocked");
      },
    },
  });
  assert.equal(readLiveDelay(), "balanced");
});

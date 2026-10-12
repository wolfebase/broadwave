import assert from "node:assert/strict";
import { test } from "node:test";
import { CACHE_FLAG, hasSnapshotFlag } from "./src/lib/snapshot.ts";

test("the home cache flag is on only after a snapshot was saved", () => {
  const store = new Map<string, string>();
  const localStorage = {
    getItem: (key: string) => store.get(key) ?? null,
    setItem: (key: string, value: string) => store.set(key, value),
    removeItem: (key: string) => store.delete(key),
  };
  Object.assign(globalThis, { localStorage });
  assert.equal(hasSnapshotFlag(), false);
  store.set(CACHE_FLAG, "1");
  assert.equal(hasSnapshotFlag(), true);
  store.set(CACHE_FLAG, "0");
  assert.equal(hasSnapshotFlag(), false);
});

test("a storage failure means there is no cached home", () => {
  Object.assign(globalThis, {
    localStorage: {
      getItem() {
        throw new Error("blocked");
      },
    },
  });
  assert.equal(hasSnapshotFlag(), false);
});

import assert from "node:assert/strict";
import { test } from "node:test";
import { layoutForCount, layoutFromParam, layoutLabel, multiviewPath, saveSet, savedSets } from "./src/features/multiview/storage.ts";

test("watch together picks a layout that fits the games", () => {
  assert.equal(layoutForCount(1), "2up");
  assert.equal(layoutForCount(2), "2up");
  assert.equal(layoutForCount(3), "1+2");
  assert.equal(layoutForCount(4), "quad");
  assert.equal(layoutForCount(6), "quad");
  assert.equal(layoutLabel("2up"), "Side by side");
  assert.equal(layoutLabel("1+2"), "One big and two");
  assert.equal(layoutLabel("quad"), "Quad");
});

test("a saved set keeps the layout it was saved with", () => {
  const store = new Map<string, string>();
  globalThis.localStorage = {
    getItem: (key: string) => store.get(key) ?? null,
    setItem: (key: string, value: string) => {
      store.set(key, value);
    },
    removeItem: (key: string) => {
      store.delete(key);
    },
    clear: () => {
      store.clear();
    },
    key: () => null,
    length: 0,
  };
  saveSet("Early games", [4, 9, 12], "1+2");
  const saved = savedSets();
  assert.equal(saved.length, 1);
  assert.equal(saved[0].name, "Early games");
  assert.deepEqual(saved[0].channels, [4, 9, 12]);
  assert.equal(saved[0].layout, "1+2");
  saveSet("Early games", [4, 9, 12], "quad");
  assert.equal(savedSets()[0].layout, "quad");
  assert.equal(savedSets().length, 1);
});

test("one big and two survives the query string", () => {
  const path = multiviewPath([1, 3, 5], "1+2", 1);
  const params = new URLSearchParams(path.slice(path.indexOf("?") + 1));
  assert.equal(params.get("layout"), "1+2");
  assert.equal(layoutFromParam("1 2"), "1+2");
  assert.equal(layoutFromParam("quad"), "quad");
  assert.equal(layoutFromParam("nope"), null);
});

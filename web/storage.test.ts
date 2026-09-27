import assert from "node:assert/strict";
import { test } from "node:test";
import { channelsOnScreen, holdShown, layoutForCount, layoutFromParam, layoutLabel, multiviewPath, rememberAuto, saveSet, savedAuto, savedSets, yieldsSound } from "./src/features/multiview/storage.ts";

test("a tile that already showed a picture keeps the sound", () => {
  assert.equal(yieldsSound("", false), false);
  assert.equal(yieldsSound("Every tuner is busy. Stop a recording or watch something already on.", false), true);
  assert.equal(yieldsSound("The picture stopped. Starting it again.", true), false);
  assert.equal(yieldsSound("The picture stopped. Trying again usually fixes it.", true), false);
});

test("a channel already on screen stays when a later plan blocks it", () => {
  const blocked = new Set([2]);
  assert.deepEqual(channelsOnScreen([1, 2], blocked, new Set(), 2), [1]);
  assert.deepEqual(channelsOnScreen([1, 2], blocked, new Set([2]), 2), [1, 2]);
  assert.deepEqual(channelsOnScreen([1, 2, 3], new Set(), new Set(), 2), [1, 2]);
  const held = holdShown(new Set(), [1, 2], blocked);
  assert.deepEqual([...held], [1]);
  assert.equal(holdShown(held, [1, 2], blocked), held);
  assert.deepEqual([...holdShown(held, [1, 2], new Set())], [1, 2]);
});

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

test("auto stays off until this browser turns it on", () => {
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
  assert.equal(savedAuto(), false);
  rememberAuto(true);
  assert.equal(savedAuto(), true);
  rememberAuto(false);
  assert.equal(savedAuto(), false);
});

test("one big and two survives the query string", () => {
  const path = multiviewPath([1, 3, 5], "1+2", 1);
  const params = new URLSearchParams(path.slice(path.indexOf("?") + 1));
  assert.equal(params.get("layout"), "1+2");
  assert.equal(layoutFromParam("1 2"), "1+2");
  assert.equal(layoutFromParam("quad"), "quad");
  assert.equal(layoutFromParam("nope"), null);
});

import assert from "node:assert/strict";
import { test } from "node:test";
import { readRecent, rememberChannel } from "./src/recent.ts";

function storage() {
  const store = new Map<string, string>();
  const localStorage = {
    getItem: (key: string) => store.get(key) ?? null,
    setItem: (key: string, value: string) => store.set(key, value),
    removeItem: (key: string) => store.delete(key),
    clear: () => store.clear(),
    key: (i: number) => [...store.keys()][i] ?? null,
    get length() {
      return store.size;
    },
  };
  Object.assign(globalThis, { localStorage });
  return store;
}

test("recent channels keep the last eight and put the one just watched first", () => {
  storage();
  assert.deepEqual(readRecent(), []);
  for (let id = 1; id <= 9; id += 1) {
    rememberChannel({ id, displayNumber: `${id}.1`, displayName: `Channel ${id}` });
  }
  const recent = readRecent();
  assert.equal(recent.length, 8);
  assert.equal(recent[0].id, 9);
  assert.equal(recent[0].name, "Channel 9");
  assert.equal(recent.some((item) => item.id === 1), false);
  rememberChannel({ id: 5, displayNumber: "5.1", displayName: "Channel 5" });
  const again = readRecent();
  assert.equal(again[0].id, 5);
  assert.equal(again.filter((item) => item.id === 5).length, 1);
});

test("a broken recent list is empty", () => {
  const store = storage();
  store.set("ota-recent", "not json");
  assert.deepEqual(readRecent(), []);
  store.set("ota-recent", '{"id":1}');
  assert.deepEqual(readRecent(), []);
  store.set("ota-recent", JSON.stringify([{ id: 1, number: "4.1", name: "News" }, { id: "x" }, null]));
  assert.deepEqual(readRecent(), [{ id: 1, number: "4.1", name: "News" }]);
});

import assert from "node:assert/strict";
import { test } from "node:test";
import { isSearchShortcut } from "./src/app/searchKey.ts";

const key = (k: string, extra: Partial<Parameters<typeof isSearchShortcut>[0]> = {}) => ({
  key: k,
  ctrlKey: false,
  metaKey: false,
  altKey: false,
  defaultPrevented: false,
  target: { tagName: "BUTTON" } as unknown as EventTarget,
  ...extra,
});

test("slash opens search from a page", () => {
  assert.equal(isSearchShortcut(key("/"), false), true);
  assert.equal(isSearchShortcut(key("?"), false), false);
});

test("a text field, a modifier, the player, or a dialog keeps the key", () => {
  assert.equal(isSearchShortcut(key("/", { target: { tagName: "INPUT", type: "search" } as unknown as EventTarget }), false), false);
  assert.equal(isSearchShortcut(key("/", { metaKey: true }), false), false);
  assert.equal(isSearchShortcut(key("/", { defaultPrevented: true }), false), false);
  assert.equal(isSearchShortcut(key("/"), true), false);
});

import assert from "node:assert/strict";
import { test } from "node:test";
import { playlistKeep } from "./src/features/settings/playlistKeep.ts";

const channels = [
  { name: "Harbor", id: "harbor.north" },
  { name: "Harbor", id: "harbor.south" },
  { name: "North Pier", id: "" },
];

test("two channels with one name stay apart by tvg-id", () => {
  assert.equal(playlistKeep(channels, [0, 1]), "harbor.north, harbor.south");
});

test("an empty id falls back to the name, in playlist order", () => {
  assert.equal(playlistKeep(channels, [2, 0]), "harbor.north, North Pier");
});

test("a missing id falls back to the name", () => {
  assert.equal(playlistKeep([{ name: "North Pier" }], [0]), "North Pier");
});

test("an index past the list is left out, and no picks keep nothing", () => {
  assert.equal(playlistKeep(channels, [9, 1]), "harbor.south");
  assert.equal(playlistKeep(channels, []), "");
});

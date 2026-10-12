import assert from "node:assert/strict";
import { test } from "node:test";
import { movedTo, onMoved } from "./src/features/player/moved.ts";

test("a channel that left tells whoever is still listening", () => {
  const heard: string[] = [];
  const stop = onMoved((name) => heard.push(name));
  movedTo("Den TV");
  stop();
  movedTo("Kitchen");
  assert.deepEqual(heard, ["Den TV"]);
});


import assert from "node:assert/strict";
import { test } from "node:test";
import { parseCaptionStyle } from "./src/features/player/captionStyle.ts";

test("a saved caption style is read back, and anything else falls back", () => {
  assert.deepEqual(parseCaptionStyle('{"size":"large","back":"solid","color":"yellow"}'), { size: "large", back: "solid", color: "yellow" });
  assert.deepEqual(parseCaptionStyle(null), { size: "medium", back: "box", color: "white" });
  assert.deepEqual(parseCaptionStyle("not json"), { size: "medium", back: "box", color: "white" });
  assert.deepEqual(parseCaptionStyle('{"size":"huge","back":"x"}'), { size: "medium", back: "box", color: "white" });
});

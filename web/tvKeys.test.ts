import assert from "node:assert/strict";
import { test } from "node:test";
import { canonicalKey, tvArrow, tvBackKind } from "./src/lib/tvKey.ts";

test("Back on a TV remote is Escape, and Backspace still deletes text", () => {
  assert.equal(tvBackKind({ key: "Escape" }), "escape");
  assert.equal(tvBackKind({ key: "Backspace" }), "backspace");
  assert.equal(tvBackKind({ key: "GoBack" }), "escape");
  assert.equal(tvBackKind({ key: "BrowserBack" }), "escape");
  assert.equal(tvBackKind({ key: "XF86Back" }), "escape");
  assert.equal(tvBackKind({ key: "Back" }), "escape");
  // LG webOS and Samsung Tizen send a code and no key name.
  assert.equal(tvBackKind({ key: "Unidentified", keyCode: 461 }), "escape");
  assert.equal(tvBackKind({ key: "Unidentified", keyCode: 10009 }), "escape");
  assert.equal(tvBackKind({ key: "", which: 461 }), "escape");
  // Fire TV's Back is Escape. Some builds only fill the code.
  assert.equal(tvBackKind({ key: "Unidentified", keyCode: 27 }), "escape");
  assert.equal(tvBackKind({ key: "Unidentified", keyCode: 8 }), "backspace");
  assert.equal(tvBackKind({ key: "a", keyCode: 461 }), "escape");
  assert.equal(tvBackKind({ key: "Enter" }), null);
  assert.equal(tvBackKind({ key: "ArrowLeft" }), null);
});

test("arrow keys still move when a TV browser omits the key name", () => {
  assert.equal(tvArrow({ key: "ArrowLeft" }), "left");
  assert.equal(tvArrow({ key: "ArrowDown" }), "down");
  assert.equal(tvArrow({ key: "Unidentified", keyCode: 37 }), "left");
  assert.equal(tvArrow({ key: "Unidentified", keyCode: 38 }), "up");
  assert.equal(tvArrow({ key: "Unidentified", keyCode: 39 }), "right");
  assert.equal(tvArrow({ key: "Unidentified", keyCode: 40 }), "down");
  // A real character keeps its meaning even if a code is attached.
  assert.equal(tvArrow({ key: "a", keyCode: 37 }), null);
  assert.equal(canonicalKey({ key: "Unidentified", keyCode: 39 }), "ArrowRight");
  assert.equal(canonicalKey({ key: "Unidentified", keyCode: 461 }), "Escape");
  assert.equal(canonicalKey({ key: "XF86Back" }), "Escape");
  assert.equal(canonicalKey({ key: "Backspace" }), "Backspace");
  assert.equal(canonicalKey({ key: "5" }), "5");
});

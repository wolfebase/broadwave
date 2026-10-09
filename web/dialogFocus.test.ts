import assert from "node:assert/strict";
import { test } from "node:test";
import { isTextField, trapTab } from "./src/lib/dialogFocus.ts";

test("Tab stays inside a dialog and wraps at the ends", () => {
  const list = ["close", "watch", "record"];
  assert.equal(trapTab(list, "watch", false), null);
  assert.equal(trapTab(list, "watch", true), null);
  assert.equal(trapTab(list, "record", false), "close");
  assert.equal(trapTab(list, "close", true), "record");
  assert.equal(trapTab(list, "outside", false), "close");
  assert.equal(trapTab(list, null, true), "record");
  assert.equal(trapTab([], "watch", false), null);
});

test("Backspace edits a text field and leaves a checkbox or a slider", () => {
  assert.equal(isTextField({ tagName: "INPUT", type: "text" }), true);
  assert.equal(isTextField({ tagName: "INPUT", type: "search" }), true);
  assert.equal(isTextField({ tagName: "TEXTAREA" }), true);
  assert.equal(isTextField({ tagName: "DIV", isContentEditable: true }), true);
  assert.equal(isTextField({ tagName: "INPUT", type: "checkbox" }), false);
  assert.equal(isTextField({ tagName: "INPUT", type: "range" }), false);
  assert.equal(isTextField({ tagName: "BUTTON" }), false);
  assert.equal(isTextField(null), false);
});

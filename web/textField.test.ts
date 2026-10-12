import assert from "node:assert/strict";
import { test } from "node:test";
import { isTextField } from "./src/lib/textField.ts";

test("backspace stays inside a text field and leaves a control", () => {
  assert.equal(isTextField(null), false);
  assert.equal(isTextField("nope"), false);
  assert.equal(isTextField({ isContentEditable: true }), true);
  assert.equal(isTextField({ tagName: "TEXTAREA" }), true);
  assert.equal(isTextField({ tagName: "INPUT" }), true);
  assert.equal(isTextField({ tagName: "INPUT", type: "SEARCH" }), true);
  assert.equal(isTextField({ tagName: "INPUT", type: "password" }), true);
  assert.equal(isTextField({ tagName: "INPUT", type: "checkbox" }), false);
  assert.equal(isTextField({ tagName: "INPUT", type: "range" }), false);
  assert.equal(isTextField({ tagName: "INPUT", type: "button" }), false);
  assert.equal(isTextField({ tagName: "BUTTON" }), false);
  assert.equal(isTextField({ tagName: "DIV" }), false);
});

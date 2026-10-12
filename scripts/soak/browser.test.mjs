import assert from "node:assert/strict";
import test from "node:test";
import { endWatch, recordPresses } from "./browser.mjs";

test("a navigation ends the watch with the page's own pagehide stop", async () => {
  let script = "";
  const page = {
    async evaluate(fn) {
      script = fn.toString();
    },
  };
  await endWatch(page);
  assert.match(script, /pagehide/);
  await endWatch(null);
});

test("record does not steal a recording that is already running", () => {
  assert.equal(recordPresses({ already: true, started: false }), 0);
  assert.equal(recordPresses({ already: true, started: true }), 0);
  assert.equal(recordPresses({ already: false, started: false }), 1);
  assert.equal(recordPresses({ already: false, started: true }), 2);
});

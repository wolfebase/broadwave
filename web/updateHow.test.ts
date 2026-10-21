import assert from "node:assert/strict";
import { test } from "node:test";
import { installKind, updateHint, updateSteps } from "./src/lib/updateHow.ts";

test("with no install kind every way to update gets one line", () => {
  const steps = updateSteps("");
  assert.deepEqual(steps.map((s) => s.label), ["Unraid", "Docker", "With the Broadwave updater", "Binary"]);
  assert.match(steps[0].text, /Apply Update/);
  assert.match(steps[1].text, /docker compose pull/);
});

test("a server that says how it was installed gets only its own steps", () => {
  assert.equal(installKind({ install: "unraid" }), "unraid");
  assert.equal(installKind({ install: "Compose" }), "docker");
  assert.equal(installKind({ install: "something-else" }), "");
  assert.equal(installKind({}), "");
  assert.equal(installKind(undefined), "");
  assert.deepEqual(updateSteps("binary").map((s) => s.label), ["Binary"]);
  assert.equal(updateHint({ updateHow: " Run the updater. " }), "Run the updater.");
  assert.equal(updateHint({ updateHow: 3 }), "");
});

import assert from "node:assert/strict";
import test from "node:test";
import { fileHlsConfig } from "./src/picture.ts";

test("a recording playlist reports a finite duration", () => {
  const cfg = fileHlsConfig(40);
  assert.equal(cfg.liveDurationInfinity, false);
  assert.equal(cfg.startPosition, 40);
});

import assert from "node:assert/strict";
import test from "node:test";
import { parseProcessTable } from "./sample.mjs";

test("a macOS zombie still counts when its name is gone", () => {
  const table = [
    "10 1 4000 SN /usr/local/bin/broadwave",
    "11 10 8000 SN /opt/homebrew/bin/ffmpeg",
    "12 10 0 Z <defunct>",
    "13 99 0 Z <defunct>",
    "14 10 1000 SN /bin/ls",
  ].join("\n");
  const got = parseProcessTable(table, 10);
  assert.equal(got.rss, 4000 * 1024);
  assert.equal(got.ffmpeg, 1);
  assert.equal(got.zombies, 1);
});

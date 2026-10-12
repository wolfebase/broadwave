import assert from "node:assert/strict";
import test from "node:test";
import { createViewer, freshSegments, onPlaylist } from "./hls.mjs";

test("a segment stays fetched after the playlist grows", () => {
  const seen = new Set();
  assert.deepEqual(freshSegments(seen, ["seg00000.m4s", "seg00001.m4s"]), ["seg00000.m4s", "seg00001.m4s"]);
  assert.deepEqual(freshSegments(seen, ["seg00000.m4s", "seg00001.m4s", "seg00002.m4s"]), ["seg00002.m4s"]);
  for (let i = 3; i < 120; i++) freshSegments(seen, [`seg${String(i).padStart(5, "0")}.m4s`]);
  assert.deepEqual(freshSegments(seen, ["seg00000.m4s"]), []);
});

test("a restarted playlist is fetched again", () => {
  const viewer = createViewer("hls 1");
  viewer.fetched.add("seg00001.m4s");
  viewer.mapFetched = true;
  viewer.window = { maxMsn: 500, edgePdt: 1 };
  const events = onPlaylist(viewer, {
    edgePdt: 2_000,
    map: "init.mp4",
    segments: [{ msn: 1, uri: "seg00001.m4s", gap: false, discontinuity: false }],
  });
  assert.match(events[0].detail, /restarted/);
  assert.equal(viewer.fetched.size, 0);
  assert.equal(viewer.mapFetched, false);
  assert.deepEqual(freshSegments(viewer.fetched, ["seg00001.m4s"]), ["seg00001.m4s"]);
});

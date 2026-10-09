import assert from "node:assert/strict";
import { test } from "node:test";
import { markersForPlayback, playbackGrowing, progressSaveAction, progressToStore, releaseFileVideo, seekableSkip, storedPlayhead, takeFileFatal } from "./src/features/recordings/filePlay.ts";

test("a fatal file error recovers once per kind, then shows", () => {
  const tried = { network: false, media: false };
  assert.equal(takeFileFatal("networkError", tried), "startLoad");
  assert.equal(takeFileFatal("networkError", tried), "show");
  assert.equal(takeFileFatal("mediaError", tried), "recoverMedia");
  assert.equal(takeFileFatal("mediaError", tried), "show");
  assert.equal(takeFileFatal("muxError", { network: false, media: false }), "show");
  assert.equal(takeFileFatal("otherError", { network: false, media: false }), "show");
});

test("a shown fatal does not spend the recovery for a kind that has not failed", () => {
  const tried = { network: false, media: false };
  assert.equal(takeFileFatal("keySystemError", tried), "show");
  assert.equal(takeFileFatal("mediaError", tried), "recoverMedia");
  assert.equal(takeFileFatal("networkError", tried), "startLoad");
});

test("playback arms one save and keeps it while the picture moves", () => {
  let armed = false;
  const actions: string[] = [];
  for (const time of [0.2, 5, 5.25, 5.5, 40, 1, 0]) {
    const action = progressSaveAction(armed, time);
    actions.push(action);
    if (action === "arm") armed = true;
  }
  assert.deepEqual(actions, ["idle", "arm", "keep", "keep", "keep", "idle", "idle"]);
  assert.equal(progressToStore(40), 40);
  assert.equal(progressToStore(1), null);
  assert.equal(progressToStore(0), null);
  // The file is still in its lead-in. The resume seek has not landed.
  assert.equal(progressToStore(4, 1200, false), null);
  assert.equal(progressToStore(4, 1200, true), 4);
  assert.equal(progressToStore(1205, 1200, false), 1205);
  assert.equal(progressToStore(4, 0, false), 4);
  assert.equal(storedPlayhead(4, { at: 0, known: false }, false), null);
  assert.equal(storedPlayhead(4, { at: 1200, known: true }, false), null);
  assert.equal(storedPlayhead(4, { at: 1200, known: true }, true), 4);
  assert.equal(storedPlayhead(30, { at: 0, known: true }, false), 30);
  assert.equal(storedPlayhead(1200, { at: 1200, known: true }, false), 1200);
});

test("releasing a file pauses it and reloads the element with no src", () => {
  const order: string[] = [];
  releaseFileVideo({
    pause() {
      order.push("pause");
    },
    removeAttribute(name) {
      order.push(name);
    },
    load() {
      order.push("load");
    },
  });
  assert.deepEqual(order, ["pause", "src", "load"]);
});

test("auto-skip waits until the marker end is seekable", () => {
  assert.equal(seekableSkip(20, 12), null);
  assert.equal(seekableSkip(20, 19.9), null);
  assert.equal(seekableSkip(20, Number.NEGATIVE_INFINITY), null);
  assert.equal(seekableSkip(Number.NaN, 30), null);
  assert.equal(seekableSkip(20, 20), 20);
  assert.equal(seekableSkip(20, 25), 20);
});

test("the next file does not use the previous recording's markers", () => {
  const previous = [
    { id: 1, recordingId: 4, start: 10, end: 20 },
    { id: 2, recordingId: 4, start: 30, end: 45 },
  ];
  const next = { id: 3, recordingId: 9, start: 5, end: 8 };
  assert.deepEqual(markersForPlayback(9, previous), []);
  assert.deepEqual(markersForPlayback(9, [...previous, next]), [next]);
  assert.deepEqual(markersForPlayback(4, previous), previous);
  assert.deepEqual(markersForPlayback(null, previous), []);
  assert.deepEqual(markersForPlayback(4, [{ id: 5, start: 1, end: 2 }]), []);
});

test("a recording that finishes during playback is not still growing", () => {
  assert.equal(playbackGrowing("recording", null), true);
  assert.equal(playbackGrowing("recording", true), true);
  assert.equal(playbackGrowing("recording", false), false);
  assert.equal(playbackGrowing("complete", true), false);
  assert.equal(playbackGrowing("failed", true), false);
  assert.equal(playbackGrowing("complete", null), false);
});

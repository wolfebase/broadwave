import assert from "node:assert/strict";
import { test } from "node:test";
import { bindFilePlayback, progressSaveAction, progressToStore, releaseFileVideo, samePlayback, storedPlayhead, takeFileFatal } from "./src/features/recordings/filePlay.ts";

function fakeVideo() {
  const listeners = new Map<string, Set<() => void>>();
  return {
    listeners,
    addEventListener(type: string, listener: () => void) {
      let set = listeners.get(type);
      if (!set) listeners.set(type, (set = new Set()));
      set.add(listener);
    },
    removeEventListener(type: string, listener: () => void) {
      listeners.get(type)?.delete(listener);
    },
    emit(type: string) {
      for (const listener of [...(listeners.get(type) ?? [])]) listener();
    },
  };
}

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

test("stopping file playback drops its resume listener and caption track", () => {
  const video = fakeVideo();
  const seeks: number[] = [];
  const track = { removed: false, remove() { this.removed = true; } };
  const stop = bindFilePlayback(video, () => seeks.push(12), { native: true, track });
  video.emit("progress");
  video.emit("loadedmetadata");
  assert.deepEqual(seeks, [12, 12]);
  stop();
  stop();
  video.emit("progress");
  video.emit("loadedmetadata");
  assert.deepEqual(seeks, [12, 12]);
  assert.equal(track.removed, true);
  assert.equal(video.listeners.get("progress")?.size ?? 0, 0);
  assert.equal(video.listeners.get("loadedmetadata")?.size ?? 0, 0);
});

test("a second file does not keep the first file's progress listener", () => {
  const video = fakeVideo();
  const seeks: number[] = [];
  const first = bindFilePlayback(video, () => seeks.push(4), { native: false });
  const second = bindFilePlayback(video, () => seeks.push(40), { native: false });
  first();
  video.emit("progress");
  assert.deepEqual(seeks, [40]);
  second();
  video.emit("progress");
  assert.deepEqual(seeks, [40]);
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

test("a tick from the previous file does not count for this one", () => {
  assert.equal(samePlayback(1, 1), true);
  assert.equal(samePlayback(1, 2), false);
  assert.equal(samePlayback(2, 1), false);
});

test("a restart does not store the playhead the save was waiting on", () => {
  const latest = { time: 80 };
  let armed = progressSaveAction(false, latest.time) === "arm";
  assert.equal(armed, true);
  // Start over moves the playhead and drops the waiting save.
  armed = false;
  latest.time = 0;
  assert.equal(progressSaveAction(armed, latest.time), "idle");
  assert.equal(progressToStore(latest.time), null);
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

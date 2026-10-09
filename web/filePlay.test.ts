import assert from "node:assert/strict";
import { test } from "node:test";
import { bindFilePlayback, releaseFileVideo, takeFileFatal } from "./src/features/recordings/filePlay.ts";

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

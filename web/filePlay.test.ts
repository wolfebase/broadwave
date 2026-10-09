import assert from "node:assert/strict";
import { test } from "node:test";
import { releaseFileVideo, takeFileFatal } from "./src/features/recordings/filePlay.ts";

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

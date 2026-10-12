import assert from "node:assert/strict";
import { test } from "node:test";
import { applySound, parseSound, readSound, saveSound, sleepDue, sleepSentence, sleepUntilFrom, soundRecord } from "./src/features/player/extras.ts";
import { copy } from "./src/strings.ts";

test("volume and mute round-trip", () => {
  assert.deepEqual(parseSound(null), { volume: 1, muted: false });
  assert.deepEqual(parseSound("nope"), { volume: 1, muted: false });
  assert.equal(soundRecord({ volume: 1.4, muted: true }), JSON.stringify({ volume: 1, muted: true }));
  assert.deepEqual(parseSound(soundRecord({ volume: 0.35, muted: false })), { volume: 0.35, muted: false });
});

test("sleep is 30, 60, or 90 minutes and then it is due", () => {
  const now = 1_000_000;
  assert.equal(sleepUntilFrom(15, now), null);
  assert.equal(sleepUntilFrom(30, now), now + 30 * 60_000);
  assert.equal(sleepDue(null, now), false);
  assert.equal(sleepDue(now + 30 * 60_000, now + 30 * 60_000 - 1), false);
  assert.equal(sleepDue(now + 30 * 60_000, now + 30 * 60_000), true);
  assert.equal(sleepSentence(30), copy.player.sleep30);
  assert.equal(sleepSentence(60), copy.player.sleep60);
  assert.equal(sleepSentence(90), copy.player.sleep90);
  assert.match(copy.player.sleep30, /^Stops in 30 minutes\.$/);
});

test("the saved level is what the next picture starts at, and a blocked store stays quiet", () => {
  const store = new Map<string, string>();
  Object.assign(globalThis, {
    localStorage: {
      getItem: (key: string) => store.get(key) ?? null,
      setItem: (key: string, value: string) => store.set(key, value),
      removeItem: (key: string) => store.delete(key),
    },
  });
  assert.deepEqual(readSound(), { volume: 1, muted: false });
  saveSound({ volume: -2, muted: true });
  assert.deepEqual(readSound(), { volume: 0, muted: true });
  const video = { volume: 1, muted: false };
  applySound(video as HTMLVideoElement);
  assert.equal(video.volume, 0);
  assert.equal(video.muted, true);

  Object.assign(globalThis, {
    localStorage: {
      getItem: () => {
        throw new Error("blocked");
      },
      setItem: () => {
        throw new Error("blocked");
      },
    },
  });
  assert.deepEqual(readSound(), { volume: 1, muted: false });
  assert.doesNotThrow(() => saveSound({ volume: 0.2, muted: false }));
});

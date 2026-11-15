import assert from "node:assert/strict";
import { test } from "node:test";
import { roomStart } from "./src/lib/roomStart.ts";
import { clampSoundDelay, readSoundDelay, saveSoundDelay, soundDelayChoices } from "./src/lib/soundDelay.ts";

test("the sound delay is kept in 25 ms steps between 300 early and 300 late", () => {
  assert.equal(clampSoundDelay(0), 0);
  assert.equal(clampSoundDelay(110), 100);
  assert.equal(clampSoundDelay(-13), -25);
  assert.equal(clampSoundDelay(-12), 0);
  assert.equal(clampSoundDelay(900), 300);
  assert.equal(clampSoundDelay(-900), -300);
  assert.equal(clampSoundDelay(Number.NaN), 0);
  const choices = soundDelayChoices();
  assert.equal(choices.length, 25);
  assert.equal(choices[0], -300);
  assert.equal(choices[12], 0);
  assert.equal(choices[24], 300);
});

test("the sound delay stays on this device and none is stored as nothing", () => {
  const store = new Map<string, string>();
  const fake = {
    getItem: (k: string) => store.get(k) ?? null,
    setItem: (k: string, v: string) => void store.set(k, v),
    removeItem: (k: string) => void store.delete(k),
  };
  const g = globalThis as { localStorage?: unknown };
  const before = g.localStorage;
  g.localStorage = fake;
  try {
    assert.equal(readSoundDelay(), 0);
    saveSoundDelay(140);
    assert.equal(readSoundDelay(), 150);
    store.set("broadwave-sound-delay", "garbage");
    assert.equal(readSoundDelay(), 0);
    saveSoundDelay(0);
    assert.equal(store.size, 0);
    g.localStorage = {
      getItem() {
        throw new Error("blocked");
      },
    };
    assert.equal(readSoundDelay(), 0);
  } finally {
    g.localStorage = before;
  }
});

test("a screen with late sound starts that far into the room's frame", () => {
  const frags = Array.from({ length: 20 }, (_, i) => ({ start: i, duration: 1, programDateTime: 1_000_000 + i * 1000 }));
  const st = { room: "r", channelId: 1, mode: "follow" as const, anchorServer: 50_000, anchorMedia: 1_005_000, rate: 1, latency: "balanced" as const, version: 1, members: 1 };
  assert.equal(roomStart(frags, st, 50_000), 5.5);
  assert.equal(roomStart(frags, st, 50_000, 300), 5.8);
  assert.equal(roomStart(frags, st, 50_000, -300), 5.2);
});

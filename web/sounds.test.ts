import assert from "node:assert/strict";
import { test } from "node:test";
import { soundFor, soundsOf } from "./src/features/player/sounds.ts";

const described = "public.accessibility.describes-video";

test("a master keeps one main mix, one other language, and described video", () => {
  const sounds = soundsOf([
    { name: "Described", lang: "en", default: false, characteristics: described },
    { name: "Spanish", lang: "es", default: false },
    { name: "English", lang: "en", default: true },
    { name: "French", lang: "fr", default: false },
  ]);
  assert.deepEqual(
    sounds.map((s) => s.role),
    ["main", "language", "described"],
  );
  assert.equal(sounds[0].name, "English");
  assert.equal(sounds[1].name, "Spanish");
  assert.equal(sounds[2].name, "Described");
});

test("a second default mix is not a second main track", () => {
  const sounds = soundsOf([
    { name: "English", default: true },
    { name: "Also English", default: true },
  ]);
  assert.deepEqual(sounds.map((s) => s.name), ["English"]);
});

test("described video wins when that rendition is also marked default", () => {
  const sounds = soundsOf([{ name: "Described", default: true, characteristics: `video,${described}` }]);
  assert.equal(sounds[0].role, "described");
});

test("a missing role plays the main mix", () => {
  const sounds = soundsOf([
    { name: "English", default: true },
    { name: "Spanish", lang: "es", default: false },
  ]);
  assert.equal(soundFor(sounds, "described")?.name, "English");
  assert.equal(soundFor(sounds, undefined)?.name, "English");
  assert.equal(soundFor(sounds, "language")?.lang, "es");
  assert.equal(soundFor([], "main"), undefined);
});

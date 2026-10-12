import assert from "node:assert/strict";
import { test } from "node:test";
import { fileHlsConfig, liveHlsConfig, markerAt, readSkip, readZoom, saveSkip, saveZoom } from "./src/picture.ts";

function storage() {
  const store = new Map<string, string>();
  Object.assign(globalThis, {
    localStorage: {
      getItem: (key: string) => store.get(key) ?? null,
      setItem: (key: string, value: string) => store.set(key, value),
      removeItem: (key: string) => store.delete(key),
      clear: () => store.clear(),
      key: (i: number) => [...store.keys()][i] ?? null,
      get length() {
        return store.size;
      },
    },
  });
  return store;
}

test("each screen size keeps a different live buffer, and a phone does not ask for a bigger picture", () => {
  const desktop = liveHlsConfig();
  assert.equal(desktop.backBufferLength, 90);
  assert.equal(desktop.liveSyncDurationCount, 3);
  assert.equal(desktop.capLevelToPlayerSize, false);
  assert.equal(desktop.lowLatencyMode, true);
  assert.equal(desktop.maxLiveSyncPlaybackRate, 1);
  assert.equal(desktop.enableCEA708Captions, false);
  assert.equal(desktop.interstitialsController, undefined);

  const phone = liveHlsConfig("phone");
  assert.equal(phone.backBufferLength, 30);
  assert.equal(phone.maxBufferLength, 16);
  assert.equal(phone.capLevelToPlayerSize, true);

  const tile = liveHlsConfig("tile");
  assert.equal(tile.backBufferLength, 20);
  assert.equal(tile.capLevelToPlayerSize, true);

  const tv = liveHlsConfig("tv");
  assert.equal(tv.liveSyncDurationCount, 4);
  assert.equal(tv.backBufferLength, 120);
  assert.equal(tv.capLevelToPlayerSize, false);
});

test("a recording starts at the saved spot only after the first two seconds", () => {
  assert.equal(fileHlsConfig(0).startPosition, -1);
  assert.equal(fileHlsConfig(2).startPosition, -1);
  assert.equal(fileHlsConfig(2.01).startPosition, 2.01);
  assert.equal(fileHlsConfig(Number.NaN).startPosition, -1);
});

test("a break marker holds the playhead until just before it ends, and the first overlap wins", () => {
  const markers = [
    { id: 1, start: 10, end: 20 },
    { id: 2, start: 19, end: 30 },
    { id: 3, start: 40, end: 40.04 },
  ];
  assert.equal(markerAt(markers, 10)?.id, 1);
  assert.equal(markerAt(markers, 19.94)?.id, 1);
  assert.equal(markerAt(markers, 19.95)?.id, 2);
  assert.equal(markerAt(markers, 9.99), undefined);
  assert.equal(markerAt(markers, 40), undefined);
  assert.equal(markerAt([], 10), undefined);
});

test("saved picture choices and the device token ride on the next request", () => {
  const store = storage();
  const headers: [string, string][] = [];
  const xhr = { setRequestHeader: (name: string, value: string) => headers.push([name, value]) };
  liveHlsConfig().xhrSetup(xhr as XMLHttpRequest);
  assert.deepEqual(headers, []);
  store.set("bw.deviceToken", "den-token");
  fileHlsConfig(10).xhrSetup(xhr as XMLHttpRequest);
  assert.deepEqual(headers, [["Authorization", "Bearer den-token"]]);
  store.delete("bw.deviceToken");

  store.set("ota-zoom", "fill");
  assert.equal(readZoom(), "fill");
  saveZoom("zoom");
  assert.equal(store.get("ota-zoom"), "zoom");
  assert.equal(readZoom(), "zoom");
  store.set("ota-zoom", "fit");
  assert.equal(readZoom(), "zoom");

  assert.equal(readSkip(), "auto");
  saveSkip("manual");
  assert.equal(readSkip(), "manual");
  store.set("ota-skip", "button");
  assert.equal(readSkip(), "button");
  store.set("ota-skip", "nope");
  assert.equal(readSkip(), "auto");
});

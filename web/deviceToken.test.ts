import assert from "node:assert/strict";
import { test } from "node:test";
import {
  isForbidden,
  isUnauthorized,
  mediaURL,
  noteIfUnauthorized,
  noteUnauthorized,
  onUnauthorized,
  readToken,
  settingsForWatcher,
  writeToken,
} from "./src/lib/deviceToken.ts";

function memory() {
  const items = new Map<string, string>();
  return {
    items,
    getItem: (key: string) => items.get(key) ?? null,
    setItem: (key: string, value: string) => {
      items.set(key, value);
    },
    removeItem: (key: string) => {
      items.delete(key);
    },
  };
}

test("a token is stored, cleared, and ignored when the store throws", () => {
  const store = memory();
  writeToken("abc", store);
  assert.equal(readToken(store), "abc");
  writeToken("", store);
  assert.equal(readToken(store), "");
  const blocked = {
    getItem: () => {
      throw new Error("private");
    },
    setItem: () => {
      throw new Error("private");
    },
    removeItem: () => {
      throw new Error("private");
    },
  };
  assert.equal(readToken(blocked), "");
  writeToken("abc", blocked);
  assert.equal(readToken(null), "");
});

test("a media url gains the token once, and only a 401 opens pairing", () => {
  assert.equal(mediaURL("http://127.0.0.1/live.m3u8", ""), "http://127.0.0.1/live.m3u8");
  assert.equal(mediaURL("http://127.0.0.1/live.m3u8?x=1", "a b"), "http://127.0.0.1/live.m3u8?x=1&access_token=a%20b");
  assert.equal(mediaURL("http://127.0.0.1/live.m3u8?access_token=kept", "new"), "http://127.0.0.1/live.m3u8?access_token=kept");
  let opens = 0;
  const stop = onUnauthorized(() => {
    opens += 1;
  });
  noteIfUnauthorized(500);
  noteIfUnauthorized(401);
  noteUnauthorized();
  stop();
  noteUnauthorized();
  assert.equal(opens, 2);
  assert.equal(isUnauthorized({ status: 401 }), true);
  assert.equal(isUnauthorized({ status: "401" }), false);
  assert.equal(isForbidden({ status: 403 }), true);
  assert.equal(isForbidden(null), false);
});

test("a watcher's saved picture wins over the server default", () => {
  const next = settingsForWatcher(
    { pictureMode: "broadcast", autoplay: "on", layout: "guide", deviceAuth: "" },
    { pictureMode: "film", autoplay: "", layout: "player" },
    "device",
  );
  assert.deepEqual(next, { pictureMode: "film", autoplay: "on", layout: "player", deviceAuth: "1" });
});

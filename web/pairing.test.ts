import assert from "node:assert/strict";
import { test } from "node:test";
import { TOKEN_KEY, authHeaders, canEditSignIn, codeAfterPoll, formatPairCode, isForbidden, isUnauthorized, kindLabel, mediaURL, normalizePairCode, noteIfUnauthorized, onUnauthorized, pairPollStep, readToken, scopeLabel, scopesForAccess, settingsForWatcher, writeToken } from "./src/lib/deviceToken.ts";

test("a pair code is six digits, with spaces and dashes ignored", () => {
  assert.equal(normalizePairCode("482913"), "482913");
  assert.equal(normalizePairCode("482 913"), "482913");
  assert.equal(normalizePairCode("482-913"), "482913");
  assert.equal(normalizePairCode("  48 29-13 "), "482913");
  assert.equal(normalizePairCode("48291"), null);
  assert.equal(normalizePairCode("4829137"), null);
  assert.equal(normalizePairCode("48291a"), null);
  assert.equal(normalizePairCode("48.2913"), null);
  assert.equal(normalizePairCode(""), null);
});

test("a six-digit code is shown with a space in the middle", () => {
  assert.equal(formatPairCode("482913"), "482 913");
  assert.equal(formatPairCode("000000"), "000 000");
  assert.equal(formatPairCode("482 913"), "482 913");
  assert.equal(formatPairCode("12345"), "12345");
  assert.equal(formatPairCode("abcdef"), "abcdef");
});

test("scopes are named in a fixed order", () => {
  assert.equal(scopeLabel([]), "Watch");
  assert.equal(scopeLabel(["watch"]), "Watch");
  assert.equal(scopeLabel(["record"]), "Record");
  assert.equal(scopeLabel(["admin"]), "Admin");
  assert.equal(scopeLabel(["admin", "watch", "record"]), "Watch, Record, Admin");
  assert.equal(scopeLabel(["watch", "watch", "admin"]), "Watch, Admin");
  assert.equal(scopeLabel(["other"]), "Watch");
});

test("a device kind has a short name", () => {
  assert.equal(kindLabel("phone"), "Phone");
  assert.equal(kindLabel("tv"), "TV");
  assert.equal(kindLabel("web"), "Browser");
  assert.equal(kindLabel("other"), "Device");
  assert.equal(kindLabel(""), "Device");
});

test("the device token is stored and sent as a bearer header", () => {
  const saved = new Map<string, string>();
  const storage = {
    getItem(key: string) {
      return saved.has(key) ? (saved.get(key) ?? null) : null;
    },
    setItem(key: string, value: string) {
      saved.set(key, value);
    },
    removeItem(key: string) {
      saved.delete(key);
    },
    clear() {
      saved.clear();
    },
    key: () => null,
    length: 0,
  };
  assert.equal(readToken(storage), "");
  writeToken("abc", storage);
  assert.equal(saved.get(TOKEN_KEY), "abc");
  assert.equal(readToken(storage), "abc");
  writeToken("", storage);
  assert.equal(saved.has(TOKEN_KEY), false);
  assert.equal(readToken(storage), "");
  assert.doesNotThrow(() => writeToken("x", null));
  assert.equal(readToken(null), "");

  const prior = "localStorage" in globalThis ? globalThis.localStorage : undefined;
  globalThis.localStorage = storage;
  try {
    writeToken("tok");
    assert.equal(readToken(), "tok");
    assert.deepEqual(authHeaders(), { Authorization: "Bearer tok" });
    writeToken("");
    assert.equal(readToken(), "");
    assert.deepEqual(authHeaders(), {});
  } finally {
    if (prior) globalThis.localStorage = prior;
  }
});

test("a blocked store is ignored", () => {
  const broken = {
    getItem() {
      throw new Error("blocked");
    },
    setItem() {
      throw new Error("blocked");
    },
    removeItem() {
      throw new Error("blocked");
    },
  };
  assert.equal(readToken(broken), "");
  assert.doesNotThrow(() => writeToken("abc", broken));
  assert.doesNotThrow(() => writeToken("", broken));
});

test("a media element can carry the token", () => {
  assert.equal(mediaURL("/media/live/1/index.m3u8", ""), "/media/live/1/index.m3u8");
  assert.equal(mediaURL("/media/live/1/index.m3u8", "bw_a"), "/media/live/1/index.m3u8?access_token=bw_a");
  assert.equal(mediaURL("/media/live/1/index.m3u8?x=1", "bw_a"), "/media/live/1/index.m3u8?x=1&access_token=bw_a");
  assert.equal(mediaURL("/media/live/1/index.m3u8?access_token=kept", "bw_a"), "/media/live/1/index.m3u8?access_token=kept");
});

test("one approved poll with no token keeps waiting", () => {
  let step = pairPollStep("pending", undefined, false);
  assert.equal(step.action, "wait");
  assert.equal(step.missed, false);
  step = pairPollStep("approved", undefined, step.missed);
  assert.equal(step.action, "wait");
  assert.equal(step.missed, true);
  step = pairPollStep("approved", "bw_x", step.missed);
  assert.equal(step.action, "ready");
  assert.equal(step.missed, false);
  step = pairPollStep("approved", "", true);
  assert.equal(step.action, "miss");
  assert.equal(pairPollStep("expired", undefined, false).action, "expired");
  assert.equal(pairPollStep("denied", undefined, false).action, "denied");
});

test("an access choice is the scopes the server stores", () => {
  assert.deepEqual(scopesForAccess("watch"), ["watch"]);
  assert.deepEqual(scopesForAccess("record"), ["watch", "record"]);
  assert.deepEqual(scopesForAccess("admin"), ["watch", "record", "admin"]);
  assert.equal(scopesForAccess("other"), null);
  assert.equal(scopesForAccess(""), null);
});

test("a missed, expired, or denied code leaves the screen", () => {
  assert.equal(codeAfterPoll("miss", "482913"), "");
  assert.equal(codeAfterPoll("expired", "482913"), "");
  assert.equal(codeAfterPoll("denied", "482913"), "");
  assert.equal(codeAfterPoll("wait", "482913"), "482913");
  assert.equal(codeAfterPoll("ready", "482913"), "482913");
});

test("a watch seat keeps the house picture choices and cannot edit sign-in", () => {
  const base = { pictureMode: "broadcast", autoplay: "1", layout: "auto", deviceAuth: "0", guideUrl: "http://example.test/guide" };
  const watched = settingsForWatcher(base, { pictureMode: "film", autoplay: "0", layout: "tv" }, "device");
  assert.equal(watched.pictureMode, "film");
  assert.equal(watched.autoplay, "0");
  assert.equal(watched.layout, "tv");
  assert.equal(watched.deviceAuth, "1");
  assert.equal(watched.guideUrl, "http://example.test/guide");
  const open = settingsForWatcher(base, null, "local-open");
  assert.equal(open.pictureMode, "broadcast");
  assert.equal(open.deviceAuth, "0");
  const blank = settingsForWatcher(base, { pictureMode: "", autoplay: "", layout: "" }, "");
  assert.equal(blank.pictureMode, "broadcast");
  assert.equal(blank.autoplay, "1");
  assert.equal(canEditSignIn(null), true);
  assert.equal(canEditSignIn({ auth: "local-open" }), true);
  assert.equal(canEditSignIn({ auth: "device", device: { scopes: ["watch"] } }), false);
  assert.equal(canEditSignIn({ auth: "device", device: { scopes: ["watch", "record"] } }), false);
  assert.equal(canEditSignIn({ auth: "device", device: { scopes: ["watch", "record", "admin"] } }), true);
  assert.equal(canEditSignIn({ auth: "device", device: null }), false);
});

test("a 401 from any request opens pairing, and a 403 does not", () => {
  const seen: string[] = [];
  const stop = onUnauthorized(() => seen.push("open"));
  try {
    noteIfUnauthorized(403);
    noteIfUnauthorized(200);
    assert.deepEqual(seen, []);
    noteIfUnauthorized(401);
    assert.deepEqual(seen, ["open"]);
  } finally {
    stop();
  }
  noteIfUnauthorized(401);
  assert.deepEqual(seen, ["open"]);
});

test("a 401 is unauthorized and anything else is not", () => {
  assert.equal(isUnauthorized({ status: 401 }), true);
  const err = new Error("no") as Error & { status: number };
  err.status = 401;
  assert.equal(isUnauthorized(err), true);
  assert.equal(isUnauthorized({ status: 403 }), false);
  assert.equal(isForbidden({ status: 403 }), true);
  assert.equal(isForbidden({ status: 401 }), false);
  assert.equal(isUnauthorized({ status: "401" }), false);
  assert.equal(isUnauthorized(new Error("no")), false);
  assert.equal(isUnauthorized(null), false);
  assert.equal(isUnauthorized(undefined), false);
  assert.equal(isUnauthorized("401"), false);
});

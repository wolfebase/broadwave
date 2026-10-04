import assert from "node:assert/strict";
import { test } from "node:test";
import { copy } from "./src/strings.ts";
import { lastSeenPhrase } from "./src/time.ts";

const now = Date.parse("2026-10-04T12:00:00Z");

test("last seen is a short relative time", () => {
  assert.equal(lastSeenPhrase(undefined, now), "");
  assert.equal(lastSeenPhrase("not-a-time", now), "");
  assert.equal(lastSeenPhrase("2026-10-04T11:59:40Z", now), "just now");
  assert.equal(lastSeenPhrase("2026-10-04T11:55:00Z", now), "5 minutes ago");
  assert.equal(lastSeenPhrase("2026-10-04T11:59:00Z", now), "1 minute ago");
  assert.equal(lastSeenPhrase("2026-10-04T11:00:00Z", now), "1 hour ago");
  assert.equal(lastSeenPhrase("2026-10-04T09:00:00Z", now), "3 hours ago");
  assert.equal(lastSeenPhrase("2026-10-03T12:00:00Z", now), "1 day ago");
  assert.equal(lastSeenPhrase("2026-10-01T12:00:00Z", now), "3 days ago");
});

test("an offline card names when the device was last seen", () => {
  assert.equal(copy.sources.offline("5 minutes ago"), "Offline. Last seen 5 minutes ago.");
  assert.equal(copy.sources.offline(""), "Offline.");
  assert.equal(copy.sources.streams(1, 2), "1 of 2 streams");
});

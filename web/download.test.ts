import assert from "node:assert/strict";
import { test } from "node:test";
import { recordingDownloadURL } from "./src/features/recordings/download.ts";

test("a recording still in progress has no file link", () => {
  assert.equal(recordingDownloadURL(4, "recording"), null);
  assert.equal(recordingDownloadURL(4, "complete"), "/api/v1/recordings/4/file");
  assert.equal(recordingDownloadURL(4, "stopped"), "/api/v1/recordings/4/file");
  assert.equal(recordingDownloadURL(4, "failed"), "/api/v1/recordings/4/file");
  assert.equal(recordingDownloadURL(4, "imported"), "/api/v1/recordings/4/file");
});

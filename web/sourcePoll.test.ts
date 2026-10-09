import assert from "node:assert/strict";
import { test } from "node:test";
import { applyDiagnostics, applySignalPoll, applyTunerPoll } from "./src/features/settings/sourcePoll.ts";

test("an older ticket returns null", () => {
  assert.equal(applyTunerPoll(1, 2, ["shown"], ["fresh"], ["ok"]), null);
});

test("tuners present and statuses null keeps the tuners from this attempt", () => {
  assert.deepEqual(applyTunerPoll(2, 2, ["shown"], ["fresh"], null), { tuners: ["fresh"], statuses: null });
});

test("tuners null keeps the list on screen", () => {
  assert.deepEqual(applyTunerPoll(2, 2, ["shown"], null, null), { tuners: ["shown"], statuses: null });
});

test("the newest ticket applies", () => {
  assert.deepEqual(applyTunerPoll(3, 3, ["shown"], ["fresh"], ["ok"]), { tuners: ["fresh"], statuses: ["ok"] });
});

test("a signal poll drops an older ticket and applies a newer one that finished", () => {
  assert.equal(applySignalPoll(1, 2, true, ["pier"]), null);
  assert.deepEqual(applySignalPoll(2, 2, false, ["pier"]), { running: false, rows: ["pier"] });
});

test("diagnostics drops an older ticket or a null body", () => {
  assert.equal(applyDiagnostics(1, 3, { os: "lab" }), null);
  assert.equal(applyDiagnostics(3, 3, null), null);
  assert.deepEqual(applyDiagnostics(3, 3, { os: "lab" }), { os: "lab" });
});

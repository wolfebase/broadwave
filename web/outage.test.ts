import assert from "node:assert/strict";
import { test } from "node:test";
import { aTunerIsFree, recoveryReady, serverStopped, tunerStopped, viewerFailure } from "./src/features/player/outage.ts";

test("a busy tuner tells the viewer what to stop, and comes back when one is free", () => {
  const failed = Object.assign(new Error("Every tuner is busy. Stop a recording or watch something already on."), {
    status: 409,
    code: "tuners_busy",
  });
  const got = viewerFailure(failed);
  assert.equal(got.recovery, "busy");
  assert.match(got.message, /tuner is busy/);
  assert.equal(recoveryReady("busy", { health: true, freeTuner: false, tunerAnswers: true }), false);
  assert.equal(recoveryReady("busy", { health: true, freeTuner: true, tunerAnswers: true }), true);
  assert.equal(aTunerIsFree([{ target: "127.0.0.1", guide: "4.1" }, { target: "127.0.0.1", guide: "5.1" }]), false);
  assert.equal(aTunerIsFree([{ target: "", guide: "" }, { target: "127.0.0.1", guide: "5.1" }]), true);
});

test("a server or tuner that stops answering is named, and waits until it answers", () => {
  const down = viewerFailure(new TypeError("Failed to fetch"));
  assert.equal(down.message, serverStopped);
  assert.equal(down.recovery, "server");
  assert.equal(recoveryReady("server", { health: false, freeTuner: false, tunerAnswers: false }), false);
  assert.equal(recoveryReady("server", { health: true, freeTuner: false, tunerAnswers: false }), true);

  const tuner = viewerFailure(Object.assign(new Error("the tuner did not answer"), { status: 500, code: "internal" }));
  assert.equal(tuner.message, tunerStopped);
  assert.equal(tuner.recovery, "tuner");
  assert.equal(recoveryReady("tuner", { health: true, freeTuner: true, tunerAnswers: false }), false);
  assert.equal(recoveryReady("tuner", { health: true, freeTuner: true, tunerAnswers: true }), true);
});

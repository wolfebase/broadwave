import assert from "node:assert/strict";
import { test } from "node:test";
import { aTunerIsFree, classifySnap, connectionDropped, noSignal, recoveryReady, serverStopped, tunerStopped, viewerFailure } from "./src/features/player/outage.ts";

test("a busy tuner tells the viewer what to stop, and comes back when one is free", () => {
  const failed = Object.assign(new Error("Every tuner is busy. Stop a recording or watch something already on."), {
    status: 409,
    code: "tuners_busy",
  });
  const got = viewerFailure(failed);
  assert.equal(got.recovery, "busy");
  assert.match(got.message, /tuner is busy/);
  assert.equal(recoveryReady("busy", { health: true, freeTuner: false, tunerAnswers: true, online: true, signalLost: false }), false);
  assert.equal(recoveryReady("busy", { health: true, freeTuner: true, tunerAnswers: true, online: true, signalLost: false }), true);
  assert.equal(aTunerIsFree([{ target: "127.0.0.1", guide: "4.1" }, { target: "127.0.0.1", guide: "5.1" }]), false);
  assert.equal(aTunerIsFree([{ target: "", guide: "" }, { target: "127.0.0.1", guide: "5.1" }]), true);
});

test("a server or tuner that stops answering is named, and waits until it answers", () => {
  const down = viewerFailure(new TypeError("Failed to fetch"));
  assert.equal(down.message, serverStopped);
  assert.equal(down.recovery, "server");
  assert.equal(recoveryReady("server", { health: false, freeTuner: false, tunerAnswers: false, online: true, signalLost: false }), false);
  assert.equal(recoveryReady("server", { health: true, freeTuner: false, tunerAnswers: false, online: true, signalLost: false }), true);

  const tuner = viewerFailure(Object.assign(new Error("the tuner did not answer"), { status: 500, code: "internal" }));
  assert.equal(tuner.message, tunerStopped);
  assert.equal(tuner.recovery, "tuner");
  assert.equal(recoveryReady("tuner", { health: true, freeTuner: true, tunerAnswers: false, online: true, signalLost: false }), false);
  assert.equal(recoveryReady("tuner", { health: true, freeTuner: true, tunerAnswers: true, online: true, signalLost: false }), true);
});

test("a dropped connection, and a channel with no signal, each wait for their own fix", () => {
  const offline = classifySnap({ health: false, freeTuner: false, tunerAnswers: false, online: false, signalLost: false });
  assert.equal(offline.message, connectionDropped);
  assert.equal(offline.recovery, "server");
  assert.equal(recoveryReady("server", { health: false, freeTuner: false, tunerAnswers: false, online: false, signalLost: false }), false);
  assert.equal(recoveryReady("server", { health: true, freeTuner: true, tunerAnswers: true, online: true, signalLost: false }), true);

  const down = classifySnap({ health: false, freeTuner: false, tunerAnswers: false, online: true, signalLost: false });
  assert.equal(down.message, serverStopped);

  const lost = classifySnap({ health: true, freeTuner: false, tunerAnswers: true, online: true, signalLost: true });
  assert.equal(lost.message, noSignal);
  assert.equal(lost.recovery, "signal");
  assert.equal(recoveryReady("signal", { health: true, freeTuner: false, tunerAnswers: true, online: true, signalLost: true }), false);
  assert.equal(recoveryReady("signal", { health: true, freeTuner: false, tunerAnswers: true, online: true, signalLost: false }), true);
});

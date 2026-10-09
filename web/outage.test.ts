import assert from "node:assert/strict";
import { test } from "node:test";
import { unreadBody } from "./src/api.ts";
import { aTunerAnswers, aTunerIsFree, autoplayRetry, channelDidNotStart, classifySnap, connectionDropped, FrozenPicture, frozenMs, holdPictureMessage, listingNote, liveFatalStep, noListing, noListingChecked, noSignal, outageAtWatchStart, pictureRestarting, pictureRetryDelay, pictureRetryEveryMs, pictureRetryForMs, pictureStopped, recoveryReady, requestFailed, restartDelayMs, serverStopped, startAttempts, startRetryMs, tunerStopped, viewerFailure, viewerMessage, watchResolution } from "./src/features/player/outage.ts";

test("checking for listings says so when nothing comes back", () => {
  assert.equal(listingNote(false), noListing);
  assert.equal(noListing, "No listing for this channel.");
  assert.equal(listingNote(true), noListingChecked);
  assert.equal(noListingChecked, "Still no listing for this channel.");
  assert.notEqual(listingNote(true), listingNote(false));
});

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

  const dark = viewerFailure(Object.assign(new Error(""), { status: 503, code: "no_signal" }));
  assert.equal(dark.message, noSignal);
  assert.equal(dark.recovery, "");

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

test("a home with only playlists never blames a tuner", () => {
  assert.equal(aTunerAnswers([]), true);
  assert.equal(aTunerAnswers([{ error: "This tuner did not answer. Check that it is on." }]), false);
  assert.equal(aTunerAnswers([{ error: "This tuner did not answer. Check that it is on." }, {}]), true);

  const stopped = classifySnap({ health: true, freeTuner: false, tunerAnswers: true, online: true, signalLost: false });
  assert.equal(stopped.message, pictureStopped);
  assert.equal(stopped.recovery, "");
  assert.equal(recoveryReady("", { health: true, freeTuner: false, tunerAnswers: true, online: true, signalLost: false }), false);
});

test("a stopped picture with no named cause tries again every 10s for 2 min", () => {
  const fires: number[] = [];
  let elapsed = 0;
  for (let i = 0; i < 20; i++) {
    const wait = pictureRetryDelay(pictureStopped, "", elapsed);
    if (wait == null) break;
    elapsed += wait;
    fires.push(elapsed);
  }
  assert.deepEqual(
    fires,
    Array.from({ length: pictureRetryForMs / pictureRetryEveryMs }, (_, i) => (i + 1) * pictureRetryEveryMs),
  );
  assert.equal(pictureRetryDelay(pictureStopped, "", elapsed), null);
  assert.equal(pictureRetryDelay(pictureStopped, "", 10_050), 9_950);
  assert.equal(pictureRetryDelay(pictureStopped, "", pictureRetryForMs), null);
  assert.equal(pictureRetryDelay(pictureStopped, "", -1), null);

  assert.equal(pictureRetryDelay(noSignal, "", 0), null);
  assert.equal(pictureRetryDelay(noSignal, "signal", 0), null);
  assert.equal(pictureRetryDelay(serverStopped, "server", 0), null);
  assert.equal(pictureRetryDelay(tunerStopped, "tuner", 0), null);
  assert.equal(pictureRetryDelay("Every tuner is busy. Stop a recording or watch something already on.", "busy", 0), null);
  assert.equal(pictureRetryDelay(pictureStopped, "server", 0), null);

  assert.equal(holdPictureMessage({ message: pictureStopped, recovery: "" }), true);
  assert.equal(holdPictureMessage({ message: "stream returned 503", recovery: "" }), true);
  assert.equal(holdPictureMessage({ message: noSignal, recovery: "" }), false);
  assert.equal(holdPictureMessage({ message: noSignal, recovery: "signal" }), false);
  assert.equal(holdPictureMessage({ message: serverStopped, recovery: "server" }), false);
  assert.equal(holdPictureMessage({ message: "Every tuner is busy.", recovery: "busy" }), false);
});

test("a playlist the server no longer has starts again at once, ahead of tuner and signal", () => {
  const snap = { health: true, freeTuner: false, tunerAnswers: false, online: true, signalLost: true, watchGone: true };
  assert.deepEqual(classifySnap(snap), { message: pictureRestarting, recovery: "restart" });
  assert.equal(recoveryReady("restart", snap), true);
  assert.equal(recoveryReady("restart", { ...snap, health: false }), false);
  assert.equal(classifySnap({ ...snap, health: false }).recovery, "server");
  assert.equal(classifySnap({ ...snap, watchGone: false }).recovery, "tuner");
  assert.equal(pictureRetryDelay(pictureRestarting, "restart", 0), null);
});

test("a dark tune the server gave back says no signal, not a restart", () => {
  const snap = { health: true, freeTuner: true, tunerAnswers: true, online: true, signalLost: true, watchGone: true };
  assert.deepEqual(classifySnap(snap), { message: noSignal, recovery: "signal" });
  assert.equal(classifySnap({ ...snap, signalLost: false }).recovery, "restart");
});

test("system text does not reach the viewer, and a server sentence does", () => {
  assert.equal(requestFailed, "That did not work. Try again.");
  assert.equal(unreadBody, requestFailed);
  assert.equal(channelDidNotStart, "This channel did not start. Try again.");
  assert.equal(viewerFailure({ status: 500, code: "http_500", message: "The server answered 500." }).message, channelDidNotStart);
  assert.equal(viewerFailure(new Error("Unexpected token < in JSON at position 0")).message, channelDidNotStart);
  assert.equal(viewerFailure(new Error("Internal Server Error")).message, channelDidNotStart);
  assert.equal(viewerMessage("The data couldn’t be read because it is missing."), channelDidNotStart);
  const noSource = "No source has this channel now. Check Sources in Settings.";
  assert.equal(viewerFailure(Object.assign(new Error(noSource), { status: 404, code: "no_source" })).message, noSource);
  assert.equal(
    viewerFailure({ status: 409, code: "pictures_full", message: "This server can play 4 pictures at once. Stop one." }).message,
    "This server can play 4 pictures at once. Stop one.",
  );
  assert.equal(viewerFailure(new Error(requestFailed)).message, requestFailed);
  assert.equal(viewerFailure({ status: 500, code: "nope" }).message, channelDidNotStart);
  const full = "All 2 streams from this playlist are in use. Stop one or raise the limit.";
  const refused = "The tuner would not start this channel. Try again.";
  const internal = "This channel did not start. The server log says why.";
  assert.deepEqual(viewerFailure({ status: 409, code: "streams_full", message: full }), { message: full, recovery: "" });
  assert.deepEqual(viewerFailure({ status: 503, code: "tuner_refused", message: refused }), { message: refused, recovery: "" });
  assert.deepEqual(viewerFailure({ status: 500, code: "internal", message: internal }), { message: internal, recovery: "" });
  assert.equal(viewerFailure(new Error("Bad Gateway")).message, channelDidNotStart);
});

test("a source that stopped sending goes on the quiet clock", () => {
  const failed = Object.assign(new Error("This channel's stream isn't answering. Trying again usually fixes it."), { status: 503, code: "stream_down" });
  const mapped = viewerFailure(failed);
  assert.deepEqual(mapped, { message: pictureStopped, recovery: "" });
  assert.notEqual(pictureRetryDelay(mapped.message, mapped.recovery, 0), null);
});

test("a full picture budget is asked again while the last layout's encodes free up", () => {
  assert.equal(startAttempts("pictures_full"), 10);
  assert.equal(startAttempts("tuners_busy"), 1);
  assert.equal(startAttempts(undefined), 1);
  assert.equal(startRetryMs, 2000);
});

test("a restart that fails at once waits longer each time", () => {
  assert.deepEqual([0, 1, 2, 3, 4, 5, 500].map(restartDelayMs), [0, 1000, 2000, 4000, 8000, 10_000, 10_000]);
});

test("six frozen steps still alternate, then the outage clock is told once", () => {
  const picture = new FrozenPicture();
  const got: Array<ReturnType<FrozenPicture["note"]>> = [];
  let now = 0;
  let time = 10;
  picture.note(time, true, now);
  now += 1000;
  time += 1;
  picture.note(time, true, now);
  const take = (n: number) => {
    for (let i = 0; i < n; i++) {
      now += 1000;
      const step = picture.note(time, true, now);
      if (step) got.push(step);
    }
  };
  take(20);
  picture.note(null, false, (now += 1000));
  take(50);
  assert.deepEqual(got, ["reload", "retune", "reload", "retune", "reload", "retune", "stopped"]);
  assert.equal(picture.reconnecting, false);
  const fresh = new FrozenPicture();
  assert.equal(fresh.note(5, true, 0), null);
  assert.equal(fresh.note(5, true, 60_000), null);

  // 20s of motion is the private settled window. A shorter run must not start over.
  for (let i = 0; i < 20; i++) {
    now += 1000;
    time += 1;
    assert.equal(picture.note(time, true, now), null);
  }
  now += 1000;
  time += 1;
  picture.note(time, true, now);
  now += frozenMs;
  assert.equal(picture.note(time, true, now), "reload");
});

test("a new watch drops the previous failure, and a quiet retune keeps it", () => {
  const prev = { error: pictureStopped, recovery: "" as const, needsConfirm: true };
  assert.deepEqual(outageAtWatchStart(false, prev), { error: "", recovery: "", needsConfirm: false });
  assert.deepEqual(outageAtWatchStart(true, prev), prev);
  assert.equal(outageAtWatchStart(true, prev), prev);
});

test("a watch that resolves after teardown does not mark the next attempt", () => {
  const prev = { channel: 4, n: 2 };
  const late = watchResolution(true, 4, prev);
  assert.equal(late.publish, false);
  assert.deepEqual(late.autoTries, prev);
  assert.equal(late.release, true);
  assert.deepEqual(watchResolution(false, 4, prev), { publish: true, autoTries: { channel: 4, n: 0 }, release: false });
});

test("a torn-down watch does not mute and play again", () => {
  assert.equal(autoplayRetry(true, "NotAllowedError"), "return");
  assert.equal(autoplayRetry(false, "NotAllowedError"), "mute-and-play");
  assert.equal(autoplayRetry(false, "AbortError"), "return");
  assert.equal(autoplayRetry(false, undefined), "return");
});

test("a fatal live error recovers once per kind before the viewer sees it", () => {
  const tried = { network: false, media: false };
  const quiet = { on: false, recovered: false };
  assert.equal(liveFatalStep("networkError", quiet, tried), "start-load");
  assert.equal(liveFatalStep("networkError", quiet, tried), "outage");
  assert.equal(liveFatalStep("mediaError", quiet, tried), "recover-media");
  assert.equal(liveFatalStep("mediaError", quiet, tried), "outage");
  assert.equal(liveFatalStep("muxError", { on: false, recovered: false }, { network: false, media: false }), "outage");
});

test("coming back from a hidden tab still holds a fatal that is not the one stale-buffer media error", () => {
  const tried = { network: false, media: false };
  assert.equal(liveFatalStep("mediaError", { on: true, recovered: false }, tried), "recover-quiet");
  assert.equal(tried.media, false);
  assert.equal(liveFatalStep("mediaError", { on: true, recovered: true }, tried), "hold-quiet");
  assert.equal(liveFatalStep("networkError", { on: true, recovered: false }, tried), "hold-quiet");
  assert.equal(tried.network, false);
  // The quiet window does not spend the recovery the picture gets once it is back.
  assert.equal(liveFatalStep("networkError", { on: false, recovered: false }, tried), "start-load");
});

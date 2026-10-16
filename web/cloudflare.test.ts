import assert from "node:assert/strict";
import { test } from "node:test";
import { addressOf, cleanLabel, countdown, hostFor, knownProblem, linkState, noticeOf, secondsLeft, stageOf, stepMarks, type CfStatus } from "./src/features/settings/cloudflareFlow.ts";

const tunnel = { state: "off" };

function status(extra: Partial<CfStatus>): CfStatus {
  return { state: "idle", managed: false, tunnel, ...extra };
}

test("each server state has one stage on the card", () => {
  assert.equal(stageOf(null, false), "loading");
  assert.equal(stageOf(null, true), "notHome");
  assert.equal(stageOf(status({ state: "idle" }), true), "notHome");
  assert.equal(stageOf(status({ state: "idle" }), false), "idle");
  assert.equal(stageOf(status({ state: "error", error: { code: "timeout", message: "" } }), false), "idle");
  assert.equal(stageOf(status({ state: "waiting", purpose: "setup" }), false), "waiting");
  assert.equal(stageOf(status({ state: "waiting", purpose: "remove" }), false), "removing");
  assert.equal(stageOf(status({ state: "authorized", purpose: "setup" }), false), "signedIn");
  assert.equal(stageOf(status({ state: "authorized", purpose: "remove" }), false), "removing");
  assert.equal(stageOf(status({ state: "provisioning", step: "dns" }), false), "working");
  assert.equal(stageOf(status({ state: "connected", managed: true }), false), "connected");
  assert.equal(stageOf(status({ state: "something new" }), false), "idle");
});

test("the countdown runs to zero and never below", () => {
  const now = Date.parse("2026-10-15T12:00:00Z");
  assert.equal(secondsLeft("2026-10-15T12:09:00Z", now), 540);
  assert.equal(secondsLeft("2026-10-15T12:00:00.400Z", now), 1);
  assert.equal(secondsLeft("2026-10-15T11:59:00Z", now), 0);
  assert.equal(secondsLeft(undefined, now), 0);
  assert.equal(secondsLeft("not a date", now), 0);
  assert.equal(countdown(540), "9:00");
  assert.equal(countdown(65), "1:05");
  assert.equal(countdown(0), "0:00");
  assert.equal(countdown(-3), "0:00");
});

test("the name becomes the address the server will make", () => {
  assert.equal(hostFor("tv", "example.com"), "tv.example.com");
  assert.equal(hostFor(" TV ", "example.com"), "tv.example.com");
  assert.equal(hostFor("tv.example.com", "example.com"), "tv.example.com");
  assert.equal(hostFor("living-room", "example.com"), "living-room.example.com");
  assert.equal(hostFor("a.b", "example.com"), "");
  assert.equal(hostFor("-tv", "example.com"), "");
  assert.equal(hostFor("t v", "example.com"), "");
  assert.equal(hostFor("", "example.com"), "");
  assert.equal(hostFor("x".repeat(64), "example.com"), "");
  // Without the domain's name, the whole name is needed.
  assert.equal(hostFor("tv", ""), "");
  assert.equal(hostFor("tv.example.com", ""), "tv.example.com");
  assert.equal(hostFor("example.com", ""), "");
  assert.equal(cleanLabel("TV.Example.com.", "example.com"), "tv");
  assert.equal(addressOf("tv.example.com"), "https://tv.example.com");
  assert.equal(addressOf(""), "");
});

test("setup steps mark what is done, what runs, and what is next", () => {
  assert.deepEqual(stepMarks("tunnel"), { tunnel: "now", dns: "next", connect: "next" });
  assert.deepEqual(stepMarks("dns"), { tunnel: "done", dns: "now", connect: "next" });
  assert.deepEqual(stepMarks("connect"), { tunnel: "done", dns: "done", connect: "now" });
  assert.deepEqual(stepMarks(undefined), { tunnel: "now", dns: "next", connect: "next" });
  assert.deepEqual(stepMarks("unknown"), { tunnel: "now", dns: "next", connect: "next" });
});

test("the tunnel dot, known problems, and notices", () => {
  assert.equal(linkState({ state: "connected" }), "connected");
  assert.equal(linkState({ state: "starting" }), "connecting");
  assert.equal(linkState({ state: "error" }), "down");
  assert.equal(linkState({ state: "missing" }), "down");
  assert.equal(linkState(undefined), "down");
  assert.equal(knownProblem("no_zone"), "no_zone");
  assert.equal(knownProblem("timeout"), "timeout");
  assert.equal(knownProblem("setup_failed"), null);
  assert.equal(knownProblem(undefined), null);
  assert.equal(noticeOf(null), null);
  assert.deepEqual(noticeOf(status({ state: "error", error: { code: "timeout", message: "m" } })), { kind: "problem", problem: { code: "timeout", message: "m" } });
  // A taken name shows on the signed-in form, not as a notice.
  assert.equal(noticeOf(status({ state: "authorized", error: { code: "name_taken", message: "m" } })), null);
  assert.deepEqual(noticeOf(status({ state: "idle", dnsLeft: "tv.example.com" })), { kind: "dnsLeft", host: "tv.example.com" });
  assert.equal(noticeOf(status({ state: "waiting", dnsLeft: "tv.example.com" })), null);
});

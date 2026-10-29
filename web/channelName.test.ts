import assert from "node:assert/strict";
import { test } from "node:test";
import { channelName } from "./src/lib/channelName.ts";

const name = (displayName: string) => channelName({ displayName });

test("feed suffixes come off a call sign", () => {
  assert.equal(name("KRVTDT1"), "KRVT");
  assert.equal(name("KRVTDT3"), "KRVT");
  assert.equal(name("WRVT-DT"), "WRVT");
  assert.equal(name("KRVT-HD"), "KRVT");
  assert.equal(name("KRVT-1"), "KRVT");
  assert.equal(name("KRV-LD"), "KRV");
});

test("names that are not a call sign with a feed suffix stay", () => {
  assert.equal(name("KRVT"), "KRVT");
  assert.equal(name("KRVT365"), "KRVT365");
  assert.equal(name("KRVTOUT"), "KRVTOUT");
  assert.equal(name("ANT TV"), "ANT TV");
  assert.equal(name("Me TV"), "Me TV");
  assert.equal(name("WHD"), "WHD");
  assert.equal(name("Snapshp"), "Snapshp");
});

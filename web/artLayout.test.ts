import assert from "node:assert/strict";
import { test } from "node:test";
import { artLayout, displayEdge } from "./src/lib/art.ts";

test("full-bleed art is a wide picture at least 1280 px, within 1.25× the slot", () => {
  assert.equal(artLayout(1280, 720, 1600), "bleed");
  assert.equal(artLayout(1920, 1080, 1920), "bleed");
  assert.equal(artLayout(1279, 720, 1279), "composed");
  assert.equal(artLayout(1280, 1280, 1280), "composed");
  assert.equal(artLayout(1280, 0, 1280), "composed");
  assert.equal(artLayout(1280, 720, 0), "composed");
  assert.equal(artLayout(1280, 720, 1601), "composed");
  assert.equal(artLayout(720, 1280, 720), "composed");
});

test("the drawn edge stays within 1.25× the picture and zero means unknown", () => {
  assert.equal(displayEdge(0, 400), 0);
  assert.equal(displayEdge(-8, 400), 0);
  assert.equal(displayEdge(400, 0), 0);
  assert.equal(displayEdge(400, -1), 0);
  assert.equal(displayEdge(400, 499), 499);
  assert.equal(displayEdge(400, 500), 500);
  assert.equal(displayEdge(400, 800), 500);
  assert.equal(displayEdge(1, 2), 1);
  assert.equal(displayEdge(5, 6), 6);
  assert.equal(displayEdge(5, 7), 6);
});

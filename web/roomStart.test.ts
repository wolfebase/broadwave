import assert from "node:assert/strict";
import { test } from "node:test";
import type { RoomState } from "./src/lib/events.ts";
import { fragTime, roomStart, roomTarget } from "./src/lib/roomStart.ts";

function room(partial: Partial<RoomState>): RoomState {
  return {
    room: "den",
    channelId: 4,
    mode: "follow",
    anchorServer: 1_000,
    anchorMedia: 5_000,
    rate: 1,
    latency: "balanced",
    version: 1,
    members: 2,
    ...partial,
  };
}

test("a paused room stays on its anchor and a playing room moves with the clock", () => {
  assert.equal(roomTarget(room({ rate: 0 }), 4_000), 5_000);
  assert.equal(roomTarget(room({ rate: 1 }), 2_500), 6_500);
});

test("a join starts half a second ahead when a fragment holds that frame", () => {
  const frags = [{ start: 10, duration: 6, programDateTime: 5_000 }];
  assert.equal(fragTime(frags, 6_500), 11.5);
  assert.equal(roomStart(frags, room({ rate: 1 }), 1_000), 10.5);
  assert.equal(roomStart(frags, room({ rate: 1 }), 20_000), null);
  assert.equal(fragTime([{ start: 0, duration: 0, programDateTime: 1 }], 1), null);
});

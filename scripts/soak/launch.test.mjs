import assert from "node:assert/strict";
import test from "node:test";
import { soakPorts } from "./launch.mjs";

test("the stack stays on the soak ports", () => {
  assert.deepEqual(soakPorts(19180), { server: 19180, fakeHttp: 19181, fakeControl: 19182 });
  assert.throws(() => soakPorts(8477), /outside the soak ports/);
  assert.throws(() => soakPorts(19188), /outside the soak ports/);
});

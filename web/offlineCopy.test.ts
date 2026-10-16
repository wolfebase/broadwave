import assert from "node:assert/strict";
import { test } from "node:test";
import { rememberServer, unreachableLine } from "./src/app/offline.ts";

test("a server that does not answer is named, and the saved guide is said", () => {
  const store = new Map<string, string>();
  const saved = Object.getOwnPropertyDescriptor(globalThis, "localStorage");
  Object.defineProperty(globalThis, "localStorage", {
    configurable: true,
    value: { getItem: (k: string) => store.get(k) ?? null, setItem: (k: string, v: string) => void store.set(k, v) },
  });
  try {
    const down = new TypeError("Failed to fetch");
    assert.equal(unreachableLine(down, true), "Can't reach Broadwave. Showing the saved guide.");
    assert.equal(unreachableLine(down, false), "Can't reach Broadwave. Trying again.");
    rememberServer("Broadwave on TUS");
    assert.equal(unreachableLine(down, true), "Can't reach Broadwave on TUS. Showing the saved guide.");
    assert.equal(unreachableLine(Object.assign(new Error(""), { status: 502 }), false), "Can't reach Broadwave on TUS. Trying again.");
    // A real answer from the server keeps its own message.
    assert.equal(unreachableLine(Object.assign(new Error("Disk full"), { status: 500 }), true), null);
    assert.equal(unreachableLine(new Error("other"), true), null);
  } finally {
    if (saved) Object.defineProperty(globalThis, "localStorage", saved);
    else delete (globalThis as { localStorage?: unknown }).localStorage;
  }
});

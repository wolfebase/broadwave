import assert from "node:assert/strict";
import { test } from "node:test";
import { installBlocked, offerInstall, readDismissed } from "./src/lib/install.ts";

test("the install offer waits for the browser, and stays away once refused or installed", () => {
  const ready = { standalone: false, dismissed: false, hasPrompt: true };
  assert.equal(offerInstall(ready), true);
  assert.equal(offerInstall({ ...ready, hasPrompt: false }), false);
  assert.equal(offerInstall({ ...ready, standalone: true }), false);
  assert.equal(offerInstall({ ...ready, dismissed: true }), false);
});

test("a dismissal is the stored flag and nothing else", () => {
  assert.equal(readDismissed({ getItem: () => "1" }), true);
  assert.equal(readDismissed({ getItem: () => "0" }), false);
  assert.equal(readDismissed({ getItem: () => null }), false);
  assert.equal(readDismissed(null), false);
  assert.equal(
    readDismissed({
      getItem: () => {
        throw new Error("private");
      },
    }),
    false,
  );
});

test("a private window that hides storage is not asked again", () => {
  const globals = globalThis as {
    localStorage?: Storage;
    navigator?: Navigator;
    window?: Window & typeof globalThis;
  };
  const saved = {
    storage: Object.getOwnPropertyDescriptor(globalThis, "localStorage"),
    navigator: Object.getOwnPropertyDescriptor(globalThis, "navigator"),
    window: Object.getOwnPropertyDescriptor(globalThis, "window"),
  };
  const restore = () => {
    for (const key of ["localStorage", "navigator", "window"] as const) {
      const desc = saved[key === "localStorage" ? "storage" : key];
      if (desc) Object.defineProperty(globalThis, key, desc);
      else delete globals[key];
    }
  };
  const browser = (storage: "throw" | "1" | null, standalone: boolean) => {
    Object.defineProperty(globalThis, "localStorage", {
      configurable: true,
      get() {
        if (storage === "throw") throw new Error("private");
        return { getItem: () => storage };
      },
    });
    Object.defineProperty(globalThis, "navigator", { configurable: true, value: {} });
    Object.defineProperty(globalThis, "window", {
      configurable: true,
      value: { matchMedia: () => ({ matches: standalone }) },
    });
  };
  try {
    browser("throw", false);
    assert.equal(installBlocked(), true);
    browser(null, false);
    assert.equal(installBlocked(), false);
    browser("1", false);
    assert.equal(installBlocked(), true);
    browser(null, true);
    assert.equal(installBlocked(), true);
  } finally {
    restore();
  }
});

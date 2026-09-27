import { mkdirSync, readFileSync, writeFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { setTimeout as sleep } from "node:timers/promises";
import type { CDPSession, Page } from "@playwright/test";
import { expect, test } from "./fixture";
import { settle } from "./snap";

const here = path.dirname(fileURLToPath(import.meta.url));
const evidence = path.resolve(here, "../../.evidence/lane/l13");
// Five minutes. A shorter absence still sits inside the buffer the tab left with.
const awayMs = Number(process.env.E2E_TAB_MS || 300_000);

type Snap = {
  width: number;
  paused: boolean;
  time: number;
  alert: string;
  stalls: number;
  latency: number | null;
  drift: string;
  offset: string;
  err: string;
  visibility: string;
};

function channelId() {
  const runtime = JSON.parse(readFileSync(path.join(here, ".run/runtime.json"), "utf8")) as { channels: { id: number }[] };
  const id = runtime.channels[0]?.id;
  if (!id) throw new Error("no channel");
  return id;
}

async function openChannel(page: Page, id: number) {
  await page.goto(`/watch?channel=${id}`);
  await settle(page);
  const setup = page.getByRole("heading", { name: "Let's set up your TV" });
  const player = page.getByRole("region", { name: "Player" });
  await expect(setup.or(player)).toBeVisible();
  if (await setup.isVisible()) {
    const cont = page.getByRole("button", { name: "Continue" });
    if (await cont.isVisible()) await cont.click();
    await page.getByRole("button", { name: "Watch", exact: true }).click();
    await expect(player).toBeVisible();
  }
  if (!page.url().includes(`channel=${id}`)) {
    await page.goto(`/watch?channel=${id}`);
    await settle(page);
  }
  await expect(player).toBeVisible();
}

async function snap(page: Page): Promise<Snap> {
  return page.locator("video.stage-video").evaluate((video: HTMLVideoElement & { hls?: { latency?: number } }) => ({
    width: video.videoWidth,
    paused: video.paused,
    time: video.currentTime,
    alert: document.querySelector("[role='alert']")?.textContent || "",
    stalls: Number(video.dataset.stalls || 0),
    latency: video.hls?.latency ?? null,
    drift: video.dataset.syncDrift || "",
    offset: video.dataset.syncOffset || "",
    err: video.dataset.hlsError || "",
    visibility: document.visibilityState,
  }));
}

function near(s: Snap) {
  const latencyOk = s.latency != null && s.latency < 15;
  const driftOk = s.drift !== "" && Math.abs(Number(s.drift)) < 3000;
  return latencyOk || driftOk;
}

/** Picture moving, on the live edge or the room, with no error. */
async function backWithin(page: Page, budgetMs: number) {
  const start = Date.now();
  let seen = -1;
  let last = "";
  while (Date.now() - start < budgetMs) {
    const s = await snap(page);
    last = JSON.stringify(s);
    if (s.alert) throw new Error(s.alert);
    if (s.width > 0 && !s.paused && near(s)) {
      if (seen >= 0 && s.time > seen + 0.05) return { ms: Date.now() - start, ...s };
      if (seen < 0) seen = s.time;
    } else {
      seen = -1;
    }
    await page.waitForTimeout(100);
  }
  throw new Error(`picture was not back within ${budgetMs}ms (${last})`);
}

async function offsetsClose(a: Page, b: Page, budgetMs: number) {
  const start = Date.now();
  let last = "";
  while (Date.now() - start < budgetMs) {
    const left = await snap(a);
    const right = await snap(b);
    const gap = Math.abs(Number(left.offset) - Number(right.offset));
    last = JSON.stringify({ gap, left, right });
    if (left.offset !== "" && right.offset !== "" && left.width > 0 && right.width > 0 && !left.paused && !right.paused && gap < 50) {
      return { ms: Date.now() - start, gap, left, right };
    }
    await a.waitForTimeout(200);
  }
  throw new Error(`tabs were not within 50ms (${last})`);
}

async function freeze(page: Page): Promise<CDPSession> {
  const client = await page.context().newCDPSession(page);
  await client.send("Page.setWebLifecycleState", { state: "frozen" });
  return client;
}

async function thaw(client: CDPSession) {
  await client.send("Page.setWebLifecycleState", { state: "active" });
}

/** Focus emulation plus a hidden document, then the visibility event. */
async function setVisible(page: Page, visible: boolean) {
  const client = await page.context().newCDPSession(page);
  await client.send("Emulation.setFocusEmulationEnabled", { enabled: visible });
  const got = await page.evaluate((show: boolean) => {
    const state = show ? "visible" : "hidden";
    const define = (obj: object) => {
      Object.defineProperty(obj, "visibilityState", { configurable: true, get: () => state });
      Object.defineProperty(obj, "hidden", { configurable: true, get: () => !show });
    };
    let applied: string;
    try {
      define(Document.prototype);
      applied = "prototype";
    } catch {
      try {
        define(document);
        applied = "document";
      } catch {
        applied = "event-only";
      }
    }
    document.dispatchEvent(new Event("visibilitychange"));
    return { applied, hidden: document.hidden, state: document.visibilityState };
  }, visible);
  return { client, ...got };
}

test("a tab that was frozen or hidden comes back on the edge", async ({ page, context }) => {
  test.skip(process.env.E2E_TAB !== "1", "Set E2E_TAB=1 to freeze and hide a playing tab.");
  test.setTimeout(awayMs * 2 + 240_000);
  const id = channelId();
  await page.setViewportSize({ width: 1440, height: 900 });
  await openChannel(page, id);
  await expect
    .poll(() => snap(page).then((s) => s.width > 0 && !s.paused && s.offset !== ""), { timeout: 40_000 })
    .toBe(true);

  const other = await context.newPage();
  await other.setViewportSize({ width: 1440, height: 900 });
  await openChannel(other, id);
  await expect
    .poll(() => snap(other).then((s) => s.width > 0 && !s.paused && s.offset !== ""), { timeout: 40_000 })
    .toBe(true);
  const locked = await offsetsClose(page, other, 20_000);

  const frozenA = await freeze(page);
  const frozenB = await freeze(other);
  await sleep(awayMs);
  await thaw(frozenA);
  await thaw(frozenB);
  const thawed = Date.now();
  const [freezeBackA, freezeBackB] = await Promise.all([backWithin(page, 3000), backWithin(other, 3000)]);
  const freezePair = await offsetsClose(page, other, Math.max(1000, 15_000 - (Date.now() - thawed)));
  const freezeStalls = freezeBackA.stalls;
  await sleep(8000);
  const freezeAfter = await snap(page);
  if (freezeAfter.alert) throw new Error(freezeAfter.alert);
  if (freezeAfter.stalls - freezeStalls > 3) throw new Error(`stall loop after freeze (${freezeAfter.stalls - freezeStalls})`);

  mkdirSync(evidence, { recursive: true });
  await page.screenshot({ path: path.join(evidence, "freeze.jpg"), animations: "disabled" });

  const hideA = await setVisible(page, false);
  const hideB = await setVisible(other, false);
  expect(hideA.hidden, JSON.stringify({ applied: hideA.applied, state: hideA.state })).toBe(true);
  expect(hideB.hidden, JSON.stringify({ applied: hideB.applied, state: hideB.state })).toBe(true);
  await sleep(awayMs);
  const showA = await setVisible(page, true);
  const showB = await setVisible(other, true);
  const shown = Date.now();
  const [hideBackA, hideBackB] = await Promise.all([backWithin(page, 3000), backWithin(other, 3000)]);
  const hidePair = await offsetsClose(page, other, Math.max(1000, 15_000 - (Date.now() - shown)));
  const hideStalls = hideBackA.stalls;
  await sleep(8000);
  const hideAfter = await snap(page);
  if (hideAfter.alert) throw new Error(hideAfter.alert);
  if (hideAfter.stalls - hideStalls > 3) throw new Error(`stall loop after hide (${hideAfter.stalls - hideStalls})`);
  await page.screenshot({ path: path.join(evidence, "hide.jpg"), animations: "disabled" });

  const report = {
    awayMs,
    locked,
    freeze: { a: freezeBackA, b: freezeBackB, pair: freezePair, after: freezeAfter },
    hide: {
      visibility: {
        hideA: { applied: hideA.applied, hidden: hideA.hidden, state: hideA.state },
        hideB: { applied: hideB.applied, hidden: hideB.hidden, state: hideB.state },
        showA: { applied: showA.applied, hidden: showA.hidden, state: showA.state },
        showB: { applied: showB.applied, hidden: showB.hidden, state: showB.state },
      },
      a: hideBackA,
      b: hideBackB,
      pair: hidePair,
      after: hideAfter,
    },
  };
  writeFileSync(path.join(evidence, "summary.json"), JSON.stringify(report, null, 2));
  expect(freezePair.gap).toBeLessThan(50);
  expect(hidePair.gap).toBeLessThan(50);
});

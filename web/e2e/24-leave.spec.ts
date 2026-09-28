import { mkdirSync, readFileSync, writeFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import type { Page } from "@playwright/test";
import { expect, test } from "./fixture";

// Chromium in this runner turns the back/forward cache off. This file needs it
// on: a kept page is the case under test. WebKit reloads instead, and Firefox
// does not launch here.
test.use({
  launchOptions: { ignoreDefaultArgs: ["--disable-back-forward-cache"] },
});

// A page left for another still counted its viewer, and on a server with a
// budget of two pictures the side by side opened next lost a tile.
const here = path.dirname(fileURLToPath(import.meta.url));
const base = (JSON.parse(readFileSync(path.join(here, ".run/server.json"), "utf8")) as { base: string }).base;

async function viewers(): Promise<number> {
  const body = (await (await fetch(`${base}/api/v1/tuners`)).json()) as { tuners?: { ours?: boolean; viewers?: number }[] };
  return (body.tuners ?? []).filter((tuner) => tuner.ours).reduce((sum, tuner) => sum + (tuner.viewers ?? 0), 0);
}

// There is no list at GET /api/v1/watch. Each watch is a viewer on a feed.
async function watches(): Promise<number> {
  const body = (await (await fetch(`${base}/api/v1/diagnostics`)).json()) as { feeds?: { viewers?: number }[] };
  return (body.feeds ?? []).reduce((sum, feed) => sum + (feed.viewers ?? 0), 0);
}

const tilesMoving = (page: Page) =>
  page.locator("video.mv-video").evaluateAll(async (videos: HTMLVideoElement[]) => {
    const from = videos.map((video) => video.currentTime);
    await new Promise((resolve) => setTimeout(resolve, 400));
    return videos.filter((video, i) => video.videoWidth > 0 && !video.paused && video.currentTime > from[i]).length;
  });

// Leaving is what frees the picture, so the test leaves its page. Runs after 01-setup.
test.afterEach(async ({ page }) => {
  await page.goto("about:blank");
});

test.beforeEach(async () => {
  await expect.poll(viewers, { timeout: 60_000, message: "the previous test let its pictures go" }).toBe(0);
});

// The server counts a viewer before its watch answers. A page that leaves
// between the answer and reading it never learns what to stop.
test("side by side plays both tiles after a page left before its watch answered", async ({ page }) => {
  const diag = (await (await fetch(`${base}/api/v1/diagnostics`)).json()) as { encoder?: { tiles?: number } };
  test.skip(diag.encoder?.tiles !== 2, "needs a budget of two pictures (E2E_SPEED=1.8)");
  let answered!: () => void;
  const lost = new Promise<void>((resolve) => (answered = resolve));
  await page.route("**/api/v1/watch", async (route) => {
    if (route.request().method() !== "POST") return route.continue();
    const res = await route.fetch();
    expect(res.status()).toBe(200);
    answered();
    await route.abort();
  });
  await page.goto("/watch?channel=1");
  await lost;
  await page.unroute("**/api/v1/watch");
  expect(await viewers()).toBe(1);

  await page.goto("/multiview?ch=1,3&layout=2up&focus=1");
  await expect.poll(() => tilesMoving(page), { timeout: 40_000, message: "both tiles play once the lost picture is freed" }).toBe(2);
  await expect(page.locator(".mv-tile [role='alert']")).toHaveCount(0);
  await expect.poll(viewers, { timeout: 15_000 }).toBe(2);
});

async function pictureMoving(page: Page): Promise<boolean> {
  const video = page.locator("video").first();
  if ((await video.count()) === 0) return false;
  return video.evaluate(async (el: HTMLVideoElement) => {
    const from = el.currentTime;
    await new Promise((resolve) => setTimeout(resolve, 400));
    return el.videoWidth > 0 && !el.paused && el.currentTime > from;
  });
}

// The watch page is frozen for Back, so it used to keep its viewer while side
// by side opened. The hide lets that viewer go. Coming back starts one watch.
test("a page kept for Back lets its watch go and watches again", async ({ page }) => {
  const diag = (await (await fetch(`${base}/api/v1/diagnostics`)).json()) as { encoder?: { tiles?: number } };
  test.skip((diag.encoder?.tiles ?? 0) < 2, "needs two pictures");
  expect((await page.request.put("/api/v1/settings", { data: { setupComplete: "1" } })).ok()).toBe(true);
  await page.addInitScript(() => {
    const mark = window as Window & { __bf?: { type: string; persisted: boolean; path: string }[] };
    mark.__bf = [];
    window.addEventListener("pagehide", (event) => {
      mark.__bf?.push({ type: "pagehide", persisted: event.persisted, path: location.pathname });
    });
    window.addEventListener("pageshow", (event) => {
      mark.__bf?.push({ type: "pageshow", persisted: event.persisted, path: location.pathname });
    });
  });

  await page.goto("/watch?channel=1");
  await expect.poll(viewers, { timeout: 30_000, message: "one viewer on the channel" }).toBe(1);
  await expect.poll(() => pictureMoving(page), { timeout: 20_000, message: "the channel is playing" }).toBe(true);

  await page.goto("/multiview?ch=1,3&layout=2up&focus=1");
  await expect.poll(viewers, { timeout: 30_000, message: "the kept page let its viewer go" }).toBe(2);
  // The stop names the rendition from the watch answer. Leaving before that
  // answer arrives cannot free the tuner.
  await expect.poll(() => tilesMoving(page), { timeout: 30_000, message: "both tiles are playing" }).toBe(2);

  const left = Date.now();
  await page.goBack({ waitUntil: "commit" });
  const kept = await page.evaluate(() => {
    const rows = (window as Window & { __bf?: { type: string; persisted: boolean; path: string }[] }).__bf ?? [];
    return {
      hide: rows.some((row) => row.type === "pagehide" && row.path === "/watch" && row.persisted),
      show: rows.some((row) => row.type === "pageshow" && row.path === "/watch" && row.persisted),
    };
  });
  expect(kept).toEqual({ hide: true, show: true });
  await expect.poll(viewers, { timeout: 5_000, message: "one viewer after Back" }).toBe(1);
  // Side by side on a two-picture server drops the full picture. Coming back
  // encodes it again, and that wait is the relay. The watch is already back.
  await expect.poll(() => pictureMoving(page), { timeout: 25_000, message: "the channel is playing again" }).toBe(true);
  await expect(page.locator(".player-error")).toHaveCount(0);
  const backMs = Date.now() - left;
  const evidence = path.resolve(here, "../../.evidence/lane/l44");
  mkdirSync(evidence, { recursive: true });
  writeFileSync(path.join(evidence, "summary.json"), JSON.stringify({ backMs, viewers: await viewers(), kept }, null, 2));
  await page.screenshot({ path: path.join(evidence, "back.jpg") });
});

// Side by side was one page with one watch. A quad is one page with a watch
// per tile. Leaving has to stop all of them, and Back has to start all of them.
test("a quad kept for Back lets every tile go and watches again", async ({ page }) => {
  const diag = (await (await fetch(`${base}/api/v1/diagnostics`)).json()) as { encoder?: { tiles?: number } };
  test.skip((diag.encoder?.tiles ?? 0) < 3, "needs three pictures");
  expect((await page.request.put("/api/v1/settings", { data: { setupComplete: "1" } })).ok()).toBe(true);
  await page.addInitScript(() => {
    const mark = window as Window & { __bf?: { type: string; persisted: boolean; path: string }[] };
    mark.__bf = [];
    window.addEventListener("pagehide", (event) => {
      mark.__bf?.push({ type: "pagehide", persisted: event.persisted, path: location.pathname });
    });
    window.addEventListener("pageshow", (event) => {
      mark.__bf?.push({ type: "pageshow", persisted: event.persisted, path: location.pathname });
    });
  });

  await page.goto("/multiview?ch=1,2,3&layout=quad&focus=1");
  await expect(page.getByRole("region", { name: "Quad" })).toBeVisible();
  await expect.poll(viewers, { timeout: 40_000, message: "one viewer per tile" }).toBe(3);
  await expect.poll(watches, { timeout: 5_000, message: "one watch per tile" }).toBe(3);
  await expect.poll(() => tilesMoving(page), { timeout: 30_000, message: "three tiles are playing" }).toBe(3);
  await expect(page.getByRole("group", { name: "4.1 WDAF, sound on" })).toBeVisible();

  const left = Date.now();
  await page.goto("/settings", { waitUntil: "commit" });
  await expect
    .poll(async () => ({ viewers: await viewers(), watches: await watches() }), {
      timeout: Math.max(200, 2_000 - (Date.now() - left)),
      message: "no viewer and no watch after leaving the quad",
    })
    .toEqual({ viewers: 0, watches: 0 });
  const clearMs = Date.now() - left;

  const back = Date.now();
  await page.goBack({ waitUntil: "commit" });
  const kept = await page.evaluate(() => {
    const rows = (window as Window & { __bf?: { type: string; persisted: boolean; path: string }[] }).__bf ?? [];
    return {
      hide: rows.some((row) => row.type === "pagehide" && row.path === "/multiview" && row.persisted),
      show: rows.some((row) => row.type === "pageshow" && row.path === "/multiview" && row.persisted),
    };
  });
  expect(kept).toEqual({ hide: true, show: true });
  await expect.poll(() => tilesMoving(page), { timeout: Math.max(500, 5_000 - (Date.now() - back)), message: "three tiles are playing again" }).toBe(3);
  await expect(page.getByRole("group", { name: "4.1 WDAF, sound on" })).toBeVisible();
  await expect(page.locator(".mv-error")).toHaveCount(0);
  const heard = await page.locator("video.mv-video").evaluateAll((videos: HTMLVideoElement[]) =>
    videos.map((video) => ({ channel: video.dataset.channel || "", muted: video.muted })),
  );
  expect(heard.find((tile) => tile.channel === "1")?.muted).toBe(false);
  expect(heard.filter((tile) => tile.channel !== "1").every((tile) => tile.muted)).toBe(true);
  const backMs = Date.now() - back;
  const evidence = path.resolve(here, "../../.evidence/lane/l50");
  mkdirSync(evidence, { recursive: true });
  writeFileSync(
    path.join(evidence, "summary.json"),
    JSON.stringify({ clearMs, backMs, viewers: await viewers(), watches: await watches(), kept, heard }, null, 2),
  );
  await page.screenshot({ path: path.join(evidence, "back.jpg") });
});

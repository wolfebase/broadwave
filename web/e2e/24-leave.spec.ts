import { readFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import type { Page } from "@playwright/test";
import { expect, test } from "./fixture";

// A page left for another still counted its viewer, and on a server with a
// budget of two pictures the side by side opened next lost a tile.
const here = path.dirname(fileURLToPath(import.meta.url));
const base = (JSON.parse(readFileSync(path.join(here, ".run/server.json"), "utf8")) as { base: string }).base;

async function viewers(): Promise<number> {
  const body = (await (await fetch(`${base}/api/v1/tuners`)).json()) as { tuners?: { ours?: boolean; viewers?: number }[] };
  return (body.tuners ?? []).filter((tuner) => tuner.ours).reduce((sum, tuner) => sum + (tuner.viewers ?? 0), 0);
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

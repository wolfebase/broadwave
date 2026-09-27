import { mkdirSync, readFileSync, writeFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { expect, test, type Page } from "@playwright/test";
import { settle } from "./snap";

const here = path.dirname(fileURLToPath(import.meta.url));
const evidence = path.resolve(here, "../../.evidence/lane/l29");

type Channel = { id: number; number: string; name: string };

function channels(): Channel[] {
  const runtime = JSON.parse(readFileSync(path.join(here, ".run/runtime.json"), "utf8")) as { channels: Channel[] };
  return runtime.channels;
}

function channel(name: string) {
  const found = channels().find((item) => item.name === name);
  if (!found) throw new Error(`no ${name}`);
  return found;
}

async function setupDone(page: Page) {
  const res = await page.request.put("/api/v1/settings", { data: { setupComplete: "1" } });
  expect(res.ok(), "setup").toBeTruthy();
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

function watchMisses(page: Page) {
  const lines: string[] = [];
  page.on("console", (msg) => {
    const text = msg.text();
    if (/\b404\b/.test(text)) lines.push(text);
  });
  return lines;
}

async function shot(page: Page, name: string) {
  mkdirSync(evidence, { recursive: true });
  await page.screenshot({ path: path.join(evidence, name), type: "jpeg", quality: 80 });
}

test("a tuned mux previews its other channel, and the pages do not 404", async ({ page }) => {
  test.setTimeout(120_000);
  await setupDone(page);
  const wdaf = channel("WDAF");
  const wdaf2 = channel("WDAF2");
  const kctv = channel("KCTV");
  const misses = watchMisses(page);

  await openChannel(page, wdaf.id);
  await expect
    .poll(
      () => page.locator("video.stage-video").evaluate((video: HTMLVideoElement) => video.videoWidth > 0 && !video.paused),
      { timeout: 45_000 },
    )
    .toBe(true);

  await expect
    .poll(
      async () => {
        const listed = await page.request.get("/api/v1/frames");
        if (!listed.ok()) return false;
        const body = (await listed.json().catch(() => null)) as { channels?: number[] } | null;
        const ids = body?.channels ?? [];
        const wide = await page.request.get(`/api/v1/channels/${wdaf.id}/frame?w=1280`);
        const sibling = await page.request.get(`/api/v1/channels/${wdaf2.id}/frame?w=480`);
        const other = await page.request.get(`/api/v1/channels/${kctv.id}/frame?w=480`);
        return ids.includes(wdaf.id) && ids.includes(wdaf2.id) && !ids.includes(kctv.id) && wide.ok() && sibling.ok() && other.status() === 404;
      },
      { timeout: 30_000 },
    )
    .toBe(true);

  misses.length = 0;
  await page.goto("/");
  await settle(page);
  const homeCard = page.locator("button.now-card", { has: page.locator(".nc-num", { hasText: /^4\.2$/ }) });
  const homePreview = homeCard.locator("img.nc-frame");
  await expect(homePreview).toBeVisible({ timeout: 15_000 });
  await expect.poll(() => homePreview.evaluate((img: HTMLImageElement) => img.naturalWidth)).toBeGreaterThan(0);
  await expect(page.locator("button.now-card", { has: page.locator(".nc-num", { hasText: /^5\.1$/ }) }).locator("img[src*='/frame']")).toHaveCount(0);
  expect(misses, misses.join("\n")).toEqual([]);
  await shot(page, "home.jpg");

  misses.length = 0;
  await page.goto("/guide");
  await settle(page);
  const guideRow = page.locator(".guide-row", { has: page.getByRole("button", { name: "Watch 4.2 WDAF2" }) });
  const guidePreview = guideRow.locator("img.cell-thumb, img.cell-frame");
  await expect(guidePreview).toBeVisible({ timeout: 15_000 });
  await expect.poll(() => guidePreview.evaluate((img: HTMLImageElement) => img.naturalWidth)).toBeGreaterThan(0);
  await expect(page.locator(".guide-row", { has: page.getByRole("button", { name: "Watch 5.1 KCTV" }) }).locator("img[src*='/frame']")).toHaveCount(0);
  expect(misses, misses.join("\n")).toEqual([]);
  await shot(page, "guide.jpg");

  misses.length = 0;
  await page.goto("/multiview?add=1");
  await settle(page);
  const picker = page.locator("button.mv-ch", { hasText: "4.2" }).locator("img.mv-frame");
  await expect(picker).toBeVisible({ timeout: 15_000 });
  await expect(page.locator("button.mv-ch", { hasText: "5.1" }).locator("img[src*='/frame']")).toHaveCount(0);
  expect(misses, misses.join("\n")).toEqual([]);
  await shot(page, "multiview.jpg");

  mkdirSync(evidence, { recursive: true });
  writeFileSync(
    path.join(evidence, "summary.json"),
    JSON.stringify({ wdaf: wdaf.id, wdaf2: wdaf2.id, kctv: kctv.id, console404s: misses.length }, null, 2),
  );
});

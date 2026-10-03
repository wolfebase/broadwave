// Opt-in with the 3.0 lineup: E2E_ATSC3=1 npm run e2e -- 39-atsc3-mv
// A multiview link that names a hidden half plays the half on the guide.
import { mkdirSync, readFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import type { Page } from "@playwright/test";
import { expect, test } from "./fixture";
import { settle } from "./snap";
import { copy } from "../src/strings";

const here = path.dirname(fileURLToPath(import.meta.url));
const evidence = path.resolve(here, "../../.evidence/lane/l79");
const note = copy.player.encrypted;

type RuntimeChannel = { id: number; number: string };
type APIChannel = { id: number; guideNumber: string; displayNumber: string; twinChoice?: string };

function byNumber(number: string): RuntimeChannel {
  const runtime = JSON.parse(readFileSync(path.join(here, ".run/runtime.json"), "utf8")) as { channels: RuntimeChannel[] };
  const found = runtime.channels.find((channel) => channel.number === number);
  if (!found) throw new Error(`${number} must be in the lineup`);
  return found;
}

async function channels(page: Page): Promise<APIChannel[]> {
  const body = (await (await page.request.get("/api/v1/channels")).json()) as { channels?: APIChannel[] };
  return body.channels ?? [];
}

async function choose(page: Page, id: number, choice: string) {
  const res = await page.request.patch(`/api/v1/channels/${id}`, { data: { twinChoice: choice } });
  expect(res.ok(), await res.text()).toBe(true);
}

async function openMultiview(page: Page, ids: number[], focus: number) {
  const target = `/multiview?ch=${ids.join(",")}&layout=2up&focus=${focus}`;
  await page.goto(target);
  await settle(page);
  const setup = page.getByRole("heading", { name: "Let's set up your TV" });
  if (await setup.isVisible()) {
    await page.getByRole("button", { name: "Continue" }).click();
    await page.getByRole("button", { name: "Watch", exact: true }).click();
    await expect(setup).toBeHidden();
    await page.goto(target);
    await settle(page);
  }
  await expect(page.getByRole("region", { name: "Side by side" })).toBeVisible();
}

async function moving(page: Page, id: number) {
  const video = page.locator(`video.mv-video[data-channel="${id}"]`);
  await expect(video).toHaveCount(1);
  await expect.poll(async () => video.evaluate((node) => (node as HTMLVideoElement).currentTime), { timeout: 30_000 }).toBeGreaterThan(0.2);
  const at = await video.evaluate((node) => (node as HTMLVideoElement).currentTime);
  await expect.poll(async () => video.evaluate((node) => (node as HTMLVideoElement).currentTime), { timeout: 8_000 }).toBeGreaterThan(at + 0.15);
}

function watchIds(page: Page): number[] {
  const ids: number[] = [];
  page.on("request", (req) => {
    if (req.method() !== "POST" || !req.url().endsWith("/api/v1/watch")) return;
    try {
      const body = req.postDataJSON() as { channelId?: number };
      if (body.channelId) ids.push(body.channelId);
    } catch {
      // A body that is not the watch envelope is not one of these tiles.
    }
  });
  return ids;
}

test("a multiview link plays the half that is on the guide", async ({ page }) => {
  test.setTimeout(120_000);
  const hd = byNumber("4.1");
  const wide = byNumber("104.1");
  const other = byNumber("4.2");
  const encrypted = byNumber("115.1");
  const clear = byNumber("5.1");

  expect((await page.request.put("/api/v1/settings", { data: { setupComplete: "1", watermarkGB: "0" } })).ok()).toBe(true);
  const before = (await channels(page)).find((channel) => channel.id === wide.id)?.twinChoice ?? "atsc3";
  const watched = watchIds(page);
  try {
    await choose(page, wide.id, "atsc3");
    const guide = (await (await page.request.get("/api/v1/channels?guide=1")).json()) as { channels?: APIChannel[] };
    const numbers = (guide.channels ?? []).map((channel) => channel.displayNumber);
    expect(numbers).toContain("104.1");
    expect(numbers).not.toContain("4.1");

    await openMultiview(page, [hd.id, other.id], hd.id);
    await expect.poll(() => page.evaluate(() => new URL(location.href).searchParams.get("ch"))).toBe(`${wide.id},${other.id}`);
    await expect.poll(() => page.evaluate(() => new URL(location.href).searchParams.get("focus"))).toBe(String(wide.id));
    await expect(page.locator(`.mv-tile[data-channel="${wide.id}"]`)).toContainText("104.1");
    await expect(page.locator(`video.mv-video[data-channel="${hd.id}"]`)).toHaveCount(0);
    await expect.poll(() => watched.includes(wide.id)).toBe(true);
    expect(watched).not.toContain(hd.id);
    await moving(page, other.id);

    await openMultiview(page, [encrypted.id, other.id], encrypted.id);
    await expect.poll(() => page.evaluate(() => new URL(location.href).searchParams.get("ch"))).toBe(`${clear.id},${other.id}`);
    await expect.poll(() => page.evaluate(() => new URL(location.href).searchParams.get("from"))).toBe(String(encrypted.id));
    const tile = page.locator(`.mv-tile[data-channel="${clear.id}"]`);
    await expect(tile.getByRole("status")).toHaveText(note);
    await expect(page.locator(`.mv-tile[data-channel="${other.id}"]`)).not.toContainText(note);
    await moving(page, clear.id);
    mkdirSync(evidence, { recursive: true });
    await page.screenshot({ path: path.join(evidence, "encrypted.jpg"), type: "jpeg", quality: 60 });
  } finally {
    await choose(page, wide.id, before);
  }
});

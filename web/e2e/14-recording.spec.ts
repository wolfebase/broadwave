import { readFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { expect, test } from "./fixture";
import { settle } from "./snap";

const here = path.dirname(fileURLToPath(import.meta.url));

type Rec = { id: number; title: string; status: string; durationSec?: number };

function base() {
  return (JSON.parse(readFileSync(path.join(here, ".run/server.json"), "utf8")) as { base: string }).base;
}

async function recordings(): Promise<Rec[]> {
  const res = await fetch(`${base()}/api/v1/recordings`);
  return ((await res.json()) as { recordings?: Rec[] }).recordings ?? [];
}

function clock(text: string) {
  return text.split(":").reduce((sum, part) => sum * 60 + Number(part), 0);
}

test("a recording stops from the library and plays back at its full length", async ({ page }) => {
  test.setTimeout(150_000);
  await page.goto("/");
  await settle(page);
  const setup = page.getByRole("heading", { name: "Let's set up your TV" });
  const hero = page.getByRole("heading", { name: "NFL: Chiefs at Bills" });
  await expect(setup.or(hero)).toBeVisible();
  if (await setup.isVisible()) {
    const cont = page.getByRole("button", { name: "Continue" });
    if (await cont.isVisible()) await cont.click();
    await page.getByRole("button", { name: "Watch", exact: true }).click();
    await expect(setup).toHaveCount(0);
    await page.goto("/");
    await settle(page);
  }
  const before = new Set((await recordings()).map((rec) => rec.id));
  await page.locator(".hero-actions").getByRole("button", { name: "Record" }).click();
  await expect.poll(async () => (await recordings()).find((rec) => !before.has(rec.id) && rec.status === "recording")?.id ?? 0, { timeout: 30_000 }).toBeGreaterThan(0);
  const id = (await recordings()).find((rec) => !before.has(rec.id))!.id;
  await page.waitForTimeout(25_000);

  await page.goto("/recordings");
  const row = page.locator(".media-card").filter({ has: page.locator(`img[src="/media/poster/${id}"]`) });
  await expect(row.getByText(/Recording/)).toBeVisible();
  await row.getByRole("button", { name: "Stop recording" }).click();
  await expect(page.getByText(/Stopped .*What it recorded is kept\./)).toBeVisible();
  await expect(row.getByRole("button", { name: "Stop recording" })).toHaveCount(0);
  await expect(row.locator(".ch-tags")).not.toContainText(/complete|stopped\b/);
  const rec = (await recordings()).find((item) => item.id === id)!;
  expect(rec.durationSec ?? 0, "the stopped recording has a length").toBeGreaterThan(15);

  await row.getByRole("button", { name: "Play" }).click();
  const player = page.getByRole("region", { name: "Player" });
  await expect(player).toBeVisible();
  const video = page.locator("video.stage-video");
  await expect.poll(() => video.evaluate((v: HTMLVideoElement) => v.currentTime > 0.5 && !v.paused), { timeout: 45_000 }).toBe(true);
  // The playlist grows while the file transcodes; the readout still shows the whole show.
  const total = clock(((await page.locator(".time-read span").textContent()) ?? "").replace("/", "").trim());
  expect(total, "total time").toBeGreaterThanOrEqual(Math.floor(rec.durationSec ?? 0) - 1);
  await expect(page.getByRole("slider", { name: "Playback position" })).toHaveAttribute("aria-valuetext", / of /);

  await player.focus();
  await page.keyboard.press("Space");
  await expect.poll(() => video.evaluate((v: HTMLVideoElement) => v.paused)).toBe(true);
  await page.keyboard.press("Space");
  await expect.poll(() => video.evaluate((v: HTMLVideoElement) => v.paused)).toBe(false);
  const at = await video.evaluate((v: HTMLVideoElement) => v.currentTime);
  await page.keyboard.press("ArrowLeft");
  await expect.poll(() => video.evaluate((v: HTMLVideoElement) => v.currentTime)).toBeLessThan(Math.max(at - 5, 0.5));
});

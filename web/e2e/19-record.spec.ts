// Opt-in: E2E_REC=1 npm run e2e. Records the show on now, chase-plays it,
// plays the finished file from the start and the middle, then deletes it.
import { spawnSync } from "node:child_process";
import { mkdirSync, readdirSync, readFileSync, writeFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import type { Page } from "@playwright/test";
import { expect, holdClock, test } from "./fixture";
import { settle } from "./snap";

const here = path.dirname(fileURLToPath(import.meta.url));
const evidence = path.resolve(here, "../../.evidence/lane/l34");

type Rec = { id: number; title: string; status: string; bytes?: number; durationSec?: number };
type Harness = { base: string; db: string; config: string };

function harness(): Harness {
  return JSON.parse(readFileSync(path.join(here, ".run/server.json"), "utf8")) as Harness;
}

function sql(db: string, statement: string) {
  const result = spawnSync("sqlite3", [db, `PRAGMA busy_timeout=5000; ${statement}`], { encoding: "utf8" });
  if (result.status !== 0) throw new Error(result.stderr || result.stdout || statement);
  return result.stdout.trim();
}

async function recordings(): Promise<Rec[]> {
  const res = await fetch(`${harness().base}/api/v1/recordings`);
  return ((await res.json()) as { recordings?: Rec[] }).recordings ?? [];
}

function video(page: Page) {
  return page.locator("video.stage-video");
}

async function at(page: Page) {
  return video(page).evaluate((node: HTMLVideoElement) => node.currentTime);
}

async function seekableEnd(page: Page) {
  return video(page).evaluate((node: HTMLVideoElement) => (node.seekable.length ? node.seekable.end(node.seekable.length - 1) : 0));
}

async function wake(page: Page) {
  const stage = page.getByRole("region", { name: "Player" });
  const box = await stage.boundingBox();
  if (!box) throw new Error("the player is not on screen");
  await page.mouse.move(box.x + 24, box.y + 24);
  await page.mouse.move(box.x + box.width / 2, box.y + 48);
  await expect(stage).not.toHaveClass(/\bidle\b/);
}

async function seekTo(page: Page, seconds: number) {
  await wake(page);
  await page.getByRole("slider", { name: "Playback position" }).evaluate((el: HTMLInputElement, value: number) => {
    // React keeps its own value tracker, so a plain fill does not reach onSeek.
    const set = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value")?.set;
    set?.call(el, String(value));
    el.dispatchEvent(new Event("input", { bubbles: true }));
    el.dispatchEvent(new Event("change", { bubbles: true }));
  }, Math.max(0, Math.round(seconds)));
}

async function moving(page: Page, from: number) {
  await expect
    .poll(async () => {
      const now = await video(page).evaluate((node: HTMLVideoElement) => ({ time: node.currentTime, paused: node.paused }));
      return !now.paused && now.time > from + 0.4;
    }, { timeout: 20_000 })
    .toBe(true);
}

function indexerRunning(filePath: string) {
  const ps = spawnSync("ps", ["-axww", "-o", "command="], { encoding: "utf8" });
  return ps.stdout.split("\n").some((line) => line.includes(filePath) && (line.includes("blackdetect") || line.includes("comskip")));
}

function leftovers(dir: string, filePath: string) {
  const stem = path.basename(filePath, path.extname(filePath));
  let names: string[] = [];
  try {
    names = readdirSync(dir);
  } catch {
    return [];
  }
  return names.filter((name) => {
    const ext = path.extname(name);
    return (ext === ".ts" || ext === ".edl" || ext === ".json") && path.basename(name, ext) === stem;
  });
}

test.skip(process.env.E2E_REC !== "1", "Set E2E_REC=1 to record, chase-play, and delete a show.");

test("a show recorded from the guide plays while it records, then leaves no files", async ({ page }) => {
  test.setTimeout(400_000);
  mkdirSync(evidence, { recursive: true });
  const { db, config } = harness();
  expect((await page.request.put("/api/v1/settings", { data: { setupComplete: "1", watermarkGB: "0" } })).ok()).toBe(true);
  await holdClock(page);
  await page.setViewportSize({ width: 1440, height: 900 });
  await page.goto("/guide");
  await settle(page);

  const cell = page.getByRole("gridcell", { name: /NFL: Chiefs at Bills/ });
  await expect(cell).toBeVisible();
  await cell.click();
  const sheet = page.getByRole("dialog", { name: "NFL: Chiefs at Bills" });
  await expect(sheet).toBeVisible();
  await sheet.getByRole("button", { name: "Record", exact: true }).click();
  await expect(sheet.getByRole("button", { name: "Stop recording" })).toBeVisible();
  await page.screenshot({ path: path.join(evidence, "guide.jpg"), type: "jpeg", quality: 70, animations: "disabled" });

  await expect
    .poll(async () => (await recordings()).find((rec) => rec.title === "NFL: Chiefs at Bills" && rec.status === "recording")?.id ?? 0, { timeout: 30_000 })
    .toBeGreaterThan(0);
  const id = (await recordings()).find((rec) => rec.status === "recording")!.id;
  await expect.poll(async () => (await recordings()).find((rec) => rec.id === id)?.bytes ?? 0, { timeout: 20_000 }).toBeGreaterThan(50_000);

  await page.goto("/recordings");
  await settle(page);
  const row = page.locator("section").filter({ has: page.getByRole("heading", { name: "NFL: Chiefs at Bills" }) }).locator(".media-card");
  await expect(row.getByText(/Recording/)).toBeVisible();
  await row.getByRole("button", { name: "Play" }).click();
  const player = page.getByRole("region", { name: "Player" });
  await expect(player).toBeVisible();
  await expect.poll(() => video(page).evaluate((node: HTMLVideoElement) => node.currentTime > 0.5 && !node.paused), { timeout: 45_000 }).toBe(true);
  const started = await at(page);
  await moving(page, started);
  await page.screenshot({ path: path.join(evidence, "chase.jpg"), type: "jpeg", quality: 70, animations: "disabled" });

  await expect.poll(() => at(page), { timeout: 90_000 }).toBeGreaterThan(32);
  const before = await at(page);
  await wake(page);
  await page.getByRole("button", { name: "Back 15 seconds" }).click();
  await page.getByRole("button", { name: "Back 15 seconds" }).click();
  await expect.poll(() => at(page), { timeout: 10_000 }).toBeLessThan(before - 24);
  const landed = await at(page);
  expect(landed).toBeGreaterThan(before - 40);
  await moving(page, landed);
  await page.screenshot({ path: path.join(evidence, "seek.jpg"), type: "jpeg", quality: 70, animations: "disabled" });
  const seeked = { before, landed, delta: before - landed };

  await wake(page);
  await player.getByRole("button", { name: "Library" }).click();
  await expect(page).toHaveURL(/\/recordings/);
  await row.getByRole("button", { name: "Stop recording" }).click();
  await expect(page.getByText("Stopped NFL: Chiefs at Bills. What it recorded is kept.")).toBeVisible();
  await expect(row.getByRole("button", { name: "Stop recording" })).toHaveCount(0);
  const filePath = sql(db, `SELECT path FROM recordings WHERE id = ${id};`);
  expect(filePath.endsWith(".ts"), filePath).toBe(true);

  await row.getByRole("button", { name: "Play" }).click();
  await expect(player).toBeVisible();
  await expect.poll(() => video(page).evaluate((node: HTMLVideoElement) => !node.paused && node.currentTime > 0.3), { timeout: 60_000 }).toBe(true);
  await wake(page);
  await page.getByRole("button", { name: "Options" }).click();
  await page.getByRole("button", { name: "Start over" }).click();
  // Start over parks on the first frame. The name on the button updates after the pause.
  await expect.poll(() => video(page).evaluate((node: HTMLVideoElement) => node.paused && node.currentTime < 1)).toBe(true);
  await page.locator(".transport-play").click();
  await expect.poll(() => at(page), { timeout: 10_000 }).toBeLessThan(2);
  const fromStart = await at(page);
  await moving(page, fromStart);
  expect(await at(page)).toBeLessThan(8);
  await page.screenshot({ path: path.join(evidence, "start.jpg"), type: "jpeg", quality: 70, animations: "disabled" });

  await expect.poll(async () => Math.max((await recordings()).find((rec) => rec.id === id)?.durationSec ?? 0, await seekableEnd(page)), { timeout: 90_000 }).toBeGreaterThan(20);
  const length = Math.max((await recordings()).find((rec) => rec.id === id)?.durationSec ?? 0, await seekableEnd(page));
  const middle = length / 2;
  // The finished file transcodes while it plays. Wait until the middle is buffered, with room to keep going.
  await expect.poll(() => seekableEnd(page), { timeout: 120_000 }).toBeGreaterThan(middle + 2);
  await seekTo(page, middle);
  await expect.poll(() => at(page), { timeout: 15_000 }).toBeGreaterThan(middle - 4);
  const midAt = await at(page);
  expect(midAt).toBeLessThan(middle + 6);
  await moving(page, midAt);
  await page.screenshot({ path: path.join(evidence, "middle.jpg"), type: "jpeg", quality: 70, animations: "disabled" });

  await wake(page);
  await player.getByRole("button", { name: "Library" }).click();
  await row.getByRole("button", { name: "Delete" }).click();
  await row.getByRole("button", { name: "Delete this file" }).click();
  await expect(page.getByText("Deleted NFL: Chiefs at Bills.")).toBeVisible();
  await expect(row).toHaveCount(0);
  await page.screenshot({ path: path.join(evidence, "deleted.jpg"), type: "jpeg", quality: 70, animations: "disabled" });

  const dir = path.join(config, "work", "recordings");
  const deadline = Date.now() + 120_000;
  let quiet = 0;
  while (Date.now() < deadline) {
    if (indexerRunning(filePath)) quiet = 0;
    else quiet += 1;
    if (quiet >= 4) break;
    await page.waitForTimeout(500);
  }
  expect(indexerRunning(filePath), "commercial indexing still running").toBe(false);
  const left = leftovers(dir, filePath);
  expect(left, `files left for ${path.basename(filePath)}`).toEqual([]);
  writeFileSync(
    path.join(evidence, "summary.json"),
    JSON.stringify({ id, file: path.basename(filePath), seeked, fromStart, middle: midAt, length, leftovers: left }, null, 2),
  );
});

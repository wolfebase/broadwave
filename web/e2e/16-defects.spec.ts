import { spawnSync } from "node:child_process";
import { mkdirSync, readFileSync, writeFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import type { Page } from "@playwright/test";
import { expect, test } from "./fixture";
import { settle } from "./snap";

const here = path.dirname(fileURLToPath(import.meta.url));
const evidence = path.resolve(here, "../../.evidence/lane/l23");

type Channel = { id: number; number: string; name: string };

function runtime() {
  return JSON.parse(readFileSync(path.join(here, ".run/runtime.json"), "utf8")) as { channels: Channel[] };
}

function channel(name: string) {
  const found = runtime().channels.find((item) => item.name === name);
  if (!found) throw new Error(`no ${name}`);
  return found;
}

function harness() {
  return JSON.parse(readFileSync(path.join(here, ".run/server.json"), "utf8")) as { db: string };
}

async function setupDone(page: Page) {
  const res = await page.request.put("/api/v1/settings", { data: { setupComplete: "1" } });
  expect(res.ok(), "setup").toBeTruthy();
}

type Picture = {
  time: number;
  stallMs: number;
  ttff: string;
  paused: boolean;
  width: number;
  url: string;
  token: string;
  events: string;
};

async function readPicture(page: Page): Promise<Picture> {
  return page.locator("video.stage-video").evaluate((video: HTMLVideoElement & { hls?: { url?: string } }) => ({
    time: video.currentTime,
    stallMs: Number(video.dataset.stallMs || 0),
    ttff: video.dataset.ttff || "",
    paused: video.paused,
    width: video.videoWidth,
    url: video.hls?.url || "",
    token: video.dataset.reopenToken || "",
    events: video.dataset.reopenEvents || "",
  }));
}

test("the guide grid names the cell the keyboard is on", async ({ page }) => {
  await setupDone(page);
  await page.goto("/guide");
  await settle(page);

  const grid = page.getByRole("grid", { name: "TV guide" });
  await grid.focus();
  const before = await grid.getAttribute("aria-activedescendant");
  expect(before).toBeTruthy();
  await expect(page.locator(`[id="${before}"]`)).toHaveCount(1);
  await page.keyboard.press("ArrowDown");
  const after = await grid.getAttribute("aria-activedescendant");
  expect(after && after !== before).toBeTruthy();
  await expect(page.locator(`[id="${after}"]`)).toHaveCount(1);
  const label = (await page.locator(`[id="${after}"]`).getAttribute("aria-label")) || "";
  expect(label.length).toBeGreaterThan(0);
});

test("reopening the player keeps the picture, and the page behind it is inert", async ({ page }) => {
  await setupDone(page);
  const wdaf = channel("WDAF");
  let watches = 0;
  let stops = 0;
  page.on("request", (req) => {
    const pathname = new URL(req.url()).pathname;
    if (req.method() !== "POST") return;
    if (pathname === "/api/v1/watch") watches += 1;
    if (/\/api\/v1\/watch\/\d+\/stop$/.test(pathname)) stops += 1;
  });

  await page.goto("/");
  await settle(page);
  await page.getByRole("button", { name: "Watch", exact: true }).click();
  await expect(page).toHaveURL(new RegExp(`/watch\\?channel=${wdaf.id}`));
  const player = page.getByRole("region", { name: "Player" });
  await expect(player).toBeVisible();
  await expect
    .poll(() => readPicture(page).then((pic) => pic.width > 0 && !pic.paused), { timeout: 45_000 })
    .toBe(true);
  const moving = await readPicture(page);
  await expect.poll(() => readPicture(page).then((pic) => pic.time), { timeout: 15_000 }).toBeGreaterThan(moving.time + 1);

  const behind = await page.evaluate(() => ({
    nav: document.querySelector("nav")?.inert ?? false,
    main: document.querySelector("main")?.inert ?? false,
  }));
  expect(behind).toEqual({ nav: true, main: true });
  await page.keyboard.press("Tab");
  const tabbed = await page.evaluate(() => {
    const el = document.activeElement;
    return {
      nav: Boolean(el?.closest("nav")),
      main: Boolean(el?.closest("main")),
      stage: Boolean(el?.closest(".stage")),
    };
  });
  expect(tabbed).toEqual({ nav: false, main: false, stage: true });

  await page.locator("video.stage-video").evaluate((video: HTMLVideoElement) => {
    video.dataset.reopenToken = "stay";
    video.dataset.reopenEvents = "";
    for (const name of ["emptied", "loadstart"]) {
      video.addEventListener(name, () => {
        video.dataset.reopenEvents = `${video.dataset.reopenEvents} ${name}`.trim();
      });
    }
  });
  const before = await readPicture(page);
  const watchesBefore = watches;
  const stopsBefore = stops;

  await page.locator(".stage").focus();
  await page.keyboard.press("Escape");
  await expect(page.getByRole("button", { name: "Open player" })).toBeVisible();
  await expect.poll(() => readPicture(page).then((pic) => pic.time), { timeout: 5_000 }).toBeGreaterThan(before.time + 0.4);
  expect(watches, "minimize started a watch").toBe(watchesBefore);
  expect(stops, "minimize stopped the watch").toBe(stopsBefore);

  const docked = await readPicture(page);
  const openedAt = Date.now();
  await page.getByRole("button", { name: "Open player" }).click();
  await expect(player).toBeVisible();
  const samples: { at: number; time: number; events: string; ttff: string; url: string; token: string; stallMs: number }[] = [];
  const deadline = Date.now() + 3_000;
  while (Date.now() < deadline) {
    const pic = await readPicture(page);
    samples.push({ at: Date.now() - openedAt, time: pic.time, events: pic.events, ttff: pic.ttff, url: pic.url, token: pic.token, stallMs: pic.stallMs });
    if (pic.time > docked.time + 0.4) break;
    await page.waitForTimeout(200);
  }
  const last = samples[samples.length - 1];
  mkdirSync(evidence, { recursive: true });
  writeFileSync(
    path.join(evidence, "reopen.json"),
    JSON.stringify({ before, docked, samples, watches, stops, watchesBefore, stopsBefore }, null, 2),
  );
  await page.screenshot({ path: path.join(evidence, "reopen.jpg"), animations: "disabled" });

  expect(last?.token, "the video element was replaced").toBe("stay");
  expect(last?.url, "playlist changed").toBe(before.url);
  expect(last?.ttff, "the watch started over").toBe(before.ttff);
  expect(last?.events, "the media element reloaded").toBe("");
  expect(watches, "reopen started a watch").toBe(watchesBefore);
  expect(stops, "reopen stopped the watch").toBe(stopsBefore);
  expect(last && last.time > docked.time + 0.4, `picture did not advance (${samples.map((s) => s.time.toFixed(2)).join(", ")})`).toBe(true);
  expect(last && last.at < 1500, `first movement at ${last?.at} ms`).toBe(true);
});

test("check for listings says when nothing came back", async ({ page }) => {
  await setupDone(page);
  const { db } = harness();
  const kctv = channel("KCTV");
  const sql = (statement: string) => {
    const result = spawnSync("sqlite3", [db, `PRAGMA busy_timeout=5000; ${statement}`], { encoding: "utf8" });
    if (result.status !== 0) throw new Error(result.stderr || result.stdout || statement);
  };
  sql(`DELETE FROM airings WHERE channel_id = ${kctv.id};`);
  try {
    await page.goto(`/watch?channel=${kctv.id}`);
    await settle(page);
    await expect(page.locator(".player-note p")).toHaveText("No listing for this channel.");
    await page.locator(".stage").hover();
    await page.getByRole("button", { name: "Check for listings" }).click();
    await expect(page.locator(".player-note p")).toHaveText("Still no listing for this channel.");
    await expect(page.getByRole("button", { name: "Check for listings" })).toBeEnabled();
    mkdirSync(evidence, { recursive: true });
    await page.screenshot({ path: path.join(evidence, "no-listing.jpg"), animations: "disabled" });
  } finally {
    const now = Date.now();
    const start = new Date(now - 5 * 60_000).toISOString().replace(/\.\d{3}Z$/, "Z");
    const end = new Date(now + 2 * 60 * 60_000).toISOString().replace(/\.\d{3}Z$/, "Z");
    sql(
      `INSERT INTO airings (channel_id, title, subtitle, description, category, starts_at, ends_at, program_id, is_live, guide_source)
       SELECT ${kctv.id}, 'The Night Show', 'A guest and a band', 'Talk.', 'Series', '${start}', '${end}', 'e2e-restore', 0, 'e2e'
       WHERE NOT EXISTS (SELECT 1 FROM airings WHERE channel_id = ${kctv.id});`,
    );
  }
});

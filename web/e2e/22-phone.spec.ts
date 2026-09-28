import { mkdirSync, readFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import type { Locator, Page } from "@playwright/test";
import { expect, test } from "./fixture";

const here = path.dirname(fileURLToPath(import.meta.url));
const evidence = path.resolve(here, "../../.evidence/lane/l43");

type Channel = { id: number; number: string; name: string };

test.use({
  viewport: { width: 390, height: 844 },
  hasTouch: true,
  isMobile: true,
});

function lineup(): Channel[] {
  const runtime = JSON.parse(readFileSync(path.join(here, ".run/runtime.json"), "utf8")) as { channels: Channel[] };
  const num = (value: string) => value.split(".").map((part) => Number(part) || 0);
  return [...runtime.channels].sort((a, b) => {
    const x = num(a.number);
    const y = num(b.number);
    return (x[0] ?? 0) - (y[0] ?? 0) || (x[1] ?? 0) - (y[1] ?? 0) || a.name.localeCompare(b.name);
  });
}

async function tap(control: Locator) {
  await expect(control).toBeVisible();
  const box = await control.boundingBox();
  const name = (await control.getAttribute("aria-label")) || (await control.innerText());
  expect(box, name).toBeTruthy();
  expect(box!.width, name).toBeGreaterThanOrEqual(44);
  expect(box!.height, name).toBeGreaterThanOrEqual(44);
  await control.tap();
}

async function fits(page: Page) {
  const width = await page.evaluate(() => document.scrollingElement?.scrollWidth ?? 0);
  expect(width).toBeLessThanOrEqual(390);
}

async function moving(page: Page) {
  await expect
    .poll(
      () =>
        page.locator("video.stage-video").evaluate(async (video: HTMLVideoElement) => {
          if (video.videoWidth === 0 || video.paused) return false;
          const at = video.currentTime;
          await new Promise((resolve) => setTimeout(resolve, 400));
          return video.videoWidth > 0 && !video.paused && video.currentTime > at + 0.05;
        }),
      { timeout: 30_000 },
    )
    .toBe(true);
}

async function chrome(page: Page) {
  return page.evaluate(() => {
    const stage = document.querySelector(".stage");
    const hud = document.querySelector(".stage-hud");
    return {
      idle: Boolean(stage?.classList.contains("idle")),
      opacity: hud ? getComputedStyle(hud).opacity : "",
    };
  });
}

test("a phone walks home, the guide, and the player by touch", async ({ page }) => {
  test.setTimeout(90_000);
  const errors: string[] = [];
  page.on("pageerror", (err) => errors.push(err.message));
  page.on("console", (msg) => {
    if (msg.type() !== "error") return;
    const where = msg.location().url;
    const text = msg.text();
    if (/\/media\/(?:art|poster)\/|\/channels\/\d+\/frame|favicon/.test(`${where} ${text}`)) return;
    errors.push(where ? `${text} (${where})` : text);
  });

  await page.addInitScript(() => {
    localStorage.setItem("ota-live", JSON.stringify({ sync: false }));
  });
  const setup = await page.request.put("/api/v1/settings", { data: { setupComplete: "1" } });
  expect(setup.ok()).toBeTruthy();

  const channels = lineup();
  expect(channels.length).toBeGreaterThan(1);
  await page.goto("/");
  await expect(page.locator("main")).toHaveAttribute("data-ready", "1");
  await expect(page.locator("html")).toHaveAttribute("data-layout", "phone");
  await fits(page);

  await tap(page.getByRole("tab", { name: "Guide" }));
  await expect(page).toHaveURL(/\/guide$/);
  const list = page.locator(".onnow-list");
  await expect(list).toBeVisible();
  const row = page.locator(".onnow-row").first();
  const rowBox = await row.boundingBox();
  expect(rowBox).toBeTruthy();
  await page.mouse.move(rowBox!.x + rowBox!.width / 2, rowBox!.y + Math.min(rowBox!.height / 2, 40));
  const before = await page.locator(".content").evaluate((el) => ({
    top: el.scrollTop,
    left: el.scrollLeft,
    room: el.scrollHeight - el.clientHeight,
  }));
  // The phone guide is the on-now list. Twenty row-heights, then two hours
  // at the wide guide's scale. The page must not pick up the sideways wheel.
  await page.mouse.wheel(0, rowBox!.height * 20);
  await page.mouse.wheel(2 * 60 * 6.4, 0);
  const after = await page.locator(".content").evaluate((el) => ({ top: el.scrollTop, left: el.scrollLeft }));
  const traveled = Math.max(0, before.room);
  expect(after.top).toBeGreaterThanOrEqual(Math.min(traveled, rowBox!.height * 20) - 2);
  expect(after.top).toBeLessThanOrEqual(traveled + 1);
  expect(after.left).toBe(0);
  await fits(page);

  await tap(page.getByRole("button", { name: "Now", exact: true }));
  await expect(page.getByRole("button", { name: "Now", exact: true })).toHaveAttribute("aria-pressed", "true");
  const onNow = page.getByRole("button", { name: /Chiefs at Bills/ });
  await expect(onNow).toBeVisible();
  mkdirSync(evidence, { recursive: true });
  await page.screenshot({ path: path.join(evidence, "guide.jpg"), type: "jpeg", quality: 70, animations: "disabled" });
  await tap(onNow);
  const sheet = page.getByRole("dialog", { name: /Chiefs at Bills/ });
  await expect(sheet).toBeVisible();
  const watch = sheet.getByRole("button", { name: "Watch", exact: true });
  await watch.scrollIntoViewIfNeeded();
  await tap(watch);
  await expect(page).toHaveURL(/\/watch\?channel=\d+/);
  const start = Number(new URL(page.url()).searchParams.get("channel"));
  const at = channels.findIndex((channel) => channel.id === start);
  expect(at).toBeGreaterThanOrEqual(0);
  const next = channels[(at + 1) % channels.length];
  const player = page.getByRole("region", { name: "Player" });
  await expect(player).toBeVisible();
  const stage = await player.boundingBox();
  expect(stage?.width).toBeGreaterThanOrEqual(389);
  expect(stage?.width).toBeLessThanOrEqual(390);
  await moving(page);
  await expect.poll(() => chrome(page), { timeout: 8_000 }).toMatchObject({ idle: true, opacity: "0" });

  const picture = await page.locator("video.stage-video").boundingBox();
  expect(picture).toBeTruthy();
  await page.touchscreen.tap(picture!.x + picture!.width / 2, picture!.y + picture!.height * 0.28);
  await expect.poll(() => page.locator("video.stage-video").evaluate((video: HTMLVideoElement) => video.paused)).toBe(false);
  await expect.poll(() => chrome(page)).toMatchObject({ idle: false });
  const shown = await page.locator(".stage-hud").evaluate((node) => Number(getComputedStyle(node).opacity));
  expect(shown).toBeGreaterThan(0);
  await page.screenshot({ path: path.join(evidence, "chrome.jpg"), type: "jpeg", quality: 70, animations: "disabled" });
  await expect.poll(() => chrome(page), { timeout: 4_000 }).toMatchObject({ idle: true, opacity: "0" });

  await page.touchscreen.tap(picture!.x + picture!.width / 2, picture!.y + picture!.height * 0.28);
  await expect.poll(() => chrome(page)).toMatchObject({ idle: false });
  await tap(page.getByRole("button", { name: "Next channel" }));
  await expect.poll(() => new URL(page.url()).searchParams.get("channel")).toBe(String(next.id));
  await moving(page);
  await fits(page);
  await expect.poll(() => chrome(page), { timeout: 8_000 }).toMatchObject({ idle: true, opacity: "0" });

  await page.touchscreen.tap(picture!.x + picture!.width / 2, picture!.y + picture!.height * 0.28);
  await expect.poll(() => chrome(page), { timeout: 8_000 }).toMatchObject({ idle: false });
  await tap(page.getByRole("button", { name: "Previous channel" }));
  await expect.poll(() => new URL(page.url()).searchParams.get("channel")).toBe(String(start));
  await moving(page);
  await expect(page.locator(".tuning")).toHaveCount(0);
  await page.screenshot({ path: path.join(evidence, "player.jpg"), type: "jpeg", quality: 70, animations: "disabled" });
  await expect.poll(() => chrome(page), { timeout: 8_000 }).toMatchObject({ idle: true, opacity: "0" });

  await page.touchscreen.tap(picture!.x + picture!.width / 2, picture!.y + picture!.height * 0.28);
  await expect.poll(() => chrome(page)).toMatchObject({ idle: false });
  await tap(page.getByRole("button", { name: "Back to browsing" }));
  await expect(page).toHaveURL(/\/guide$/);
  const mini = page.locator(".stage.mini");
  await expect(mini).toBeVisible();
  for (const name of ["Pause", "Stop watching"]) {
    const box = await mini.getByRole("button", { name }).boundingBox();
    expect(box, name).toBeTruthy();
    expect(box!.width, name).toBeGreaterThanOrEqual(44);
    expect(box!.height, name).toBeGreaterThanOrEqual(44);
  }
  await fits(page);
  await page.screenshot({ path: path.join(evidence, "mini.jpg"), type: "jpeg", quality: 70, animations: "disabled" });

  await tap(page.getByRole("tab", { name: "Recordings" }));
  await expect(page).toHaveURL(/\/recordings$/);
  await expect(page.getByRole("heading", { name: "Recordings" })).toBeVisible();
  await fits(page);

  await tap(page.getByRole("button", { name: "Settings" }));
  await expect(page).toHaveURL(/\/settings$/);
  await expect(page.getByRole("heading", { name: "Settings", level: 1 })).toBeVisible();
  await fits(page);
  await page.screenshot({ path: path.join(evidence, "settings.jpg"), type: "jpeg", quality: 70, animations: "disabled" });

  // The mini player is still watching. Closing the page is not a reliable stop:
  // the next spec waits for no viewers, and a missed pagehide holds a tuner
  // until the server drops it. An earlier spec can still be in that window,
  // so this only checks that our own watch is gone.
  const base = (JSON.parse(readFileSync(path.join(here, ".run/server.json"), "utf8")) as { base: string }).base;
  const viewers = async () => {
    const body = (await (await fetch(`${base}/api/v1/tuners`)).json()) as { tuners?: { ours?: boolean; viewers?: number }[] };
    return (body.tuners ?? []).filter((tuner) => tuner.ours).reduce((sum, tuner) => sum + (tuner.viewers ?? 0), 0);
  };
  const held = await viewers();
  expect(held).toBeGreaterThan(0);
  await tap(mini.getByRole("button", { name: "Stop watching" }));
  await expect(mini).toHaveCount(0);
  await expect.poll(viewers).toBeLessThan(held);

  expect(errors, errors.join("\n")).toEqual([]);
});

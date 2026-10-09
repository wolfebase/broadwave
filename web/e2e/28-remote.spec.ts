// A TV browser has no pointer. Arrows, Enter, and Back have to reach every screen.
import { mkdirSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import type { Page } from "@playwright/test";
import { expect, holdClock, test } from "./fixture";

const evidence = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "../../.evidence/lane/l60");

type Ring = { body: boolean; visible: boolean; drawn: boolean; label: string; role: string; tag: string };
type Grid = { id: string; row: number; now: boolean; inView: boolean; onGrid: boolean };
type Tile = { id: string; sound: boolean; active: boolean };

function readRing(): Ring {
  const el = document.activeElement instanceof HTMLElement ? document.activeElement : null;
  const body = !el || el === document.body || el === document.documentElement;
  const style = el ? getComputedStyle(el) : null;
  const outline = style && style.outlineStyle !== "none" ? Number.parseFloat(style.outlineWidth) || 0 : 0;
  const shadow = style?.boxShadow ?? "none";
  return {
    body,
    visible: Boolean(el?.matches(":focus-visible")),
    drawn: outline > 0 || (shadow !== "none" && shadow !== ""),
    label: (el?.getAttribute("aria-label") || el?.textContent || "").replace(/\s+/g, " ").trim().slice(0, 90),
    role: el?.getAttribute("role") ?? "",
    tag: el?.tagName ?? "",
  };
}

function readGrid(): Grid {
  const grid = document.querySelector<HTMLElement>("[role='grid']");
  const id = grid?.getAttribute("aria-activedescendant") || "";
  const cell = id ? document.getElementById(id) : null;
  const row = cell?.closest("[role='row']");
  const scroller = document.querySelector(".guide-scroll");
  const scrollBox = scroller?.getBoundingClientRect();
  const boxOf = (el: Element | null | undefined) => el?.getBoundingClientRect();
  const cellBox = boxOf(cell);
  const rowBox = boxOf(row);
  // A show can be wider than the screen. On screen means the focused cell meets the grid, not that the whole block fits.
  const seen = (box: DOMRect | undefined) =>
    Boolean(
      box &&
        scrollBox &&
        box.bottom > scrollBox.top + 1 &&
        box.top < scrollBox.bottom - 1 &&
        box.right > scrollBox.left + 1 &&
        box.left < scrollBox.right - 1,
    );
  return {
    id,
    row: Number(row?.getAttribute("aria-rowindex") || 0),
    now: Boolean(cell?.classList.contains("now")),
    inView: seen(cellBox) && seen(rowBox),
    onGrid: document.activeElement?.getAttribute("role") === "grid",
  };
}

function readTiles(): Tile[] {
  return [...document.querySelectorAll<HTMLElement>(".mv-tile")].map((el) => ({
    id: el.getAttribute("data-channel") || "",
    sound: el.classList.contains("focused"),
    active: el === document.activeElement,
  }));
}

async function expectRing(page: Page) {
  await expect
    .poll(async () => {
      const ring = await page.evaluate(readRing);
      return !ring.body && ring.visible && ring.drawn ? "ok" : JSON.stringify(ring);
    }, { timeout: 4_000 })
    .toBe("ok");
}

async function press(page: Page, key: string) {
  await page.keyboard.press(key);
  await expectRing(page);
}

async function until(page: Page, key: string, done: () => Promise<boolean>, limit = 24) {
  for (let i = 0; i < limit; i++) {
    if (await done()) return;
    await press(page, key);
  }
  const ring = await page.evaluate(readRing);
  expect(await done(), `${key} did not arrive ${JSON.stringify(ring)}`).toBe(true);
}

async function labelIs(page: Page, label: string) {
  const ring = await page.evaluate(readRing);
  return ring.label === label;
}

/** The top bar, from wherever a closed control left the keys. Sideways moves stay on the bar. */
async function reachTop(page: Page, label: string) {
  if (!(await page.evaluate(() => Boolean(document.activeElement?.closest(".topbar"))))) {
    await until(page, "ArrowDown", async () => page.evaluate(() => Boolean(document.activeElement?.closest(".topbar"))), 8);
  }
  for (const key of ["ArrowRight", "ArrowLeft"] as const) {
    for (let i = 0; i < 10; i++) {
      if (await labelIs(page, label)) return;
      await press(page, key);
      const onBar = await page.evaluate(() => Boolean(document.activeElement?.closest(".topbar")));
      if (!onBar) break;
    }
  }
  expect(await labelIs(page, label), await page.evaluate(readRing)).toBe(true);
}

function tv(page: Page) {
  return page.addInitScript(() => {
    const orig = window.matchMedia.bind(window);
    window.matchMedia = (query: string) => {
      const q = String(query);
      if (q.includes("pointer") && q.includes("coarse")) {
        return {
          matches: true,
          media: q,
          onchange: null,
          addListener() {},
          removeListener() {},
          addEventListener() {},
          removeEventListener() {},
          dispatchEvent() {
            return false;
          },
        } as MediaQueryList;
      }
      return orig(q);
    };
  });
}

test("a remote reaches home, the guide, the player, multiview, settings, and recordings", async ({ page }) => {
  test.setTimeout(160_000);
  mkdirSync(evidence, { recursive: true });
  await holdClock(page);
  await tv(page);
  const setup = await page.request.put("/api/v1/settings", { data: { setupComplete: "1", watermarkGB: "0" } });
  expect(setup.ok()).toBeTruthy();
  const listed = (await (await page.request.get("/api/v1/channels")).json()) as { channels?: { id: number }[] };
  const channelId = listed.channels?.[0]?.id ?? 0;
  expect(channelId).toBeGreaterThan(0);

  await page.setViewportSize({ width: 1920, height: 1080 });
  await page.goto("/");
  await expect(page.locator("main[data-ready='1']")).toBeVisible();
  const notNow = page.getByRole("button", { name: "Not now" });
  if ((await notNow.count()) > 0) {
    await until(page, "ArrowDown", async () => notNow.evaluate((el) => el === document.activeElement).catch(() => false), 16);
    await press(page, "Enter");
    await expect(notNow).toHaveCount(0);
  }
  await expect(page.locator("html")).toHaveAttribute("data-layout", "tv");
  await expectRing(page);

  await until(page, "ArrowDown", async () => page.evaluate(() => Boolean(document.activeElement?.closest(".now-card"))), 20);
  await page.screenshot({ path: path.join(evidence, "home.jpg"), type: "jpeg", quality: 60 });
  await press(page, "Enter");
  await expect(page).toHaveURL(/\/watch\?channel=\d+/);
  const player = page.getByRole("region", { name: "Player" });
  await expect(player).toBeVisible();
  await expectRing(page);

  await until(page, "ArrowRight", async () => labelIs(page, "Channels"), 8);
  await press(page, "Enter");
  const mini = page.getByRole("listbox", { name: "Channels" });
  await expect(mini).toBeVisible();
  await expect.poll(async () => (await page.evaluate(readRing)).role).toBe("option");
  const beforeRow = (await mini.locator("[aria-selected='true']").innerText()).replace(/\s+/g, " ").trim();
  await press(page, "ArrowDown");
  const afterRow = (await mini.locator("[aria-selected='true']").innerText()).replace(/\s+/g, " ").trim();
  expect(afterRow).not.toBe(beforeRow);
  await expect.poll(async () => page.evaluate(() => document.activeElement?.getAttribute("aria-selected"))).toBe("true");
  await page.screenshot({ path: path.join(evidence, "mini-guide.jpg"), type: "jpeg", quality: 60 });
  const first = new URL(page.url()).searchParams.get("channel");
  await press(page, "Enter");
  await expect(mini).toHaveCount(0);
  await expect.poll(() => new URL(page.url()).searchParams.get("channel")).not.toBe(first);
  await expect.poll(async () => labelIs(page, "Channels")).toBe(true);

  await press(page, "Enter");
  await expect(mini).toBeVisible();
  await press(page, "Escape");
  await expect(mini).toHaveCount(0);
  await expect.poll(async () => labelIs(page, "Channels")).toBe(true);
  await expectRing(page);

  await until(page, "ArrowRight", async () => labelIs(page, "Side by side"), 8);
  await press(page, "Enter");
  await expect(page).toHaveURL(/\/multiview\?/);
  await expect(page.getByRole("listbox", { name: "Add a channel" })).toBeVisible();
  expect(await page.evaluate(() => document.activeElement?.getAttribute("aria-selected") !== "true")).toBe(true);
  await press(page, "Enter");
  await expect(page.locator(".mv-cell")).toHaveCount(2, { timeout: 15_000 });
  await expectRing(page);

  const opened = await page.evaluate(readTiles);
  const sound = opened.find((tile) => tile.sound)?.id || new URL(page.url()).searchParams.get("focus") || "";
  const soundIndex = opened.findIndex((tile) => tile.id === sound);
  expect(sound).toBeTruthy();
  await press(page, soundIndex > 0 ? "ArrowLeft" : "ArrowRight");
  const pointed = await page.evaluate(readTiles);
  const marked = pointed.find((tile) => tile.active);
  expect(marked?.id, JSON.stringify(pointed)).toBeTruthy();
  expect(marked?.id).not.toBe(sound);
  expect(new URL(page.url()).searchParams.get("focus")).toBe(sound);
  expect(pointed.find((tile) => tile.sound)?.id).toBe(sound);
  await press(page, "Enter");
  await expect.poll(() => new URL(page.url()).searchParams.get("focus")).toBe(marked?.id ?? "");
  await expect.poll(async () => (await page.evaluate(readTiles)).find((tile) => tile.sound)?.id).toBe(marked?.id ?? "");
  await page.screenshot({ path: path.join(evidence, "multiview.jpg"), type: "jpeg", quality: 60 });

  await press(page, "Backspace");
  await expect(page).toHaveURL(/\/watch\?channel=\d+/);
  await expect(player).toBeVisible();
  await press(page, "Escape");
  await expect(page).toHaveURL(/\/$/);
  await expect(player).toHaveCount(0);

  // The mini player sits to the right of the last card in the On now row.
  await until(page, "ArrowDown", async () => page.evaluate(() => Boolean(document.activeElement?.closest(".shelf .now-card"))), 16);
  await until(page, "ArrowRight", async () => page.evaluate(() => {
    const el = document.activeElement;
    const cards = [...document.querySelectorAll(".shelf .shelf-row .now-card")].filter((card) => card.closest(".shelf") === document.querySelector(".shelf"));
    return Boolean(el && el === cards[cards.length - 1]);
  }), 6);
  await until(page, "ArrowRight", async () => page.evaluate(() => Boolean(document.activeElement?.closest(".mini-bar"))), 4);
  await until(page, "ArrowRight", async () => labelIs(page, "Stop watching"), 4);
  // Closing the mini player removes the button that had focus.
  await page.keyboard.press("Enter");
  await expect(page.getByRole("button", { name: "Stop watching" })).toHaveCount(0);

  const started = await page.request.post("/api/v1/recordings", { data: { channelId, minutes: 2, title: "Night Shift" } });
  if (!started.ok()) throw new Error(`record ${started.status()} ${await started.text()}`);
  const recordingId = ((await started.json()) as { id: number }).id;
  expect(recordingId).toBeGreaterThan(0);
  try {
    await expect
      .poll(async () => {
        const body = (await (await page.request.get("/api/v1/recordings")).json()) as { recordings?: { id: number; bytes?: number }[] };
        return body.recordings?.find((rec) => rec.id === recordingId)?.bytes ?? 0;
      }, { timeout: 20_000 })
      .toBeGreaterThan(10_000);

    await reachTop(page, "Guide");
    await press(page, "Enter");
    await expect(page).toHaveURL(/\/guide$/);
    await until(page, "ArrowDown", async () => page.evaluate(() => document.activeElement?.getAttribute("role") === "grid"), 16);
    const cell = await page.evaluate(readGrid);
    expect(cell.now).toBe(true);
    expect(cell.onGrid).toBe(true);

    await press(page, "ArrowRight");
    const right = await page.evaluate(readGrid);
    expect(right.id).not.toBe(cell.id);
    expect(right.now).toBe(false);
    expect(right.row).toBe(cell.row);
    expect(right.inView).toBe(true);
    expect(right.onGrid).toBe(true);

    await press(page, "ArrowDown");
    await expect.poll(async () => page.evaluate(readGrid)).toMatchObject({ row: right.row + 1, inView: true, onGrid: true });
    await page.screenshot({ path: path.join(evidence, "guide.jpg"), type: "jpeg", quality: 60 });

    await press(page, "ArrowUp");
    expect((await page.evaluate(readGrid)).row).toBe(right.row);
    await press(page, "ArrowUp");
    expect(
      await page.evaluate(() => Boolean(document.activeElement?.closest(".guide-jump")) && (document.activeElement?.textContent || "").replace(/\s+/g, " ").trim() === "Now"),
    ).toBe(true);
    await press(page, "Enter");
    await expect.poll(async () => (await page.evaluate(readGrid)).now).toBe(true);

    await until(page, "ArrowDown", async () => (await page.evaluate(readGrid)).onGrid, 8);
    await press(page, "Enter");
    const sheet = page.getByRole("dialog");
    await expect(sheet).toBeVisible();
    await expectRing(page);
    await press(page, "Escape");
    await expect(sheet).toHaveCount(0);
    await expect.poll(async () => page.evaluate(() => document.activeElement?.getAttribute("role") ?? "body")).toBe("grid");
    await expectRing(page);

    await press(page, "Escape");
    await expect(page).toHaveURL(/\/$/);

    await until(page, "ArrowRight", async () => labelIs(page, "Settings"), 10);
    await press(page, "Enter");
    await expect(page).toHaveURL(/\/settings$/);
    await expect(page.getByRole("button", { name: "Diagnostics" })).toBeVisible();
    await until(page, "ArrowDown", async () => page.evaluate(() => Boolean(document.activeElement?.closest("[aria-label='Picture motion']"))), 60);
    await until(page, "ArrowLeft", async () => labelIs(page, "Broadcast"), 4);
    await press(page, "Enter");
    await expect(page.locator("[aria-label='Picture motion'] .seg.on")).toHaveText("Broadcast");
    await page.screenshot({ path: path.join(evidence, "settings.jpg"), type: "jpeg", quality: 60 });

    const stopped = await page.request.post(`/api/v1/recordings/${recordingId}/stop`);
    expect(stopped.ok()).toBeTruthy();
    await expect
      .poll(async () => {
        const body = (await (await page.request.get("/api/v1/recordings")).json()) as { recordings?: { id: number; status: string }[] };
        return body.recordings?.find((rec) => rec.id === recordingId)?.status ?? "";
      })
      .not.toBe("recording");

    await until(page, "ArrowUp", async () => page.evaluate(() => Boolean(document.activeElement?.closest(".topbar"))), 80);
    await reachTop(page, "Recordings");
    await press(page, "Enter");
    await expect(page).toHaveURL(/\/recordings$/);
    const card = page.locator(".media-card", { has: page.locator(`a[href="/api/v1/recordings/${recordingId}/file"]`) });
    await expect(card).toBeVisible();
    await until(page, "ArrowDown", async () => page.evaluate((id) => {
      const el = document.activeElement;
      const row = el?.closest(".media-card");
      return Boolean(row?.querySelector(`a[href="/api/v1/recordings/${id}/file"]`));
    }, recordingId), 30);
    await until(page, "ArrowLeft", async () => page.evaluate(() => (document.activeElement?.textContent || "").replace(/\s+/g, " ").trim() === "Play"), 8);
    await press(page, "Enter");
    await expect(page).toHaveURL(new RegExp(`/play\\?recording=${recordingId}`));
    await expectRing(page);
    await press(page, "Escape");
    await expect(page).toHaveURL(/\/recordings$/);
    await expect.poll(async () => (await page.evaluate(readRing)).body).toBe(false);

    await until(page, "ArrowDown", async () => page.evaluate((id) => {
      const el = document.activeElement;
      const row = el?.closest(".media-card");
      return Boolean(row?.querySelector(`a[href="/api/v1/recordings/${id}/file"]`));
    }, recordingId), 30);
    await until(page, "ArrowRight", async () => page.evaluate((id) => {
      const el = document.activeElement;
      return Boolean(el?.getAttribute("href")?.includes(`/recordings/${id}/file`) && el?.hasAttribute("download"));
    }, recordingId), 8);
    await page.screenshot({ path: path.join(evidence, "recordings.jpg"), type: "jpeg", quality: 60 });
    const download = page.waitForEvent("download");
    await press(page, "Enter");
    await (await download).cancel();

    await until(page, "ArrowRight", async () => page.evaluate((id) => {
      const el = document.activeElement;
      const row = el?.closest(".media-card");
      const text = (el?.textContent || "").replace(/\s+/g, " ").trim();
      return Boolean(row?.querySelector(`a[href="/api/v1/recordings/${id}/file"]`) && text === "Delete");
    }, recordingId), 6);
    await press(page, "Enter");
    await expect.poll(async () => (await page.evaluate(readRing)).label).toMatch(/^Delete this file /);
    await expectRing(page);
    await page.screenshot({ path: path.join(evidence, "delete.jpg"), type: "jpeg", quality: 60 });
    await press(page, "Enter");
    await expect(card).toHaveCount(0);
    await expect.poll(async () => (await page.evaluate(readRing)).body).toBe(false);
  } finally {
    await page.request.post(`/api/v1/recordings/${recordingId}/stop`).catch(() => undefined);
    await page.request.delete(`/api/v1/recordings/${recordingId}`).catch(() => undefined);
  }
});

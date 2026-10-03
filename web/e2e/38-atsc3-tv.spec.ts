// Opt-in with the 3.0 lineup: E2E_ATSC3=1 npm run e2e -- 38-atsc3-tv
// 1920×1080, remote keys only. The 3.0 tag, the Settings choice, and the encrypted line.
import { mkdirSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import type { Locator, Page } from "@playwright/test";
import { expect, holdClock, test } from "./fixture";
import { settle } from "./snap";
import { copy } from "../src/strings";

const evidence = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "../../.evidence/lane/l75");

type Ring = { body: boolean; visible: boolean; drawn: boolean; label: string };

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
    label: (el?.getAttribute("aria-label") || el?.textContent || "").replace(/\s+/g, " ").trim().slice(0, 100),
  };
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
  expect(await done(), `stopped on ${ring.label}`).toBe(true);
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

async function shot(locator: Locator, name: string) {
  mkdirSync(evidence, { recursive: true });
  await locator.screenshot({ path: path.join(evidence, name), type: "jpeg", quality: 70 });
}

async function contained(tag: Locator, host: Locator) {
  const inner = await tag.boundingBox();
  const outer = await host.boundingBox();
  expect(inner, "tag").toBeTruthy();
  expect(outer, "host").toBeTruthy();
  expect(inner!.width).toBeGreaterThan(16);
  expect(inner!.width).toBeLessThan(96);
  expect(inner!.x).toBeGreaterThanOrEqual(outer!.x - 1);
  expect(inner!.y).toBeGreaterThanOrEqual(outer!.y - 1);
  expect(inner!.x + inner!.width).toBeLessThanOrEqual(outer!.x + outer!.width + 1);
  expect(inner!.y + inner!.height).toBeLessThanOrEqual(outer!.y + outer!.height + 1);
}

async function inView(page: Page, locator: Locator) {
  const box = await locator.boundingBox();
  const view = page.viewportSize();
  expect(box).toBeTruthy();
  expect(view).toBeTruthy();
  const bar = await page.locator(".topbar").boundingBox();
  const top = bar ? bar.y + bar.height : 0;
  expect(box!.y).toBeGreaterThanOrEqual(top - 1);
  expect(box!.x).toBeGreaterThanOrEqual(-1);
  expect(box!.x + box!.width).toBeLessThanOrEqual(view!.width + 1);
  expect(box!.y + box!.height).toBeLessThanOrEqual(view!.height + 1);
}

test("a remote sees the 3.0 tag, the 3.0/1.0 choice, and the encrypted line", async ({ page }) => {
  test.setTimeout(180_000);
  await tv(page);
  expect((await page.request.put("/api/v1/settings", { data: { setupComplete: "1", watermarkGB: "0" } })).ok()).toBe(true);
  await holdClock(page);
  await page.setViewportSize({ width: 1920, height: 1080 });
  await page.goto("/");
  await settle(page);
  await expect(page.locator("html")).toHaveAttribute("data-layout", "tv");
  await expectRing(page);

  const card = (number: string) => page.locator(".now-card").filter({ hasText: number });

  await until(page, "ArrowDown", async () => (await page.locator(".now-card").evaluateAll((nodes) => nodes.some((node) => node === document.activeElement))), 8);
  await until(page, "ArrowRight", async () => page.evaluate(() => (document.activeElement?.textContent || "").includes("104.1")), 8);
  const clearCard = card("104.1");
  await expect(clearCard.locator(".tag")).toHaveText("3.0");
  await expect(clearCard).toContainText("KBWV");
  await contained(clearCard.locator(".tag"), clearCard);
  await inView(page, clearCard.locator(".tag"));
  await shot(clearCard, "now.jpg");

  // The encrypted row stays off Home and the guide.
  await expect(card("115.1")).toHaveCount(0);

  await until(page, "ArrowRight", async () => page.evaluate(() => document.activeElement?.textContent?.trim() === "Guide"), 14);
  await press(page, "Enter");
  await expect(page).toHaveURL(/\/guide$/);
  await until(page, "ArrowDown", async () => page.evaluate(() => document.activeElement?.getAttribute("role") === "grid"), 12);

  async function guideRow(number: string) {
    await until(
      page,
      "ArrowDown",
      async () =>
        page.evaluate((want) => {
          const id = document.querySelector(".guide-canvas")?.getAttribute("aria-activedescendant");
          const cell = id ? document.getElementById(id) : null;
          return (cell?.closest("[role='row']")?.textContent || "").includes(want);
        }, number),
      8,
    );
    const row = page.locator("[role='row']").filter({ hasText: number });
    const channel = row.locator(".guide-channel");
    await expect(channel).toHaveAttribute("aria-label", new RegExp(`${number}.*ATSC 3.0`));
    await expect(channel.locator(".tag")).toHaveText("3.0");
    await contained(channel.locator(".tag"), channel);
    await inView(page, channel.locator(".tag"));
    return channel;
  }

  await shot(await guideRow("104.1"), "guide.jpg");
  await expect(page.locator("[role='row']").filter({ hasText: "115.1" })).toHaveCount(0);

  await until(page, "ArrowUp", async () => page.evaluate(() => Boolean(document.activeElement?.closest("nav"))), 16);
  await until(page, "ArrowRight", async () => page.evaluate(() => document.activeElement?.textContent?.replace(/\s+/g, " ").trim() === "Search"), 8);
  await press(page, "Enter");
  await expect(page).toHaveURL(/\/search/);
  await expect(page.getByLabel("Search shows, people, and recordings")).toBeFocused();
  await page.keyboard.type("Sealed");
  await page.keyboard.press("Enter");
  const found = page.locator("button.search-main").filter({ hasText: "Sealed Signal" });
  await expect(found).toBeVisible();
  await until(page, "ArrowDown", async () => page.evaluate(() => (document.activeElement?.textContent || "").includes("Sealed Signal")), 6);
  await expect(found.locator(".tag")).toHaveText("3.0");
  await expect(found).toContainText("115.1");
  await contained(found.locator(".tag"), found);
  await inView(page, found.locator(".tag"));
  await shot(found, "search.jpg");

  // Its show plays the regular broadcast, with the note clear of the tuning card.
  await press(page, "Enter");
  const sheet = page.getByRole("dialog", { name: /Sealed Signal/ });
  await expect(sheet).toBeVisible();
  await until(page, "ArrowDown", async () => page.evaluate(() => document.activeElement?.textContent?.trim() === "Watch"), 8);
  await press(page, "Enter");
  const note = page.locator(".player-note");
  await expect(note).toHaveText(copy.player.encrypted);
  await inView(page, note);
  const overlapsTuning = await page.evaluate(() => {
    const message = document.querySelector(".player-note");
    const card = document.querySelector(".tuning-card");
    if (!message || !card) return false;
    const a = message.getBoundingClientRect();
    const b = card.getBoundingClientRect();
    return a.top < b.bottom && a.bottom > b.top && a.left < b.right && a.right > b.left;
  });
  expect(overlapsTuning).toBe(false);
  await shot(page.locator(".stage"), "player.jpg");
  await press(page, "Escape");
  await expect(page).not.toHaveURL(/\/watch/);

  await until(page, "ArrowUp", async () => page.evaluate(() => Boolean(document.activeElement?.closest("nav"))), 8);
  await until(page, "ArrowRight", async () => page.evaluate(() => document.activeElement?.getAttribute("aria-label") === "Settings"), 8);
  await press(page, "Enter");
  await expect(page).toHaveURL(/\/settings/);
  await expect(page.getByRole("heading", { name: "Tuners and channels" })).toBeVisible();

  const choice = (number: string) => page.getByLabel(`Show ${number}`, { exact: true });
  await expect(choice("104.1")).toBeAttached();
  await until(page, "ArrowDown", async () => page.evaluate(() => document.activeElement?.getAttribute("aria-label") === "Show 104.1"), 120);
  const before = await choice("104.1").inputValue();
  await inView(page, choice("104.1"));
  await page.keyboard.press("ArrowRight");
  await expect(choice("104.1")).not.toHaveValue(before);
  await expect(choice("104.1")).toBeFocused();
  await inView(page, choice("104.1"));
  await page.keyboard.press("ArrowLeft");
  await expect(choice("104.1")).toHaveValue(before);
  await shot(page.locator(".source-row").filter({ has: choice("104.1") }), "choice.jpg");

  const playsAs = copy.sources.playsAs("5.1");
  await until(
    page,
    "ArrowDown",
    async () => page.evaluate((sentence) => Boolean(document.activeElement?.closest(".source-row")?.textContent?.includes(sentence)), playsAs),
    12,
  );
  const sealedRow = page.locator(".source-row").filter({ hasText: playsAs });
  const sealedLine = sealedRow.locator(".protected-note");
  await expect(sealedLine).toHaveText(playsAs);
  await inView(page, sealedLine);
  const numberClipped = await sealedRow.locator(".num-input").evaluate((node) => (node as HTMLInputElement).scrollWidth > node.clientWidth + 2);
  expect(numberClipped).toBe(false);
  const clipped = await sealedLine.evaluate((node) => node.scrollHeight > node.clientHeight + 4 || node.scrollWidth > node.clientWidth + 4);
  expect(clipped).toBe(false);
  await shot(sealedRow, "settings.jpg");

  const locked = page.locator(".protected-note").filter({ hasText: copy.sources.encrypted(true) });
  await expect(locked).toHaveCount(1);
  await inView(page, locked);
  await shot(locked, "encrypted.jpg");
});

import { mkdirSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import type { Page } from "@playwright/test";
import { expect, test } from "./fixture";
import { settle } from "./snap";

const evidence = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "../../.evidence/lane/l38");

type Ring = { body: boolean; visible: boolean; drawn: boolean; label: string; role: string };

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

async function until(page: Page, key: string, done: () => Promise<boolean>, limit = 16) {
  for (let i = 0; i < limit; i++) {
    if (await done()) return;
    await press(page, key);
  }
  expect(await done(), key).toBe(true);
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

test("a remote walks home, the guide, the player, and back", async ({ page }) => {
  test.setTimeout(90_000);
  await tv(page);
  const setup = await page.request.put("/api/v1/settings", { data: { setupComplete: "1" } });
  expect(setup.ok()).toBeTruthy();
  await page.setViewportSize({ width: 1920, height: 1080 });
  await page.goto("/");
  await settle(page);
  await expect(page.locator("html")).toHaveAttribute("data-layout", "tv");
  await expectRing(page);

  await until(page, "ArrowRight", async () =>
    page.evaluate(() => {
      const el = document.activeElement;
      return Boolean(el?.closest(".tabs")) && (el?.textContent || "").replace(/\s+/g, " ").trim() === "Guide";
    }),
  );
  await press(page, "Enter");
  await expect(page).toHaveURL(/\/guide$/);
  await expectRing(page);

  await until(
    page,
    "ArrowDown",
    async () => page.evaluate(() => document.activeElement?.getAttribute("role") === "grid"),
    12,
  );
  const cellId = await page.locator(".guide-canvas").getAttribute("aria-activedescendant");
  expect(cellId).toBeTruthy();
  const cell = page.locator(`[id="${cellId}"]`);
  await expect(cell).toHaveClass(/now/);
  const title = ((await cell.getAttribute("aria-label")) || "").split(",")[0].trim();
  expect(title.length).toBeGreaterThan(0);

  await press(page, "Enter");
  const sheet = page.getByRole("dialog", { name: title });
  await expect(sheet).toBeVisible();
  await expect(sheet.getByRole("button", { name: "Watch", exact: true })).toBeFocused();
  await expectRing(page);
  mkdirSync(evidence, { recursive: true });
  await page.screenshot({ path: path.join(evidence, "on-now.jpg"), animations: "disabled" });

  await press(page, "Backspace");
  await expect(sheet).toHaveCount(0);
  await expect(page).toHaveURL(/\/guide$/);
  await expectRing(page);

  await press(page, "Enter");
  await expect(sheet).toBeVisible();
  await press(page, "Enter");
  await expect(page).toHaveURL(/\/watch\?channel=\d+/);
  const player = page.getByRole("region", { name: "Player" });
  await expect(player).toBeVisible();
  await expectRing(page);
  const first = new URL(page.url()).searchParams.get("channel");
  expect(first).toBeTruthy();

  await until(page, "ArrowRight", async () => page.evaluate(() => document.activeElement?.getAttribute("aria-label") === "Channels"), 8);
  await press(page, "Enter");
  const guide = page.getByRole("listbox", { name: "Channels" });
  await expect(guide).toBeVisible();
  await expectRing(page);
  await page.screenshot({ path: path.join(evidence, "mini-guide.jpg"), animations: "disabled" });

  await press(page, "Escape");
  await expect(guide).toHaveCount(0);
  await expect(page).toHaveURL(new RegExp(`/watch\\?channel=${first}`));
  await expectRing(page);

  await press(page, "Enter");
  await expect(guide).toBeVisible();
  const before = (await guide.locator("[aria-selected='true']").innerText()).replace(/\s+/g, " ").trim();
  await press(page, "ArrowDown");
  const after = (await guide.locator("[aria-selected='true']").innerText()).replace(/\s+/g, " ").trim();
  expect(after).not.toBe(before);
  await press(page, "Enter");
  await expect(guide).toHaveCount(0);
  await expect.poll(() => new URL(page.url()).searchParams.get("channel")).not.toBe(first);
  const down = new URL(page.url()).searchParams.get("channel");
  await expectRing(page);

  await press(page, "Enter");
  await expect(guide).toBeVisible();
  await press(page, "ArrowUp");
  await press(page, "Enter");
  await expect.poll(() => new URL(page.url()).searchParams.get("channel")).toBe(first);
  await expectRing(page);

  await until(page, "ArrowRight", async () => page.evaluate(() => document.activeElement?.getAttribute("aria-label") === "Side by side"), 8);
  await press(page, "Enter");
  await expect(page).toHaveURL(/\/multiview\?/);
  await expect(page.getByRole("listbox", { name: "Add a channel" })).toBeVisible();
  await expectRing(page);
  const picked = await page.evaluate(() => document.activeElement?.getAttribute("aria-selected") !== "true");
  expect(picked).toBe(true);
  await press(page, "Enter");
  await expect.poll(() => new URL(page.url()).searchParams.get("ch")?.split(",").filter(Boolean).length ?? 0).toBe(2);
  await expect(page.locator(".mv-cell")).toHaveCount(2, { timeout: 15_000 });
  await expectRing(page);
  await page.screenshot({ path: path.join(evidence, "side-by-side.jpg"), animations: "disabled" });

  await press(page, "Backspace");
  await expect(page).toHaveURL(new RegExp(`/watch\\?channel=${first}`));
  await expect(player).toBeVisible();
  await expectRing(page);

  await press(page, "Escape");
  await expect(page).toHaveURL(/\/guide$/);
  await expect(player).toHaveCount(0);
  await expectRing(page);

  await press(page, "Backspace");
  await expect(page).toHaveURL(/\/$/);
  await expectRing(page);
  await page.screenshot({ path: path.join(evidence, "home.jpg"), animations: "disabled" });

  await press(page, "Escape");
  await expect(page).toHaveURL(/\/$/);
  expect(down).toBeTruthy();
});

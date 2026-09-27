import { spawnSync } from "node:child_process";
import { readFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { expect, test } from "./fixture";
import { settle } from "./snap";

const here = path.dirname(fileURLToPath(import.meta.url));

function sql(statement: string) {
  const server = JSON.parse(readFileSync(path.join(here, ".run/server.json"), "utf8")) as { db: string };
  const result = spawnSync("sqlite3", [server.db, statement], { encoding: "utf8" });
  if (result.status !== 0) throw new Error(result.stderr || result.stdout || "sqlite failed");
}

async function setupDone(page: import("@playwright/test").Page) {
  const res = await page.request.put("/api/v1/settings", { data: { setupComplete: "1" } });
  expect(res.ok()).toBeTruthy();
}

async function inView(page: import("@playwright/test").Page, name: string) {
  const button = page.getByRole("button", { name, exact: true });
  await expect(button).toBeEnabled();
  const box = await button.boundingBox();
  const viewport = page.viewportSize();
  expect(box, name).toBeTruthy();
  expect(viewport).toBeTruthy();
  expect(box!.y).toBeGreaterThanOrEqual(-1);
  expect(box!.y + box!.height).toBeLessThanOrEqual(viewport!.height + 1);
  expect(box!.x).toBeGreaterThanOrEqual(-1);
  expect(box!.x + box!.width).toBeLessThanOrEqual(viewport!.width + 1);
}

test("phone guide shows the filters, now, and tonight", async ({ page }) => {
  await setupDone(page);
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto("/guide");
  await settle(page);
  const prime = await page.evaluate(() => {
    const at = new Date();
    at.setHours(20, 0, 0, 0);
    if (at.getTime() < Date.now()) at.setDate(at.getDate() + 1);
    const end = new Date(at.getTime() + 2 * 60 * 60_000);
    const stamp = (date: Date) => date.toISOString().replace(/\.\d{3}Z$/, "Z");
    return { start: stamp(at), end: stamp(end) };
  });
  sql(`
PRAGMA busy_timeout=5000;
-- Channel 2 carries nothing this test checks; clear its slot so Tonight shows the movie at any hour.
DELETE FROM airings WHERE channel_id = 2 AND starts_at < '${prime.end}' AND ends_at > '${prime.start}';
INSERT INTO airings (channel_id, title, subtitle, description, category, starts_at, ends_at, program_id, is_live, guide_source)
VALUES (2, 'Prime Movie', 'Late showing', 'A movie.', 'Movies', '${prime.start}', '${prime.end}', 'e2e-prime', 0, 'e2e');
`);
  await page.reload();
  await settle(page);
  const chips = page.locator(".guide-chips");
  const chipBox = await chips.boundingBox();
  const favorites = await page.getByRole("button", { name: "Favorites" }).boundingBox();
  expect(chipBox && favorites).toBeTruthy();
  expect(chipBox!.width).toBeGreaterThan(240);
  expect(favorites!.x).toBeGreaterThanOrEqual(chipBox!.x - 1);
  expect(favorites!.x + 24).toBeLessThanOrEqual(chipBox!.x + chipBox!.width);
  await expect(page.getByRole("button", { name: "Now", exact: true })).toHaveAttribute("aria-pressed", "true");
  await expect(page.getByRole("button", { name: /Chiefs at Bills/ })).toBeVisible();
  await page.getByRole("button", { name: "Tonight", exact: true }).click();
  await expect(page.getByRole("button", { name: "Tonight", exact: true })).toHaveAttribute("aria-pressed", "true");
  await expect(page.getByRole("button", { name: /Prime Movie/ })).toBeVisible();
  await page.getByRole("button", { name: "Now", exact: true }).click();
  await expect(page.getByRole("button", { name: /Chiefs at Bills/ })).toBeVisible();
  // It can still show as the channel's next show; it is not what is on.
  await expect(page.locator(".onnow-title", { hasText: "Prime Movie" })).toHaveCount(0);
});

test("arrow keys, Enter, and Escape drive the TV layout", async ({ page }) => {
  await page.addInitScript(() => {
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
  await setupDone(page);
  await page.setViewportSize({ width: 1920, height: 1080 });
  await page.goto("/");
  await settle(page);
  await expect(page.locator("html")).toHaveAttribute("data-layout", "tv");
  await expect(page.getByRole("tab", { name: "Guide" })).toBeVisible();
  await page.evaluate(() => {
    const el = document.activeElement;
    if (el instanceof HTMLElement) el.blur();
  });
  expect(await page.evaluate(() => document.activeElement?.tagName ?? "")).toBe("BODY");
  for (let i = 0; i < 14; i++) {
    const step = await page.evaluate(() => {
      const el = document.activeElement instanceof HTMLElement ? document.activeElement : null;
      const tabs = [...document.querySelectorAll<HTMLElement>(".tabs [role='tab']")];
      const guide = tabs.find((node) => (node.textContent || "").replace(/\s+/g, " ").trim() === "Guide");
      const name = (el?.textContent || "").replace(/\s+/g, " ").trim() ?? "";
      const inTabs = Boolean(el?.closest(".tabs"));
      const body = !el || el === document.body || el === document.documentElement;
      if (inTabs && name === "Guide") return "done";
      if (body) return "ArrowDown";
      if (!inTabs || !guide || !el) return "ArrowUp";
      const dx = guide.getBoundingClientRect().x - el.getBoundingClientRect().x;
      return dx > 4 ? "ArrowRight" : "ArrowLeft";
    });
    if (step === "done") break;
    await page.keyboard.press(step);
  }
  await expect(page.locator(".tabs :focus")).toHaveText("Guide");
  await page.keyboard.press("Enter");
  await expect(page).toHaveURL(/\/guide/);
  await expect(page.getByRole("grid", { name: "TV guide" })).toBeVisible();
  for (let i = 0; i < 10; i++) {
    const role = await page.evaluate(() => document.activeElement?.getAttribute("role") ?? "");
    if (role === "grid") break;
    await page.keyboard.press("ArrowDown");
  }
  await expect(page.locator(":focus")).toHaveAttribute("role", "grid");
  await page.keyboard.press("Enter");
  await expect(page.getByRole("dialog")).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(page.getByRole("dialog")).toHaveCount(0);
  await expect(page).toHaveURL(/\/guide/);
  await page.keyboard.press("Escape");
  await expect(page).toHaveURL(/\/$/);
  await page.keyboard.press("Escape");
  await expect(page).toHaveURL(/\/$/);
});

test("setup's main button stays on screen at phone and TV sizes", async ({ page }) => {
  await page.addInitScript(() => {
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
  const put = async (value: string) => {
    const res = await page.request.put("/api/v1/settings", { data: { setupComplete: value } });
    expect(res.ok()).toBeTruthy();
  };
  try {
    await put("0");
    await page.setViewportSize({ width: 390, height: 844 });
    await page.goto("/");
    await expect(page.getByRole("heading", { name: "Let's set up your TV" })).toBeVisible();
    await expect(page.locator("html")).toHaveAttribute("data-layout", "phone");
    await page.getByRole("button", { name: "Sources" }).click();
    await inView(page, "Continue");
    await page.getByRole("button", { name: "Ready" }).click();
    await inView(page, "Watch");
    await page.setViewportSize({ width: 1920, height: 1080 });
    await expect(page.locator("html")).toHaveAttribute("data-layout", "tv");
    await inView(page, "Watch");
  } finally {
    await put("1");
  }
});

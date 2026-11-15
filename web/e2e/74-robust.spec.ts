// The layouts hold up with data and screens the default lineup never shows:
// long names and titles, a 320 px phone, a phone on its side, a TV browser that
// reports 960 px, and Windows High Contrast.
import { spawnSync } from "node:child_process";
import { readFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import type { Page } from "@playwright/test";
import { expect, test } from "./fixture";
import { settle } from "./snap";

const here = path.dirname(fileURLToPath(import.meta.url));

function sql(statement: string) {
  const server = JSON.parse(readFileSync(path.join(here, ".run/server.json"), "utf8")) as { db: string };
  const result = spawnSync("sqlite3", [server.db, `PRAGMA busy_timeout=5000;\n${statement}`], { encoding: "utf8" });
  if (result.status !== 0) throw new Error(result.stderr || result.stdout || "sqlite failed");
  return result.stdout.trim();
}

async function setupDone(page: Page) {
  expect((await page.request.put("/api/v1/settings", { data: { setupComplete: "1" } })).ok()).toBeTruthy();
}

type Box = { x: number; y: number; width: number; height: number };

function overlaps(a: Box, b: Box) {
  return a.x < b.x + b.width - 1 && b.x < a.x + a.width - 1 && a.y < b.y + b.height - 1 && b.y < a.y + a.height - 1;
}

const longName = "KXYZ-DT Metropolitan Public Broadcasting Network HD";
const longTitle = "NCAA Football: Southern Methodist University Mustangs at Northwestern State Demons";

test("a long channel name and a long title stay in their boxes on a phone", async ({ page }) => {
  await setupDone(page);
  sql(`UPDATE channels SET custom_name = '${longName}' WHERE id = 2;`);
  // The title comes from the listings the page fetches, so the long one is put there.
  await page.route("**/api/v1/airings**", async (route) => {
    const res = await route.fetch();
    const body = (await res.text()).replaceAll("NFL: Bears at Bills", longTitle);
    await route.fulfill({ response: res, body });
  });
  try {
    await page.setViewportSize({ width: 320, height: 568 });
    await page.goto("/guide");
    await settle(page);
    const row = page.locator(".onnow-row").filter({ hasText: "4.2" }).first();
    await expect(row.locator(".ch-badge-name")).toHaveText(longName);
    // The name is cut inside its column, not run under the title beside it.
    const fit = await row.evaluate((el) => {
      const name = el.querySelector(".ch-badge-name")!.getBoundingClientRect();
      const body = el.querySelector(".onnow-body")!.getBoundingClientRect();
      return name.right <= body.left + 1;
    });
    expect(fit).toBe(true);

    await page.goto("/");
    await settle(page);
    const hero = page.locator(".hero-title");
    await expect(hero).toHaveText(longTitle);
    const lines = await hero.evaluate((el) => {
      const style = getComputedStyle(el);
      return el.getBoundingClientRect().height / (parseFloat(style.lineHeight) || parseFloat(style.fontSize));
    });
    expect(lines).toBeLessThan(3.5);
  } finally {
    sql(`UPDATE channels SET custom_name = '' WHERE id = 2;`);
  }
});

test("a TV browser that reports 960 px keeps the tabs and chips apart", async ({ page }) => {
  await setupDone(page);
  await page.setViewportSize({ width: 960, height: 540 });
  await page.goto("/guide?layout=tv");
  await settle(page);
  await expect(page.locator("html")).toHaveAttribute("data-layout", "tv");
  const tabs = await page.locator(".tabs .tab").evaluateAll((els) => els.map((el) => el.getBoundingClientRect().toJSON() as Box));
  expect(tabs.length).toBeGreaterThan(4);
  for (let i = 1; i < tabs.length; i++) expect(overlaps(tabs[i - 1], tabs[i]), `tab ${i}`).toBe(false);
  // A chip is as wide as its label.
  const cut = await page.locator(".guide-chips > *").evaluateAll((els) => els.filter((el) => el.scrollWidth > el.clientWidth + 1).length);
  expect(cut).toBe(0);
  // The 10-foot type scales with the page; at 1920 it is the full size.
  const small = await page.evaluate(() => parseFloat(getComputedStyle(document.body).fontSize));
  await page.setViewportSize({ width: 1920, height: 1080 });
  await expect.poll(() => page.evaluate(() => parseFloat(getComputedStyle(document.body).fontSize))).toBe(26);
  expect(small).toBeLessThan(26);
  expect(small).toBeGreaterThanOrEqual(16);
  await page.goto("/?layout=auto");
});

test("a phone on its side keeps the guide's controls on one row", async ({ page }) => {
  await setupDone(page);
  await page.setViewportSize({ width: 568, height: 320 });
  await page.goto("/guide");
  await settle(page);
  await expect(page.locator("html")).toHaveAttribute("data-layout", "phone");
  const controls = await page.locator(".guide-controls").boundingBox();
  const search = await page.locator(".guide-search").boundingBox();
  const now = await page.getByRole("button", { name: "Now", exact: true }).boundingBox();
  expect(controls && search && now).toBeTruthy();
  expect(Math.abs(search!.y - now!.y)).toBeLessThan(8);
  expect(controls!.height).toBeLessThan(64);
});

test("settings on a 320 px phone put a menu under its name", async ({ page }) => {
  await setupDone(page);
  await page.setViewportSize({ width: 320, height: 568 });
  await page.goto("/settings/recording");
  await settle(page);
  const row = page.locator(".set-row").filter({ has: page.locator("select") }).first();
  await expect(row).toBeVisible();
  const placed = await row.evaluate((el) => {
    const label = el.querySelector(".set-row-label")!.getBoundingClientRect();
    const select = el.querySelector("select")!.getBoundingClientRect();
    return { below: select.top >= label.bottom - 1, labelWidth: label.width };
  });
  expect(placed.below).toBe(true);
  expect(placed.labelWidth).toBeGreaterThan(150);
});

test("multiview's top bar stays on a 320 px screen", async ({ page }) => {
  await setupDone(page);
  await page.setViewportSize({ width: 320, height: 568 });
  await page.goto("/multiview");
  const bar = page.locator(".mv-top");
  await expect(bar).toBeVisible();
  const outside = await bar.evaluate((el) =>
    [...el.querySelectorAll("button")].filter((b) => {
      const r = b.getBoundingClientRect();
      return r.width > 0 && (r.right > window.innerWidth + 1 || r.left < -1 || r.width < 30);
    }).length,
  );
  expect(outside).toBe(0);
});

test.describe("forced colors", () => {
  test.use({ forcedColors: "active" });

  test("focus and the chosen tab show in Windows High Contrast", async ({ page }) => {
    await setupDone(page);
    await page.goto("/guide");
    await settle(page);
    const current = page.locator(".tabs .tab.on");
    await expect(current).toHaveCount(1);
    const fill = await current.evaluate((el) => getComputedStyle(el).backgroundColor);
    expect(fill).not.toBe("rgba(0, 0, 0, 0)");
    const other = page.locator(".tabs .tab:not(.on)").first();
    await other.focus();
    await page.keyboard.press("Shift+Tab");
    await page.keyboard.press("Tab");
    const ring = await page.evaluate(() => {
      const el = document.activeElement as HTMLElement;
      const style = getComputedStyle(el);
      return { visible: el.matches(":focus-visible"), outline: style.outlineStyle, width: parseFloat(style.outlineWidth) };
    });
    expect(ring.visible).toBe(true);
    expect(ring.outline).not.toBe("none");
    expect(ring.width).toBeGreaterThanOrEqual(2);
  });
});

test("installed on a phone, the pages start below the status bar and clear the notch", async ({ page, context }) => {
  await setupDone(page);
  await page.setViewportSize({ width: 390, height: 844 });
  const cdp = await context.newCDPSession(page);
  await cdp.send("Emulation.setSafeAreaInsetsOverride", { insets: { top: 59, bottom: 34, left: 0, right: 0 } });
  await page.goto("/guide");
  await settle(page);
  const top = await page.getByRole("button", { name: "Now", exact: true }).boundingBox();
  expect(top!.y).toBeGreaterThanOrEqual(59);
  const bar = await page.locator(".topbar").boundingBox();
  expect(bar!.y + bar!.height).toBeLessThanOrEqual(844 - 34);
  await page.goto("/multiview");
  const close = await page.locator(".mv-top .glass-icon").boundingBox();
  expect(close!.y).toBeGreaterThanOrEqual(59);
  await page.setViewportSize({ width: 844, height: 390 });
  await cdp.send("Emulation.setSafeAreaInsetsOverride", { insets: { top: 0, bottom: 21, left: 59, right: 59 } });
  await page.goto("/guide");
  await settle(page);
  const grid = await page.locator(".guide-scroll").boundingBox();
  expect(grid!.x).toBeGreaterThanOrEqual(59);
  expect(grid!.x + grid!.width).toBeLessThanOrEqual(844 - 59 + 1);
});

async function spill(page: Page, selector: string) {
  return page.locator(selector).first().evaluate((el) => {
    const edge = el.getBoundingClientRect().right;
    return [...el.querySelectorAll("input, button, select, p")]
      .filter((child) => {
        const r = child.getBoundingClientRect();
        return r.width > 0 && r.right > edge + 1;
      })
      .map((child) => child.textContent?.trim().slice(0, 30) || (child as HTMLInputElement).type || child.tagName);
  });
}

test("setup and the tuner settings hold everything in their cards on a 320 px phone", async ({ page }) => {
  await setupDone(page);
  await page.setViewportSize({ width: 320, height: 568 });
  await page.goto("/setup");
  const card = page.locator(".setup-card").first();
  await expect(card.getByText("Not seeing your tuner?")).toBeVisible();
  await card.locator("summary").filter({ hasText: "Other sources" }).click();
  await expect(card.getByText("Playlist or Xtream server")).toBeVisible();
  expect(await spill(page, ".setup-card")).toEqual([]);

  await page.goto("/settings/tuners");
  await settle(page);
  const cards = page.locator(".set-card");
  await expect(cards.first()).toBeVisible();
  for (let i = 0; i < (await cards.count()); i++) {
    expect(await spill(page, `.set-card >> nth=${i}`), `card ${i}`).toEqual([]);
  }
});

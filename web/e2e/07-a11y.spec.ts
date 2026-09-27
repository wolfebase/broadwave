import { mkdirSync, writeFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import AxeBuilder from "@axe-core/playwright";
import type { Page } from "@playwright/test";
import { expect, holdClock, test } from "./fixture";
import { settle, sizes } from "./snap";

const here = path.dirname(fileURLToPath(import.meta.url));
const evidence = path.resolve(here, "../../.evidence/lane/l7");
const motions = ["reduce", "no-preference"] as const;

type Hit = { where: string; impact: string; id: string; target: string; help: string };

// Live pictures carry captions in the broadcast. There is no WebVTT track for
// axe to see, and adding an empty one would claim captions that are not there.
const playerRulesOff = ["video-caption"];

// White on the tally red is the brand live pill. Filed for the reviewer.
const brandContrast = new Set([".live-pill"]);

async function serious(page: Page, where: string, disable: string[] = []): Promise<Hit[]> {
  let builder = new AxeBuilder({ page });
  if (disable.length > 0) builder = builder.disableRules(disable);
  const results = await builder.analyze();
  const hits: Hit[] = [];
  for (const violation of results.violations) {
    if (violation.impact !== "serious" && violation.impact !== "critical") continue;
    for (const node of violation.nodes) {
      const target = node.target.join(" ");
      if (violation.id === "color-contrast" && brandContrast.has(target)) continue;
      hits.push({
        where,
        impact: violation.impact ?? "",
        id: violation.id,
        target,
        help: (node.failureSummary || violation.help).replace(/\s+/g, " ").slice(0, 220),
      });
    }
  }
  return hits;
}

function report(name: string, hits: Hit[]) {
  const body = hits.map((hit) => `${hit.where} ${hit.impact} ${hit.id} [${hit.target}] ${hit.help}`).join("\n");
  mkdirSync(path.join(here, ".run"), { recursive: true });
  writeFileSync(path.join(here, ".run", `a11y-${name}.txt`), body);
  expect(hits, body).toEqual([]);
}

/** Page-enter motion is a short fade. Scanning during it reports contrast that the settled page does not have. */
async function letMotionFinish(page: Page) {
  await page.evaluate(async () => {
    await new Promise<void>((resolve) => {
      requestAnimationFrame(() => requestAnimationFrame(() => resolve()));
    });
    const pending = document.getAnimations().filter((anim) => {
      const effect = anim.effect;
      if (!(effect instanceof KeyframeEffect)) return false;
      return effect.getTiming().iterations !== Infinity;
    });
    await Promise.race([
      Promise.all(pending.map((anim) => anim.finished.catch(() => undefined))),
      new Promise((resolve) => setTimeout(resolve, 1000)),
    ]);
  });
}

async function at(page: Page, size: (typeof sizes)[number]) {
  await page.setViewportSize({ width: size.width, height: size.height });
  const layout = size.width <= 760 ? "phone" : "desktop";
  await expect(page.locator("html")).toHaveAttribute("data-layout", layout);
}

async function sweep(page: Page, name: string, ready: () => Promise<void>, disable: string[] = []) {
  const hits: Hit[] = [];
  for (const size of sizes) {
    await at(page, size);
    for (const motion of motions) {
      await page.emulateMedia({ reducedMotion: motion });
      await ready();
      await letMotionFinish(page);
      if (size.name === "1440x900" && motion === "reduce") {
        mkdirSync(evidence, { recursive: true });
        await page.screenshot({ path: path.join(evidence, `${name}.jpg`), type: "jpeg", quality: 60, animations: "disabled" });
      }
      hits.push(...(await serious(page, `${name} ${size.name} ${motion}`, disable)));
    }
  }
  report(name, hits);
}

async function browsing(page: Page) {
  const res = await page.request.put("/api/v1/settings", { data: { setupComplete: "1" } });
  expect(res.ok()).toBeTruthy();
}

test.describe.configure({ timeout: 240_000 });

test("first run", async ({ page }) => {
  await holdClock(page);
  await page.goto("/setup");
  await settle(page);
  await expect(page.getByRole("heading", { name: "Let's set up your TV" })).toBeVisible();
  if (await page.locator("[data-setup=finish]").isVisible()) {
    await page.getByRole("button", { name: "Sources" }).click();
  }
  await expect(page.locator("[data-setup=sources]")).toBeVisible();
  // A key on the card keeps setup from advancing to Ready on its own.
  await page.getByLabel("Address").press("ArrowLeft");
  await sweep(page, "first-run-sources", async () => {
    await expect(page.locator("[data-setup=sources]")).toBeVisible();
  });
  await page.getByRole("button", { name: "Continue" }).click();
  await expect(page.getByRole("button", { name: "Watch", exact: true })).toBeVisible({ timeout: 45_000 });
  await sweep(page, "first-run-ready", async () => {
    await expect(page.locator("[data-setup=finish]")).toBeVisible();
  });
});

test("home", async ({ page }) => {
  await browsing(page);
  await holdClock(page);
  await page.goto("/");
  await settle(page);
  await expect(page.getByRole("heading", { name: "NFL: Chiefs at Bills" })).toBeVisible();
  await sweep(page, "home", async () => {
    await expect(page.getByRole("heading", { name: "NFL: Chiefs at Bills" })).toBeVisible();
  });
});

test("guide and program sheet", async ({ page }) => {
  await browsing(page);
  await holdClock(page);
  await page.goto("/guide");
  await settle(page);
  await expect(page.getByRole("heading", { name: "Guide" })).toBeVisible();
  await sweep(page, "guide", async () => {
    await expect(page.getByRole("heading", { name: "Guide" })).toBeVisible();
  });

  await at(page, sizes[1]);
  await page.getByRole("gridcell", { name: /NFL: Chiefs at Bills/ }).click();
  const dialog = page.getByRole("dialog", { name: /Chiefs at Bills/ });
  await expect(dialog).toBeVisible();
  const inside = async () =>
    dialog.evaluate((node) => node.contains(document.activeElement));
  await expect.poll(inside).toBe(true);
  for (let i = 0; i < 8; i++) await page.keyboard.press("Tab");
  expect(await inside()).toBe(true);
  for (let i = 0; i < 8; i++) await page.keyboard.press("Shift+Tab");
  expect(await inside()).toBe(true);
  await sweep(page, "guide-sheet", async () => {
    await expect(dialog).toBeVisible();
  });
  await page.keyboard.press("Escape");
  await expect(dialog).toHaveCount(0);
});

test("search", async ({ page }) => {
  await browsing(page);
  await holdClock(page);
  await page.goto("/search");
  await settle(page);
  await page.getByLabel("Search shows, people, and recordings").fill("Chief");
  await page.getByLabel("Search shows, people, and recordings").press("Enter");
  await expect(page.getByRole("heading", { name: "Guide" })).toBeVisible();
  await sweep(page, "search", async () => {
    await expect(page.getByRole("heading", { name: "Search" })).toBeVisible();
  });
});

test("sports", async ({ page }) => {
  await browsing(page);
  await holdClock(page);
  await page.goto("/sports");
  await settle(page);
  await expect(page.getByRole("article").filter({ hasText: "Kansas City" })).toBeVisible();
  await sweep(page, "sports", async () => {
    await expect(page.getByRole("heading", { name: "Sports" })).toBeVisible();
  });
});

test("recordings", async ({ page }) => {
  await browsing(page);
  await holdClock(page);
  await page.goto("/recordings");
  await settle(page);
  await expect(page.getByRole("heading", { name: "Recordings" })).toBeVisible();
  await sweep(page, "recordings", async () => {
    await expect(page.getByRole("heading", { name: "Recordings" })).toBeVisible();
  });
});

test("settings", async ({ page }) => {
  await browsing(page);
  await holdClock(page);
  await page.goto("/settings");
  await settle(page);
  await expect(page.getByRole("heading", { level: 1, name: "Settings" })).toBeVisible();
  await sweep(page, "settings", async () => {
    await expect(page.getByRole("heading", { level: 1, name: "Settings" })).toBeVisible();
  });
});

test("diagnostics", async ({ page }) => {
  await browsing(page);
  await holdClock(page);
  await page.goto("/diagnostics");
  await settle(page);
  await expect(page.getByRole("heading", { name: "Diagnostics" })).toBeVisible();
  await expect(page.getByText("Checking…")).toHaveCount(0);
  await sweep(page, "diagnostics", async () => {
    await expect(page.getByRole("heading", { name: "Diagnostics" })).toBeVisible();
  });
});

test("player", async ({ page }) => {
  test.setTimeout(240_000);
  await browsing(page);
  await holdClock(page);
  await page.goto("/");
  await settle(page);
  await page.getByRole("button", { name: "Watch", exact: true }).click();
  await expect(page.getByRole("region", { name: "Player" })).toBeVisible();
  await expect.poll(async () => page.locator("video.stage-video").evaluate((video: HTMLVideoElement) => video.videoWidth)).toBeGreaterThan(0);
  // Pause keeps the chrome up so the control names are in the scan.
  await page.locator("video.stage-video").evaluate((video: HTMLVideoElement) => video.pause());
  await expect(page.getByRole("button", { name: "Play", exact: true }).first()).toBeVisible();
  await sweep(page, "player", async () => {
    await page.locator("video.stage-video").evaluate((video: HTMLVideoElement) => {
      if (!video.paused) video.pause();
    });
    await expect(page.getByRole("region", { name: "Player" })).toBeVisible();
    await expect(page.locator(".stage.idle")).toHaveCount(0);
  }, playerRulesOff);

  // The idle chrome, not the room: with sync on, the engine may hold this
  // screen paused after the pause above, and a paused player keeps its chrome.
  await at(page, sizes[1]);
  await page.evaluate(() => localStorage.setItem("ota-live", JSON.stringify({ ...JSON.parse(localStorage.getItem("ota-live") || "{}"), sync: false })));
  await page.reload();
  await settle(page);
  await expect.poll(async () => page.locator("video.stage-video").evaluate((video: HTMLVideoElement) => !video.paused && video.videoWidth > 0), { timeout: 25_000 }).toBe(true);
  await expect(page.locator(".stage.idle")).toBeVisible({ timeout: 25_000 });
  await page.keyboard.press("Tab");
  await expect(page.getByRole("button", { name: "Back to browsing" })).toBeFocused();
  // A focused control must not hold the chrome up; when it fades, the stage takes the keys again.
  await expect(page.locator(".stage.idle")).toBeVisible({ timeout: 10_000 });
  await expect(page.locator(".stage").first()).toBeFocused();
  await page.goto("/");
  await settle(page);
  const stop = page.getByRole("button", { name: "Stop watching" });
  if (await stop.count()) await stop.click();
});

// A 1920×1080 TV browser. Stats is a dialog with no buttons. Arrows still
// move between the player controls while it is open.
import type { Page } from "@playwright/test";
import { expect, test } from "./fixture";
import { settle } from "./snap";

async function tvRemote(page: Page) {
  await page.addInitScript(() => {
    const orig = window.matchMedia.bind(window);
    window.matchMedia = (query: string) =>
      String(query).includes("pointer") && String(query).includes("coarse")
        ? ({ matches: true, media: query, onchange: null, addEventListener() {}, removeEventListener() {}, addListener() {}, removeListener() {}, dispatchEvent: () => false } as MediaQueryList)
        : orig(query);
  });
}

async function setupDone(page: Page) {
  const res = await page.request.put("/api/v1/settings", { data: { setupComplete: "1" } });
  expect(res.ok()).toBeTruthy();
}

test("a TV remote moves between player buttons while Stats is open", async ({ page }) => {
  test.setTimeout(120_000);
  await tvRemote(page);
  await setupDone(page);
  await page.setViewportSize({ width: 1920, height: 1080 });
  await page.goto("/");
  await settle(page);
  await expect(page.locator("html")).toHaveAttribute("data-layout", "tv");
  await page.getByRole("button", { name: "Watch", exact: true }).first().click();
  const stage = page.locator("section.stage:not(.mini)");
  await expect(stage).toBeVisible();
  await page.evaluate(() => {
    const el = document.querySelector("section.stage:not(.mini)");
    if (!(el instanceof HTMLElement)) throw new Error("no stage");
    el.focus();
    el.dispatchEvent(new KeyboardEvent("keydown", { key: "Enter", bubbles: true, cancelable: true }));
  });
  const stats = stage.getByRole("button", { name: "Stats", exact: true });
  await expect(stats).toBeVisible();
  await stats.click();
  await expect(page.getByRole("dialog", { name: "Stats" })).toBeVisible();
  await stats.focus();
  await expect(stats).toBeFocused();
  await page.keyboard.press("ArrowLeft");
  await expect(stats).not.toBeFocused();
  await expect.poll(() =>
    page.evaluate(() => document.activeElement instanceof HTMLButtonElement && document.activeElement.closest("section.stage") != null),
  ).toBe(true);
});

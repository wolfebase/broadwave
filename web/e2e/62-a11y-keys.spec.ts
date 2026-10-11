// TV layout: Tab, Shift-Tab, arrows, and Escape operate the main pages, focus stays
// visible, and a dialog gives the keys back to whatever opened it.
import type { Page } from "@playwright/test";
import { expect, holdClock, test } from "./fixture";
import { settle } from "./snap";

async function tv(page: Page) {
  await page.addInitScript(() => {
    const orig = window.matchMedia.bind(window);
    window.matchMedia = (query: string) =>
      String(query).includes("pointer") && String(query).includes("coarse")
        ? ({ matches: true, media: query, onchange: null, addEventListener() {}, removeEventListener() {}, addListener() {}, removeListener() {}, dispatchEvent: () => false } as MediaQueryList)
        : orig(query);
  });
}

function focused(page: Page) {
  return page.evaluate(() => {
    const el = document.activeElement as HTMLElement | null;
    if (!el || el === document.body || el === document.documentElement) return { where: "body", ring: false, label: "" };
    const grid = el.getAttribute("role") === "grid";
    return {
      where: grid ? "grid" : el.closest("[role='dialog']") ? "dialog" : el.tagName.toLowerCase(),
      ring: el.matches(":focus-visible") && getComputedStyle(el).boxShadow !== "none",
      label: (el.getAttribute("aria-label") || el.textContent || "").replace(/\s+/g, " ").trim().slice(0, 80),
    };
  });
}

test.describe.configure({ timeout: 180_000 });

test("a TV remote reaches home, guide, search, recordings, and settings", async ({ page }) => {
  await tv(page);
  expect((await page.request.put("/api/v1/settings", { data: { setupComplete: "1" } })).ok()).toBeTruthy();
  await holdClock(page);
  await page.setViewportSize({ width: 1920, height: 1080 });

  await page.goto("/");
  await settle(page);
  await expect(page.locator("html")).toHaveAttribute("data-layout", "tv");
  await expect.poll(() => page.locator(".hero-glow").evaluate((el) => getComputedStyle(el).animationName)).toBe("none");
  await expect.poll(() => page.locator(".shelf-row").first().evaluate((el) => getComputedStyle(el).scrollSnapType)).toBe("none");
  await page.emulateMedia({ reducedMotion: "no-preference" });
  await expect.poll(() => page.locator(".hero-glow").evaluate((el) => getComputedStyle(el).animationName)).toBe("breathe");
  // Chromium omits the initial strictness, so "x proximity" is reported as "x".
  await expect.poll(() => page.locator(".shelf-row").first().evaluate((el) => getComputedStyle(el).scrollSnapType)).toBe("x");
  await page.emulateMedia({ reducedMotion: "reduce" });

  for (const path of ["/", "/guide", "/search", "/recordings", "/settings"]) {
    await page.goto(path);
    await settle(page);
    if (path === "/search") {
      const box = page.getByRole("searchbox", { name: "Search shows, people, and recordings" });
      await expect.poll(() => box.evaluate((el) => el === document.activeElement && el.matches(":focus-visible") && getComputedStyle(el).boxShadow !== "none")).toBe(true);
    }
    // Start on a known control. A bare Tab from the last field leaves the page.
    const start = path === "/settings" ? page.getByRole("button", { name: "Settings", exact: true }) : page.getByRole("tab", { selected: true });
    await start.evaluate((el) => (el as HTMLElement).focus({ focusVisible: true }));
    await expect.poll(() => focused(page)).toMatchObject({ ring: true });
    await page.keyboard.press("Tab");
    await expect.poll(() => focused(page)).toMatchObject({ ring: true });
    const landed = await focused(page);
    await page.keyboard.press("Shift+Tab");
    await expect.poll(() => focused(page)).toMatchObject({ ring: true });
    await page.keyboard.press("Tab");
    await expect.poll(() => focused(page)).toMatchObject({ label: landed.label, ring: true });
    await page.keyboard.press("ArrowDown");
    await expect.poll(() => focused(page)).toMatchObject({ ring: true });
    expect((await focused(page)).where).not.toBe("body");
  }

  await page.goto("/settings");
  await settle(page);
  const motion = page.getByRole("group", { name: "Picture motion" });
  await expect(motion.getByRole("button", { name: "Broadcast" })).toHaveAttribute("aria-pressed", "true");
  await expect(motion.getByRole("button", { name: "Smooth" })).toHaveAttribute("aria-pressed", "false");
});

test("the guide sheet keeps Tab inside and returns focus", async ({ page }) => {
  await tv(page);
  expect((await page.request.put("/api/v1/settings", { data: { setupComplete: "1" } })).ok()).toBeTruthy();
  await holdClock(page);
  await page.setViewportSize({ width: 1920, height: 1080 });
  await page.goto("/guide");
  await settle(page);
  const grid = page.getByRole("grid", { name: "TV guide" });
  await grid.focus();
  await page.keyboard.press("Enter");
  const dialog = page.getByRole("dialog");
  await expect(dialog).toBeVisible();
  const inside = () => dialog.evaluate((node) => node.contains(document.activeElement));
  await expect.poll(inside).toBe(true);
  for (let i = 0; i < 6; i++) await page.keyboard.press("Tab");
  expect(await inside()).toBe(true);
  for (let i = 0; i < 6; i++) await page.keyboard.press("Shift+Tab");
  expect(await inside()).toBe(true);
  await page.keyboard.press("Escape");
  await expect(dialog).toHaveCount(0);
  await expect.poll(() => focused(page)).toMatchObject({ where: "grid", ring: true });
});

test("reduced motion jumps the guide instead of gliding", async ({ page }) => {
  await tv(page);
  expect((await page.request.put("/api/v1/settings", { data: { setupComplete: "1" } })).ok()).toBeTruthy();
  await holdClock(page);
  await page.setViewportSize({ width: 1920, height: 1080 });
  await page.emulateMedia({ reducedMotion: "reduce" });
  await page.goto("/guide");
  await settle(page);
  await page.locator(".guide-scroll").evaluate((el) => {
    const node = el as HTMLElement;
    const orig = node.scrollTo.bind(node);
    (window as unknown as { __scrolls?: string[] }).__scrolls = [];
    node.scrollTo = (options?: ScrollToOptions) => {
      (window as unknown as { __scrolls?: string[] }).__scrolls?.push(String(options?.behavior ?? ""));
      orig(options);
    };
  });
  await page.getByRole("button", { name: "Tonight", exact: true }).click();
  await expect.poll(() => page.evaluate(() => (window as unknown as { __scrolls?: string[] }).__scrolls ?? [])).toContain("auto");
});

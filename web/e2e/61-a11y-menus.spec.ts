// Captions and audio sit in the player options. On a TV they are named, show which
// one is on, and a remote can reach them.
import type { Page } from "@playwright/test";
import { expect, test } from "./fixture";
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

test("captions and audio are labelled and a remote can change them", async ({ page }) => {
  test.setTimeout(150_000);
  await tv(page);
  expect((await page.request.put("/api/v1/settings", { data: { setupComplete: "1" } })).ok()).toBeTruthy();
  await page.setViewportSize({ width: 1920, height: 1080 });
  await page.goto("/");
  await settle(page);
  await expect(page.locator("html")).toHaveAttribute("data-layout", "tv");
  await page.getByRole("button", { name: "Watch", exact: true }).click();
  const stage = page.getByRole("region", { name: "Player" });
  await expect(stage).toBeVisible();
  await stage.evaluate((el: HTMLElement) => el.focus({ focusVisible: true } as FocusOptions));
  await expect.poll(() =>
    stage.evaluate((el) => {
      const style = getComputedStyle(el);
      return el.matches(":focus-visible") && style.outlineStyle === "solid" && Number.parseFloat(style.outlineWidth) >= 4 && Number.parseFloat(style.outlineOffset) < 0;
    }),
  ).toBe(true);
  const video = page.locator("video.stage-video");
  await expect.poll(() => video.evaluate((el: HTMLVideoElement) => el.videoWidth > 0), { timeout: 45_000 }).toBe(true);
  await video.evaluate((el: HTMLVideoElement) => el.pause());
  await expect(page.getByRole("button", { name: "Play", exact: true }).first()).toBeVisible();

  const sync = page.locator("[data-sync-live]");
  await expect(sync).toHaveAttribute("role", "status", { timeout: 20_000 });
  await expect(sync).toHaveAttribute("aria-live", "polite");
  await expect(sync).not.toBeEmpty();

  const options = page.getByRole("button", { name: "Options" });
  await options.focus();
  await expect(options).toHaveAttribute("aria-expanded", "false");
  await page.keyboard.press("Enter");
  await expect(options).toHaveAttribute("aria-expanded", "true");
  await expect(page.getByRole("region", { name: "Options" })).toBeVisible();
  const audio = page.getByRole("group", { name: "Audio" });
  const captions = page.getByRole("group", { name: "Captions" });
  const volume = page.getByRole("slider", { name: "Volume" });
  await expect(volume).toBeVisible();
  await expect(volume).toHaveAttribute("aria-valuetext", /%$/);
  await expect(audio).toBeVisible();
  await expect(captions).toBeVisible();
  await expect(audio.getByRole("button", { pressed: true })).toHaveCount(1);
  await expect(captions.getByRole("button", { name: "Off" })).toHaveAttribute("aria-pressed", "true");

  const audioButtons = audio.getByRole("button");
  if ((await audioButtons.count()) > 1) {
    await audioButtons.first().focus();
    await page.keyboard.press("Tab");
    const next = ((await page.evaluate(() => document.activeElement?.textContent)) || "").replace(/\s+/g, " ").trim();
    expect(next.length).toBeGreaterThan(0);
    await page.keyboard.press("Enter");
    await expect(audio.getByRole("button", { name: next, exact: true })).toHaveAttribute("aria-pressed", "true");
  }

  await captions.getByRole("button", { name: "On" }).focus();
  await page.keyboard.press("Enter");
  await expect(captions.getByRole("button", { name: "On" })).toHaveAttribute("aria-pressed", "true");
  await expect(captions.getByRole("button", { name: "Off" })).toHaveAttribute("aria-pressed", "false");

  await page.keyboard.press("Escape");
  await expect(options).toBeFocused();
  await expect(options).toHaveAttribute("aria-expanded", "false");
  await expect(page.getByRole("region", { name: "Options" })).toHaveCount(0);
  await expect(page).toHaveURL(/\/watch/);
  await page.keyboard.press("Escape");
  await expect(page).not.toHaveURL(/\/watch/);
});

// Tab and Shift+Tab stay on the full player's own controls. Past Options they
// used to leave for the navigation hidden under the picture, and a Tab while
// the bar was hidden skipped the controls entirely.
import { mkdirSync, writeFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import type { Page } from "@playwright/test";
import { expect, test } from "./fixture";
import { settle } from "./snap";

const here = path.dirname(fileURLToPath(import.meta.url));
const evidence = path.resolve(here, "../../.evidence/lane/l93");

type Channel = { id: number };

async function quietSync(page: Page) {
  await page.addInitScript(() => {
    localStorage.setItem("ota-live", JSON.stringify({ sync: false }));
  });
}

function focusNow(page: Page) {
  return page.evaluate(() => {
    const el = document.activeElement as HTMLElement | null;
    const stage = document.querySelector(".stage:not(.mini)");
    const inside = Boolean(el && stage?.contains(el) && el !== stage);
    let visible = false;
    if (el && el !== document.body) {
      const style = getComputedStyle(el);
      const box = el.getBoundingClientRect();
      visible =
        style.visibility !== "hidden" &&
        style.display !== "none" &&
        Number(style.opacity) > 0.5 &&
        box.width > 1 &&
        box.height > 1 &&
        (typeof el.checkVisibility !== "function" || el.checkVisibility({ checkOpacity: true, checkVisibilityCSS: true }));
    }
    const label = (el?.getAttribute("aria-label") || el?.textContent || "").replace(/\s+/g, " ").trim().slice(0, 80);
    return {
      inside,
      visible,
      label,
      idle: Boolean(stage?.classList.contains("idle")),
      nav: Boolean(el?.closest("nav, main")),
      ring: Boolean(el?.matches(":focus-visible")),
    };
  });
}

async function playing(page: Page) {
  await expect
    .poll(
      () => page.locator("video.stage-video").evaluate((video: HTMLVideoElement) => video.videoWidth > 0 && !video.paused && video.currentTime > 0.3),
      { timeout: 45_000 },
    )
    .toBe(true);
  await expect(page.locator(".tuning")).toHaveCount(0);
}

/** One Tab. Focus has to be a visible control inside the player, never the page behind it. */
async function tabOnce(page: Page, shift: boolean) {
  await page.keyboard.press(shift ? "Shift+Tab" : "Tab");
  let state = await focusNow(page);
  await expect
    .poll(async () => {
      state = await focusNow(page);
      return state.inside && state.visible && !state.nav && state.ring;
    }, { timeout: 2_000, intervals: [50] })
    .toBe(true);
  return state;
}

test("Tab cycles the player bar and the first Tab from a hidden bar shows it", async ({ page }) => {
  test.setTimeout(120_000);
  await quietSync(page);
  expect((await page.request.put("/api/v1/settings", { data: { setupComplete: "1" } })).ok()).toBeTruthy();
  const { channels } = (await (await page.request.get("/api/v1/channels?guide=1")).json()) as { channels: Channel[] };
  expect(channels.length).toBeGreaterThan(1);
  mkdirSync(evidence, { recursive: true });

  await page.setViewportSize({ width: 1440, height: 900 });
  await page.goto(`/watch?channel=${channels[0].id}`);
  await settle(page);
  await playing(page);

  const labels = await page.locator(".stage:not(.mini)").evaluate((stage) =>
    [...stage.querySelectorAll<HTMLElement>("button, input, a[href]")].map((el) => el.getAttribute("aria-label") || el.textContent || "").map((text) => text.replace(/\s+/g, " ").trim()),
  );
  for (const name of ["Back to browsing", "Pause", "Back 15 seconds", "Forward 30 seconds", "Channels", "Mute", "Options", "Playback position"]) {
    expect(labels, name).toContain(name);
  }

  const steps: { size: string; dir: string; label: string }[] = [];
  for (const size of [
    { width: 1440, height: 900, name: "1440" },
    { width: 1920, height: 1080, name: "1920" },
  ]) {
    await page.setViewportSize({ width: size.width, height: size.height });
    await page.locator(".stage:not(.mini)").focus();
    await page.mouse.move(2, 2);
    await expect(page.locator(".stage.idle")).toHaveCount(1, { timeout: 6_000 });

    await page.keyboard.press("Tab");
    await expect.poll(async () => focusNow(page), { timeout: 2_000 }).toMatchObject({
      inside: true,
      visible: true,
      nav: false,
      idle: false,
      ring: true,
      label: "Back to browsing",
    });
    await page.screenshot({ path: path.join(evidence, `tab-${size.name}.jpg`), type: "jpeg", quality: 60, animations: "disabled" });

    for (let i = 0; i < 15; i++) {
      const state = await tabOnce(page, false);
      steps.push({ size: size.name, dir: "forward", label: state.label });
    }
    for (let i = 0; i < 15; i++) {
      const state = await tabOnce(page, true);
      steps.push({ size: size.name, dir: "back", label: state.label });
    }
  }

  // Options is part of the same cycle, not a way out.
  const options = page.getByRole("button", { name: "Options", exact: true });
  await options.focus();
  await page.keyboard.press("Enter");
  await expect(options).toHaveAttribute("aria-expanded", "true");
  const inOptions = await tabOnce(page, false);
  expect(inOptions.label.length).toBeGreaterThan(0);
  await expect.poll(() => page.evaluate(() => Boolean(document.activeElement?.closest(".options-grid")))).toBe(true);
  await page.screenshot({ path: path.join(evidence, "options.jpg"), type: "jpeg", quality: 60, animations: "disabled" });
  await page.keyboard.press("Shift+Tab");
  await expect(options).toBeFocused();
  await page.keyboard.press("Enter");
  await expect(options).toHaveAttribute("aria-expanded", "false");

  const before = new URL(page.url()).searchParams.get("channel");
  await page.locator(".stage:not(.mini)").focus();
  await page.keyboard.press("ArrowDown");
  await expect.poll(() => new URL(page.url()).searchParams.get("channel")).not.toBe(before);
  const next = new URL(page.url()).searchParams.get("channel");
  await playing(page);
  await page.locator(".stage:not(.mini)").focus();
  await page.keyboard.press("ArrowLeft");
  await page.waitForTimeout(400);
  expect(new URL(page.url()).searchParams.get("channel")).toBe(next);
  await page.keyboard.press("Escape");
  await expect(page).not.toHaveURL(/\/watch/);
  await expect(page.getByRole("button", { name: "Open player" })).toBeVisible();

  writeFileSync(path.join(evidence, "tabs.json"), JSON.stringify({ steps }, null, 2));
});

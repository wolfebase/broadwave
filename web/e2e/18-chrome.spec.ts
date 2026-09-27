import { mkdirSync, writeFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import type { Page } from "@playwright/test";
import { expect, test } from "./fixture";
import { settle } from "./snap";

const here = path.dirname(fileURLToPath(import.meta.url));
const evidence = path.resolve(here, "../../.evidence/lane/l30");

async function setupDone(page: Page) {
  const res = await page.request.put("/api/v1/settings", { data: { setupComplete: "1" } });
  expect(res.ok(), "setup").toBeTruthy();
}

/** Sync can hold the picture paused, and a paused picture keeps the chrome up. */
async function quietSync(page: Page) {
  await page.addInitScript(() => {
    localStorage.setItem("ota-live", JSON.stringify({ sync: false }));
  });
}

async function playing(page: Page) {
  await expect
    .poll(
      () =>
        page.locator("video.stage-video").evaluate((video: HTMLVideoElement) => video.videoWidth > 0 && !video.paused && video.currentTime > 0.3),
      { timeout: 45_000 },
    )
    .toBe(true);
  await expect(page.locator(".tuning")).toHaveCount(0);
}

async function chromeState(page: Page) {
  return page.evaluate(() => {
    const stage = document.querySelector(".stage");
    const hud = document.querySelector(".stage-hud");
    const opacity = hud ? getComputedStyle(hud).opacity : "";
    return {
      idle: Boolean(stage?.classList.contains("idle")),
      opacity,
      stageFocused: document.activeElement === stage,
    };
  });
}

test("a mute click fades the chrome, the stage takes Space, and Tab opens Back", async ({ page }) => {
  await quietSync(page);
  await setupDone(page);
  await page.setViewportSize({ width: 1440, height: 900 });
  await page.goto("/");
  await settle(page);
  await page.getByRole("button", { name: "Watch", exact: true }).click();
  await expect(page.getByRole("region", { name: "Player" })).toBeVisible();
  await playing(page);

  const mute = page.getByRole("button", { name: "Mute" });
  await mute.click();
  const clickedAt = Date.now();
  await expect.poll(async () => chromeState(page), { timeout: 4_000, intervals: [100] }).toEqual({ idle: true, opacity: "0", stageFocused: true });
  const fadedMs = Date.now() - clickedAt;
  await expect.poll(() => page.locator("video.stage-video").evaluate((video: HTMLVideoElement) => video.muted)).toBe(true);

  mkdirSync(evidence, { recursive: true });
  await page.screenshot({ path: path.join(evidence, "idle.jpg"), animations: "disabled" });

  await page.keyboard.press("Space");
  await expect.poll(() => page.locator("video.stage-video").evaluate((video: HTMLVideoElement) => video.paused)).toBe(true);
  await page.keyboard.press("Space");
  await expect
    .poll(() => page.locator("video.stage-video").evaluate((video: HTMLVideoElement) => !video.paused && video.videoWidth > 0))
    .toBe(true);
  await expect.poll(async () => chromeState(page), { timeout: 5_000, intervals: [100] }).toEqual({ idle: true, opacity: "0", stageFocused: true });

  await page.keyboard.press("Tab");
  await expect(page.getByRole("button", { name: "Back to browsing" })).toBeFocused();
  await expect(page.locator(".stage.idle")).toHaveCount(0);
  const hudOpacity = await page.locator(".stage-hud").evaluate((node) => getComputedStyle(node).opacity);
  expect(Number(hudOpacity)).toBeGreaterThan(0);

  writeFileSync(path.join(evidence, "desktop.json"), JSON.stringify({ fadedMs, hudOpacity }, null, 2));
  await page.screenshot({ path: path.join(evidence, "tab.jpg"), animations: "disabled" });

  await page.keyboard.press("Escape");
  await page.getByRole("button", { name: "Stop watching" }).click();
});

test("on a TV, arrows and Enter drive the chrome and Escape leaves", async ({ page }) => {
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
  await quietSync(page);
  await setupDone(page);
  await page.setViewportSize({ width: 1920, height: 1080 });
  await page.goto("/");
  await settle(page);
  await expect(page.locator("html")).toHaveAttribute("data-layout", "tv");
  await page.getByRole("button", { name: "Watch", exact: true }).click();
  const player = page.getByRole("region", { name: "Player" });
  await expect(player).toBeVisible();
  await playing(page);
  const channel = new URL(page.url()).searchParams.get("channel");
  expect(channel).toBeTruthy();

  // A still mouse would fade the chrome and make it inert. A hover keeps the controls up for the remote.
  await page.locator(".stage").hover();
  await expect(page.locator(".stage.idle")).toHaveCount(0);
  const mute = page.getByRole("button", { name: "Mute" });
  await mute.focus();
  await expect(mute).toBeFocused();

  await page.keyboard.press("ArrowLeft");
  const moved = await page.evaluate(() => {
    const el = document.activeElement;
    const label = (el?.getAttribute("aria-label") || el?.textContent || "").replace(/\s+/g, " ").trim();
    return {
      label,
      hud: Boolean(el?.closest(".stage-hud")),
      mute: label === "Mute",
    };
  });
  expect(moved.hud, JSON.stringify(moved)).toBe(true);
  expect(moved.mute, JSON.stringify(moved)).toBe(false);
  expect(new URL(page.url()).searchParams.get("channel")).toBe(channel);

  const backToMute = async () => {
    for (let i = 0; i < 12; i++) {
      const label = await page.evaluate(() => (document.activeElement?.getAttribute("aria-label") || "").trim());
      if (label === "Mute") return true;
      await page.keyboard.press("ArrowRight");
    }
    return false;
  };
  expect(await backToMute(), "Mute was not to the right of the control ArrowLeft landed on").toBe(true);
  await expect(mute).toBeFocused();
  expect(new URL(page.url()).searchParams.get("channel")).toBe(channel);

  await page.keyboard.press("Enter");
  await expect.poll(() => page.locator("video.stage-video").evaluate((video: HTMLVideoElement) => video.muted)).toBe(true);
  expect(new URL(page.url()).searchParams.get("channel")).toBe(channel);

  mkdirSync(evidence, { recursive: true });
  await page.screenshot({ path: path.join(evidence, "tv-hud.jpg"), animations: "disabled" });
  writeFileSync(path.join(evidence, "tv.json"), JSON.stringify({ channel, moved }, null, 2));

  await page.keyboard.press("Escape");
  await expect(page).not.toHaveURL(/\/watch/);
  await expect(player).toHaveCount(0);
  await expect(page.getByRole("button", { name: "Open player" })).toBeVisible();
  await page.screenshot({ path: path.join(evidence, "tv-left.jpg"), animations: "disabled" });
  await page.getByRole("button", { name: "Stop watching" }).click();
});

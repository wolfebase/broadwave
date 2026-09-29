import { mkdirSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import type { Page } from "@playwright/test";
import { expect, test } from "./fixture";
import { settle } from "./snap";

const here = path.dirname(fileURLToPath(import.meta.url));
const evidence = path.resolve(here, "../../.evidence/lane/l63");

async function captionMode(page: Page) {
  return page.locator("video.stage-video").evaluate((el: HTMLVideoElement) => {
    const track = Array.from(el.textTracks).find((item) => item.label === "English CC");
    return track?.mode ?? "none";
  });
}

async function activeCue(page: Page) {
  return page.locator("video.stage-video").evaluate((el: HTMLVideoElement) => {
    const track = Array.from(el.textTracks).find((item) => item.label === "English CC");
    return Array.from(track?.activeCues ?? [])
      .map((cue) => (cue as VTTCue).text)
      .join(" ")
      .trim();
  });
}

test("c toggles captions and the mini player shows a cue", async ({ page }) => {
  test.setTimeout(process.env.E2E_SOURCE ? 150_000 : 90_000);
  const errors: string[] = [];
  const captionReqs: number[] = [];
  page.on("pageerror", (err) => errors.push(err.message));
  page.on("request", (req) => {
    if (req.url().includes("captions.m3u8")) captionReqs.push(Date.now());
  });

  expect((await page.request.put("/api/v1/settings", { data: { setupComplete: "1" } })).ok()).toBe(true);
  await page.setViewportSize({ width: 1440, height: 900 });
  // Open from the page, so Back keeps the channel docked. A direct /watch load has no channel to dock.
  await page.goto("/");
  await settle(page);
  await page.getByRole("button", { name: "Watch", exact: true }).click();
  await expect(page).toHaveURL(/\/watch\?channel=\d+/);
  const id = new URL(page.url()).searchParams.get("channel");
  const video = page.locator("video.stage-video");
  await expect.poll(() => video.evaluate((el: HTMLVideoElement) => el.currentTime > 0.4 && el.videoWidth > 0), { timeout: 45_000 }).toBe(true);

  const player = page.getByRole("region", { name: "Player" });
  await player.focus();
  await page.keyboard.press("?");
  const help = page.getByRole("dialog", { name: "Keyboard" });
  await expect(help.getByText("Captions", { exact: true })).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(help).toHaveCount(0);

  // A text field keeps C. The player has none of its own, so the check uses one placed on the stage.
  await page.locator(".stage").evaluate((stage: HTMLElement) => {
    const input = document.createElement("input");
    input.type = "search";
    input.setAttribute("aria-label", "Find");
    stage.appendChild(input);
    input.focus();
  });
  await page.keyboard.press("c");
  expect(await captionMode(page)).toBe("none");
  await page.locator(".stage input[aria-label='Find']").evaluate((input: HTMLInputElement) => input.remove());

  await player.hover();
  await page.getByRole("button", { name: "Options" }).click();
  const row = page.getByRole("group", { name: "Captions" });
  await expect(row.getByRole("button", { name: "Off" })).toHaveAttribute("aria-pressed", "true");
  await expect(row.getByRole("button", { name: "On" })).toHaveAttribute("aria-pressed", "false");

  await player.focus();
  await page.keyboard.press("c");
  await expect.poll(() => captionMode(page)).toBe("showing");
  await expect(row.getByRole("button", { name: "On" })).toHaveAttribute("aria-pressed", "true");
  await expect(row.getByRole("button", { name: "Off" })).toHaveAttribute("aria-pressed", "false");
  await expect.poll(() => page.evaluate(() => JSON.parse(localStorage.getItem("ota-live") || "{}").captions)).toBe(true);
  await expect.poll(() => captionReqs.length, { timeout: 20_000 }).toBeGreaterThan(0);

  await page.keyboard.press("Escape");
  const mini = page.getByRole("region", { name: /Now playing/ });
  await expect(mini).toBeVisible();
  await expect.poll(() => captionMode(page)).toBe("showing");
  if (process.env.E2E_SOURCE) {
    await expect.poll(() => activeCue(page), { timeout: 45_000 }).not.toBe("");
  }
  mkdirSync(evidence, { recursive: true });
  await page.screenshot({ path: path.join(evidence, "mini.jpg"), type: "jpeg", quality: 60, animations: "disabled" });

  await mini.getByRole("button", { name: "Stop watching" }).click();
  await expect(video).toHaveCount(0);
  const closedAt = Date.now();
  await page.waitForTimeout(3500);
  expect(captionReqs.filter((at) => at > closedAt)).toEqual([]);

  await page.goto(`/watch?channel=${id}`);
  await settle(page);
  await expect.poll(() => video.evaluate((el: HTMLVideoElement) => el.currentTime > 0.4 && el.videoWidth > 0), { timeout: 45_000 }).toBe(true);
  await expect.poll(() => captionMode(page), { timeout: 20_000 }).toBe("showing");
  await page.getByRole("region", { name: "Player" }).focus();
  await page.keyboard.press("c");
  await expect.poll(() => captionMode(page)).toBe("disabled");
  await expect.poll(() => page.evaluate(() => JSON.parse(localStorage.getItem("ota-live") || "{}").captions)).toBe(false);
  expect(errors).toEqual([]);
});

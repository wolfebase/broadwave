import { mkdirSync, readFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import type { Page } from "@playwright/test";
import { expect, test } from "./fixture";
import { settle } from "./snap";

const here = path.dirname(fileURLToPath(import.meta.url));
const evidence = path.resolve(here, "../../.evidence/lane/l51");

type Channel = { id: number; number: string; name: string };

function lineup(): Channel[] {
  const runtime = JSON.parse(readFileSync(path.join(here, ".run/runtime.json"), "utf8")) as { channels: Channel[] };
  return runtime.channels;
}

async function playing(page: Page, id: number) {
  await expect
    .poll(
      () =>
        page.locator("video.stage-video").evaluate((video: HTMLVideoElement, channelId: number) => {
          const on = video.videoWidth > 0 && !video.paused && video.currentTime > 0.2;
          const current = new URLSearchParams(location.search).get("channel");
          return on && current === String(channelId) ? "ok" : `${current} ${video.videoWidth} ${video.paused}`;
        }, id),
      { timeout: 30_000 },
    )
    .toBe("ok");
}

async function tabUntil(page: Page, ready: () => Promise<boolean>) {
  for (let i = 0; i < 40; i++) {
    if (await ready()) return;
    await page.keyboard.press("Tab");
  }
  throw new Error("the key never landed");
}

test.afterEach(async ({ page }) => {
  await page.goto("about:blank");
});

test("stats, help, last channel, a typed number, sleep, volume, and theater", async ({ page }) => {
  test.setTimeout(90_000);
  const errors: string[] = [];
  page.on("pageerror", (err) => errors.push(err.message));
  page.on("console", (msg) => {
    if (msg.type() !== "error") return;
    const text = `${msg.text()} ${msg.location().url}`;
    if (/\/media\/(?:art|poster)\/|\/channels\/\d+\/frame|favicon/.test(text)) return;
    errors.push(text);
  });

  const channels = lineup();
  const first = channels.find((channel) => channel.number === "4.1");
  const fourTwo = channels.find((channel) => channel.number === "4.2");
  const five = channels.find((channel) => channel.number === "5.1");
  const fiveTwo = channels.find((channel) => channel.number === "5.2");
  expect(first && fourTwo && five).toBeTruthy();

  await page.addInitScript(() => {
    localStorage.setItem("ota-live", JSON.stringify({ sync: false }));
  });
  expect((await page.request.put("/api/v1/settings", { data: { setupComplete: "1" } })).ok()).toBe(true);
  await page.setViewportSize({ width: 1440, height: 900 });
  await page.goto(`/watch?channel=${first!.id}`);
  await settle(page);
  await expect(page.locator("html")).toHaveAttribute("data-layout", "desktop");
  await playing(page, first!.id);

  await page.locator("video.stage-video").evaluate((video: HTMLVideoElement) => {
    video.dataset.syncDrift = "12";
  });
  await page.keyboard.press("i");
  const stats = page.getByRole("dialog", { name: "Stats" });
  await expect(stats).toBeVisible();
  await expect(stats.getByText("Bitrate", { exact: true })).toBeVisible();
  await expect(stats.getByText("Dropped frames", { exact: true })).toBeVisible();
  await expect(stats.getByText("Buffer ahead", { exact: true })).toBeVisible();
  await expect(stats.getByText("Behind live", { exact: true })).toBeVisible();
  await expect(stats.getByText("Sync drift", { exact: true })).toBeVisible();
  await expect(stats.getByText("12 ms", { exact: true })).toBeVisible({ timeout: 3_000 });
  await expect(stats.getByText("Rendition", { exact: true })).toBeVisible();
  await expect(stats.getByText("Encoder", { exact: true })).toBeVisible();
  const named = await stats.locator("dd").evaluateAll((nodes) => nodes.map((node) => node.textContent?.trim() ?? ""));
  expect(named.filter((line) => line && line !== "Waiting").length).toBeGreaterThan(1);
  mkdirSync(evidence, { recursive: true });
  await page.screenshot({ path: path.join(evidence, "stats-1440.jpg"), type: "jpeg", quality: 60, animations: "disabled" });
  await page.keyboard.press("i");
  await expect(stats).toHaveCount(0);

  await page.keyboard.press("?");
  const help = page.getByRole("dialog", { name: "Keyboard" });
  await expect(help).toBeVisible();
  await expect(help.getByRole("button", { name: "Close" })).toBeFocused();
  await page.keyboard.press("Tab");
  await expect
    .poll(() => page.evaluate(() => document.querySelector("[aria-label='Keyboard']")?.contains(document.activeElement) ?? false))
    .toBe(true);
  await page.screenshot({ path: path.join(evidence, "help-1440.jpg"), type: "jpeg", quality: 60, animations: "disabled" });

  await page.setViewportSize({ width: 390, height: 844 });
  await expect(page.locator("html")).toHaveAttribute("data-layout", "phone");
  await page.screenshot({ path: path.join(evidence, "help-390.jpg"), type: "jpeg", quality: 60, animations: "disabled" });
  await page.setViewportSize({ width: 1920, height: 1080 });
  await page.screenshot({ path: path.join(evidence, "help-1920.jpg"), type: "jpeg", quality: 60, animations: "disabled" });
  await page.setViewportSize({ width: 1440, height: 900 });
  await expect(page.locator("html")).toHaveAttribute("data-layout", "desktop");
  await page.keyboard.press("Escape");
  await expect(help).toHaveCount(0);

  await page.keyboard.press("ArrowDown");
  await expect.poll(() => new URL(page.url()).searchParams.get("channel")).not.toBe(String(first!.id));
  const nextId = Number(new URL(page.url()).searchParams.get("channel"));
  await playing(page, nextId);
  await page.keyboard.press("Backspace");
  await playing(page, first!.id);
  await page.keyboard.press("l");
  await playing(page, nextId);

  // 4.1 and 4.2 share a first digit, so "42" waits. "51" does the same when 5.2 is in the lineup.
  const typed = fiveTwo ? { digits: ["5", "1"], echo: ["Channel 5", "Channel 51"], id: five!.id } : { digits: ["4", "2"], echo: ["Channel 4", "Channel 42"], id: fourTwo!.id };
  if (new URL(page.url()).searchParams.get("channel") === String(typed.id)) {
    await page.keyboard.press("Backspace");
    await playing(page, first!.id);
  }
  await page.keyboard.press(typed.digits[0]);
  await expect(page.getByRole("status", { name: typed.echo[0] })).toBeVisible();
  await page.keyboard.press(typed.digits[1]);
  await expect(page.getByRole("status", { name: typed.echo[1] })).toBeVisible();
  await playing(page, typed.id);

  await tabUntil(page, () =>
    page.evaluate(() => {
      const label = document.activeElement?.getAttribute("aria-label");
      return label === "Mute" || label === "Unmute";
    }),
  );
  if ((await page.evaluate(() => document.activeElement?.getAttribute("aria-label"))) === "Mute") await page.keyboard.press("Enter");
  await expect.poll(() => page.locator("video.stage-video").evaluate((video: HTMLVideoElement) => video.muted)).toBe(true);
  await tabUntil(page, () => page.evaluate(() => (document.activeElement?.textContent || "").replace(/\s+/g, " ").trim() === "Options"));
  await page.keyboard.press("Enter");
  await tabUntil(page, () => page.evaluate(() => document.activeElement?.getAttribute("aria-label") === "Volume"));
  await page.keyboard.press("ArrowLeft");
  const level = await page.locator("video.stage-video").evaluate((video: HTMLVideoElement) => Math.round(video.volume * 100) / 100);
  expect(level).toBeLessThan(1);

  await page.reload();
  await playing(page, typed.id);
  await expect
    .poll(() =>
      page.locator("video.stage-video").evaluate((video: HTMLVideoElement) => ({
        muted: video.muted,
        volume: Math.round(video.volume * 100) / 100,
      })),
    )
    .toEqual({ muted: true, volume: level });

  await tabUntil(page, () => page.evaluate(() => (document.activeElement?.textContent || "").replace(/\s+/g, " ").trim() === "Options"));
  await page.keyboard.press("Enter");
  await tabUntil(page, () => page.evaluate(() => (document.activeElement?.textContent || "").trim() === "30 min"));
  await page.keyboard.press("Enter");
  await expect(page.getByRole("status").filter({ hasText: "Stops in 30 minutes." })).toBeVisible();
  await tabUntil(page, () => page.evaluate(() => (document.activeElement?.textContent || "").trim() === "Stats"));
  await page.keyboard.press("Enter");
  await expect(page.getByRole("dialog", { name: "Stats" })).toBeVisible();
  await page.keyboard.press("Escape");

  await page.keyboard.press("t");
  await expect(page.locator("html")).toHaveAttribute("data-theater", "1");
  await expect.poll(() => page.locator("nav[aria-label='Primary']").evaluate((nav) => getComputedStyle(nav).display)).toBe("none");
  await page.screenshot({ path: path.join(evidence, "theater-1440.jpg"), type: "jpeg", quality: 60, animations: "disabled" });
  await page.keyboard.press("t");
  await expect(page.locator("html")).not.toHaveAttribute("data-theater", "1");

  await page.setViewportSize({ width: 390, height: 844 });
  await expect(page.locator("html")).toHaveAttribute("data-layout", "phone");
  await page.keyboard.press("t");
  await expect(page.locator("html")).not.toHaveAttribute("data-theater", "1");
  await expect.poll(() => page.locator("nav[aria-label='Primary']").evaluate((nav) => getComputedStyle(nav).display)).not.toBe("none");

  expect(errors).toEqual([]);
});

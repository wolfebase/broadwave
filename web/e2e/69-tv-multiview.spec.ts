// A 1920×1080 TV browser. Arrows move between multiview tiles. At the edge
// of the grid they move to the buttons around it.
import { readFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import type { Page } from "@playwright/test";
import { expect, test } from "./fixture";

const here = path.dirname(fileURLToPath(import.meta.url));

type Channel = { id: number; number: string; name: string };

function channel(name: string): Channel {
  const runtime = JSON.parse(readFileSync(path.join(here, ".run/runtime.json"), "utf8")) as { channels: Channel[] };
  const found = runtime.channels.find((item) => item.name === name);
  if (!found) throw new Error(`no ${name}`);
  return found;
}

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

test("a TV remote leaves a multiview at the edge of the grid", async ({ page }) => {
  test.setTimeout(120_000);
  await tvRemote(page);
  await setupDone(page);
  await page.setViewportSize({ width: 1920, height: 1080 });
  const left = channel("KBWV");
  const right = channel("WTST");
  await page.goto(`/multiview?ch=${left.id},${right.id}&layout=2up&focus=${left.id}`);
  await expect(page.locator("html")).toHaveAttribute("data-layout", "tv");
  const first = page.locator(`.mv-tile[data-channel="${left.id}"]`);
  const second = page.locator(`.mv-tile[data-channel="${right.id}"]`);
  await expect(first).toBeVisible();
  await expect(second).toBeVisible();

  await first.focus();
  await page.keyboard.press("ArrowUp");
  await expect(page.getByRole("button", { name: "Back to one channel" })).toBeFocused();

  await first.focus();
  await page.keyboard.press("ArrowRight");
  await expect(second).toBeFocused();
  await page.keyboard.press("ArrowLeft");
  await expect(first).toBeFocused();
  await page.keyboard.press("ArrowRight");
  await expect(second).toBeFocused();
  await page.keyboard.press("ArrowDown");
  await expect(page.locator(".mv-bottom button:focus")).toBeVisible();
});

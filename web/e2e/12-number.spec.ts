import { readFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import type { Page } from "@playwright/test";
import { expect, test } from "./fixture";
import { settle } from "./snap";

const here = path.dirname(fileURLToPath(import.meta.url));

function channelId(number: string) {
  const runtime = JSON.parse(readFileSync(path.join(here, ".run/runtime.json"), "utf8")) as { channels: { id: number; number: string }[] };
  const hit = runtime.channels.find((c) => c.number === number);
  if (!hit) throw new Error(`no channel ${number}`);
  return hit.id;
}

async function openChannel(page: Page, id: number) {
  await page.goto(`/watch?channel=${id}`);
  await settle(page);
  const setup = page.getByRole("heading", { name: "Let's set up your TV" });
  const player = page.getByRole("region", { name: "Player" });
  await expect(setup.or(player)).toBeVisible();
  if (await setup.isVisible()) {
    const cont = page.getByRole("button", { name: "Continue" });
    if (await cont.isVisible()) await cont.click();
    await page.getByRole("button", { name: "Watch", exact: true }).click();
    await expect(player).toBeVisible();
  }
  if (!page.url().includes(`channel=${id}`)) {
    await page.goto(`/watch?channel=${id}`);
    await settle(page);
  }
  await expect(player).toBeVisible();
}

function watches(page: Page) {
  const ids: number[] = [];
  page.on("request", (req) => {
    if (req.method() !== "POST" || !req.url().endsWith("/api/v1/watch")) return;
    ids.push((JSON.parse(req.postData() || "{}") as { channelId: number }).channelId);
  });
  return ids;
}

test("a typed channel number tunes once, when it is complete", async ({ page }) => {
  const start = channelId("5.1");
  const first = channelId("4.1");
  const target = channelId("4.2");
  await openChannel(page, start);
  const ids = watches(page);
  const stage = page.getByRole("region", { name: "Player" });

  // "4" and "4." could still be 4.1 or 4.2, so nothing tunes until "4.2".
  await stage.press("4");
  await expect(page.getByRole("status", { name: "Channel 4" })).toBeVisible();
  await page.waitForTimeout(300);
  await stage.press(".");
  await page.waitForTimeout(300);
  expect(ids).toEqual([]);
  await stage.press("2");
  await expect.poll(() => new URL(page.url()).searchParams.get("channel")).toBe(String(target));
  await expect(page.getByRole("status", { name: /^Channel / })).toHaveCount(0);
  await page.waitForTimeout(500);
  expect(ids).not.toContain(first);

  // A number that stays ambiguous tunes when the viewer stops typing.
  await stage.press("4");
  await page.waitForTimeout(800);
  expect(new URL(page.url()).searchParams.get("channel")).toBe(String(target));
  await expect.poll(() => new URL(page.url()).searchParams.get("channel"), { timeout: 5_000 }).toBe(String(first));

  // Only 5.1 starts with "5", so it tunes at once; Enter finishes an ambiguous number.
  await stage.press("5");
  await expect.poll(() => new URL(page.url()).searchParams.get("channel"), { timeout: 1_000 }).toBe(String(start));
  await stage.press("4");
  await stage.press("Enter");
  await expect.poll(() => new URL(page.url()).searchParams.get("channel"), { timeout: 1_000 }).toBe(String(first));
});

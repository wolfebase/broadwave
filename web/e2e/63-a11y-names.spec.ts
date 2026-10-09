// Buttons that repeat once per show say which show, so a screen reader can tell them apart.
import { spawnSync } from "node:child_process";
import { readFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { expect, holdClock, test } from "./fixture";
import { settle } from "./snap";

const here = path.dirname(fileURLToPath(import.meta.url));

function harness() {
  return JSON.parse(readFileSync(path.join(here, ".run/server.json"), "utf8")) as { db: string };
}

function sql(statement: string) {
  const result = spawnSync("sqlite3", [harness().db, `PRAGMA busy_timeout=5000; ${statement}`], { encoding: "utf8" });
  if (result.status !== 0) throw new Error(result.stderr || result.stdout || statement);
}

test("repeated actions name the show", async ({ page }) => {
  expect((await page.request.put("/api/v1/settings", { data: { setupComplete: "1" } })).ok()).toBeTruthy();
  await holdClock(page);
  await page.setViewportSize({ width: 1440, height: 900 });

  await page.goto("/");
  await settle(page);
  await expect(page.locator(".hero-actions").getByRole("button", { name: "Record NFL: Bears at Bills", exact: true })).toBeVisible();

  await page.goto("/sports");
  await settle(page);
  const game = page.getByRole("article").filter({ hasText: "Chicago" });
  await expect(game.getByRole("button", { name: "Watch Chicago at Buffalo", exact: true })).toBeVisible();
  await expect(game.getByRole("button", { name: "Record Chicago at Buffalo", exact: true })).toBeVisible();
  await expect(game.getByRole("button", { name: "Record all Chicago at Buffalo", exact: true })).toBeVisible();

  await page.goto("/search");
  await settle(page);
  await page.getByLabel("Search shows, people, and recordings").fill("Bears");
  await page.getByLabel("Search shows, people, and recordings").press("Enter");
  await expect(page.getByRole("button", { name: "Record every airing NFL: Bears at Bills", exact: true })).toBeVisible();

  await page.goto("/guide");
  await settle(page);
  await page.getByRole("gridcell", { name: /NFL: Bears at Bills/ }).click();
  const sheet = page.getByRole("dialog", { name: "NFL: Bears at Bills" });
  await expect(sheet.getByRole("button", { name: "Record NFL: Bears at Bills", exact: true })).toBeVisible();
  await expect(sheet.getByRole("button", { name: "Record every airing NFL: Bears at Bills", exact: true })).toBeVisible();
  await page.keyboard.press("Escape");

  const started = new Date(Date.now() - 86_400_000).toISOString().replace(/\.\d+Z$/, "Z");
  sql(
    `INSERT INTO recordings (channel_id, guide_number, title, subtitle, path, status, started_at, duration_sec, season, episode)
     VALUES (1, '4.1', 'Desk Names', 'The Quiet Hour', '/tmp/broadwave-a11y-missing.ts', 'complete', '${started}', 4, 1, 4);`,
  );
  await page.goto("/recordings");
  await settle(page);
  const row = page.locator(".media-card").filter({ hasText: "The Quiet Hour" });
  await expect(row.getByRole("button", { name: "Delete S1 E4, Desk Names, The Quiet Hour", exact: true })).toBeVisible();
  await row.getByRole("button", { name: "Delete S1 E4, Desk Names, The Quiet Hour", exact: true }).click();
  await expect(row.getByRole("button", { name: "Remove from the list S1 E4, Desk Names, The Quiet Hour", exact: true })).toBeFocused();
});

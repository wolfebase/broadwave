// Buttons that repeat once per show say which show, so a screen reader can tell them apart.
import { spawnSync } from "node:child_process";
import { mkdirSync, readFileSync, writeFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { expect, holdClock, test } from "./fixture";
import { settle } from "./snap";

const here = path.dirname(fileURLToPath(import.meta.url));

function harness() {
  return JSON.parse(readFileSync(path.join(here, ".run/server.json"), "utf8")) as { db: string; config: string };
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
  const kept = path.join(harness().config, "work", "recordings", "quiet-file.ts");
  mkdirSync(path.dirname(kept), { recursive: true });
  writeFileSync(kept, Buffer.alloc(188));
  sql(
    `INSERT INTO recordings (channel_id, guide_number, title, subtitle, path, status, started_at, duration_sec, season, episode)
     VALUES (1, '4.1', 'Desk Names', 'The Quiet Hour', '/tmp/broadwave-a11y-missing.ts', 'complete', '${started}', 4, 1, 4);`,
  );
  sql(
    `INSERT INTO recordings (channel_id, guide_number, title, subtitle, path, status, started_at, duration_sec)
     VALUES (1, '4.1', 'Harbor Desk', 'Quiet File', '${kept}', 'complete', '${started}', 4);`,
  );
  await page.goto("/recordings");
  await settle(page);
  const row = page.locator(".media-card").filter({ hasText: "The Quiet Hour" });
  await expect(row.getByRole("button", { name: "Delete S1 E4, Desk Names, The Quiet Hour", exact: true })).toBeVisible();
  const keptRow = page.locator(".media-card").filter({ hasText: "Quiet File" });
  await expect(keptRow.getByRole("button", { name: "Rename file Harbor Desk, Quiet File", exact: true })).toBeVisible();
  await row.getByRole("button", { name: "Delete S1 E4, Desk Names, The Quiet Hour", exact: true }).click();
  await expect(row.getByRole("button", { name: "Remove from the list S1 E4, Desk Names, The Quiet Hour", exact: true })).toBeFocused();

  await page.route("**/api/v1/home**", (route) =>
    route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        places: [
          { id: "roof", group: "tuners", kind: "hdhr", name: "Roof Tuner", action: "add", addr: "127.0.0.1:9" },
          { id: "spare", group: "screens", kind: "screen", name: "Spare Box", action: "use" },
        ],
        tunerAddress: "127.0.0.1:5004",
        sharing: false,
      }),
    }),
  );
  await page.route("**/api/v1/sources/free", (route) => {
    if (route.request().method() !== "GET") return route.continue();
    return route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        found: [{ kind: "m3u", name: "Harbor Feed", addr: "127.0.0.1:9", playlist: "http://127.0.0.1:9/p.m3u", guide: "" }],
        guide: "",
      }),
    });
  });
  await page.route("**/api/v1/sources/look", (route) =>
    route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({ found: [{ kind: "hdhr", name: "Attic Tuner", addr: "127.0.0.1:8" }] }),
    }),
  );

  await page.goto("/settings#sources");
  await settle(page);
  await expect(page.getByRole("checkbox", { name: "On guide, 4.1 KBWV", exact: true })).toBeVisible();
  await expect(page.getByRole("checkbox", { name: "Hidden, 4.1 KBWV", exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: "Scan channels Fake HDHomeRun", exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: "Add Roof Tuner", exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: "Use Spare Box as tuner", exact: true })).toBeVisible();
  await page.getByRole("button", { name: "Add free channels" }).click();
  await expect(page.getByRole("button", { name: "Add Harbor Feed", exact: true })).toBeVisible();
  await page.getByRole("button", { name: "Look harder" }).click();
  await expect(page.getByRole("button", { name: "Add Attic Tuner", exact: true })).toBeVisible();
});

// Find commercials says it is working, keeps that one request if the viewer leaves, then says how many.
import { spawnSync } from "node:child_process";
import { mkdirSync, readFileSync, writeFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { copy } from "../src/strings";
import { expect, test } from "./fixture";
import { settle } from "./snap";

const here = path.dirname(fileURLToPath(import.meta.url));
const evidence = path.resolve(here, "../../.evidence/lane/l98");

function harness() {
  return JSON.parse(readFileSync(path.join(here, ".run/server.json"), "utf8")) as { base: string; db: string; config: string };
}

function sql(statement: string) {
  const result = spawnSync("sqlite3", [harness().db, `PRAGMA busy_timeout=5000; ${statement}`], { encoding: "utf8" });
  if (result.status !== 0) throw new Error(result.stderr || result.stdout || statement);
  return result.stdout.trim().split("\n").pop() ?? "";
}

test("finding commercials stays on the one request and then says how many", async ({ page }) => {
  const { config } = harness();
  const file = path.join(config, "work", "recordings", "night-desk.ts");
  mkdirSync(path.dirname(file), { recursive: true });
  writeFileSync(file, Buffer.alloc(188));
  const id = Number(
    sql(
      `INSERT INTO recordings (channel_id, guide_number, title, path, status, started_at, duration_sec) VALUES (1, '4.1', 'Night Desk', '${file}', 'complete', '${new Date(Date.now() - 3_600_000).toISOString().replace(/\.\d+Z$/, "Z")}', 1800); SELECT last_insert_rowid();`,
    ),
  );
  expect(id).toBeGreaterThan(0);
  const setup = await fetch(`${harness().base}/api/v1/settings`, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ setupComplete: "1" }),
  });
  expect(setup.ok).toBe(true);

  let posts = 0;
  let release = () => {};
  const hold = new Promise<void>((resolve) => {
    release = resolve;
  });
  await page.route(`**/api/v1/recordings/${id}/detect`, async (route) => {
    posts += 1;
    await hold;
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        markers: [
          { id: 1, recordingId: id, start: 12, end: 42 },
          { id: 2, recordingId: id, start: 90, end: 120 },
          { id: 3, recordingId: id, start: 200, end: 230 },
        ],
      }),
    });
  });

  try {
    await page.goto("/recordings");
    await settle(page);
    const card = page.locator(".media-card", { hasText: "Night Desk" });
    await card.getByRole("button", { name: "Play" }).click();
    await expect(page.getByRole("heading", { name: "Night Desk" })).toBeVisible();
    await page.getByRole("button", { name: "Options" }).click();
    await page.getByRole("button", { name: copy.library.findCommercials }).click();
    const working = page.getByRole("button", { name: copy.library.findingCommercials });
    await expect(working).toBeDisabled();
    await expect(working).toHaveAttribute("aria-busy", "true");
    await expect.poll(() => posts).toBe(1);
    mkdirSync(evidence, { recursive: true });
    await page.screenshot({ path: path.join(evidence, "finding.jpg"), type: "jpeg", quality: 70 });

    await page.getByRole("button", { name: "Library" }).click();
    await expect(page).toHaveURL(/\/recordings/);
    await card.getByRole("button", { name: "Play" }).click();
    await page.getByRole("button", { name: "Options" }).click();
    await expect(working).toBeDisabled();
    await expect(working).toHaveAttribute("aria-busy", "true");
    expect(posts).toBe(1);

    release();
    await expect(page.getByRole("status")).toHaveText(copy.library.foundBreaks(3));
    await expect(page.getByRole("button", { name: copy.library.findCommercials })).toBeEnabled();
    expect(posts).toBe(1);
  } finally {
    release();
    sql(`DELETE FROM recordings WHERE id = ${id};`);
  }
});

// A recording whose file was moved or deleted outside Broadwave says so and can only be removed.
import { spawnSync } from "node:child_process";
import { mkdirSync, readFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { copy } from "../src/strings";
import { expect, test } from "./fixture";
import { settle } from "./snap";

const here = path.dirname(fileURLToPath(import.meta.url));
const evidence = path.resolve(here, "../../.evidence/j098");

function harness() {
  return JSON.parse(readFileSync(path.join(here, ".run/server.json"), "utf8")) as { base: string; db: string; config: string };
}

function sql(statement: string) {
  const result = spawnSync("sqlite3", [harness().db, `PRAGMA busy_timeout=5000; ${statement}`], { encoding: "utf8" });
  if (result.status !== 0) throw new Error(result.stderr || result.stdout || statement);
  return result.stdout.trim().split("\n").pop() ?? "";
}

test("a recording whose file is gone says so and asks for no poster", async ({ page }) => {
  const { base, config } = harness();
  const gone = path.join(config, "work", "recordings", "moved-away.ts");
  const id = Number(
    sql(
      `INSERT INTO recordings (channel_id, guide_number, title, path, status, started_at, duration_sec) VALUES (1, '4.1', 'Moved Away', '${gone}', 'complete', '${new Date(Date.now() - 3_600_000).toISOString().replace(/\.\d+Z$/, "Z")}', 2040); SELECT last_insert_rowid();`,
    ),
  );
  expect(id).toBeGreaterThan(0);
  const setup = await fetch(`${base}/api/v1/settings`, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ setupComplete: "1" }),
  });
  expect(setup.ok).toBe(true);
  const posters: string[] = [];
  page.on("request", (req) => {
    if (req.url().includes(`/media/poster/${id}`)) posters.push(req.url());
  });
  try {
    const list = (await (await fetch(`${base}/api/v1/recordings`)).json()) as { recordings: { id: number; missing?: boolean; durationSec?: number }[] };
    const row = list.recordings.find((rec) => rec.id === id);
    expect(row?.missing).toBe(true);
    expect(row?.durationSec ?? 0).toBe(0);

    await page.goto("/");
    await settle(page);
    await expect(page.getByText("Moved Away")).toHaveCount(0);

    await page.goto("/library");
    await settle(page);
    const card = page.locator(".media-card", { hasText: "Moved Away" });
    await expect(card).toBeVisible();
    await expect(card.getByText(copy.library.gone)).toBeVisible();
    await expect(card.getByRole("button", { name: "Play" })).toHaveCount(0);
    mkdirSync(evidence, { recursive: true });
    await card.scrollIntoViewIfNeeded();
    await page.screenshot({ path: path.join(evidence, "library-missing.jpg"), type: "jpeg", quality: 70 });
    await card.getByRole("button", { name: "Delete" }).click();
    await card.getByRole("button", { name: copy.library.removeGone }).click();
    await expect(card).toHaveCount(0);
    expect(posters).toEqual([]);
  } finally {
    sql(`DELETE FROM recordings WHERE id = ${id};`);
  }
});

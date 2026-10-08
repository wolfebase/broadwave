// Storage clean-up settings save, and a recording can be kept forever.
import { spawnSync } from "node:child_process";
import { mkdirSync, readFileSync, writeFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { expect, test } from "./fixture";

const here = path.dirname(fileURLToPath(import.meta.url));

function harness() {
  return JSON.parse(readFileSync(path.join(here, ".run/server.json"), "utf8")) as { base: string; db: string; config: string };
}

function sql(statement: string) {
  const result = spawnSync("sqlite3", [harness().db, `PRAGMA busy_timeout=5000; ${statement}`], { encoding: "utf8" });
  if (result.status !== 0) throw new Error(result.stderr || result.stdout || statement);
  return result.stdout.trim().split("\n").pop() ?? "";
}

async function api<T>(route: string, init?: RequestInit) {
  return (await (await fetch(`${harness().base}/api/v1${route}`, init)).json()) as T;
}

const iso = (ms: number) => new Date(ms).toISOString().replace(/\.\d+Z$/, "Z");

test.afterAll(async () => {
  // Later specs share this server: leave nothing that deletes.
  await fetch(`${harness().base}/api/v1/settings`, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ deleteWatchedDays: "0", makeRoom: "0" }),
  });
});

test("clean-up settings save and a recording is kept forever", async ({ page }) => {
  await page.goto("/settings");
  const room = page.getByLabel("When space runs low");
  await expect(room).toHaveValue("0");
  await room.selectOption("1");
  const days = page.getByLabel("Delete watched recordings");
  await expect(days).toHaveValue("0");
  await days.selectOption("14");
  await expect
    .poll(async () => {
      const s = await api<Record<string, string>>("/settings");
      return `${s.makeRoom} ${s.deleteWatchedDays}`;
    })
    .toBe("1 14");

  const file = path.join(harness().config, "work", "recordings", "keepsake.ts");
  mkdirSync(path.dirname(file), { recursive: true });
  writeFileSync(file, Buffer.alloc(188 * 10));
  const id = Number(
    sql(
      `INSERT INTO recordings (channel_id, guide_number, title, subtitle, path, status, started_at, duration_sec)
       VALUES (1, '4.1', 'Harbor Watch', 'The Keepsake', '${file}', 'complete', '${iso(Date.now() - 86_400_000)}', 1800); SELECT last_insert_rowid();`,
    ),
  );

  await page.goto("/recordings");
  const card = page.locator(".media-card", { hasText: "The Keepsake" });
  const keep = card.getByRole("button", { name: "Keep forever" });
  await expect(keep).toHaveAttribute("aria-pressed", "false");
  await keep.click();
  await expect(card).toContainText("Kept forever");
  await expect(keep).toHaveAttribute("aria-pressed", "true");
  const listed = await api<{ recordings: { id: number; keep?: boolean }[] }>("/recordings");
  expect(listed.recordings.find((r) => r.id === id)?.keep).toBe(true);

  await keep.click();
  await expect(card).not.toContainText("Kept forever");
  await expect(keep).toHaveAttribute("aria-pressed", "false");

  await page.goto("/settings");
  await page.getByLabel("When space runs low").selectOption("0");
  await page.getByLabel("Delete watched recordings").selectOption("0");
  await expect
    .poll(async () => {
      const s = await api<Record<string, string>>("/settings");
      return `${s.makeRoom} ${s.deleteWatchedDays}`;
    })
    .toBe("0 0");
});

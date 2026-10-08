// A recording the signal ruined says so and records the episode's next airing.
import { spawnSync } from "node:child_process";
import { mkdirSync, readFileSync, writeFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { expect, test } from "./fixture";

const here = path.dirname(fileURLToPath(import.meta.url));
const evidence = path.resolve(here, "../../.evidence/e6");

function harness() {
  return JSON.parse(readFileSync(path.join(here, ".run/server.json"), "utf8")) as { base: string; db: string; config: string };
}

function sql(statement: string) {
  const result = spawnSync("sqlite3", [harness().db, `PRAGMA busy_timeout=5000; ${statement}`], { encoding: "utf8" });
  if (result.status !== 0) throw new Error(result.stderr || result.stdout || statement);
  return result.stdout.trim().split("\n").pop() ?? "";
}

const iso = (ms: number) => new Date(ms).toISOString().replace(/\.\d+Z$/, "Z");

test("a damaged recording records its next airing", async ({ page }) => {
  const file = path.join(harness().config, "work", "recordings", "lighthouse.ts");
  mkdirSync(path.dirname(file), { recursive: true });
  writeFileSync(file, Buffer.alloc(188 * 10));
  const now = Date.now();
  sql(
    `INSERT INTO recordings (channel_id, guide_number, title, subtitle, program_id, path, status, started_at, duration_sec,
       continuity_errors, transport_errors, sync_losses, packets, gaps, lost_seconds)
     VALUES (1, '4.1', 'Harbor Watch', 'The Lighthouse', 'EP900001', '${file}', 'complete', '${iso(now - 86_400_000)}', 1800, 41, 0, 0, 900000, 3, 12.4);`,
  );
  const next = now + 2 * 86_400_000;
  sql(`INSERT INTO airings (channel_id, title, subtitle, program_id, starts_at, ends_at) VALUES (1, 'Harbor Watch', 'The Lighthouse', 'EP900001', '${iso(next)}', '${iso(next + 1_800_000)}');`);

  await page.goto("/recordings");
  const card = page.locator(".media-card", { hasText: "The Lighthouse" });
  await expect(card).toContainText("Signal dropped for 12 s");
  await card.getByRole("button", { name: "Record it again" }).click();
  await expect(card.getByRole("status")).toContainText("Records again");
  mkdirSync(evidence, { recursive: true });
  await page.screenshot({ path: path.join(evidence, "record-again.jpg"), type: "jpeg", quality: 70 });

  const passes = (await (await fetch(`${harness().base}/api/v1/passes`)).json()) as { passes: { title: string; kind: string; airingStart?: string }[] };
  const once = passes.passes.find((p) => p.title === "Harbor Watch" && p.kind === "once");
  expect(once?.airingStart && Date.parse(once.airingStart)).toBe(Date.parse(iso(next)));
});

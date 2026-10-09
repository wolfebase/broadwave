// A finished recording shows Captions only when the file has cues, and On puts them on screen.
import { spawnSync } from "node:child_process";
import { mkdirSync, readFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { expect, test } from "./fixture";

const here = path.dirname(fileURLToPath(import.meta.url));

function harness() {
  return JSON.parse(readFileSync(path.join(here, ".run/server.json"), "utf8")) as { config: string; db: string };
}

function sql(statement: string) {
  const result = spawnSync("sqlite3", [harness().db, `PRAGMA busy_timeout=5000; ${statement}`], { encoding: "utf8" });
  if (result.status !== 0) throw new Error(result.stderr || result.stdout || statement);
  return result.stdout.trim().split("\n").pop() ?? "";
}

test("recording captions turn on when the file has cues", async ({ page }) => {
  test.setTimeout(90_000);
  const file = path.join(harness().config, "work", "recordings", "rec-captions.ts");
  mkdirSync(path.dirname(file), { recursive: true });
  const made = spawnSync(
    "ffmpeg",
    ["-hide_banner", "-loglevel", "error", "-y", "-f", "lavfi", "-i", "testsrc2=size=320x180:rate=30", "-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000", "-t", "4", "-c:v", "libx264", "-preset", "ultrafast", "-pix_fmt", "yuv420p", "-c:a", "aac", "-ac", "2", "-f", "mpegts", file],
    { encoding: "utf8" },
  );
  if (made.status !== 0) throw new Error(made.stderr);
  const started = new Date(Date.now() - 86_400_000).toISOString().replace(/\.\d+Z$/, "Z");
  const id = sql(
    `INSERT INTO recordings (channel_id, guide_number, title, subtitle, path, status, started_at, duration_sec, breaks_scanned)
     VALUES (1, '4.1', 'Desk Captions', 'The Quiet Hour', '${file}', 'complete', '${started}', 4, 1); SELECT last_insert_rowid();`,
  );

  await page.route("**/captions.vtt", (route) =>
    route.fulfill({
      contentType: "text/vtt",
      body: "WEBVTT\n\n00:00:00.000 --> 00:00:02.000\nHello from captions\n",
    }),
  );
  await page.goto(`/play?recording=${id}`);
  await expect(page.getByRole("heading", { name: "Desk Captions · The Quiet Hour" })).toBeVisible();
  await page.locator("video").evaluate((v: HTMLVideoElement) => {
    v.muted = true;
  });
  await page.getByRole("button", { name: "Options" }).click();
  const captions = page.getByRole("group", { name: "Captions" });
  await expect(captions.getByRole("button", { name: "Off" })).toHaveAttribute("aria-pressed", "true");
  await captions.getByRole("button", { name: "On" }).click();
  await expect(captions.getByRole("button", { name: "On" })).toHaveAttribute("aria-pressed", "true");
  await expect(captions.getByRole("button", { name: "Off" })).toHaveAttribute("aria-pressed", "false");
  await expect.poll(() =>
    page.locator("video").evaluate((v: HTMLVideoElement) => {
      const text = [...v.textTracks].find((track) => track.kind === "captions" || track.kind === "subtitles");
      return text?.mode ?? "";
    }),
  ).toBe("showing");
});

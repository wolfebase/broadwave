import { mkdirSync, writeFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { expect, test } from "./fixture";
import { settle } from "./snap";

const here = path.dirname(fileURLToPath(import.meta.url));
const evidence = path.resolve(here, "../../.evidence/f1");

// Each captions segment's MPEGTS is its video segment's first frame, so a
// placed segment starts where hls.js put that video fragment. With E2E_SOURCE
// set to a captioned broadcast, cues also show on screen.
test("captions turned on follow the picture and go when turned off", async ({ page }) => {
  test.setTimeout(150_000);
  const errors: string[] = [];
  page.on("pageerror", (err) => errors.push(err.message));
  await page.goto("/");
  await settle(page);
  await page.getByRole("button", { name: "Watch", exact: true }).click();
  await expect(page.getByRole("region", { name: "Player" })).toBeVisible();
  const video = page.locator("video.stage-video");
  await expect.poll(() => video.evaluate((el: HTMLVideoElement) => el.currentTime > 1 && el.videoWidth > 0), { timeout: 45_000 }).toBe(true);

  await page.getByRole("button", { name: "Options" }).click();
  const row = page.getByRole("group", { name: "Captions" });
  await expect(row).toBeVisible();
  await row.getByRole("button", { name: "On" }).click();

  const samples: { sn: number; at: number; frag: number | null }[] = [];
  const until = Date.now() + 30_000;
  while (Date.now() < until && samples.length < 12) {
    const got = await video.evaluate((el: HTMLVideoElement & { hls?: { levels: { details?: { fragments: { sn: number; start: number }[] } }[] } }) => {
      const mark = el.dataset.captionSegment;
      if (!mark) return null;
      const [sn, at] = mark.split(" ").map(Number);
      let frag: number | null = null;
      for (const level of el.hls?.levels ?? []) {
        const hit = level.details?.fragments.find((f) => f.sn === sn);
        if (hit) frag = hit.start;
      }
      return { sn, at, frag };
    });
    if (got && !samples.some((s) => s.sn === got.sn)) samples.push(got);
    await page.waitForTimeout(1_000);
  }
  const track = await video.evaluate((el: HTMLVideoElement) => {
    const found = Array.from(el.textTracks).find((t) => t.label === "English CC");
    return found ? { mode: found.mode, cues: found.cues?.length ?? 0, active: Array.from(found.activeCues ?? []).map((c) => (c as VTTCue).text) } : null;
  });
  mkdirSync(evidence, { recursive: true });
  await page.screenshot({ path: path.join(evidence, "player-captions.png") });
  writeFileSync(path.join(evidence, "player-captions.json"), JSON.stringify({ samples, track }, null, 2));

  expect(track?.mode).toBe("showing");
  // hls.js's own in-band 608 track would be a second, empty "English" track.
  const showing = await video.evaluate((el: HTMLVideoElement) => Array.from(el.textTracks).filter((t) => t.mode !== "disabled").map((t) => t.label));
  expect(showing).toEqual(["English CC"]);
  const placed = samples.filter((s) => s.frag != null);
  expect(placed.length, JSON.stringify(samples)).toBeGreaterThanOrEqual(3);
  for (const s of placed) expect(Math.abs(s.at - s.frag!), JSON.stringify(s)).toBeLessThan(0.05);
  if (process.env.E2E_SOURCE) expect(track?.cues ?? 0, "cues from a captioned source").toBeGreaterThan(0);

  // Behind live, segments never fetched at the join are fetched and placed.
  await expect.poll(() => video.evaluate((el: HTMLVideoElement) => el.currentTime), { timeout: 60_000, intervals: [2_000] }).toBeGreaterThan(45);
  const before = await video.evaluate((el: HTMLVideoElement) => el.currentTime);
  const back = page.getByRole("button", { name: "Back 15 seconds" });
  for (let i = 0; i < 3; i++) await back.click();
  await expect.poll(() => video.evaluate((el: HTMLVideoElement) => el.currentTime), { timeout: 10_000 }).toBeLessThan(before - 25);
  await expect
    .poll(
      () =>
        video.evaluate((el: HTMLVideoElement, was: number) => {
          const at = Number((el.dataset.captionSegment ?? "").split(" ")[1]);
          return at < was - 20 && Math.abs(at - el.currentTime) < 15;
        }, before),
      { timeout: 20_000 },
    )
    .toBe(true);

  await row.getByRole("button", { name: "Off" }).click();
  await expect.poll(() => video.evaluate((el: HTMLVideoElement) => Array.from(el.textTracks).find((t) => t.label === "English CC")?.mode)).toBe("disabled");
  expect(errors).toEqual([]);
});

import { mkdirSync, readFileSync, writeFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import type { Page } from "@playwright/test";
import { expect, test } from "./fixture";
import { settle } from "./snap";

const here = path.dirname(fileURLToPath(import.meta.url));
const evidence = path.resolve(here, "../../.evidence/lane");

type Channel = { id: number; number: string; name: string };

function channels(): Channel[] {
  const runtime = JSON.parse(readFileSync(path.join(here, ".run/runtime.json"), "utf8")) as { channels: Channel[] };
  const num = (value: string) => value.split(".").map((part) => Number(part) || 0);
  return [...runtime.channels].sort((a, b) => {
    const x = num(a.number);
    const y = num(b.number);
    return (x[0] ?? 0) - (y[0] ?? 0) || (x[1] ?? 0) - (y[1] ?? 0) || a.name.localeCompare(b.name);
  });
}

async function openChannel(page: Page, id: number) {
  await page.goto(`/watch?channel=${id}&layout=tv`);
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
    await page.goto(`/watch?channel=${id}&layout=tv`);
    await settle(page);
  }
  await expect(player).toBeVisible();
  await expect(page.locator("html")).toHaveAttribute("data-layout", "tv");
}

async function tune(page: Page, id: number) {
  await expect
    .poll(
      () =>
        page.locator("video.stage-video").evaluate((video: HTMLVideoElement & { hls?: { url?: string } }, channelId: number) => {
          const url = video.hls?.url || "";
          return url.includes(`/media/live/${channelId}/`) && video.videoWidth > 0 && !video.paused;
        }, id),
      { timeout: 30_000 },
    )
    .toBe(true);
}

/** Picture for this channel, and the playhead moving, or throws. */
async function movingWithin(page: Page, id: number, budgetMs: number) {
  const start = Date.now();
  let seen = -1;
  let last = "";
  while (Date.now() - start < budgetMs) {
    last = await page.locator("video.stage-video").evaluate((video: HTMLVideoElement & { hls?: { url?: string } }, channelId: number) => {
      const url = video.hls?.url || "";
      const alert = document.querySelector("[role='alert']")?.textContent || "";
      return JSON.stringify({
        match: url.includes(`/media/live/${channelId}/`),
        width: video.videoWidth,
        paused: video.paused,
        time: video.currentTime,
        alert,
      });
    }, id);
    const snap = JSON.parse(last) as { match: boolean; width: number; paused: boolean; time: number; alert: string };
    if (snap.alert) throw new Error(snap.alert);
    if (snap.match && snap.width > 0 && !snap.paused) {
      if (seen >= 0 && snap.time > seen + 0.05) return Date.now() - start;
      if (seen < 0) seen = snap.time;
    } else {
      seen = -1;
    }
    await page.waitForTimeout(200);
  }
  throw new Error(`picture was not channel ${id} within ${budgetMs}ms (${last})`);
}

test.describe("rapid channel changes", () => {
  test.setTimeout(240_000);

  for (const size of [
    { name: "1440x900", width: 1440, height: 900 },
    { name: "1920x1080", width: 1920, height: 1080 },
  ]) {
    test(`lands on the last channel at ${size.name}`, async ({ page }) => {
      const lineup = channels();
      expect(lineup.length).toBeGreaterThan(2);
      const freqs = new Set(lineup.map((channel) => channel.number.split(".")[0]));
      expect(freqs.size).toBeGreaterThan(1);

      // The tuner check at the end counts every tuner that is ours.
      await expect
        .poll(
          async () => {
            const body = (await (await page.request.get("/api/v1/tuners")).json()) as { tuners?: { ours?: boolean }[] };
            return (body.tuners ?? []).some((tuner) => tuner.ours);
          },
          { timeout: 90_000 },
        )
        .toBe(false);

      await page.setViewportSize({ width: size.width, height: size.height });
      const start = lineup[1];
      await openChannel(page, start.id);
      await tune(page, start.id);
      await page.getByRole("region", { name: "Player" }).focus();

      const steps = 10;
      for (let i = 0; i < steps; i++) {
        await page.keyboard.press("ArrowDown");
        if (i < steps - 1) await page.waitForTimeout(2000);
      }
      const expected = lineup[(1 + steps) % lineup.length];
      const landedMs = await movingWithin(page, expected.id, 6000);

      await page.waitForTimeout(20_000);
      const tuners = (await (await page.request.get("/api/v1/tuners")).json()) as {
        tuners?: { guide?: string; ours?: boolean; target?: string }[];
      };
      const prefix = expected.number.split(".")[0];
      const all = tuners.tuners ?? [];
      const held = all.filter((tuner) => tuner.ours);
      // The harness status often omits the guide number. Two tuners still marked
      // ours means the frequency we left is held. A guide number, when present,
      // has to be the channel that is on screen.
      const skipped = held.filter((tuner) => tuner.guide && !tuner.guide.startsWith(`${prefix}.`) && tuner.guide !== prefix);

      mkdirSync(evidence, { recursive: true });
      await page.screenshot({ path: path.join(evidence, `l12-${size.name}.jpg`), animations: "disabled" });
      const summary = { size: size.name, channel: expected.number, landedMs, tuners: all, held, skipped };
      writeFileSync(path.join(evidence, `l12-${size.name}.json`), JSON.stringify(summary, null, 2));
      expect(held, JSON.stringify(all)).toHaveLength(1);
      expect(skipped, JSON.stringify(held)).toEqual([]);
    });
  }
});

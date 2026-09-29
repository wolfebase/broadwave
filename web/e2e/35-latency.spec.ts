import type { Page } from "@playwright/test";
import { expect, test } from "./fixture";
import { settle } from "./snap";

// How far behind live each Live delay plays: server wall time minus the
// program time of the frame on screen. The harness server runs on this
// machine, so the page clock is the server clock.

type Sample = { behind: number; waiting: number; dropped: number; total: number; ahead: number; rate: number };

async function sample(page: Page): Promise<Sample> {
  return page.locator("video.stage-video").evaluate((video) => {
    const el = video as HTMLVideoElement & { hls?: { playingDate?: Date | null }; __waiting?: number };
    if (el.__waiting === undefined) {
      el.__waiting = 0;
      el.addEventListener("waiting", () => (el.__waiting = (el.__waiting ?? 0) + 1));
    }
    const q = el.getVideoPlaybackQuality();
    const media = el.hls?.playingDate?.getTime() ?? 0;
    const b = el.buffered;
    const ahead = b.length ? b.end(b.length - 1) - el.currentTime : 0;
    return {
      behind: media ? Date.now() - media : 0,
      waiting: el.__waiting,
      dropped: q.droppedVideoFrames,
      total: q.totalVideoFrames,
      ahead: Math.round(ahead * 1000),
      rate: el.playbackRate,
    };
  });
}

async function pick(page: Page, name: string) {
  const row = page.getByRole("group", { name: "Live delay" });
  await page.mouse.move(600, 400);
  await page.mouse.move(640, 420);
  if (!(await row.isVisible())) await page.getByRole("button", { name: "Options" }).click();
  await row.getByRole("button", { name, exact: true }).click();
  await expect(row.getByRole("button", { name, exact: true })).toHaveClass(/\bon\b/);
}

const targets = { Lowest: 6_000, Balanced: 16_000, Stable: 20_000 } as const;

test("each Live delay plays at its distance from live", async ({ page }, info) => {
  test.setTimeout(240_000);
  const joins: string[] = [];
  page.on("websocket", (ws) => ws.on("framesent", (f) => typeof f.payload === "string" && f.payload.includes("sync.join") && joins.push(f.payload)));
  await page.goto("/");
  await settle(page);
  await page.evaluate(() => localStorage.setItem("broadwave-live-delay", "stable"));
  await page.getByRole("button", { name: "Watch", exact: true }).click();
  await expect(page.getByRole("region", { name: "Player" })).toBeVisible();
  await expect.poll(async () => (await sample(page)).behind, { timeout: 30_000 }).toBeGreaterThan(0);
  expect(joins.some((j) => j.includes('"latency":"stable"')), `joins: ${joins.join(" ")}`).toBe(true);
  // A fresh tune plays near its first frame until the buffer covers the delay.
  await page.waitForTimeout(25_000);

  const lines: string[] = [];
  for (const name of ["Stable", "Lowest", "Balanced"] as const) {
    await pick(page, name);
    await page.waitForTimeout(8_000);
    const start = await sample(page);
    const behind: number[] = [];
    const ahead: number[] = [];
    const rates = new Set<number>();
    let last = start;
    for (let i = 0; i < 20; i++) {
      await page.waitForTimeout(1_000);
      last = await sample(page);
      behind.push(last.behind);
      ahead.push(last.ahead);
      rates.add(last.rate);
    }
    behind.sort((a, b) => a - b);
    const median = behind[behind.length >> 1];
    const stalls = last.waiting - start.waiting;
    const dropped = last.dropped - start.dropped;
    const frames = last.total - start.total;
    lines.push(`${name}: behind median ${median} ms (min ${behind[0]}, max ${behind[behind.length - 1]}), stalls ${stalls}, dropped ${dropped}/${frames}, buffered ahead min ${Math.min(...ahead)} ms, rates ${[...rates].join("/")}`);
    console.log(lines.at(-1));
    expect(Math.abs(median - targets[name]), lines.at(-1)).toBeLessThan(1_500);
    expect(stalls, lines.at(-1)).toBe(0);
  }
  await page.goto("/settings");
  await settle(page);
  const setting = page.getByRole("group", { name: "Live delay" });
  // The last pick in Options is this device's default now.
  await expect(setting.getByRole("button", { name: "Balanced" })).toHaveAttribute("aria-pressed", "true");
  await setting.getByRole("button", { name: "Lowest" }).click();
  await expect(setting.getByRole("button", { name: "Lowest" })).toHaveAttribute("aria-pressed", "true");
  expect(await page.evaluate(() => localStorage.getItem("broadwave-live-delay"))).toBe("lowest");
  info.annotations.push({ type: "latency", description: lines.join("; ") });
});

import type { BrowserContext, Page } from "@playwright/test";
import { expect, test } from "./fixture";
import { settle } from "./snap";

// Two browsers, not two tabs: each has its own socket and its own player,
// the way two people in a house would.

async function routes(context: BrowserContext) {
  await context.route("**/api/v1/sports/scoreboard**", (route) => route.fulfill({ json: { games: [] } }));
  await context.route("**/api/v1/home**", (route) => route.fulfill({ json: { places: [], tunerAddress: "127.0.0.1:8478", sharing: false } }));
  await context.route("**/api/v1/channels/*/frame**", (route) => route.fulfill({ status: 404, body: "" }));
  await context.route("**/media/art/**", (route) => route.fulfill({ status: 404, body: "" }));
}

type Frame = { media: number; paused: boolean };

async function frame(page: Page): Promise<Frame> {
  return page.locator("video.stage-video").evaluate((video) => {
    const el = video as HTMLVideoElement & { hls?: { playingDate?: Date | null } };
    return { media: el.hls?.playingDate?.getTime() ?? 0, paused: el.paused };
  });
}

async function control(page: Page, name: string) {
  // Any move shows the controls; the middle of a paused picture is its Play button.
  await page.mouse.move(40, 300);
  await page.mouse.move(60, 320);
  await page.getByRole("button", { name, exact: true }).last().click();
}

async function onOneFrame(a: Page, b: Page, paused: boolean, within = 150) {
  let gap = Infinity;
  await expect
    .poll(
      async () => {
        const [fa, fb] = await Promise.all([frame(a), frame(b)]);
        gap = Math.abs(fa.media - fb.media);
        return fa.media > 0 && fb.media > 0 && fa.paused === paused && fb.paused === paused && gap <= within;
      },
      { timeout: 20_000, intervals: [250] },
    )
    .toBe(true);
  return gap;
}

const held = (p: Page) => p.locator("video.stage-video").evaluate((v: HTMLVideoElement) => (v.seekable.length ? v.seekable.end(0) - v.seekable.start(0) : 0));
const screens = (p: Page) => p.getByRole("list", { name: "Screens watching together" }).getByRole("listitem");

test("two browsers watch together: who is in, pause, rewind, play, leave", async ({ page, browser }, info) => {
  test.setTimeout(120_000);
  await page.goto("/");
  await settle(page);
  await page.getByRole("button", { name: "Watch", exact: true }).click();
  await expect(page.getByRole("region", { name: "Player" })).toBeVisible();
  await expect.poll(async () => (await frame(page)).media, { timeout: 30_000 }).toBeGreaterThan(0);

  const pill = page.getByRole("button", { name: "Whole-Home Sync" });
  await pill.click();
  await page.getByRole("dialog", { name: "Whole-Home Sync" }).getByRole("button", { name: "Watch together" }).click();
  await expect(pill).toContainText("Together");
  await expect(page.getByRole("dialog", { name: "Watching together" })).toBeVisible();
  await expect(screens(page)).toHaveCount(1);

  const second = await browser.newContext({ viewport: { width: 1440, height: 900 } });
  try {
    await routes(second);
    const other = await second.newPage();
    await other.goto(page.url());
    await settle(other);
    const otherPill = other.getByRole("button", { name: "Whole-Home Sync" });
    await otherPill.click();
    const invite = other.getByRole("dialog", { name: "Whole-Home Sync" });
    await expect(invite).toContainText("is watching together", { timeout: 15_000 });
    await invite.getByRole("button", { name: "Join" }).click();
    await expect(otherPill).toContainText("Together · 2", { timeout: 15_000 });
    await expect(pill).toContainText("Together · 2");
    await expect(screens(page)).toHaveCount(2);
    await onOneFrame(page, other, false, 250);

    // A fresh tune has seconds of picture; a rewind needs the 15 s to be there.
    await expect.poll(async () => Math.min(await held(page), await held(other)), { timeout: 60_000 }).toBeGreaterThan(20);
    await control(page, "Pause");
    const paused = await onOneFrame(page, other, true, 100);
    const before = (await frame(page)).media;

    // A rewind stops a second after the oldest frame the rewinding screen holds.
    const room = await other.locator("video.stage-video").evaluate((v: HTMLVideoElement) => (v.currentTime - v.seekable.start(0) - 1) * 1000);
    const want = Math.min(15_000, room);
    expect(want, "enough picture behind the room to rewind").toBeGreaterThan(5_000);
    await control(other, "Back 15 seconds");
    await expect.poll(async () => before - (await frame(page)).media, { timeout: 20_000 }).toBeGreaterThan(want - 1_000);
    const rewound = await onOneFrame(page, other, true, 100);
    const after = (await frame(page)).media;
    expect(Math.abs(before - after - want), "the rewind moved both screens back together").toBeLessThan(1_000);

    await control(other, "Play");
    const playing = await onOneFrame(page, other, false, 150);

    // The scrubber moves the group once it rests, to the same frame on both
    // screens. The room plays well behind live, so there is room ahead.
    const slider = page.getByRole("slider", { name: "Playback position" });
    await page.mouse.move(40, 300);
    const at = Number(await slider.inputValue());
    const playingAt = (await frame(page)).media;
    const t0 = Date.now();
    await slider.fill(String(Math.floor(at) + 6));
    await expect.poll(async () => (await frame(other)).media - playingAt - (Date.now() - t0), { timeout: 20_000 }).toBeGreaterThan(4_000);
    const scrubbed = await onOneFrame(page, other, false, 150);

    const numbers = JSON.stringify({ pausedGapMs: paused, rewoundGapMs: rewound, playingGapMs: playing, scrubbedGapMs: scrubbed, rewindMs: before - after, wantMs: Math.round(want) });
    console.log(`together ${numbers}`);
    await info.attach("together.json", { body: numbers, contentType: "application/json" });

    await other.getByRole("dialog", { name: "Watching together" }).getByRole("button", { name: "Leave" }).click();
    await expect(screens(page)).toHaveCount(1, { timeout: 15_000 });
    await expect(otherPill).toContainText(/Synced|screens/);
  } finally {
    await second.close();
  }
});

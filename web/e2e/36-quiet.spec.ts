import { readFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import type { Page } from "@playwright/test";
import { expect, test } from "./fixture";
import { settle } from "./snap";

// A tuner that goes quiet and then carries on with the next frame, the way an
// ATSC 3.0 tuner does while it finds the signal again. The program date-times
// after the pause have to stay on the wall clock, or every player believes it
// fell a minute behind and chases a live edge that is not there.

const here = path.dirname(fileURLToPath(import.meta.url));

type Channel = { id: number; number: string; name: string };
type Harness = { base: string; admin: string };

function harness(): Harness {
  return JSON.parse(readFileSync(path.join(here, ".run/server.json"), "utf8")) as Harness;
}

function channel(name: string): Channel {
  const runtime = JSON.parse(readFileSync(path.join(here, ".run/runtime.json"), "utf8")) as { channels: Channel[] };
  const found = runtime.channels.find((item) => item.name === name);
  if (!found) throw new Error(`no ${name}`);
  return found;
}

async function post(url: string) {
  // main.m3u8 is a master; the dates are in the media playlist beside it.
  const res = await fetch(url, { method: "POST", signal: AbortSignal.timeout(40_000) });
  if (!res.ok) throw new Error(`${url} ${res.status} ${await res.text()}`);
}

async function openChannel(page: Page, id: number) {
  await page.goto(`/watch?channel=${id}`);
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
    await page.goto(`/watch?channel=${id}`);
    await settle(page);
  }
  await expect(player).toBeVisible();
}

const sleep = (ms: number) => new Promise((resolve) => setTimeout(resolve, ms));

async function playing(page: Page) {
  return page.locator("video.stage-video").evaluate((video: HTMLVideoElement) => ({
    ok: video.videoWidth > 0 && !video.paused && video.readyState >= 3,
    at: video.currentTime,
  }));
}

/** How far the newest program date-time in the playlist is behind now, in seconds. */
async function playlistLag(base: string, playlist: string) {
  // main.m3u8 is a master; the dates are in the media playlist beside it.
  const res = await fetch(base + playlist.replace(/main\.m3u8$/, "index.m3u8"));
  const text = await res.text();
  const dates = [...text.matchAll(/#EXT-X-PROGRAM-DATE-TIME:(\S+)/g)].map((m) => Date.parse(m[1]));
  if (dates.length === 0) return Number.NaN;
  return (Date.now() - Math.max(...dates)) / 1000;
}

test("a channel that goes quiet and carries on plays again on the wall clock", async ({ page }) => {
  test.setTimeout(240_000);
  const { base, admin } = harness();
  const wtst = channel("WTST");
  let playlist = "";
  page.on("response", async (res) => {
    if (res.request().method() === "POST" && res.url().endsWith("/api/v1/watch") && res.ok()) {
      const body = (await res.json().catch(() => ({}))) as { mainPlaylist?: string; playlist?: string };
      playlist = body.mainPlaylist || body.playlist || playlist;
    }
  });
  try {
    await openChannel(page, wtst.id);
    await expect.poll(async () => (await playing(page)).ok, { timeout: 45_000 }).toBe(true);
    await sleep(8_000);
    expect(playlist).not.toBe("");
    const before = await playlistLag(base, playlist);
    await post(`${admin}/dark?channel=${encodeURIComponent(wtst.number)}`);
    await sleep(25_000);
    await post(`${admin}/light?channel=${encodeURIComponent(wtst.number)}`);
    // Playing again with no click.
    await expect.poll(async () => (await playing(page)).ok, { timeout: 45_000 }).toBe(true);
    await sleep(6_000);
    const after = await playlistLag(base, playlist);
    // The newest segment is as fresh as before the pause, not 25 s older.
    expect(after, `playlist lag before ${before.toFixed(1)} s, after ${after.toFixed(1)} s`).toBeLessThan(before + 8);
    // The picture comes back a second behind live; the room holds once to get
    // back to its delay. After that it plays on: 10 s of picture in 10 s.
    await sleep(20_000);
    const start = await playing(page);
    await sleep(10_000);
    const end = await playing(page);
    expect(end.ok).toBe(true);
    expect(end.at - start.at).toBeGreaterThan(8.5);
  } finally {
    await post(`${admin}/light?channel=${encodeURIComponent(wtst.number)}`).catch(() => undefined);
  }
});

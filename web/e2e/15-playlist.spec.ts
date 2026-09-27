import { spawn, type ChildProcess } from "node:child_process";
import { mkdirSync, readFileSync, writeFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import type { Page } from "@playwright/test";
import { expect, test } from "./fixture";
import { settle } from "./snap";

const here = path.dirname(fileURLToPath(import.meta.url));
const evidence = path.resolve(here, "../../.evidence/lane/l25");
const picture = "The picture stopped. Trying again usually fixes it.";
const tuner = "This tuner did not answer. Check that it is on.";

type Channel = { id: number; number: string; name: string };
type Harness = { base: string; origin: string };
type Device = { baseUrl?: string; tunerCount?: number; modelNumber?: string };

function harness(): Harness {
  return JSON.parse(readFileSync(path.join(here, ".run/server.json"), "utf8")) as Harness;
}

function channel(): Channel {
  const runtime = JSON.parse(readFileSync(path.join(here, ".run/runtime.json"), "utf8")) as { channels: Channel[] };
  const found = runtime.channels.find((item) => item.name === "Local News") ?? runtime.channels[0];
  if (!found) throw new Error("no playlist channel");
  return found;
}

async function post(url: string) {
  const res = await fetch(url, { method: "POST" });
  if (!res.ok && res.status !== 204) throw new Error(`${url} ${res.status} ${await res.text()}`);
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

async function moving(page: Page) {
  return page.locator("video.stage-video").evaluate(async (video: HTMLVideoElement) => {
    const from = video.currentTime;
    await new Promise((resolve) => setTimeout(resolve, 800));
    const alert = document.querySelector("[role='alert']")?.textContent || "";
    return video.videoWidth > 0 && !video.paused && video.currentTime > from + 0.2 && alert === "";
  });
}

async function expectMoving(page: Page) {
  await expect.poll(() => moving(page), { timeout: 45_000, intervals: [1_000] }).toBe(true);
}

function notice(page: Page, text: string) {
  return page.getByRole("alert").filter({ hasText: text });
}

test.skip(process.env.E2E_PLAYLIST !== "1", "Set E2E_PLAYLIST=1 to stop a playlist stream and bring it back.");

test.afterEach(async () => {
  await post(`${harness().origin}/start`).catch(() => undefined);
});

test("a playlist stream that dies names the picture and plays again without a reload", async ({ page }) => {
  const { base, origin } = harness();
  const news = channel();
  const devices = (await (await fetch(`${base}/api/v1/devices`)).json()) as { devices?: Device[] };
  const health = (await (await fetch(`${base}/api/v1/devices/health`)).json()) as { devices?: unknown[] };
  expect(health.devices ?? [], "a playlist is not a tuner").toEqual([]);
  expect(
    (devices.devices ?? []).every((device) => device.tunerCount === 0 && device.baseUrl === "source"),
    "the lineup is only the playlist",
  ).toBe(true);

  await openChannel(page, news.id);
  await expectMoving(page);
  await page.evaluate(() => {
    document.documentElement.dataset.lane = "stay";
  });
  const url = page.url();
  const stoppedAt = Date.now();
  await post(`${origin}/stop`);

  await expect(notice(page, picture)).toBeVisible({ timeout: 50_000 });
  await expect(page.getByText(tuner)).toHaveCount(0);
  const messageMs = Date.now() - stoppedAt;
  mkdirSync(evidence, { recursive: true });
  await page.screenshot({ path: path.join(evidence, "stopped.jpg"), animations: "disabled" });

  const remain = 30_000 - (Date.now() - stoppedAt);
  if (remain > 0) await page.waitForTimeout(remain);
  await expect(notice(page, picture)).toBeVisible();
  await expect(page.getByText(tuner)).toHaveCount(0);
  const outageMs = Date.now() - stoppedAt;

  await post(`${origin}/start`);
  // Nothing in health, devices, or signals changes when the stream sends
  // again. The player starts the next watch on its own. Try again stays
  // only after two minutes of that.
  const againAt = Date.now();
  let auto = false;
  let recoverMs = 0;
  try {
    await expect
      .poll(() => moving(page), {
        timeout: 15_000,
        intervals: [400],
        message: "the picture starts a new watch on its own within 15s of the stream returning",
      })
      .toBe(true);
    auto = true;
    recoverMs = Date.now() - againAt;
  } finally {
    if (!auto) recoverMs = Date.now() - againAt;
    await page.screenshot({ path: path.join(evidence, auto ? "back.jpg" : "stuck.jpg"), animations: "disabled" }).catch(() => undefined);
    writeFileSync(
      path.join(evidence, "summary.json"),
      JSON.stringify(
        {
          channel: news,
          messageMs,
          outageMs,
          auto,
          recoverMs,
          alert: picture,
        },
        null,
        2,
      ),
    );
  }
  await expect(notice(page, picture)).toHaveCount(0);
  await expect(page.getByText(tuner)).toHaveCount(0);
  await expect(page.locator("html")).toHaveAttribute("data-lane", "stay");
  expect(page.url()).toBe(url);
});

test("leaving the player stops a quiet retry", async ({ page }) => {
  const { origin } = harness();
  const news = channel();
  let left = false;
  let watches = 0;
  page.on("request", (req) => {
    if (!left || req.method() !== "POST") return;
    if (new URL(req.url()).pathname === "/api/v1/watch") watches += 1;
  });
  try {
    await openChannel(page, news.id);
    await expectMoving(page);
    await post(`${origin}/stop`);
    await expect(notice(page, picture)).toBeVisible({ timeout: 50_000 });
    // Opened from the address, Back leaves the player instead of docking it.
    left = true;
    await page.getByRole("button", { name: "Back to browsing" }).click({ timeout: 5_000 });
    await expect(page.getByRole("region", { name: /Now playing|Player/ })).toHaveCount(0);
    await page.waitForTimeout(12_000);
    expect(watches, "a player that has gone does not start another watch").toBe(0);
  } finally {
    await post(`${origin}/start`).catch(() => undefined);
  }
});

test("a playlist channel opened while its stream is down starts once it sends", async ({ page }) => {
  const { origin } = harness();
  const news = channel();
  try {
    await post(`${origin}/stop`);
    await openChannel(page, news.id);
    await expect(notice(page, picture)).toBeVisible({ timeout: 20_000 });
    await expect(page.getByText(/stream returned/i)).toHaveCount(0);
    await page.screenshot({ path: path.join(evidence, "down-at-open.jpg"), animations: "disabled" });
    await post(`${origin}/start`);
    const back = Date.now();
    await expect
      .poll(() => moving(page), { timeout: 20_000, intervals: [400], message: "the picture starts with no click" })
      .toBe(true);
    console.log(`down at open: picture moving ${Date.now() - back} ms after the stream returned`);
    await expect(notice(page, picture)).toHaveCount(0);
  } finally {
    await post(`${origin}/start`).catch(() => undefined);
  }
});

function startFake(sample: string): Promise<{ base: string; child: ChildProcess }> {
  // -realtime encodes a pattern for as long as the tuner is open. -source loops a
  // short file, and that seam is what would stop the picture.
  const child = spawn(path.join(here, ".run/fakehdhr"), ["-realtime", "-ts", sample], { stdio: ["ignore", "pipe", "pipe"] });
  return new Promise((resolve, reject) => {
    let out = "";
    const timer = setTimeout(() => {
      child.kill("SIGKILL");
      reject(new Error(`fake tuner did not start\n${out}`));
    }, 10_000);
    const take = (chunk: Buffer) => {
      out += chunk.toString();
      const base = out.match(/^BASE=(.+)$/m)?.[1]?.trim();
      if (base && out.includes("CONTROL_PORT=")) {
        clearTimeout(timer);
        resolve({ base, child });
      }
    };
    child.stdout?.on("data", take);
    child.stderr?.on("data", take);
    child.on("exit", (code) => {
      clearTimeout(timer);
      reject(new Error(`fake tuner exited ${code}\n${out}`));
    });
  });
}

test("a playlist tile beside a tuner comes back on its own", async ({ page }) => {
  const { base, origin } = harness();
  const lane = path.resolve(here, "../../.evidence/lane/l27");
  const fake = await startFake(path.join(here, ".run/sample.ts"));
  const host = fake.base.replace(/^https?:\/\//, "");
  try {
    let devices: Device[] = [];
    for (let i = 0; i < 20; i++) {
      const res = await fetch(`${base}/api/v1/sources/discover`, {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({ ip: host }),
      });
      const body = (await res.json()) as { devices?: Device[]; found?: number };
      devices = body.devices ?? [];
      if (res.ok && devices.some((device) => (device.tunerCount ?? 0) > 0)) break;
      await page.waitForTimeout(500);
    }
    expect(devices.some((device) => (device.tunerCount ?? 0) > 0), "the fake tuner joined the playlist").toBe(true);

    const list = (await (await fetch(`${base}/api/v1/channels`)).json()) as { channels?: { id: number; displayNumber: string; displayName: string }[] };
    const news = (list.channels ?? []).find((item) => item.displayName === "Local News");
    const wdaf = (list.channels ?? []).find((item) => item.displayName === "WDAF");
    if (!news || !wdaf) throw new Error(`lineup ${JSON.stringify(list.channels)}`);

    await page.goto(`/multiview?ch=${news.id},${wdaf.id}&layout=2up&focus=${wdaf.id}`);
    await settle(page);
    await expect(page.getByRole("region", { name: "Side by side" })).toBeVisible();
    const tile = (id: number) => page.locator(`video.mv-video[data-channel="${id}"]`);
    const moving = (id: number) =>
      tile(id).evaluate(async (video: HTMLVideoElement) => {
        const from = video.currentTime;
        await new Promise((resolve) => setTimeout(resolve, 700));
        return video.videoWidth > 0 && !video.paused && video.currentTime > from + 0.15;
      });
    await expect.poll(async () => (await moving(wdaf.id)) && (await moving(news.id)), { timeout: 45_000, intervals: [1_000] }).toBe(true);

    await tile(wdaf.id).evaluate((video: HTMLVideoElement) => {
      const box = window as unknown as { __tunerPauses: number };
      box.__tunerPauses = 0;
      video.addEventListener("pause", () => {
        box.__tunerPauses += 1;
      });
    });
    const pauses = () => page.evaluate(() => (window as unknown as { __tunerPauses: number }).__tunerPauses ?? 0);

    const began = await tile(wdaf.id).evaluate((video: HTMLVideoElement) => video.currentTime);
    const stoppedAt = Date.now();
    await post(`${origin}/stop`);
    let messageMs = 0;
    const newsAlert = page.locator(`.mv-tile:has(video[data-channel="${news.id}"])`).getByRole("alert");
    while (Date.now() - stoppedAt < 30_000) {
      const state = await tile(wdaf.id).evaluate((video: HTMLVideoElement) => ({ paused: video.paused, width: video.videoWidth, time: video.currentTime }));
      expect(state.paused, "the tuner picture paused while the playlist was down").toBe(false);
      expect(state.width, "the tuner picture went black while the playlist was down").toBeGreaterThan(0);
      expect(await pauses(), "the tuner picture paused while the playlist was down").toBe(0);
      if (!messageMs && (await newsAlert.isVisible().catch(() => false)) && (await newsAlert.innerText()).includes(picture)) {
        messageMs = Date.now() - stoppedAt;
        mkdirSync(lane, { recursive: true });
        await page.screenshot({ path: path.join(lane, "playlist-stopped.jpg"), animations: "disabled" });
      }
      await page.waitForTimeout(1_000);
    }
    expect(messageMs, "the playlist tile names the picture").toBeGreaterThan(0);
    const advanced = await tile(wdaf.id).evaluate((video: HTMLVideoElement) => video.currentTime);
    expect(advanced, "the tuner picture kept moving").toBeGreaterThan(began + 8);
    await expect(newsAlert).toContainText(picture);
    await expect(page.getByText(tuner)).toHaveCount(0);
    expect(await pauses()).toBe(0);

    await post(`${origin}/start`);
    const againAt = Date.now();
    await expect
      .poll(
        () =>
          tile(news.id).evaluate(async (video: HTMLVideoElement) => {
            const from = video.currentTime;
            await new Promise((resolve) => setTimeout(resolve, 700));
            const alert = video.closest(".mv-tile")?.querySelector("[role='alert']")?.textContent || "";
            return video.videoWidth > 0 && !video.paused && video.currentTime > from + 0.15 && alert === "";
          }),
        { timeout: 20_000, intervals: [500], message: "the playlist picture starts again with no click" },
      )
      .toBe(true);
    const recoverMs = Date.now() - againAt;
    expect(await pauses(), "the tuner picture paused").toBe(0);
    const still = await tile(wdaf.id).evaluate((video: HTMLVideoElement) => ({
      paused: video.paused,
      width: video.videoWidth,
      time: video.currentTime,
      error: video.dataset.hlsError || "",
    }));
    expect(still.paused, `the tuner picture paused ${JSON.stringify(still)}`).toBe(false);
    expect(still.width, `the tuner picture went black ${JSON.stringify(still)}`).toBeGreaterThan(0);
    expect(still.time, `the tuner picture stopped advancing ${JSON.stringify(still)}`).toBeGreaterThan(advanced);
    await expect(page.getByText(tuner)).toHaveCount(0);
    // A playing link holds no tuner, and a playing channel keeps the one it has.
    const plan = (await (
      await fetch(`${base}/api/v1/multiview/plan`, {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({ channelIds: [news.id, wdaf.id] }),
      })
    ).json()) as { blocked?: unknown[] };
    expect(plan.blocked, `the plan blocked a playing channel ${JSON.stringify(plan)}`).toEqual([]);
    mkdirSync(lane, { recursive: true });
    await page.screenshot({ path: path.join(lane, "playlist-back.jpg"), animations: "disabled" });
    const summaryPath = path.join(lane, "summary.json");
    const prior = (() => {
      try {
        return JSON.parse(readFileSync(summaryPath, "utf8")) as Record<string, unknown>;
      } catch {
        return {};
      }
    })();
    writeFileSync(summaryPath, JSON.stringify({ ...prior, playlist: { messageMs, recoverMs, pauses: await pauses() } }, null, 2));
    console.log(`playlist tile back ${recoverMs} ms after the stream returned; message at ${messageMs} ms`);
  } finally {
    fake.child.kill("SIGKILL");
    await post(`${origin}/start`).catch(() => undefined);
  }
});

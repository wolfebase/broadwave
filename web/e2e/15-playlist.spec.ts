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

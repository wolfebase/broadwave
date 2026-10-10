// Move to another screen: one browser sends its channel to another, which
// starts playing it and says who sent it. Two browsers, not two tabs: tabs of
// one browser share a screen id and count as one screen.
import type { BrowserContext, Page } from "@playwright/test";
import { expect, test } from "./fixture";
import { settle } from "./snap";

type Channel = { id: number; displayNumber?: string; guideNumber: string };
type Screen = { id: string; name: string; kind: string; channelId?: number };

async function routes(context: BrowserContext) {
  await context.route("**/api/v1/sports/scoreboard**", (route) => route.fulfill({ json: { games: [] } }));
  await context.route("**/api/v1/home**", (route) => route.fulfill({ json: { places: [], tunerAddress: "127.0.0.1:8478", sharing: false } }));
  await context.route("**/api/v1/channels/*/frame**", (route) => route.fulfill({ status: 404, body: "" }));
  await context.route("**/media/art/**", (route) => route.fulfill({ status: 404, body: "" }));
}

const media = (page: Page) =>
  page.locator("video.stage-video").evaluate((video) => {
    const el = video as HTMLVideoElement & { hls?: { playingDate?: Date | null } };
    return el.hls?.playingDate?.getTime() ?? 0;
  });

async function screens(page: Page) {
  return ((await (await page.request.get("/api/v1/screens")).json()) as { screens: Screen[] }).screens;
}

const screenId = (page: Page) => page.evaluate(() => localStorage.getItem("broadwave-screen-id") ?? "");

/** A browser on Home, open on the server as a screen, listening for a channel. */
async function onHome(context: BrowserContext) {
  await routes(context);
  const page = await context.newPage();
  await page.goto("/");
  await settle(page);
  await expect.poll(() => page.evaluate(() => performance.getEntriesByType("resource").some((r) => r.name.includes("ScreenMoves")))).toBe(true);
  const id = await screenId(page);
  expect(id).toMatch(/^[A-Za-z0-9-]{1,64}$/);
  await expect.poll(async () => (await screens(page)).some((s) => s.id === id)).toBe(true);
  const me = (await screens(page)).find((s) => s.id === id)!;
  return { page, id, name: me.name };
}

async function playing(page: Page, channel: Channel) {
  await page.goto(`/watch?channel=${channel.id}`);
  await settle(page);
  await expect(page.getByRole("region", { name: "Player" })).toBeVisible();
  await expect.poll(() => media(page), { timeout: 30_000 }).toBeGreaterThan(0);
}

async function openMove(page: Page) {
  await page.mouse.move(40, 300);
  await page.mouse.move(60, 320);
  const options = page.getByRole("button", { name: "Options" });
  if ((await options.getAttribute("aria-expanded")) !== "true") await options.click();
  await page.getByRole("button", { name: "Move to another screen" }).click();
  const dialog = page.getByRole("dialog", { name: "Move to another screen" });
  await expect(dialog).toBeVisible();
  return dialog;
}

test.beforeEach(async ({ page }) => {
  const setup = await page.request.put("/api/v1/settings", { data: { setupComplete: "1" } });
  expect(setup.ok()).toBeTruthy();
});

test("a playing channel moves to another screen, which plays it and says who sent it", async ({ page, browser }) => {
  test.setTimeout(120_000);
  const { channels } = (await (await page.request.get("/api/v1/channels?guide=1")).json()) as { channels: Channel[] };
  const channel = channels[0];
  const number = channel.displayNumber || channel.guideNumber;
  const second = await browser.newContext({ viewport: { width: 1440, height: 900 } });
  try {
    const b = await onHome(second);
    await playing(page, channel);
    const mine = await screenId(page);
    expect(mine).not.toBe(b.id);
    // The server knows what this screen is watching; the list says it.
    await expect.poll(async () => (await screens(page)).find((s) => s.id === mine)?.channelId).toBe(channel.id);

    const dialog = await openMove(page);
    const list = dialog.getByRole("list", { name: "Screens" });
    // This browser is not in its own list.
    await expect(list.getByRole("button")).toHaveCount(1);
    const target = list.getByRole("button", { name: b.name });
    await expect(target).toBeVisible();
    await expect(target).not.toContainText("Watching");

    // Arrows walk the panel instead of changing the channel; Escape closes it.
    await expect(target).toBeFocused();
    await page.keyboard.press("ArrowDown");
    await expect(dialog.getByRole("button", { name: "Cancel" })).toBeFocused();
    await page.keyboard.press("ArrowUp");
    await expect(target).toBeFocused();
    await page.keyboard.press("Escape");
    await expect(dialog).toHaveCount(0);
    await expect(page.getByRole("button", { name: "Move to another screen" })).toBeFocused();
    await expect(page).toHaveURL(new RegExp(`channel=${channel.id}(&|$)`));

    await page.keyboard.press("Enter");
    await expect(dialog).toBeVisible();
    await expect(target).toBeFocused();
    await page.keyboard.press("Enter");

    // The sender waits for the other screen to be on the channel, then stops and leaves the player.
    await expect(dialog.getByRole("status")).toHaveText(`Starting on ${b.name}…`);
    await expect(target).toBeDisabled();
    await expect(page.getByRole("status").filter({ hasText: `Playing on ${b.name}` })).toBeVisible();
    await expect(page.locator(".stage")).toHaveCount(0);
    await expect(page).not.toHaveURL(/\/watch/);

    // The other screen plays the channel, and its picture moves.
    await expect(b.page).toHaveURL(new RegExp(`channel=${channel.id}(&|$)`));
    await expect(b.page.getByRole("region", { name: "Player" })).toBeVisible();
    const sender = (await screens(page)).find((s) => s.id === mine)?.name ?? "";
    expect(sender).not.toBe("");
    await expect(b.page.getByRole("status").filter({ hasText: `From ${sender}` })).toBeVisible();
    await expect.poll(() => media(b.page), { timeout: 30_000 }).toBeGreaterThan(0);
    // The note's time starts with the picture, so a slow tune does not use it up.
    await expect(b.page.getByRole("status").filter({ hasText: `From ${sender}` })).toBeVisible();
    const first = await media(b.page);
    await expect.poll(() => media(b.page), { timeout: 15_000 }).toBeGreaterThan(first + 1_000);
    await expect(b.page.getByRole("status").filter({ hasText: `From ${sender}` })).toHaveCount(0, { timeout: 15_000 });

    // Now the receiving screen is the one watching, and a list on another screen says so.
    await expect.poll(async () => (await screens(page)).find((s) => s.id === b.id)?.channelId).toBe(channel.id);
    await playing(page, channel);
    const again = await openMove(page);
    await expect(again.getByRole("list", { name: "Screens" }).getByRole("button", { name: b.name })).toContainText(`Watching ${number}`);
  } finally {
    await second.close();
  }
});

for (const size of [
  { name: "desktop", width: 1440, height: 900 },
  { name: "phone", width: 390, height: 844 },
  { name: "tv", width: 1920, height: 1080 },
]) {
  test(`a screen that closed says so, and the channel keeps playing (${size.name})`, async ({ page, browser }) => {
    test.setTimeout(90_000);
    await page.setViewportSize({ width: size.width, height: size.height });
    const { channels } = (await (await page.request.get("/api/v1/channels?guide=1")).json()) as { channels: Channel[] };
    const channel = channels[0];
    const second = await browser.newContext({ viewport: { width: 1440, height: 900 } });
    const b = await onHome(second);
    await playing(page, channel);

    const dialog = await openMove(page);
    const target = dialog.getByRole("list", { name: "Screens" }).getByRole("button", { name: b.name });
    await expect(target).toBeVisible();
    await second.close();
    await expect.poll(async () => (await screens(page)).some((s) => s.id === b.id)).toBe(false);

    await target.click();
    await expect(dialog.getByRole("alert")).toHaveText("That screen isn't open right now.");
    // The list is asked again, so the closed screen is gone from it.
    await expect(dialog).toContainText("Open Broadwave on another screen and it shows up here.");
    await expect(target).toHaveCount(0);
    await expect(page.getByRole("region", { name: "Player" })).toBeVisible();
    await expect(page).toHaveURL(new RegExp(`channel=${channel.id}(&|$)`));
    const at = await media(page);
    await expect.poll(() => media(page), { timeout: 15_000 }).toBeGreaterThan(at + 1_000);

    // Opened again, the list is empty and says how to fill it.
    await dialog.getByRole("button", { name: "Cancel" }).click();
    const again = await openMove(page);
    await expect(again).toContainText("Open Broadwave on another screen and it shows up here.");
    await page.keyboard.press("Escape");
    await expect(again).toHaveCount(0);
    await expect(page.getByRole("button", { name: "Move to another screen" })).toBeFocused();
  });
}

test("a screen that takes the channel but never plays it leaves this one playing, and setup is not interrupted", async ({ page, browser }) => {
  test.setTimeout(90_000);
  const { channels } = (await (await page.request.get("/api/v1/channels?guide=1")).json()) as { channels: Channel[] };
  const channel = channels[0];
  const second = await browser.newContext({ viewport: { width: 1440, height: 900 } });
  try {
    const b = await onHome(second);
    // Mid-setup, the screen is open but ignores what it is sent, as a phone asleep with its socket up would.
    await b.page.goto("/setup");
    await expect.poll(async () => (await screens(page)).some((s) => s.id === b.id)).toBe(true);
    await expect.poll(() => b.page.evaluate(() => performance.getEntriesByType("resource").some((r) => r.name.includes("ScreenMoves")))).toBe(true);
    await playing(page, channel);

    const dialog = await openMove(page);
    const target = dialog.getByRole("list", { name: "Screens" }).getByRole("button", { name: b.name });
    await target.click();
    await expect(dialog.getByRole("status")).toHaveText(`Starting on ${b.name}…`);
    await expect(dialog.getByRole("alert")).toHaveText(`${b.name} didn't start it. It may be asleep.`, { timeout: 20_000 });
    await expect(dialog.getByRole("status")).toHaveCount(0);
    await expect(target).toBeEnabled();

    // This screen kept playing, and the other one is still in setup.
    await expect(page.getByRole("region", { name: "Player" })).toBeVisible();
    await expect(page).toHaveURL(new RegExp(`channel=${channel.id}(&|$)`));
    const at = await media(page);
    await expect.poll(() => media(page), { timeout: 15_000 }).toBeGreaterThan(at + 1_000);
    await expect(b.page).toHaveURL(/\/setup/);
    await expect(b.page.getByRole("region", { name: "Player" })).toHaveCount(0);
  } finally {
    await second.close();
  }
});

test("watching together has no Move to another screen", async ({ page }) => {
  test.setTimeout(60_000);
  const { channels } = (await (await page.request.get("/api/v1/channels?guide=1")).json()) as { channels: Channel[] };
  await playing(page, channels[0]);
  await page.mouse.move(40, 300);
  const options = page.getByRole("button", { name: "Options" });
  await options.click();
  await expect(page.getByRole("button", { name: "Move to another screen" })).toBeVisible();
  await options.click();
  const pill = page.getByRole("button", { name: "Whole-Home Sync" });
  await pill.click();
  await page.getByRole("dialog", { name: "Whole-Home Sync" }).getByRole("button", { name: "Watch together" }).click();
  await expect(pill).toContainText("Together");
  await options.click();
  await expect(page.getByRole("button", { name: "Even volume" }).or(page.getByRole("group", { name: "Quality" }))).toBeVisible();
  await expect(page.getByRole("button", { name: "Move to another screen" })).toHaveCount(0);
});

test("a screen with sync off takes the channel, and the sender knows it did", async ({ page, browser }) => {
  test.setTimeout(120_000);
  const { channels } = (await (await page.request.get("/api/v1/channels?guide=1")).json()) as { channels: Channel[] };
  const channel = channels[0];
  const second = await browser.newContext({ viewport: { width: 1440, height: 900 } });
  try {
    // Sync off joins no room, so the screen tells the server what it plays.
    await second.addInitScript(() => localStorage.setItem("ota-live", JSON.stringify({ sync: false })));
    const b = await onHome(second);
    await playing(page, channel);
    const dialog = await openMove(page);
    await dialog.getByRole("list", { name: "Screens" }).getByRole("button", { name: b.name }).click();
    await expect(page.getByRole("status").filter({ hasText: `Playing on ${b.name}` })).toBeVisible({ timeout: 20_000 });
    await expect(page.locator(".stage")).toHaveCount(0);
    await expect(b.page).toHaveURL(new RegExp(`channel=${channel.id}(&|$)`));
    await expect.poll(async () => (await screens(page)).find((s) => s.id === b.id)?.channelId).toBe(channel.id);
    // Leaving the player says so too.
    await b.page.goto("/");
    await expect.poll(async () => (await screens(page)).find((s) => s.id === b.id)?.channelId ?? 0).toBe(0);
  } finally {
    await second.close();
  }
});

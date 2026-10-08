// A game alert from the server shows with Watch, opens that channel, and
// waits off the player. The server's rules (followed team starting, close
// finish, no close finish for a recording) are Go tests.
import path from "node:path";
import { fileURLToPath } from "node:url";
import type { Page, WebSocketRoute } from "@playwright/test";
import { expect, test } from "./fixture";
import { settle } from "./snap";

const here = path.dirname(fileURLToPath(import.meta.url));
const evidence = path.resolve(here, "../../.evidence/d8");

type Channel = { id: number; number: string; guideNumber: string; displayNumber?: string };

async function socket(page: Page) {
  let route: WebSocketRoute | undefined;
  await page.routeWebSocket(/\/api\/v1\/ws$/, (ws) => {
    ws.connectToServer();
    route = ws;
  });
  return async (data: Record<string, unknown>) => {
    await expect.poll(() => Boolean(route)).toBe(true);
    // The listener loads on its own after the page; one sent before it
    // subscribes is gone (as on any socket), so wait until it is there.
    await expect.poll(() => page.evaluate(() => performance.getEntriesByType("resource").some((r) => r.name.includes("GameAlerts")))).toBe(true);
    await page.waitForTimeout(300);
    route?.send(JSON.stringify({ type: "game.alert", data }));
  };
}

async function firstChannel(page: Page) {
  const { channels } = (await (await page.request.get("/api/v1/channels?guide=1")).json()) as { channels: Channel[] };
  const first = channels[0];
  return { ...first, number: first.displayNumber || first.guideNumber };
}

test.beforeEach(async ({ page }) => {
  const setup = await page.request.put("/api/v1/settings", { data: { setupComplete: "1" } });
  expect(setup.ok()).toBeTruthy();
});

for (const size of [
  { name: "desktop", width: 1440, height: 900 },
  { name: "phone", width: 390, height: 844 },
  { name: "tv", width: 1920, height: 1080 },
]) {
  test(`a close game shows with Watch, and Watch opens its channel (${size.name})`, async ({ page }) => {
    await page.setViewportSize({ width: size.width, height: size.height });
    const channel = await firstChannel(page);
    const alert = await socket(page);
    await page.goto("/guide");
    await settle(page);
    await alert({
      id: "e2e-close:close", kind: "close", gameId: "e2e-close", channelId: channel.id, channel: channel.number,
      text: "Close game: BUF at MIA", detail: "BUF 21, MIA 24 · 4th 3:20",
    });
    const notice = page.getByRole("status").filter({ hasText: "Close game: BUF at MIA" });
    await expect(notice).toBeVisible();
    await expect(notice).toContainText("BUF 21, MIA 24 · 4th 3:20");
    // The same alert again (a reconnect) is not shown twice.
    await alert({ id: "e2e-close:close", kind: "close", gameId: "e2e-close", channelId: channel.id, channel: channel.number, text: "Close game: BUF at MIA" });
    await page.screenshot({ path: path.join(evidence, `close-${size.name}.jpg`), type: "jpeg", quality: 70, animations: "disabled" });
    await notice.getByRole("button", { name: `Watch ${channel.number}` }).click();
    await expect(notice).toHaveCount(0);
    await expect(page.getByRole("region", { name: "Player" })).toBeVisible();
    await expect(page).toHaveURL(new RegExp(`channel=${channel.id}`));
  });
}

test("an alert waits off the player, and Not now lets it go", async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 900 });
  const channel = await firstChannel(page);
  const alert = await socket(page);
  await page.goto(`/watch?channel=${channel.id}`);
  await settle(page);
  await expect(page.getByRole("region", { name: "Player" })).toBeVisible();
  await alert({ id: "e2e-start:start", kind: "start", gameId: "e2e-start", channelId: channel.id, channel: channel.number, text: "Starting now: CHI at LV" });
  const notice = page.getByRole("status").filter({ hasText: "Starting now: CHI at LV" });
  await page.waitForTimeout(1500);
  await expect(notice).toHaveCount(0);
  await page.keyboard.press("Escape");
  await expect(notice).toBeVisible();
  await notice.getByRole("button", { name: "Not now" }).click();
  await expect(notice).toHaveCount(0);
});

test("game alerts can be turned off", async ({ page }) => {
  await page.goto("/settings");
  const picker = page.getByLabel("Game alerts");
  await expect(picker).toHaveValue("all");
  for (const value of ["teams", "off"]) {
    await picker.selectOption(value);
    await expect
      .poll(async () => ((await (await page.request.get("/api/v1/settings")).json()) as { gameAlerts?: string }).gameAlerts)
      .toBe(value);
  }
  await picker.selectOption("all");
  await expect
    .poll(async () => ((await (await page.request.get("/api/v1/settings")).json()) as { gameAlerts?: string }).gameAlerts)
    .toBe("all");
});

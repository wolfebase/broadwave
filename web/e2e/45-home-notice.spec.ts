// "New … found" goes by itself, gives way to a control it covers, and waits
// off the player and multiview.
import type { Page, WebSocketRoute } from "@playwright/test";
import { expect, test } from "./fixture";
import { settle } from "./snap";

type Channel = { id: number };

const message = "New Apple TV found: Den.";

async function socket(page: Page) {
  let route: WebSocketRoute | undefined;
  await page.routeWebSocket(/\/api\/v1\/ws$/, (ws) => {
    ws.connectToServer();
    route = ws;
  });
  return {
    announce: async () => {
      await expect.poll(() => Boolean(route)).toBe(true);
      route?.send(JSON.stringify({ type: "activity", data: { kind: "home", message } }));
    },
  };
}

test.beforeEach(async ({ page }) => {
  const setup = await page.request.put("/api/v1/settings", { data: { setupComplete: "1" } });
  expect(setup.ok()).toBeTruthy();
  await page.setViewportSize({ width: 1440, height: 900 });
});

test("the notice hides itself about 12 s after it shows", async ({ page }) => {
  const ws = await socket(page);
  await page.goto("/guide");
  await settle(page);
  await ws.announce();
  const notice = page.getByRole("status").filter({ hasText: message });
  await expect(notice).toBeVisible();
  const shown = Date.now();
  await expect(notice).toHaveCount(0, { timeout: 20_000 });
  expect(Date.now() - shown).toBeGreaterThan(10_000);
});

test("a control under the notice that takes focus wins", async ({ page }) => {
  const ws = await socket(page);
  await page.goto("/guide");
  await settle(page);
  await ws.announce();
  const notice = page.getByRole("status").filter({ hasText: message });
  await expect(notice).toBeVisible();
  const covered = await page.evaluate(() => {
    const box = document.querySelector(".banner-home")!.getBoundingClientRect();
    const all = [...document.querySelectorAll<HTMLElement>("button, a[href], input, [tabindex='0']")];
    const under = all.find((el) => {
      if (el.closest(".banner-home")) return false;
      const r = el.getBoundingClientRect();
      return r.width > 0 && r.left < box.right && box.left < r.right && r.top < box.bottom && box.top < r.bottom;
    });
    under?.focus();
    return Boolean(under);
  });
  expect(covered).toBe(true);
  await expect(notice).toHaveCount(0);
});

test("the notice waits off the player and multiview", async ({ page }) => {
  const { channels } = (await (await page.request.get("/api/v1/channels?guide=1")).json()) as { channels: Channel[] };
  const ws = await socket(page);
  await page.goto(`/multiview?ch=${channels[0].id}&layout=2up`);
  await settle(page);
  await ws.announce();
  const notice = page.getByRole("status").filter({ hasText: message });
  await page.waitForTimeout(1500);
  await expect(notice).toHaveCount(0);

  await page.goto(`/watch?channel=${channels[0].id}`);
  await settle(page);
  await expect(page.getByRole("region", { name: "Player" })).toBeVisible();
  await ws.announce();
  await page.waitForTimeout(1500);
  await expect(notice).toHaveCount(0);
  // Back to browsing with the mini player: now it shows.
  await page.keyboard.press("Escape");
  await expect(notice).toBeVisible();
});

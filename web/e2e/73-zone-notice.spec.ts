// A server whose time zone is not this browser's tells an admin across the top.
// On a TV the remote reaches that notice with Up, Down from the tabs goes to the
// page under it, and closing it keeps a visible ring on the page.
import type { Page } from "@playwright/test";
import { expect, test } from "./fixture";

// The worker's own fetches skip page.route, and this test answers /server itself.
test.use({ serviceWorkers: "block" });

async function tvRemote(page: Page) {
  await page.addInitScript(() => {
    const orig = window.matchMedia.bind(window);
    window.matchMedia = (query: string) =>
      String(query).includes("pointer") && String(query).includes("coarse")
        ? ({ matches: true, media: query, onchange: null, addEventListener() {}, removeEventListener() {}, addListener() {}, removeListener() {}, dispatchEvent: () => false } as MediaQueryList)
        : orig(query);
  });
}

/** The server says it runs on Brisbane time, far from the browser's (UTC here, Chicago in the image run). */
async function farServer(page: Page) {
  await page.route("**/api/v1/server", async (route) => {
    const res = await route.fetch();
    const body = (await res.json()) as Record<string, unknown>;
    await route.fulfill({ response: res, json: { ...body, timeZone: "Australia/Brisbane", timeZoneValid: true, utcOffsetMinutes: 600 } });
  });
}

function focused(page: Page) {
  return page.evaluate(() => {
    const el = document.activeElement as HTMLElement | null;
    if (!el || el === document.body) return { label: "", ring: false, notice: false, bar: false };
    return {
      label: (el.textContent || "").replace(/\s+/g, " ").trim(),
      ring: el.matches(":focus-visible"),
      notice: Boolean(el.closest(".banner-zone")),
      bar: Boolean(el.closest(".topbar")),
    };
  });
}

test("the time zone notice: Up reaches it, Down from the tabs passes it, and Not now keeps the ring", async ({ page }) => {
  await tvRemote(page);
  await farServer(page);
  expect((await page.request.put("/api/v1/settings", { data: { setupComplete: "1" } })).ok()).toBeTruthy();
  await page.setViewportSize({ width: 1920, height: 1080 });
  await page.goto("/");
  // Not settle(): it closes every notice, and this one is the subject.
  await expect(page.locator("main")).toHaveAttribute("data-ready", "1");
  await expect(page.locator("html")).toHaveAttribute("data-layout", "tv");
  const notice = page.locator(".banner-zone");
  await expect(notice).toContainText("Australia/Brisbane");
  const watch = page.locator(".hero-actions .btn").first();
  await expect(watch).toBeFocused();

  // Up from Watch lands on the notice's Not now, not past it on the tabs.
  await page.keyboard.press("ArrowUp");
  await expect.poll(async () => (await focused(page)).notice).toBe(true);
  expect((await focused(page)).label).toBe("Not now");

  // Up again is the tab bar; Down from a tab enters the page, not the far end of the notice.
  await page.keyboard.press("ArrowUp");
  await expect.poll(async () => (await focused(page)).bar).toBe(true);
  await page.keyboard.press("ArrowDown");
  await expect.poll(() => page.evaluate(() => Boolean(document.activeElement?.closest(".hero-actions")))).toBe(true);

  // Not now from the remote closes it, and the ring stays on the page.
  await page.keyboard.press("ArrowUp");
  await expect.poll(async () => (await focused(page)).notice).toBe(true);
  await page.keyboard.press("Enter");
  await expect(notice).toHaveCount(0);
  await expect.poll(async () => (await focused(page)).ring).toBe(true);
  expect((await focused(page)).bar).toBe(false);
});

test("a notice that floats over the page sits under the time zone notice, not on its words", async ({ page }) => {
  let route: import("@playwright/test").WebSocketRoute | undefined;
  await page.routeWebSocket(/\/api\/v1\/ws$/, (ws) => {
    ws.connectToServer();
    route = ws;
  });
  await farServer(page);
  expect((await page.request.put("/api/v1/settings", { data: { setupComplete: "1" } })).ok()).toBeTruthy();
  await page.setViewportSize({ width: 1440, height: 900 });
  await page.goto("/guide");
  await expect(page.locator("main")).toHaveAttribute("data-ready", "1");
  await expect(page.locator(".banner-zone")).toBeVisible();
  await expect.poll(() => Boolean(route)).toBe(true);
  route?.send(JSON.stringify({ type: "activity", data: { kind: "home", message: "New Apple TV found: Den." } }));
  const floating = page.locator(".banner-home");
  await expect(floating).toBeVisible();
  const zone = await page.locator(".banner-zone").boundingBox();
  const box = await floating.boundingBox();
  expect(box!.y).toBeGreaterThanOrEqual(zone!.y + zone!.height);
});

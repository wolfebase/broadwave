// Cold first paint of Home. A new context with the HTTP cache off, so the
// numbers are the page, not a warm bundle. Budgets are the CI ceilings from
// the task. This lane does not push, so they are not tightened to a CI median
// and the spec stays out of the gating list until three runner runs are green.
import { expect, holdClock, test } from "./fixture";

const fcpBudget = 1500;
const cardBudget = 2500;

const sizes = [
  { name: "1440x900", width: 1440, height: 900 },
  { name: "390x844", width: 390, height: 844 },
] as const;

test.beforeEach(async ({ page }) => {
  const setup = await page.request.put("/api/v1/settings", { data: { setupComplete: "1" } });
  expect(setup.ok()).toBeTruthy();
  await holdClock(page);
  // Stamp the first On now card on the page clock, before Playwright's wait.
  // A card can be inserted with a zero box, so keep looking on animation frames
  // until it has a size. That time is the card, not the poll that noticed it.
  await page.addInitScript(() => {
    const w = window as Window & { __fcp?: number; __homeCard?: number };
    // The paint entry can land a frame after the card is in the DOM. Keep the
    // timestamp from the observer so a fast phone load is not read as missing.
    try {
      new PerformanceObserver((list) => {
        for (const entry of list.getEntries()) {
          if (entry.name === "first-contentful-paint" && w.__fcp == null) w.__fcp = entry.startTime;
        }
      }).observe({ type: "paint", buffered: true });
    } catch {
      // Paint timing is still read from the timeline below.
    }
    const stamp = () => {
      if (w.__homeCard != null) return true;
      const card = document.querySelector(".home button.now-card");
      if (!card) return false;
      const box = card.getBoundingClientRect();
      if (box.width <= 0 || box.height <= 0) return false;
      w.__homeCard = performance.now();
      return true;
    };
    const tick = () => {
      if (!stamp()) requestAnimationFrame(tick);
    };
    if (document.documentElement) tick();
    else document.addEventListener("DOMContentLoaded", tick);
  });
});

for (const size of sizes) {
  test(`home first paint at ${size.name}`, async ({ page }, info) => {
    await page.setViewportSize({ width: size.width, height: size.height });
    const client = await page.context().newCDPSession(page);
    await client.send("Network.setCacheDisabled", { cacheDisabled: true });
    await client.send("Network.clearBrowserCache");
    await page.goto("/", { waitUntil: "commit" });
    await expect(page.locator(".home button.now-card").first()).toBeVisible();
    await expect.poll(() => page.evaluate(() => {
      const w = window as Window & { __fcp?: number; __homeCard?: number };
      const fcp = performance.getEntriesByType("paint").find((entry) => entry.name === "first-contentful-paint");
      return w.__fcp ?? fcp?.startTime ?? null;
    })).not.toBeNull();
    const times = await page.evaluate(() => {
      const w = window as Window & { __fcp?: number; __homeCard?: number };
      const fcp = performance.getEntriesByType("paint").find((entry) => entry.name === "first-contentful-paint");
      return { fcp: w.__fcp ?? fcp?.startTime ?? null, card: w.__homeCard ?? null };
    });
    console.log(`home-paint ${size.name} fcp=${times.fcp == null ? "missing" : Math.round(times.fcp)} card=${times.card == null ? "missing" : Math.round(times.card)}`);
    info.annotations.push(
      { type: "fcp-ms", description: times.fcp == null ? "missing" : String(Math.round(times.fcp)) },
      { type: "card-ms", description: times.card == null ? "missing" : String(Math.round(times.card)) },
    );
    expect(times.fcp, "first-contentful-paint").toBeLessThan(fcpBudget);
    expect(times.card, "first On now card").toBeLessThan(cardBudget);
  });
}

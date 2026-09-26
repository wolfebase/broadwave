import { readFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { test as base, type Page } from "@playwright/test";

const here = path.dirname(fileURLToPath(import.meta.url));

/** Pins the page clock to the seeded listings so a later run sees the same grid. */
export async function holdClock(page: Page) {
  const runtime = JSON.parse(readFileSync(path.join(here, ".run/runtime.json"), "utf8")) as { now: number };
  await page.addInitScript((frozen: number) => {
    Date.now = () => frozen;
  }, runtime.now);
}

export const test = base.extend({
  page: async ({ page, context }, use) => {
    await context.route("**/api/v1/sports/scoreboard**", (route) => route.fulfill({ json: { games: [] } }));
    await context.route("**/api/v1/home**", (route) =>
      route.fulfill({ json: { places: [], tunerAddress: "127.0.0.1:8478", sharing: false } }),
    );
    await context.route("**/api/v1/channels/*/frame**", (route) => route.fulfill({ status: 404, body: "" }));
    await context.route("**/media/art/**", (route) => route.fulfill({ status: 404, body: "" }));
    await use(page);
  },
});

export { expect } from "@playwright/test";

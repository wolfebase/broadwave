// A TV remote's Enter on a later show opens its sheet with Record focused. A
// second or held Enter must not record: the first press asks, and the next
// press after the key was let go records.
import type { Page } from "@playwright/test";
import { expect, holdClock, test } from "./fixture";
import { settle } from "./snap";

type Pass = { id: number; title: string; kind: string };

async function passes(page: Page) {
  return ((await (await page.request.get("/api/v1/passes")).json()) as { passes: Pass[] }).passes.filter((p) => p.title === "Late Local News");
}

async function tvRemote(page: Page) {
  await page.addInitScript(() => {
    const orig = window.matchMedia.bind(window);
    window.matchMedia = (query: string) =>
      String(query).includes("pointer") && String(query).includes("coarse")
        ? ({ matches: true, media: query, onchange: null, addEventListener() {}, removeEventListener() {}, addListener() {}, removeListener() {}, dispatchEvent: () => false } as MediaQueryList)
        : orig(query);
  });
}

function activeCell(page: Page) {
  return page.evaluate(() => {
    const grid = document.querySelector(".guide-canvas");
    const id = grid?.getAttribute("aria-activedescendant");
    return (id ? document.getElementById(id)?.textContent : "") ?? "";
  });
}

async function walkTo(page: Page, key: "ArrowDown" | "ArrowRight", text: string) {
  for (let i = 0; i < 12 && !(await activeCell(page)).includes(text); i++) await page.keyboard.press(key);
  expect(await activeCell(page)).toContain(text);
}

test("Enter on a later show asks before Record and Record series", async ({ page }) => {
  for (const pass of await passes(page)) await page.request.delete(`/api/v1/passes/${pass.id}`);
  expect((await page.request.put("/api/v1/settings", { data: { setupComplete: "1" } })).ok()).toBe(true);
  await tvRemote(page);
  await holdClock(page);
  await page.setViewportSize({ width: 1920, height: 1080 });
  await page.goto("/guide");
  await settle(page);
  await expect(page.locator("html")).toHaveAttribute("data-layout", "tv");

  await page.locator(".guide-canvas").focus();
  await walkTo(page, "ArrowDown", "Bears at Bills");
  await walkTo(page, "ArrowRight", "Late Local News");
  await page.keyboard.press("Enter");
  const sheet = page.getByRole("dialog", { name: "Late Local News" });
  await expect(sheet).toBeVisible();
  const record = sheet.getByRole("button", { name: /^(Record|Press again to record) Late Local News$/ });
  await expect(record).toBeFocused();
  await page.screenshot({ path: test.info().outputPath("sheet-open.png") });

  // A held Enter: the key repeats and is never let go.
  await page.keyboard.down("Enter");
  await page.keyboard.down("Enter");
  await page.keyboard.down("Enter");
  await expect(record).toHaveAccessibleName("Press again to record Late Local News");
  await page.waitForTimeout(500);
  expect(await passes(page)).toHaveLength(0);
  await page.keyboard.up("Enter");
  await page.screenshot({ path: test.info().outputPath("sheet-asks.png") });

  // A deliberate second press records just that airing.
  await page.keyboard.press("Enter");
  await expect.poll(async () => (await passes(page)).map((p) => p.kind)).toEqual(["once"]);

  const series = sheet.getByRole("button", { name: /^(Record series|Press again to record every episode) Late Local News$/ });
  await series.focus();
  await page.keyboard.press("Enter");
  await expect(series).toHaveAccessibleName("Press again to record every episode Late Local News");
  expect((await passes(page)).filter((p) => p.kind !== "once")).toHaveLength(0);
  // Moving away puts the question back.
  await page.keyboard.press("Tab");
  await expect(series).toHaveAccessibleName("Record series Late Local News");
  await series.focus();
  await page.keyboard.press("Enter");
  await page.keyboard.press("Enter");
  await expect.poll(async () => (await passes(page)).filter((p) => p.kind !== "once").length).toBe(1);
  await page.screenshot({ path: test.info().outputPath("sheet-series.png") });

  for (const pass of await passes(page)) await page.request.delete(`/api/v1/passes/${pass.id}`);
});

test("a click on a desktop records at once", async ({ page }) => {
  for (const pass of await passes(page)) await page.request.delete(`/api/v1/passes/${pass.id}`);
  expect((await page.request.put("/api/v1/settings", { data: { setupComplete: "1" } })).ok()).toBe(true);
  await page.goto("/search?q=Late%20Local");
  await settle(page);
  await page.getByRole("button", { name: /^Late Local News/ }).click();
  const sheet = page.getByRole("dialog", { name: "Late Local News" });
  await sheet.getByRole("button", { name: "Record Late Local News", exact: true }).click();
  await expect.poll(async () => (await passes(page)).map((p) => p.kind)).toEqual(["once"]);
  for (const pass of await passes(page)) await page.request.delete(`/api/v1/passes/${pass.id}`);
});

// A 1920×1080 TV browser. Arrows walk rows and lists. Back, including the
// key codes LG and Samsung send, leaves the screen.
import type { Page } from "@playwright/test";
import { expect, test } from "./fixture";
import { settle } from "./snap";

async function tvRemote(page: Page) {
  await page.addInitScript(() => {
    const orig = window.matchMedia.bind(window);
    window.matchMedia = (query: string) =>
      String(query).includes("pointer") && String(query).includes("coarse")
        ? ({ matches: true, media: query, onchange: null, addEventListener() {}, removeEventListener() {}, addListener() {}, removeListener() {}, dispatchEvent: () => false } as MediaQueryList)
        : orig(query);
  });
}

async function setupDone(page: Page) {
  const res = await page.request.put("/api/v1/settings", { data: { setupComplete: "1" } });
  expect(res.ok()).toBeTruthy();
}

function focused(page: Page) {
  return page.evaluate(() => {
    const el = document.activeElement as HTMLElement | null;
    if (!el || el === document.body || el === document.documentElement) return null;
    const box = el.getBoundingClientRect();
    return {
      label: (el.getAttribute("aria-label") || el.textContent || "").replace(/\s+/g, " ").trim().slice(0, 140),
      className: typeof el.className === "string" ? el.className : "",
      x: Math.round(box.left),
      y: Math.round(box.top),
    };
  });
}

async function pressCode(page: Page, keyCode: number) {
  await page.evaluate((code) => {
    // Chrome leaves keyCode at 0 when it is passed to the constructor.
    const event = new KeyboardEvent("keydown", { key: "Unidentified", bubbles: true, cancelable: true });
    Object.defineProperty(event, "keyCode", { get: () => code });
    Object.defineProperty(event, "which", { get: () => code });
    const target = document.activeElement instanceof HTMLElement ? document.activeElement : window;
    target.dispatchEvent(event);
  }, keyCode);
}

test("a TV remote walks every screen and Back leaves", async ({ page }) => {
  test.setTimeout(120_000);
  await tvRemote(page);
  await setupDone(page);
  await page.setViewportSize({ width: 1920, height: 1080 });
  await page.goto("/");
  await settle(page);
  await expect(page.locator("html")).toHaveAttribute("data-layout", "tv");

  const cards = page.locator(".shelf-row .now-card");
  await expect(cards.nth(1)).toBeVisible();
  await cards.first().focus();
  const first = await focused(page);
  expect(first?.className).toContain("now-card");
  await page.keyboard.press("ArrowRight");
  const second = await focused(page);
  expect(second?.className).toContain("now-card");
  expect(second!.x).toBeGreaterThan(first!.x);
  expect(Math.abs(second!.y - first!.y)).toBeLessThan(8);

  await cards.first().focus();
  await pressCode(page, 39);
  const byCode = await focused(page);
  expect(byCode?.className).toContain("now-card");
  expect(byCode!.x).toBeGreaterThan(first!.x);

  // Right at the end of a shelf stays in that shelf: on its heading's action
  // when it has one, never in the next shelf.
  const last = cards.last();
  await last.focus();
  await page.keyboard.press("ArrowRight");
  const stayed = await page.evaluate(() => {
    const all = document.querySelectorAll(".shelf-row .now-card");
    const end = all[all.length - 1];
    const el = document.activeElement;
    return el === end || Boolean(el?.closest(".section-header") && el.closest(".shelf") === end?.closest(".shelf"));
  });
  expect(stayed).toBe(true);

  await page.locator(".topbar").getByRole("link", { name: "Guide", exact: true }).click();
  await expect(page).toHaveURL(/\/guide$/);
  await settle(page);
  const grid = page.getByRole("grid", { name: "TV guide" });
  await grid.focus();
  const startRow = await page.evaluate(() => document.querySelector("[role='grid']")?.getAttribute("aria-activedescendant") ?? "");
  await pressCode(page, 40);
  await expect(grid).toBeFocused();
  await expect.poll(() => page.evaluate(() => document.querySelector("[role='grid']")?.getAttribute("aria-activedescendant") ?? "")).not.toBe(startRow);
  await page.keyboard.press("ArrowDown");
  await page.keyboard.press("ArrowDown");
  const row = await page.evaluate(() => document.querySelector("[role='grid']")?.getAttribute("aria-activedescendant") ?? "");
  expect(row).not.toBe("");
  await pressCode(page, 461);
  await expect(page).toHaveURL(/\/$/);

  await page.locator(".topbar").getByRole("link", { name: "Search", exact: true }).click();
  await expect(page).toHaveURL(/\/search/);
  await settle(page);
  const field = page.getByRole("searchbox", { name: "Search shows, people, and recordings" });
  await field.fill("News");
  await expect(page).toHaveURL(/\/search\?q=News/);
  const result = page.locator(".search-list .search-main").first();
  await expect(result).toBeVisible();
  await field.focus();
  const fieldBox = await focused(page);
  await page.keyboard.press("ArrowDown");
  const landed = await focused(page);
  expect(landed!.y).toBeGreaterThan(fieldBox!.y);
  expect(await page.evaluate(() => document.activeElement?.closest(".search-list li") != null)).toBe(true);
  await result.focus();
  const resultBox = await focused(page);
  await page.keyboard.press("ArrowRight");
  const record = await focused(page);
  // Right from a show reaches that show's own actions: Watch now first when it is on.
  expect(record?.label).toMatch(/^(Watch now|Record series|Record every airing) Evening News/);
  expect(record!.x).toBeGreaterThan(resultBox!.x);
  await page.keyboard.press("ArrowDown");
  const below = await focused(page);
  expect(below!.y).toBeGreaterThan(record!.y);

  await page.locator(".topbar").getByRole("link", { name: "Sports", exact: true }).click();
  await expect(page).toHaveURL(/\/sports$/);
  await settle(page);
  const live = page.getByRole("button", { name: "Live now" });
  await live.focus();
  const liveBox = await focused(page);
  await page.keyboard.press("ArrowRight");
  const today = await focused(page);
  expect(today?.label).toContain("Today");
  expect(today!.x).toBeGreaterThan(liveBox!.x);
  const watch = page.locator(".game-card").getByRole("button", { name: /^Watch / }).first();
  if (await watch.count()) {
    await watch.focus();
    const watchBox = await focused(page);
    await page.keyboard.press("ArrowDown");
    const next = await focused(page);
    expect(next!.label).not.toBe(watchBox!.label);
  }

  await page.locator(".topbar").getByRole("link", { name: "Recordings", exact: true }).click();
  await expect(page).toHaveURL(/\/recordings$/);
  await settle(page);
  // An empty library starts on Open the guide. With recordings, the last filter chip
  // ends its group, and Right reaches Sort beside it.
  const sort = page.getByRole("combobox", { name: "Sort" });
  await expect(page.getByRole("button", { name: "Open the guide" }).or(sort)).toBeVisible();
  if ((await sort.count()) === 0) {
    await expect(page.getByRole("button", { name: "Open the guide" })).toBeFocused();
  } else {
    // The remote starts on the newest recording once the list is in.
    await expect.poll(() => page.evaluate(() => Boolean(document.activeElement?.closest(".media-card")))).toBe(true);
    await page.getByRole("group", { name: "Library filter" }).getByRole("button").last().focus();
    await page.keyboard.press("ArrowRight");
    await expect(sort).toBeFocused();
  }
  await pressCode(page, 10009);
  await expect(page).toHaveURL(/\/sports$/);

  await page.locator(".topbar").getByRole("link", { name: "Schedule", exact: true }).click();
  await expect(page).toHaveURL(/\/schedule$/);
  await settle(page);
  const made = page.getByRole("button", { name: "New pass" });
  await made.focus();
  await page.keyboard.press("Enter");
  const editor = page.getByRole("form", { name: "New pass" });
  await expect(editor).toBeVisible();
  await editor.getByRole("textbox", { name: "Title" }).fill("Evening News");
  await editor.getByRole("button", { name: "Add pass" }).click();
  await expect(editor).toHaveCount(0);
  await page.getByRole("button", { name: "Edit Evening News" }).click();
  const editing = page.getByRole("form", { name: "Edit Evening News" });
  await expect(editing).toBeVisible();
  const title = editing.getByRole("textbox", { name: "Title" });
  await title.focus();
  const titleBox = await focused(page);
  await page.keyboard.press("ArrowDown");
  const underTitle = await focused(page);
  expect(underTitle!.y).toBeGreaterThan(titleBox!.y);
  expect(await page.evaluate(() => document.activeElement?.closest("form")?.getAttribute("aria-label"))).toBe("Edit Evening News");
  await page.keyboard.press("Escape");
  await expect(editing).toHaveCount(0);
  await expect(page.getByRole("button", { name: "New pass" })).toBeVisible();
  await expect(page).toHaveURL(/\/schedule$/);
  await page.keyboard.press("Escape");
  await expect(page).toHaveURL(/\/sports$/);

  await page.getByRole("link", { name: "Settings", exact: true }).click();
  await expect(page).toHaveURL(/\/settings$/);
  await settle(page);
  // Settings is a list of sections. Name this screen is under Devices, Motion under Playback.
  await page.getByRole("link", { name: /^Devices/ }).click();
  await expect(page).toHaveURL(/\/settings\/devices$/);
  // A section starts the remote on its first control.
  const name = page.getByRole("textbox", { name: "Name this screen" });
  await expect(name).toBeFocused();
  const nameBox = await focused(page);
  await page.keyboard.press("ArrowDown");
  const afterName = await focused(page);
  expect(afterName!.y).toBeGreaterThan(nameBox!.y);
  // Back from a section returns to the list. In a text field the first Back only leaves the field.
  await name.focus();
  await page.keyboard.press("Escape");
  await expect(page).toHaveURL(/\/settings\/devices$/);
  await expect(name).not.toBeFocused();
  await page.keyboard.press("Escape");
  await expect(page).toHaveURL(/\/settings$/);
  await page.getByRole("link", { name: /^Playback/ }).click();
  await expect(page).toHaveURL(/\/settings\/playback$/);
  const motion = page.getByRole("radiogroup", { name: "Motion" });
  await expect.poll(() => page.evaluate(() => Boolean(document.activeElement?.closest(".settings-body")))).toBe(true);
  await motion.getByRole("radio", { name: "Broadcast" }).focus();
  await page.keyboard.press("ArrowRight");
  expect((await focused(page))?.label).toBe("Smooth");
  await motion.getByRole("radio", { name: "Film" }).focus();
  await page.keyboard.press("ArrowRight");
  expect((await focused(page))?.label).toBe("Film");
  await page.keyboard.press("Escape");
  await expect(page).not.toHaveURL(/\/settings\/playback$/);

  await page.locator(".topbar").getByRole("link", { name: "Home", exact: true }).click();
  await expect(page).toHaveURL(/\/$/);
  await settle(page);
  await page.getByRole("button", { name: "Watch", exact: true }).click();
  const stage = page.locator("section.stage:not(.mini)");
  await expect(stage).toBeVisible();
  // Focus and the arrow in one turn, so the page cannot move focus first.
  await page.evaluate(() => {
    const el = document.querySelector("section.stage:not(.mini)");
    if (!(el instanceof HTMLElement)) throw new Error("no stage");
    el.focus();
    el.dispatchEvent(new KeyboardEvent("keydown", { key: "Enter", bubbles: true, cancelable: true }));
  });
  await expect.poll(() => page.evaluate(() => document.activeElement instanceof HTMLButtonElement && document.activeElement.closest("section.stage") != null)).toBe(true);
  const back = stage.getByRole("button", { name: "Back to browsing" });
  await back.focus();
  await page.keyboard.press("ArrowDown");
  await expect.poll(() => page.evaluate(() => document.activeElement?.matches("section.stage") === true)).toBe(false);
});

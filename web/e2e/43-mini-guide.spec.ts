// The player's mini-guide opens on the playing channel with the keys in it,
// arrows move it, Enter tunes, and Escape gives the keys back to the player.
import type { Page } from "@playwright/test";
import { expect, test } from "./fixture";
import { settle } from "./snap";

type Channel = { id: number; displayNumber: string };

async function tvRemote(page: Page) {
  await page.addInitScript(() => {
    const orig = window.matchMedia.bind(window);
    window.matchMedia = (query: string) =>
      String(query).includes("pointer") && String(query).includes("coarse")
        ? ({ matches: true, media: query, onchange: null, addEventListener() {}, removeEventListener() {}, addListener() {}, removeListener() {}, dispatchEvent: () => false } as MediaQueryList)
        : orig(query);
  });
}

/** The focused option, and whether all of it shows inside the list. */
function focusedRow(page: Page) {
  return page.evaluate(() => {
    const el = document.activeElement as HTMLElement | null;
    const list = document.querySelector(".mini-guide");
    if (!el || !list || el.getAttribute("role") !== "option") return null;
    const a = el.getBoundingClientRect();
    const b = list.getBoundingClientRect();
    return { number: el.querySelector(".mg-num")?.textContent ?? "", inView: a.top >= b.top - 1 && a.bottom <= b.bottom + 1 };
  });
}

for (const layout of ["desktop", "tv"] as const) {
  test(`the mini-guide opens on the playing channel (${layout})`, async ({ page }) => {
    test.setTimeout(90_000);
    if (layout === "tv") await tvRemote(page);
    const setup = await page.request.put("/api/v1/settings", { data: { setupComplete: "1" } });
    expect(setup.ok()).toBeTruthy();
    const { channels } = (await (await page.request.get("/api/v1/channels?guide=1")).json()) as { channels: Channel[] };
    expect(channels.length).toBeGreaterThan(2);
    const last = channels[channels.length - 1];
    const before = channels[channels.length - 2];
    await page.setViewportSize({ width: 1920, height: 1080 });
    await page.goto(`/watch?channel=${last.id}`);
    await settle(page);
    await expect(page.locator("html")).toHaveAttribute("data-layout", layout);
    const player = page.getByRole("region", { name: "Player" });
    await expect(player).toBeVisible();

    // Move off the playing row, close, and open again: it starts on the playing channel.
    if (layout === "tv") {
      await page.getByRole("button", { name: "Channels" }).focus();
      await page.keyboard.press("Enter");
    } else {
      await page.keyboard.press("g");
    }
    const mini = page.getByRole("listbox", { name: "Channels" });
    await expect(mini).toBeVisible();
    await expect.poll(() => focusedRow(page)).toEqual({ number: last.displayNumber, inView: true });
    await page.keyboard.press("ArrowUp");
    await expect.poll(() => focusedRow(page)).toEqual({ number: before.displayNumber, inView: true });
    await page.keyboard.press("Escape");
    await expect(mini).toHaveCount(0);
    await expect(page).toHaveURL(new RegExp(`channel=${last.id}`));
    const keysInPlayer = await page.evaluate(() => Boolean(document.activeElement?.closest("[aria-label=Player]")));
    expect(keysInPlayer).toBe(true);

    await page.keyboard.press(layout === "tv" ? "Enter" : "g");
    await expect(mini).toBeVisible();
    await expect.poll(() => focusedRow(page)).toEqual({ number: last.displayNumber, inView: true });
    await page.keyboard.press("ArrowUp");
    await page.keyboard.press("Enter");
    await expect(mini).toHaveCount(0);
    await expect(page).toHaveURL(new RegExp(`channel=${before.id}`));
    const keysAfterTune = await page.evaluate(() => Boolean(document.activeElement?.closest("[aria-label=Player]")));
    expect(keysAfterTune).toBe(true);
  });
}

test("the player keeps its keys when focus falls to the page", async ({ page }) => {
  const setup = await page.request.put("/api/v1/settings", { data: { setupComplete: "1" } });
  expect(setup.ok()).toBeTruthy();
  const { channels } = (await (await page.request.get("/api/v1/channels?guide=1")).json()) as { channels: Channel[] };
  await page.setViewportSize({ width: 1440, height: 900 });
  await page.goto(`/watch?channel=${channels[0].id}`);
  await settle(page);
  await expect(page.getByRole("region", { name: "Player" })).toBeVisible();
  await page.evaluate(() => (document.activeElement as HTMLElement | null)?.blur());
  expect(await page.evaluate(() => document.activeElement === document.body)).toBe(true);
  await page.keyboard.press("g");
  await expect(page.getByRole("listbox", { name: "Channels" })).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(page.getByRole("listbox", { name: "Channels" })).toHaveCount(0);
  expect(await page.evaluate(() => Boolean(document.activeElement?.closest("[aria-label=Player]")))).toBe(true);
});

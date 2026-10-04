// The player's help panel closes from its own Close button with Enter, and the
// keys go back to where they were.
import type { Page } from "@playwright/test";
import { copy } from "../src/strings";
import { expect, test } from "./fixture";
import { settle } from "./snap";

type Channel = { id: number };

async function tvRemote(page: Page) {
  await page.addInitScript(() => {
    const orig = window.matchMedia.bind(window);
    window.matchMedia = (query: string) =>
      String(query).includes("pointer") && String(query).includes("coarse")
        ? ({ matches: true, media: query, onchange: null, addEventListener() {}, removeEventListener() {}, addListener() {}, removeListener() {}, dispatchEvent: () => false } as MediaQueryList)
        : orig(query);
  });
}

function marker(page: Page) {
  return page.evaluate(() => {
    const el = document.activeElement as HTMLElement | null;
    return el ? `${el.tagName} ${el.getAttribute("aria-label") ?? ""}` : "";
  });
}

for (const layout of ["desktop", "tv"] as const) {
  test(`Enter on Close closes the help panel (${layout})`, async ({ page }) => {
    if (layout === "tv") await tvRemote(page);
    const setup = await page.request.put("/api/v1/settings", { data: { setupComplete: "1" } });
    expect(setup.ok()).toBeTruthy();
    const { channels } = (await (await page.request.get("/api/v1/channels?guide=1")).json()) as { channels: Channel[] };
    await page.setViewportSize({ width: 1920, height: 1080 });
    await page.goto(`/watch?channel=${channels[0].id}`);
    await settle(page);
    await expect(page.locator("html")).toHaveAttribute("data-layout", layout);
    await expect(page.getByRole("region", { name: "Player" })).toBeVisible();
    if (layout === "tv") await page.getByRole("button", { name: "Channels" }).focus();
    const before = await marker(page);
    expect(before).not.toBe("");

    await page.keyboard.press("?");
    const help = page.getByRole("dialog", { name: copy.player.helpTitle, exact: true });
    await expect(help).toBeVisible();
    const close = help.getByRole("button", { name: copy.player.close });
    await expect(close).toBeFocused();
    // The player's keys stay off while it is open.
    await page.keyboard.press("g");
    await expect(page.getByRole("listbox", { name: "Channels" })).toHaveCount(0);
    await page.keyboard.press("Enter");
    await expect(help).toHaveCount(0);
    await expect.poll(() => marker(page)).toBe(before);

    // Space on Close does the same, and Escape still closes it.
    await page.keyboard.press("?");
    await expect(close).toBeFocused();
    await page.keyboard.press(" ");
    await expect(help).toHaveCount(0);
    await page.keyboard.press("?");
    await expect(help).toBeVisible();
    await page.keyboard.press("Escape");
    await expect(help).toHaveCount(0);
    await expect.poll(() => marker(page)).toBe(before);
  });
}

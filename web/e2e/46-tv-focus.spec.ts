// On a TV the remote keeps a visible ring: back from the player it lands on the
// grid at the channel that played, Up from the mini player reaches the grid, Down
// from a tab enters under it, and a selected chip still shows it is focused.
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

/** Where the keys are, and whether the viewer can see it. */
function focused(page: Page) {
  return page.evaluate(() => {
    const el = document.activeElement as HTMLElement | null;
    if (!el || el === document.body) return { where: "body", ring: false, number: "", label: "", x: 0 };
    const grid = el.getAttribute("role") === "grid";
    const id = grid ? el.getAttribute("aria-activedescendant") : "";
    const cell = id ? document.getElementById(id) : null;
    const box = el.getBoundingClientRect();
    return {
      where: grid ? "grid" : el.matches(".stage.mini") ? "mini player" : el.closest(".topbar") ? "bar" : el.tagName.toLowerCase(),
      ring: el.matches(":focus-visible") && getComputedStyle(el).boxShadow !== "none",
      number: cell?.closest("[role='row']")?.querySelector(".gc-num")?.textContent ?? "",
      label: (el.getAttribute("aria-label") || el.textContent || "").replace(/\s+/g, " ").trim(),
      x: (box.left + box.right) / 2,
    };
  });
}

test("a TV remote keeps a visible ring on the guide and the mini player", async ({ page }) => {
  test.setTimeout(90_000);
  await tvRemote(page);
  const setup = await page.request.put("/api/v1/settings", { data: { setupComplete: "1" } });
  expect(setup.ok()).toBeTruthy();
  const { channels } = (await (await page.request.get("/api/v1/channels?guide=1")).json()) as { channels: Channel[] };
  expect(channels.length).toBeGreaterThan(2);
  await page.setViewportSize({ width: 1920, height: 1080 });
  await page.goto("/guide");
  await settle(page);
  await expect(page.locator("html")).toHaveAttribute("data-layout", "tv");

  // Down two rows and Space plays that channel.
  const grid = page.getByRole("grid", { name: "TV guide" });
  await grid.focus();
  await page.keyboard.press("ArrowDown");
  await page.keyboard.press("ArrowDown");
  const picked = (await focused(page)).number;
  const channel = channels.find((c) => c.displayNumber === picked);
  expect(channel, `row ${picked}`).toBeTruthy();
  await page.keyboard.press(" ");
  await expect(page).toHaveURL(new RegExp(`/watch\\?channel=${channel!.id}`));
  await expect(page.getByRole("region", { name: "Player" })).toBeVisible();

  // Back: the grid has the keys, on the channel that plays in the corner.
  await page.keyboard.press("Escape");
  await expect(page).toHaveURL(/\/guide$/);
  await expect(page.locator(".stage.mini")).toBeVisible();
  await expect.poll(() => focused(page)).toMatchObject({ where: "grid", ring: true, number: picked });

  // Up from the mini player's Pause reaches the grid, not the search field above it.
  await page.locator(".stage.mini button[aria-label='Pause'], .stage.mini button[aria-label='Play']").first().focus();
  await page.keyboard.press("ArrowUp");
  await expect.poll(() => focused(page)).toMatchObject({ where: "grid", ring: true });

  // Down from Recordings enters the page under that tab, not at its far left.
  const tab = page.locator(".topbar").getByRole("tab", { name: "Recordings" });
  await tab.focus();
  const tabBox = await tab.boundingBox();
  await page.keyboard.press("ArrowDown");
  const below = await focused(page);
  expect(below.where).not.toBe("bar");
  expect(below.label).not.toBe("Now");
  expect(Math.abs(below.x - (tabBox!.x + tabBox!.width / 2))).toBeLessThan(300);
  expect(below.ring).toBe(true);

  // The selected chip draws the ring when focused, on top of its own glow.
  const chip = page.locator(".guide-chips .chip.on");
  const rest = await chip.evaluate((el) => getComputedStyle(el).boxShadow);
  await chip.evaluate((el) => (el as HTMLElement).focus({ focusVisible: true } as FocusOptions));
  const ring = await chip.evaluate((el) => getComputedStyle(el).boxShadow);
  expect(ring).not.toBe(rest);
  expect(ring).toContain("61, 123, 255");
});

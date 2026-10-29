import AxeBuilder from "@axe-core/playwright";
import { expect, holdClock, test } from "./fixture";
import { atSize, settle, snap } from "./snap";

const viewports = [
  { name: "390x844", width: 390, height: 844 },
  { name: "1440x900", width: 1440, height: 900 },
] as const;

// The server version, the play state, and the playhead move between runs.
const still = `
  .page-sub, .transport-play svg, .stage-fab { visibility: hidden !important; }
  .scrub { --at: 0% !important; }
  .scrub::-webkit-slider-thumb { opacity: 0 !important; }
`;

async function shot(page: import("@playwright/test").Page, name: string, info: import("@playwright/test").TestInfo) {
  await page.addStyleTag({ content: still });
  await snap(page, name, info);
}

async function finishSetup(page: import("@playwright/test").Page) {
  const res = await page.request.put("/api/v1/settings", { data: { setupComplete: "1" } });
  expect(res.ok()).toBeTruthy();
}

async function expectAccessible(page: import("@playwright/test").Page) {
  const results = await new AxeBuilder({ page }).analyze();
  const hits = results.violations
    .filter((item) => item.impact === "serious" || item.impact === "critical")
    .map((item) => `${item.id}: ${item.nodes.map((node) => node.target.join(" ")).join(", ")}`);
  expect(hits, hits.join("\n")).toEqual([]);
}

test.describe("appearance", () => {
  for (const theme of ["dark", "light"] as const) {
    test(`home, guide, and settings in ${theme}`, async ({ page }, info) => {
      test.setTimeout(120_000);
      // Accent is left unset so a later choice survives reload. The boot script defaults it to blue.
      await page.addInitScript((mode) => {
        localStorage.setItem("broadwave-theme", mode);
      }, theme);
      await page.emulateMedia({ colorScheme: theme });
      await holdClock(page);
      await finishSetup(page);
      await page.goto("/");
      await settle(page);
      await expect(page.locator("html")).toHaveAttribute("data-theme", theme);
      await expect(page.locator("html")).toHaveAttribute("data-accent", "blue");
      await expect(page.getByRole("heading", { name: "NFL: Bears at Bills" })).toBeVisible();
      await page.setViewportSize({ width: 1440, height: 900 });
      await expectAccessible(page);
      for (const size of viewports) {
        await atSize(page, size);
        await shot(page, `home-${theme}-${size.name}`, info);
      }

      await page.getByRole("link", { name: "Guide", exact: true }).click();
      await expect(page.getByText("NFL: Bears at Bills").first()).toBeVisible();
      await page.setViewportSize({ width: 1440, height: 900 });
      await expectAccessible(page);
      for (const size of viewports) {
        await atSize(page, size);
        await shot(page, `guide-${theme}-${size.name}`, info);
      }
      // Phone is the layout that presses Now. A black pill there ignores the accent.
      await atSize(page, viewports[0]);
      const now = page.getByRole("button", { name: "Now", exact: true });
      await expect(now).toHaveAttribute("aria-pressed", "true");
      await expect(now).toHaveCSS("background-color", theme === "light" ? "rgb(29, 78, 216)" : "rgb(245, 247, 250)");
      await expect(now).toHaveCSS("color", theme === "light" ? "rgb(255, 255, 255)" : "rgb(7, 10, 16)");

      // Desktop shows the section list beside Appearance; a phone shows the list alone.
      await page.setViewportSize({ width: 1440, height: 900 });
      await page.getByRole("link", { name: "Settings", exact: true }).click();
      await expect(page.getByRole("heading", { name: "Appearance" })).toBeVisible();
      await expect(page.getByRole("radio", { name: theme === "light" ? "Light" : "Dark", exact: true })).toHaveAttribute("aria-checked", "true");
      await expectAccessible(page);
      for (const size of viewports) {
        await atSize(page, size);
        await shot(page, `settings-${theme}-${size.name}`, info);
      }

      await page.setViewportSize({ width: 1440, height: 900 });
      await page.getByRole("link", { name: "Search", exact: true }).click();
      await page.getByLabel("Search shows, people, and recordings").fill("Bears");
      await page.getByLabel("Search shows, people, and recordings").press("Enter");
      await expect(page.getByRole("heading", { name: "Guide" })).toBeVisible();
      await expect(page.getByText("NFL: Bears at Bills").first()).toBeVisible();
      for (const size of viewports) {
        await atSize(page, size);
        await shot(page, `search-${theme}-${size.name}`, info);
      }

      await page.setViewportSize({ width: 1440, height: 900 });
      await page.getByRole("link", { name: "Sports", exact: true }).click();
      await expect(page.getByRole("heading", { name: "Sports" })).toBeVisible();
      await expect(page.getByRole("article").filter({ hasText: "Chicago" })).toBeVisible();
      for (const size of viewports) {
        await atSize(page, size);
        await shot(page, `sports-${theme}-${size.name}`, info);
      }

      await page.setViewportSize({ width: 1440, height: 900 });
      // Earlier specs leave recordings and passes behind, so these two pages get a scan, not a picture.
      await page.getByRole("link", { name: "Recordings", exact: true }).click();
      await expect(page.getByRole("heading", { level: 1, name: "Recordings" })).toBeVisible();
      await expectAccessible(page);

      await page.getByRole("link", { name: "Schedule", exact: true }).click();
      await expect(page.getByRole("heading", { level: 1, name: "Schedule" })).toBeVisible();
      await expectAccessible(page);

      await page.setViewportSize({ width: 1440, height: 900 });
      await page.getByRole("link", { name: "For You", exact: true }).click();
      await expect(page.getByRole("heading", { name: "NFL: Bears at Bills" })).toBeVisible();
      await page.getByRole("button", { name: "Watch", exact: true }).first().click();
      await expect(page).toHaveURL(/\/watch\?channel=/);
      const stage = page.locator(".stage");
      await expect(stage.locator("h2")).toBeVisible();
      const chrome = await stage.evaluate((el) => {
        const css = getComputedStyle(el);
        return {
          text: css.getPropertyValue("--color-text").trim().toLowerCase(),
          glass: css.getPropertyValue("--color-glass-fill").trim().toLowerCase(),
          accent: css.getPropertyValue("--color-accent").trim().toLowerCase(),
        };
      });
      expect(chrome).toEqual({ text: "#f5f7fa", glass: "#1a2130b8", accent: "#3d7bff" });
      await expect(stage.locator("h2")).toHaveCSS("color", "rgb(245, 247, 250)");
      await expect(stage.locator(".glass-icon").first()).toHaveCSS("color", "rgb(245, 247, 250)");
      for (const size of viewports) {
        await atSize(page, size);
        await shot(page, `player-${theme}-${size.name}`, info);
      }
      // Phone chrome has no Stop control. Leave the full player; the mini player would cover the next shot.
      await page.keyboard.press("Escape");
      await expect(page).not.toHaveURL(/\/watch/);
      await page.getByRole("button", { name: "Stop watching" }).click();
      await expect(stage).toHaveCount(0);

      if (theme !== "light") return;
      await page.setViewportSize({ width: 1440, height: 900 });
      await page.getByRole("link", { name: "Settings", exact: true }).click();
      await expect(page.getByRole("heading", { name: "Appearance" })).toBeVisible();
      await page.getByRole("radio", { name: "Teal" }).check();
      await expect(page.locator("html")).toHaveAttribute("data-accent", "teal");
      await expect(page.getByRole("radio", { name: "Teal" })).toBeChecked();
      await shot(page, "settings-teal-light-1440x900", info);
      await page.reload();
      await settle(page);
      await expect(page.locator("html")).toHaveAttribute("data-theme", "light");
      await expect(page.locator("html")).toHaveAttribute("data-accent", "teal");
    });
  }

  test("dark until a choice, and System follows the device", async ({ page }) => {
    await page.emulateMedia({ colorScheme: "light" });
    await holdClock(page);
    await finishSetup(page);
    await page.goto("/");
    await settle(page);
    await expect(page.locator("html")).toHaveAttribute("data-theme", "dark");

    await page.getByRole("link", { name: "Settings", exact: true }).click();
    await expect(page.getByRole("radio", { name: "Dark", exact: true })).toHaveAttribute("aria-checked", "true");
    await page.getByRole("radio", { name: "System", exact: true }).click();
    await expect(page.locator("html")).toHaveAttribute("data-theme", "light");
    await page.emulateMedia({ colorScheme: "dark" });
    await expect(page.locator("html")).toHaveAttribute("data-theme", "dark");
    await page.emulateMedia({ colorScheme: "light" });
    await page.reload();
    await settle(page);
    await expect(page.locator("html")).toHaveAttribute("data-theme", "light");
  });
});

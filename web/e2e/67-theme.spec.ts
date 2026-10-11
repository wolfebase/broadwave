import AxeBuilder from "@axe-core/playwright";
import { expect, holdClock, test } from "./fixture";
import { atSize, settle, snap } from "./snap";

const viewports = [
  { name: "390x844", width: 390, height: 844 },
  { name: "1440x900", width: 1440, height: 900 },
] as const;

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
        await snap(page, `home-${theme}-${size.name}`, info);
      }

      await page.getByRole("tab", { name: "Guide" }).click();
      await expect(page.getByText("NFL: Bears at Bills").first()).toBeVisible();
      await page.setViewportSize({ width: 1440, height: 900 });
      await expectAccessible(page);
      for (const size of viewports) {
        await atSize(page, size);
        await snap(page, `guide-${theme}-${size.name}`, info);
      }

      await page.getByRole("button", { name: "Settings", exact: true }).click();
      await expect(page.getByRole("heading", { name: "Appearance" })).toBeVisible();
      await expect(page.getByRole("button", { name: theme === "light" ? "Light" : "Dark" })).toHaveAttribute("aria-pressed", "true");
      await page.setViewportSize({ width: 1440, height: 900 });
      await expectAccessible(page);
      for (const size of viewports) {
        await atSize(page, size);
        await snap(page, `settings-${theme}-${size.name}`, info);
      }

      if (theme !== "light") return;
      await page.setViewportSize({ width: 1440, height: 900 });
      await page.getByRole("radio", { name: "Teal" }).check();
      await expect(page.locator("html")).toHaveAttribute("data-accent", "teal");
      await expect(page.getByRole("radio", { name: "Teal" })).toBeChecked();
      await snap(page, "settings-teal-light-1440x900", info);
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

    await page.getByRole("button", { name: "Settings", exact: true }).click();
    await expect(page.getByRole("button", { name: "Dark" })).toHaveAttribute("aria-pressed", "true");
    await page.getByRole("button", { name: "System" }).click();
    await expect(page.locator("html")).toHaveAttribute("data-theme", "light");
    await page.emulateMedia({ colorScheme: "dark" });
    await expect(page.locator("html")).toHaveAttribute("data-theme", "dark");
    await page.emulateMedia({ colorScheme: "light" });
    await page.reload();
    await settle(page);
    await expect(page.locator("html")).toHaveAttribute("data-theme", "light");
  });
});

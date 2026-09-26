import { expect, holdClock, test } from "./fixture";
import { atSize, settle, sizes, snap } from "./snap";

test("setup wizard finds the fake tuner", async ({ page }, info) => {
  await holdClock(page);
  for (const size of sizes) {
    await page.setViewportSize({ width: size.width, height: size.height });
    await page.goto("/");
    await settle(page);
    await expect(page.getByRole("heading", { name: "Let's set up your TV" })).toBeVisible();
    await expect(page.locator('[data-setup="finish"]')).toBeVisible({ timeout: 15_000 });
    await page.getByRole("button", { name: "Sources" }).click();
    await expect(page.locator('[data-setup="sources"]')).toBeVisible();
    await expect(page.getByRole("heading", { name: "Your tuner" })).toBeVisible();
    await expect(page.getByText("Fake HDHomeRun")).toBeVisible();
    await atSize(page, size);
    await snap(page, `setup-sources-${size.name}`, info);
  }

  await page.getByRole("button", { name: "Continue" }).click();
  await expect(page.getByRole("button", { name: "Watch" })).toBeVisible({ timeout: 45_000 });
  for (const size of sizes) {
    await atSize(page, size);
    await snap(page, `setup-ready-${size.name}`, info);
  }
  await page.getByRole("button", { name: "Watch" }).click();
  await expect(page).toHaveURL(/\/watch\?channel=/);
  await expect(page.getByRole("region", { name: "Player" })).toBeVisible();
});

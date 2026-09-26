import { expect, holdClock, test } from "./fixture";
import { atSize, settle, sizes, snap } from "./snap";

test("home, guide, search, and sports", async ({ page }, info) => {
  await holdClock(page);
  await page.goto("/");
  await settle(page);
  await expect(page.getByRole("heading", { name: "NFL: Chiefs at Bills" })).toBeVisible();
  for (const size of sizes) {
    await atSize(page, size);
    await snap(page, `home-${size.name}`, info);
  }

  await page.getByRole("tab", { name: "Guide" }).click();
  await expect(page.getByText("NFL: Chiefs at Bills").first()).toBeVisible();
  for (const size of sizes) {
    await atSize(page, size);
    await snap(page, `guide-${size.name}`, info);
  }
  await page.setViewportSize({ width: 1440, height: 900 });
  await expect(page.locator("html")).toHaveAttribute("data-layout", "desktop");
  await page.getByRole("group", { name: "Filter" }).getByRole("button", { name: /^Sports/ }).click();
  await expect(page.getByRole("gridcell", { name: /Chiefs at Bills/ }).first()).toBeVisible();
  await expect(page.getByRole("gridcell", { name: /Evening News/ })).toHaveCount(0);
  await page.getByRole("group", { name: "Filter" }).getByRole("button", { name: "All" }).click();
  await page.getByRole("gridcell", { name: /NFL: Chiefs at Bills/ }).click();
  const sheet = page.getByRole("dialog", { name: /Chiefs at Bills/ });
  await expect(sheet).toBeVisible();
  await sheet.getByRole("button", { name: "Watch", exact: true }).click();
  await expect(page).toHaveURL(/\/watch\?channel=/);
  await page.getByRole("button", { name: "Back to browsing" }).click();

  await page.getByRole("tab", { name: "Search" }).click();
  await page.getByLabel("Search shows, people, and recordings").fill("Chief");
  await page.getByLabel("Search shows, people, and recordings").press("Enter");
  await expect(page.getByRole("heading", { name: "Guide" })).toBeVisible();
  await expect(page.getByText("NFL: Chiefs at Bills").first()).toBeVisible();
  for (const size of sizes) {
    await atSize(page, size);
    await snap(page, `search-${size.name}`, info);
  }

  await page.getByRole("tab", { name: "Sports" }).click();
  await expect(page.getByRole("article").filter({ hasText: "Kansas City" })).toBeVisible();
  await page.getByRole("button", { name: "Coming up" }).click();
  await expect(page.getByRole("article").filter({ hasText: "Boston" })).toBeVisible();
  await page.getByRole("button", { name: "Today" }).click();
  await expect(page.getByRole("article").filter({ hasText: "Kansas City" })).toBeVisible();
  await page.getByRole("button", { name: "Live now" }).click();
  await expect(page.getByRole("article").filter({ hasText: "Kansas City" })).toBeVisible();
  await expect(page.getByRole("article").filter({ hasText: "Boston" })).toHaveCount(0);
  for (const size of sizes) {
    await atSize(page, size);
    await snap(page, `sports-${size.name}`, info);
  }
});

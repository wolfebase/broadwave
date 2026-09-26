import { expect, holdClock, test } from "./fixture";
import { atSize, settle, sizes, snap } from "./snap";

test("multiview adds a channel and swaps the one with sound", async ({ page }, info) => {
  await holdClock(page);
  await page.goto("/");
  await settle(page);
  await page.getByRole("button", { name: "Watch", exact: true }).click();
  await expect(page.getByRole("region", { name: "Player" })).toBeVisible();
  await page.getByRole("region", { name: "Player" }).press("m");

  await expect(page).toHaveURL(/\/multiview/);
  await expect(page.getByRole("listbox", { name: "Add a channel" })).toBeVisible();
  await page.getByRole("option", { name: /5\.1\s*KCTV/ }).click();
  await expect(page.getByRole("group", { name: "5.1 KCTV, sound on" })).toBeVisible();
  await expect(page.getByRole("group", { name: "4.1 WDAF", exact: true })).toBeVisible();

  await page.getByRole("group", { name: "4.1 WDAF", exact: true }).click();
  await expect(page.getByRole("group", { name: "4.1 WDAF, sound on" })).toBeVisible();
  await expect(page.getByRole("group", { name: "5.1 KCTV", exact: true })).toBeVisible();

  await page.getByRole("button", { name: "Quad", exact: true }).click();
  await expect(page.getByRole("region", { name: "Quad" })).toBeVisible();
  for (const size of sizes) {
    await atSize(page, size);
    await snap(page, `multiview-${size.name}`, info);
  }
});

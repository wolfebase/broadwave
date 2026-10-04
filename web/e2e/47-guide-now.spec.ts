// The floating Now shows only when the now line has scrolled out of the grid.
import { expect, test } from "./fixture";
import { settle } from "./snap";

for (const size of [{ width: 1440, height: 900 }, { width: 1920, height: 1080 }]) {
  test(`the floating Now waits until now is off the grid (${size.width})`, async ({ page }) => {
    const setup = await page.request.put("/api/v1/settings", { data: { setupComplete: "1" } });
    expect(setup.ok()).toBeTruthy();
    await page.setViewportSize(size);
    await page.goto("/guide");
    await settle(page);
    const line = page.locator(".now-line");
    await expect(line).toBeVisible();
    const float = page.locator(".guide-now-float");
    await expect(float).toHaveCount(0);

    await page.locator(".guide-scroll").evaluate((el) => el.scrollBy({ left: 3000, behavior: "instant" }));
    await expect(float).toBeVisible();
    await float.click();
    await expect(float).toHaveCount(0);
    const seen = await page.evaluate(() => {
      const l = document.querySelector(".now-line")!.getBoundingClientRect();
      const s = document.querySelector(".guide-scroll")!.getBoundingClientRect();
      return l.left > s.left && l.left < s.right;
    });
    expect(seen).toBe(true);
  });
}

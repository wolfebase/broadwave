import { expect, test } from "./fixture";
import { settle } from "./snap";

type Pass = { id: number; title: string; kind: string; channelId?: number; airingStart?: string };

async function passes(page: import("@playwright/test").Page) {
  return ((await (await page.request.get("/api/v1/passes")).json()) as { passes: Pass[] }).passes;
}

test("search lists what is on now first, as you type", async ({ page }) => {
  expect((await page.request.put("/api/v1/settings", { data: { setupComplete: "1" } })).ok()).toBe(true);
  await page.goto("/search");
  await settle(page);
  const field = page.getByLabel("Search shows, people, and recordings");
  await field.pressSequentially("news", { delay: 40 });
  const guide = page.locator("section", { has: page.getByRole("heading", { name: "Guide" }) });
  const titles = guide.locator(".search-list > li");
  await expect(titles.first()).toContainText("Evening News");
  await expect(titles.filter({ hasText: "Late Local News" })).toHaveCount(1);
  await expect(titles.filter({ hasText: "Morning News" })).toHaveCount(0);
  const order = await titles.allInnerTexts();
  expect(order.findIndex((text) => text.includes("Evening News"))).toBeLessThan(order.findIndex((text) => text.includes("Late Local News")));
  await expect(page).toHaveURL(/\/search\?q=news$/);
  await page.goBack();
  await expect(page).not.toHaveURL(/\/search/);
});

test("a search result opens, and records just that airing", async ({ page }) => {
  // Run alone, the fresh catalog would open on setup.
  expect((await page.request.put("/api/v1/settings", { data: { setupComplete: "1" } })).ok()).toBe(true);
  await page.goto("/search");
  await settle(page);
  const field = page.getByLabel("Search shows, people, and recordings");
  await expect(field).toBeFocused();
  await field.fill("Late Local");
  await field.press("Enter");

  const row = page.getByRole("button", { name: /Late Local News/ });
  await expect(row).toContainText(/(Today|Tomorrow) · /);
  await row.click();
  const sheet = page.getByRole("dialog", { name: "Late Local News" });
  await expect(sheet).toBeVisible();
  await expect(sheet.getByRole("button", { name: "Watch", exact: true })).toHaveCount(0);

  await sheet.getByRole("button", { name: "Record Late Local News", exact: true }).click();
  await expect(sheet.getByRole("button", { name: "Don't record" })).toBeVisible();
  const once = (await passes(page)).filter((pass) => pass.kind === "once");
  expect(once).toHaveLength(1);
  expect(once[0].title).toBe("Late Local News");
  expect(once[0].airingStart).toBeTruthy();
  const schedule = (await (await page.request.get("/api/v1/schedule")).json()) as { items: { airing: { title: string } }[] };
  expect(schedule.items.map((item) => item.airing.title)).toContain("Late Local News");

  await page.keyboard.press("Escape");
  await page.getByRole("tab", { name: "Schedule" }).click();
  await expect(page.getByText(/^Late Local News · .+ only$/)).toBeVisible();
  // One airing takes pads, not series rules.
  await page.getByRole("listitem").filter({ hasText: /Late Local News · .+ only/ }).getByRole("button", { name: "Edit Late Local News" }).click();
  const editor = page.getByRole("form", { name: "Edit Late Local News" });
  await expect(editor.getByLabel("After", { exact: true })).toBeVisible();
  await expect(editor.getByRole("combobox", { name: "Episodes" })).toHaveCount(0);
  await page.goto("/search?q=Late%20Local");
  await row.click();
  await sheet.getByRole("button", { name: "Don't record" }).click();
  await expect(sheet.getByRole("button", { name: "Record Late Local News", exact: true })).toBeVisible();
  expect((await passes(page)).filter((pass) => pass.kind === "once")).toHaveLength(0);

  // Under a series pass, removing the one airing would change nothing.
  await sheet.getByRole("button", { name: "Record Late Local News", exact: true }).click();
  await sheet.getByRole("button", { name: "Record series" }).click();
  await expect(sheet.getByRole("button", { name: "Series is recording" })).toBeDisabled();
  await expect(sheet.getByRole("button", { name: "Don't record" })).toHaveCount(0);
  await expect(sheet.getByRole("button", { name: "Record Late Local News", exact: true })).toHaveCount(0);
  for (const pass of await passes(page)) {
    if (pass.title === "Late Local News") expect((await page.request.delete(`/api/v1/passes/${pass.id}`)).ok()).toBe(true);
  }

  await page.keyboard.press("Escape");
  await expect(row).toBeFocused();
  await field.fill("Bears");
  await field.press("Enter");
  await page.getByRole("button", { name: /NFL: Bears at Bills/ }).first().click();
  const live = page.getByRole("dialog", { name: "NFL: Bears at Bills" });
  await live.getByRole("button", { name: "Watch", exact: true }).click();
  await expect(page).toHaveURL(/\/watch\?channel=/);
});

// Naming this screen updates the open socket, so other screens see it
// without a reload.
import { mkdirSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { expect, test } from "./fixture";
import { settle } from "./snap";

type Screen = { id: string; name: string };

const shotDir = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "../../.evidence/lane/l118");

test.beforeEach(async ({ page }) => {
  const setup = await page.request.put("/api/v1/settings", { data: { setupComplete: "1" } });
  expect(setup.ok()).toBeTruthy();
});

test("renaming this screen shows up for the others", async ({ page }) => {
  mkdirSync(shotDir, { recursive: true });
  await page.setViewportSize({ width: 1440, height: 900 });
  await page.goto("/settings");
  await settle(page);
  const field = page.getByRole("textbox", { name: "Name this screen" });
  await expect(field).toBeVisible();
  await field.scrollIntoViewIfNeeded();
  await page.screenshot({ path: path.join(shotDir, "web-1440.jpg"), type: "jpeg", quality: 70 });

  await page.setViewportSize({ width: 390, height: 844 });
  await field.scrollIntoViewIfNeeded();
  await page.screenshot({ path: path.join(shotDir, "web-390.jpg"), type: "jpeg", quality: 70 });
  await page.setViewportSize({ width: 1440, height: 900 });

  const id = await page.evaluate(() => localStorage.getItem("broadwave-screen-id") ?? "");
  expect(id).toMatch(/^[A-Za-z0-9-]{1,64}$/);
  await expect.poll(async () => (await screens(page)).some((s) => s.id === id)).toBe(true);

  await field.fill("Den TV");
  await field.press("Enter");
  await expect.poll(async () => (await screens(page)).find((s) => s.id === id)?.name).toBe("Den TV");
  // Same page, same socket: the list changed without a reload.
  expect(page.url()).toContain("/settings");

  await field.fill("");
  await field.press("Enter");
  const back = await field.inputValue();
  expect(back.length).toBeGreaterThan(0);
  expect(back).not.toBe("Den TV");
  await expect.poll(async () => (await screens(page)).find((s) => s.id === id)?.name).toBe(back);
});

async function screens(page: import("@playwright/test").Page) {
  return ((await (await page.request.get("/api/v1/screens")).json()) as { screens: Screen[] }).screens;
}

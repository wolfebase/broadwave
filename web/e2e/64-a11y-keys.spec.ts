// Escape closes the layer that is open, then returns focus, and stays on the page.
import { spawnSync } from "node:child_process";
import { mkdirSync, readFileSync, writeFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { expect, test } from "./fixture";
import { settle } from "./snap";

const here = path.dirname(fileURLToPath(import.meta.url));

function harness() {
  return JSON.parse(readFileSync(path.join(here, ".run/server.json"), "utf8")) as { db: string; config: string };
}

function sql(statement: string) {
  const result = spawnSync("sqlite3", [harness().db, `PRAGMA busy_timeout=5000; ${statement}`], { encoding: "utf8" });
  if (result.status !== 0) throw new Error(result.stderr || result.stdout || statement);
}

test("Escape closes a delete confirm, a pass editor, and a multiview menu", async ({ page }) => {
  expect((await page.request.put("/api/v1/settings", { data: { setupComplete: "1" } })).ok()).toBeTruthy();
  await page.setViewportSize({ width: 1440, height: 900 });

  await page.goto("/schedule");
  await settle(page);
  await page.getByRole("button", { name: "New pass" }).click();
  const draft = page.getByRole("form", { name: "New pass" });
  await draft.getByRole("textbox", { name: "Title", exact: true }).focus();
  await page.keyboard.press("Escape");
  await expect(draft).toHaveCount(0);
  await expect(page.getByRole("button", { name: "New pass" })).toBeFocused();
  await expect(page).toHaveURL(/\/schedule/);

  const started = new Date(Date.now() - 86_400_000).toISOString().replace(/\.\d+Z$/, "Z");
  const file = path.join(harness().config, "work", "recordings", "layer-keys.ts");
  mkdirSync(path.dirname(file), { recursive: true });
  writeFileSync(file, Buffer.alloc(188));
  sql(
    `INSERT INTO recordings (channel_id, guide_number, title, subtitle, path, status, started_at, duration_sec)
     VALUES (1, '4.1', 'Layer Keys', 'The Return', '${file}', 'complete', '${started}', 4);`,
  );
  await page.goto("/recordings");
  await settle(page);
  const row = page.locator(".media-card").filter({ hasText: "The Return" });
  const remove = row.getByRole("button", { name: "Delete Layer Keys, The Return", exact: true });
  await remove.click();
  await expect(row.getByRole("button", { name: "Delete this file Layer Keys, The Return", exact: true })).toBeFocused();
  await page.keyboard.press("Escape");
  await expect(remove).toBeFocused();
  await expect(page).toHaveURL(/\/recordings/);

  await row.getByRole("button", { name: "Rename file Layer Keys, The Return", exact: true }).click();
  const name = row.getByLabel("File name in the recordings folder");
  await expect(name).toBeFocused();
  await page.keyboard.press("Backspace");
  await expect(name).toBeFocused();
  await page.keyboard.press("Escape");
  await expect(row.getByRole("button", { name: "Rename file Layer Keys, The Return", exact: true })).toBeFocused();
  await expect(page).toHaveURL(/\/recordings/);

  const runtime = JSON.parse(readFileSync(path.join(here, ".run/runtime.json"), "utf8")) as { channels: { id: number; name: string }[] };
  const kbwv = runtime.channels.find((item) => item.name === "KBWV");
  const kbwv2 = runtime.channels.find((item) => item.name === "KBWV2");
  expect(kbwv && kbwv2).toBeTruthy();
  await page.goto(`/multiview?ch=${kbwv!.id},${kbwv2!.id}&layout=2up&focus=${kbwv!.id}`);
  await settle(page);
  const tile = page.locator(`.mv-tile[data-channel="${kbwv!.id}"]`);
  await tile.focus();
  await page.keyboard.press("o");
  const menu = tile.getByRole("menu", { name: "4.1 KBWV" });
  await expect(menu.getByRole("menuitem", { name: "Make big 4.1 KBWV", exact: true })).toBeFocused();
  await page.keyboard.press("ArrowDown");
  await expect(menu.getByRole("menuitem", { name: /^Record .+ on 4\.1 KBWV$/ })).toBeFocused();
  await page.keyboard.press("Escape");
  await expect(tile).toBeFocused();
  await expect(menu).toHaveCount(0);
  await expect(page).toHaveURL(/\/multiview/);
  const grid = page.locator(".mv");
  await grid.evaluate((el: HTMLElement) => el.focus({ focusVisible: true } as FocusOptions));
  await expect.poll(() =>
    grid.evaluate((el) => {
      const style = getComputedStyle(el);
      return el.matches(":focus-visible") && style.outlineStyle === "solid" && Number.parseFloat(style.outlineWidth) >= 4 && Number.parseFloat(style.outlineOffset) < 0;
    }),
  ).toBe(true);
});

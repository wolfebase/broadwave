// The guide grid and the player's mini-guide list the same channels in the same
// order: the server's channel-number order, not the order channels were found in.
import { readFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { expect, test } from "./fixture";
import { settle } from "./snap";

const here = path.dirname(fileURLToPath(import.meta.url));

type Channel = { id: number; displayNumber: string };

function harness() {
  return JSON.parse(readFileSync(path.join(here, ".run/server.json"), "utf8")) as { base: string };
}

async function patch(id: number, customNumber: string) {
  const res = await fetch(`${harness().base}/api/v1/channels/${id}`, {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ customNumber }),
  });
  expect(res.ok, await res.clone().text()).toBe(true);
}

test("the guide and the mini-guide list channels in number order", async ({ page }) => {
  test.setTimeout(90_000);
  const { base } = harness();
  const listed = (await (await fetch(`${base}/api/v1/channels?guide=1`)).json()) as { channels: Channel[] };
  // The last channel found gets the lowest number, so an order by id puts it last.
  const late = listed.channels.reduce((a, b) => (b.id > a.id ? b : a));
  await patch(late.id, "1.1");
  try {
    const setup = await page.request.put("/api/v1/settings", { data: { setupComplete: "1" } });
    expect(setup.ok()).toBeTruthy();
    await page.setViewportSize({ width: 1920, height: 1080 });
    await page.goto("/guide");
    await settle(page);
    await page.evaluate(() => localStorage.removeItem("broadwave-guide-order"));
    await page.reload();
    await settle(page);
    const grid = page.locator(".guide-row .gc-num");
    await expect(grid.first()).toHaveText("1.1");
    const rows = await grid.allTextContents();

    await page.goto(`/watch?channel=${late.id}`);
    await expect(page.getByRole("region", { name: "Player" })).toBeVisible();
    await page.keyboard.press("g");
    const mini = page.getByRole("listbox", { name: "Channels" });
    await expect(mini).toBeVisible();
    const listedInPlayer = await mini.locator(".mg-num").allTextContents();
    expect(rows).toEqual(listedInPlayer);
  } finally {
    await patch(late.id, "");
  }
});

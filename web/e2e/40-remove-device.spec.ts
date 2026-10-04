// A tuner or playlist that is gone for good can be removed from Settings.
import { mkdirSync, readFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { expect, test } from "./fixture";
import { settle } from "./snap";

const here = path.dirname(fileURLToPath(import.meta.url));
const evidence = path.resolve(here, "../../.evidence/j082");

type Device = { deviceId: string; friendlyName?: string };

function harness() {
  return JSON.parse(readFileSync(path.join(here, ".run/server.json"), "utf8")) as { base: string };
}

async function devices(): Promise<Device[]> {
  const res = await fetch(`${harness().base}/api/v1/devices`);
  return ((await res.json()) as { devices?: Device[] }).devices ?? [];
}

test("Remove takes a playlist and its channels off the server", async ({ page }) => {
  const { base } = harness();
  const added = await fetch(`${base}/api/v1/sources`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ kind: "link", name: "Old Box", url: "http://127.0.0.1:9/old.ts" }),
  });
  expect(added.ok, await added.clone().text()).toBe(true);
  const before = await devices();
  const old = before.find((device) => device.friendlyName === "Old Box");
  expect(old, JSON.stringify(before)).toBeTruthy();

  await page.goto("/settings#sources");
  await settle(page);
  const remove = page.getByRole("button", { name: "Remove Old Box" });
  await expect(remove).toBeVisible();
  mkdirSync(evidence, { recursive: true });
  for (const [name, width, height] of [["phone", 390, 844], ["desktop", 1440, 900], ["tv", 1920, 1080]] as const) {
    await page.setViewportSize({ width, height });
    await remove.scrollIntoViewIfNeeded();
    await page.screenshot({ path: path.join(evidence, `remove-${name}.png`) });
  }
  await remove.focus();
  await expect(remove).toBeFocused();
  page.once("dialog", (dialog) => {
    expect(dialog.message()).toContain("Recordings stay.");
    void dialog.accept();
  });
  await page.keyboard.press("Enter");
  await expect(remove).toHaveCount(0);

  const after = await devices();
  expect(after.map((device) => device.deviceId)).not.toContain(old?.deviceId);
  expect(after.length).toBe(before.length - 1);
  const channels = (await (await fetch(`${base}/api/v1/channels`)).json()) as { channels?: { deviceId?: string }[] };
  expect((channels.channels ?? []).some((channel) => channel.deviceId === old?.deviceId)).toBe(false);
});

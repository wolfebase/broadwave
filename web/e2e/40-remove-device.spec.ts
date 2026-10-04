// A tuner or playlist that is gone for good can be removed from Settings.
import { spawnSync } from "node:child_process";
import { mkdirSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { copy } from "../src/strings";
import { lastSeenPhrase } from "../src/time";
import { expect, test } from "./fixture";
import { settle } from "./snap";

const here = path.dirname(fileURLToPath(import.meta.url));
const evidence = path.resolve(here, "../../.evidence/j082");
const offlineShots = path.resolve(here, "../../.evidence/lane/l85");
const playlistShots = path.resolve(here, "../../.evidence/l87");

type Device = { deviceId: string; friendlyName?: string; lastSeen?: string };

function harness() {
  return JSON.parse(readFileSync(path.join(here, ".run/server.json"), "utf8")) as { base: string; db: string; config: string };
}

function sql(statement: string) {
  const result = spawnSync("sqlite3", [harness().db, `PRAGMA busy_timeout=5000; ${statement}`], { encoding: "utf8" });
  if (result.status !== 0) throw new Error(result.stderr || result.stdout || statement);
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
  const setup = await fetch(`${base}/api/v1/settings`, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ setupComplete: "1" }),
  });
  expect(setup.ok).toBe(true);

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

test("an offline playlist says so on its card", async ({ page }) => {
  const { base } = harness();
  const added = await fetch(`${base}/api/v1/sources`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ kind: "link", name: "Quiet Box", url: "http://127.0.0.1:9/quiet.ts" }),
  });
  expect(added.ok, await added.clone().text()).toBe(true);
  const box = (await devices()).find((device) => device.friendlyName === "Quiet Box");
  expect(box).toBeTruthy();
  const seen = new Date(Date.now() - 5 * 60_000).toISOString().replace(/\.\d{3}Z$/, "Z");
  sql(`UPDATE sources SET health='unreachable' WHERE device_id='${box?.deviceId}';`);
  sql(`UPDATE devices SET last_seen='${seen}' WHERE device_id='${box?.deviceId}';`);
  const setup = await fetch(`${base}/api/v1/settings`, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ setupComplete: "1" }),
  });
  expect(setup.ok).toBe(true);
  try {
    await page.goto("/settings#sources");
    await settle(page);
    const card = page.locator(".device-card", { has: page.getByRole("heading", { name: "Quiet Box" }) });
    const line = copy.sources.offline(lastSeenPhrase(seen));
    await expect(card).toBeVisible();
    await expect(card.getByText(line)).toBeVisible();
    await expect(card.getByRole("button").first()).toHaveAccessibleName("Remove Quiet Box");
    await expect(page.locator(".source-row", { hasText: "Quiet Box" }).filter({ hasText: "Offline" })).toHaveCount(0);
    const tuner = page.locator(".device-card", { has: page.getByRole("heading", { name: "Fake HDHomeRun" }) });
    await expect(tuner.getByText(/Offline/)).toHaveCount(0);
    mkdirSync(offlineShots, { recursive: true });
    for (const [name, width, height] of [["phone", 390, 844], ["desktop", 1440, 900], ["tv", 1920, 1080]] as const) {
      await page.setViewportSize({ width, height });
      await card.scrollIntoViewIfNeeded();
      await page.screenshot({ path: path.join(offlineShots, `${name}.jpg`), type: "jpeg", quality: 70, animations: "disabled" });
    }
  } finally {
    await fetch(`${base}/api/v1/devices/${encodeURIComponent(box?.deviceId ?? "")}`, { method: "DELETE" });
  }
});

test("a playlist card skips scan and firmware", async ({ page }) => {
  const { base, config } = harness();
  // The config folder is the one path a server in a container shares with the test.
  const file = path.join(config, "night-list.m3u");
  writeFileSync(file, "#EXTM3U\n#EXTINF:-1,Night Owl\nhttp://127.0.0.1:9/night.ts\n");
  const added = await fetch(`${base}/api/v1/sources`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ kind: "m3u", name: "Night List", url: file }),
  });
  expect(added.ok, await added.clone().text()).toBe(true);
  const list = (await devices()).find((device) => device.friendlyName === "Night List");
  expect(list).toBeTruthy();
  const setup = await fetch(`${base}/api/v1/settings`, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ setupComplete: "1" }),
  });
  expect(setup.ok).toBe(true);
  try {
    await page.goto("/settings#sources");
    await settle(page);
    const card = page.locator(".device-card", { has: page.getByRole("heading", { name: "Night List" }) });
    await expect(card).toBeVisible();
    await expect(card.getByRole("button", { name: "Scan channels" })).toHaveCount(0);
    await expect(card.getByText("Firmware")).toHaveCount(0);
    await expect(card.getByText("0 tuners")).toHaveCount(0);
    await expect(card.getByRole("button", { name: "Remove Night List" })).toBeVisible();
    const tuner = page.locator(".device-card", { has: page.getByRole("heading", { name: "Fake HDHomeRun" }) });
    await expect(tuner.getByRole("button", { name: "Scan channels" })).toBeVisible();
    await expect(tuner.getByText("Firmware 20260101")).toBeVisible();
    await page.setViewportSize({ width: 1440, height: 900 });
    const grid = page.locator(".device-grid");
    await grid.scrollIntoViewIfNeeded();
    mkdirSync(playlistShots, { recursive: true });
    await page.screenshot({ path: path.join(playlistShots, "cards.jpg"), type: "jpeg", quality: 70, animations: "disabled" });
  } finally {
    await fetch(`${base}/api/v1/devices/${encodeURIComponent(list?.deviceId ?? "")}`, { method: "DELETE" });
    rmSync(file, { force: true });
  }
});

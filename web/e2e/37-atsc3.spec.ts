// Opt-in: E2E_ATSC3=1 npm run e2e. An encrypted 3.0 station plays and records its clear 1.0 twin.
import { readFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import type { Page } from "@playwright/test";
import { expect, holdClock, test } from "./fixture";
import { settle } from "./snap";
import { copy } from "../src/strings";

const here = path.dirname(fileURLToPath(import.meta.url));
const note = copy.player.encrypted;

type Channel = { id: number; number: string; name: string };
type Recording = { id: number; title: string; status: string; guideNumber: string; channelId: number };
type Tuner = { guide?: string; ours?: boolean };

function lineup(): Channel[] {
  const runtime = JSON.parse(readFileSync(path.join(here, ".run/runtime.json"), "utf8")) as { channels: Channel[] };
  return runtime.channels;
}

async function ready(page: Page) {
  expect((await page.request.put("/api/v1/settings", { data: { setupComplete: "1", watermarkGB: "0" } })).ok()).toBe(true);
  await holdClock(page);
  await page.goto("/");
  await settle(page);
  await expect(page.getByRole("heading", { name: "Let's set up your TV" })).toHaveCount(0);
}

async function playingTwin(page: Page, clearId: number, encryptedId: number) {
  await expect(page).toHaveURL(new RegExp(`/watch\\?channel=${clearId}&from=${encryptedId}`));
  await expect(page.locator(".player-note")).toHaveText(note);
  await expect(page.locator(".eyebrow-num")).toHaveText("5.1");
  const video = page.locator("video.stage-video");
  await expect.poll(async () => video.evaluate((node) => (node as HTMLVideoElement).currentTime), { timeout: 30_000 }).toBeGreaterThan(0.2);
  const at = await video.evaluate((node) => (node as HTMLVideoElement).currentTime);
  await expect.poll(async () => video.evaluate((node) => (node as HTMLVideoElement).currentTime), { timeout: 8_000 }).toBeGreaterThan(at);
}

test("an encrypted 3.0 station plays and records its clear twin", async ({ page }) => {
  test.setTimeout(120_000);
  const channels = lineup();
  const encrypted = channels.find((channel) => channel.number === "115.1");
  const clear = channels.find((channel) => channel.number === "5.1");
  if (!encrypted || !clear) throw new Error("115.1 and 5.1 must both be in the lineup");

  await ready(page);
  // The encrypted row stays off the guide. Search still finds its shows.
  await page.getByRole("tab", { name: "Guide" }).click();
  await expect(page.getByRole("gridcell", { name: /Night Show/ }).first()).toBeVisible();
  await expect(page.getByRole("gridcell", { name: /Sealed Signal/ })).toHaveCount(0);

  await page.getByRole("tab", { name: "Search" }).click();
  await page.getByLabel("Search shows, people, and recordings").fill("Sealed");
  await page.getByLabel("Search shows, people, and recordings").press("Enter");
  const found = page.locator("section").filter({ has: page.getByRole("heading", { name: "Guide" }) });
  await found.getByRole("button", { name: /Sealed Signal/ }).click();
  const sheet = page.getByRole("dialog", { name: /Sealed Signal/ });
  await expect(sheet).toBeVisible();
  await sheet.getByRole("button", { name: "Record Sealed Signal", exact: true }).click();

  await expect
    .poll(async () => {
      const body = (await (await page.request.get("/api/v1/recordings")).json()) as { recordings?: Recording[] };
      return (body.recordings ?? []).find((row) => row.title === "Sealed Signal");
    }, { timeout: 20_000 })
    .toMatchObject({ status: "recording", guideNumber: "5.1", channelId: encrypted.id });

  const tuners = (await (await page.request.get("/api/v1/tuners")).json()) as { tuners?: Tuner[] };
  const numbers = (tuners.tuners ?? []).map((tuner) => tuner.guide).filter((guide): guide is string => Boolean(guide));
  expect(numbers, JSON.stringify(tuners.tuners)).toContain("5.1");
  expect(numbers).not.toContain("115.1");

  await sheet.getByRole("button", { name: "Watch", exact: true }).click();
  await playingTwin(page, clear.id, encrypted.id);

  // A link to the encrypted channel (voice, a bookmark) plays the twin too.
  await page.getByRole("button", { name: "Back to browsing" }).click();
  await page.goto(`/watch?channel=${encrypted.id}`);
  await playingTwin(page, clear.id, encrypted.id);

  const listed = (await (await page.request.get("/api/v1/recordings")).json()) as { recordings?: Recording[] };
  const id = listed.recordings?.find((row) => row.title === "Sealed Signal")?.id;
  if (id) await page.request.post(`/api/v1/recordings/${id}/stop`, { data: {} });
});

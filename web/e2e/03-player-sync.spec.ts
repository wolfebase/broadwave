import { mkdirSync, writeFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import type { Page } from "@playwright/test";
import { expect, test } from "./fixture";
import { atSize, settle, sizes, snap } from "./snap";

const here = path.dirname(fileURLToPath(import.meta.url));

type Play = { url: string; body: string; width: number; offset: string; drift: string };

async function readPlay(page: Page): Promise<Play> {
  let last: Play = { url: "", body: "", width: 0, offset: "", drift: "" };
  await expect
    .poll(async () => {
      last = await page.locator("video.stage-video").evaluate(async (video) => {
        const el = video as HTMLVideoElement & { hls?: { url?: string } };
        const url = el.hls?.url || "";
        let body = "";
        if (url) {
          const res = await fetch(url);
          body = res.ok ? await res.text() : "";
        }
        return { url, body, width: el.videoWidth || 0, offset: el.dataset.syncOffset ?? "", drift: el.dataset.syncDrift ?? "" };
      });
      return last.body.includes("#EXT-X-PROGRAM-DATE-TIME");
    }, { timeout: 30_000 })
    .toBe(true);
  return last;
}

function programTimes(body: string) {
  return [...body.matchAll(/#EXT-X-PROGRAM-DATE-TIME:([^\s]+)/g)].map((match) => match[1]);
}

test("player shell and two screens on one timeline", async ({ page }, info) => {
  await page.goto("/");
  await settle(page);
  await page.getByRole("button", { name: "Watch", exact: true }).click();
  await expect(page.getByRole("region", { name: "Player" })).toBeVisible();
  await expect(page.getByRole("heading", { name: /Chiefs at Bills|WDAF/ })).toBeVisible();
  const opened = await readPlay(page);
  expect(opened.url).toContain("/media/live/");

  const other = await page.context().newPage();
  await other.goto(page.url());
  await settle(other);
  await expect(page.getByRole("button", { name: "Whole-Home Sync" })).toContainText("2 screens", { timeout: 25_000 });
  await expect(other.getByRole("button", { name: "Whole-Home Sync" })).toContainText("2 screens");

  const left = await readPlay(page);
  const right = await readPlay(other);
  let leftOffset = left.offset;
  let rightOffset = right.offset;
  let leftDrift = left.drift;
  let rightDrift = right.drift;
  let delta = Number.POSITIVE_INFINITY;
  const offsetDeadline = Date.now() + 20_000;
  while (Date.now() < offsetDeadline) {
    leftOffset = (await page.locator("video.stage-video").getAttribute("data-sync-offset")) ?? "";
    rightOffset = (await other.locator("video.stage-video").getAttribute("data-sync-offset")) ?? "";
    leftDrift = (await page.locator("video.stage-video").getAttribute("data-sync-drift")) ?? "";
    rightDrift = (await other.locator("video.stage-video").getAttribute("data-sync-drift")) ?? "";
    if (leftOffset !== "" && rightOffset !== "" && leftDrift !== "" && rightDrift !== "") {
      delta = Math.abs(Number(leftOffset) - Number(rightOffset));
      const settled = Math.abs(Number(leftDrift)) <= 40 && Math.abs(Number(rightDrift)) <= 40;
      if (settled && delta < 50) break;
    }
    await new Promise((resolve) => setTimeout(resolve, 250));
  }

  const times = new Set(programTimes(left.body));
  const sharedClock = programTimes(right.body).some((stamp) => times.has(stamp));
  const widths = [
    await page.locator("video.stage-video").evaluate((video: HTMLVideoElement) => video.videoWidth),
    await other.locator("video.stage-video").evaluate((video: HTMLVideoElement) => video.videoWidth),
  ];
  const decoded = widths[0] > 0 && widths[1] > 0;
  mkdirSync(path.join(here, ".run"), { recursive: true });
  writeFileSync(
    path.join(here, ".run/picture.json"),
    JSON.stringify(
      {
        decoded,
        videoWidth: widths,
        playlist: left.url,
        sharedClock,
        offsetDeltaMs: Number.isFinite(delta) ? Math.round(delta) : null,
        offsets: [leftOffset, rightOffset],
        drifts: [leftDrift, rightDrift],
      },
      null,
      2,
    ),
  );
  expect(sharedClock, "both playlists carry the same program date-time").toBe(true);
  expect(delta, `offsets ${leftOffset} and ${rightOffset}, drifts ${leftDrift} and ${rightDrift}`).toBeLessThan(50);
  expect(decoded, `video widths ${widths.join(", ")}`).toBe(true);
  for (const size of sizes) {
    await atSize(page, size);
    await snap(page, `player-${size.name}`, info);
  }
  await other.close();
});

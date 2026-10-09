import { mkdirSync, readFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import type { Page } from "@playwright/test";
import { expect, test } from "./fixture";
import { settle } from "./snap";

const here = path.dirname(fileURLToPath(import.meta.url));
const evidence = path.resolve(here, "../../.evidence/lane/l39");

type Channel = { id: number; number: string; name: string };

type Head = {
  time: number;
  history: number;
  width: number;
  paused: boolean;
  drift: string;
  url: string;
};

function lineup(): Channel[] {
  const runtime = JSON.parse(readFileSync(path.join(here, ".run/runtime.json"), "utf8")) as { channels: Channel[] };
  const num = (value: string) => value.split(".").map((part) => Number(part) || 0);
  return [...runtime.channels].sort((a, b) => {
    const x = num(a.number);
    const y = num(b.number);
    return (x[0] ?? 0) - (y[0] ?? 0) || (x[1] ?? 0) - (y[1] ?? 0) || a.name.localeCompare(b.name);
  });
}

function readHead(): Head {
  const video = document.querySelector("video.stage-video") as (HTMLVideoElement & { hls?: { url?: string } }) | null;
  if (!video) return { time: 0, history: 0, width: 0, paused: true, drift: "", url: "" };
  const start = video.seekable.length ? video.seekable.start(0) : video.currentTime;
  return {
    time: video.currentTime,
    history: video.currentTime - start,
    width: video.videoWidth,
    paused: video.paused,
    drift: video.dataset.syncDrift ?? "",
    url: video.hls?.url ?? "",
  };
}

async function head(page: Page): Promise<Head> {
  return page.evaluate(readHead);
}

async function playing(page: Page, id: number) {
  await expect
    .poll(async () => {
      const snap = await head(page);
      const on = snap.url.includes(`/media/live/${id}/`) && snap.width > 0 && !snap.paused;
      return on ? "ok" : JSON.stringify(snap);
    }, { timeout: 30_000 })
    .toBe("ok");
}

test("channel changes, go to live, sports, and search", async ({ page }) => {
  test.setTimeout(120_000);
  const errors: string[] = [];
  page.on("pageerror", (err) => errors.push(err.message));
  page.on("console", (msg) => {
    if (msg.type() !== "error") return;
    const where = msg.location().url;
    const text = msg.text();
    // The harness answers art and preview frames with 404, and a recording left
    // by an earlier spec has no poster. Those are not app errors.
    if (/\/media\/(?:art|poster)\/|\/channels\/\d+\/frame|favicon/.test(`${where} ${text}`)) return;
    errors.push(where ? `${text} (${where})` : text);
  });

  const channels = lineup();
  expect(channels.length).toBeGreaterThan(2);
  const start = channels[0];
  const next = channels[1];
  const bears = channels.find((channel) => channel.name === "KBWV");
  expect(bears, "seeded NFL channel").toBeTruthy();

  const setup = await page.request.put("/api/v1/settings", { data: { setupComplete: "1" } });
  expect(setup.ok()).toBeTruthy();
  await page.setViewportSize({ width: 1440, height: 900 });
  await page.goto(`/watch?channel=${start.id}`);
  await settle(page);
  const player = page.getByRole("region", { name: "Player" });
  await expect(player).toBeVisible();
  await expect(page.locator("html")).toHaveAttribute("data-layout", "desktop");
  await playing(page, start.id);

  await player.press("ArrowDown");
  await expect.poll(() => new URL(page.url()).searchParams.get("channel")).toBe(String(next.id));
  await playing(page, next.id);

  await player.press("ArrowUp");
  await expect.poll(() => new URL(page.url()).searchParams.get("channel")).toBe(String(start.id));
  await playing(page, start.id);

  await expect.poll(async () => (await head(page)).history, { timeout: 50_000 }).toBeGreaterThan(32);
  const before = await head(page);
  await player.press("ArrowLeft");
  await player.press("ArrowLeft");
  await expect.poll(async () => (await head(page)).time, { timeout: 5_000 }).toBeLessThan(before.time - 24);
  const rewound = (await head(page)).time;
  // The picture keeps moving from the earlier spot. A snap back to live fails this.
  await expect
    .poll(async () => {
      const snap = await head(page);
      const held = snap.time > rewound + 0.15 && snap.time < before.time - 20 && !snap.paused;
      return held ? "ok" : JSON.stringify(snap);
    }, { timeout: 5_000 })
    .toBe("ok");

  await page.mouse.move(720, 400);
  const goLive = player.getByRole("button", { name: "Go to live" });
  await expect(goLive).toBeVisible();
  await goLive.click();
  await expect
    .poll(async () => {
      const snap = await head(page);
      const drift = Number(snap.drift);
      const back =
        snap.width > 0 &&
        !snap.paused &&
        snap.drift !== "" &&
        Number.isFinite(drift) &&
        Math.abs(drift) <= 1000 &&
        snap.time > rewound + 8;
      return back ? "ok" : JSON.stringify(snap);
    }, { timeout: 15_000 })
    .toBe("ok");
  await expect(player.getByRole("button", { name: "Live", exact: true })).toBeVisible();
  const settled = await head(page);
  console.log(`go live drift ${settled.drift} ms, playhead ${settled.time.toFixed(2)} s`);

  mkdirSync(evidence, { recursive: true });
  await page.screenshot({ path: path.join(evidence, "live.jpg"), type: "jpeg", quality: 70, animations: "disabled" });

  await player.press("Escape");
  await expect(page).toHaveURL(/\/guide/);
  await page.getByRole("tab", { name: "Sports" }).click();
  await expect(page).toHaveURL(/\/sports/);
  const game = page.getByRole("article").filter({ hasText: "NFL" }).filter({ hasText: "Chicago" });
  await expect(game).toBeVisible();
  await page.screenshot({ path: path.join(evidence, "sports.jpg"), type: "jpeg", quality: 70, animations: "disabled" });
  await game.getByRole("button", { name: /^Watch / }).click();
  await expect.poll(() => new URL(page.url()).searchParams.get("channel")).toBe(String(bears!.id));
  await playing(page, bears!.id);
  await page.screenshot({ path: path.join(evidence, "sports-watch.jpg"), type: "jpeg", quality: 70, animations: "disabled" });

  await player.press("Escape");
  await page.getByRole("tab", { name: "Search" }).click();
  await page.getByLabel("Search shows, people, and recordings").fill("Bears");
  await page.getByLabel("Search shows, people, and recordings").press("Enter");
  await expect(page).toHaveURL(/\/search\?q=Bears/);
  await expect(page.getByRole("heading", { name: "Guide" })).toBeVisible();
  await expect(page.locator("button.search-main", { hasText: "NFL: Bears at Bills" })).toBeVisible();
  await page.screenshot({ path: path.join(evidence, "search.jpg"), type: "jpeg", quality: 70, animations: "disabled" });

  expect(errors, errors.join("\n")).toEqual([]);
});

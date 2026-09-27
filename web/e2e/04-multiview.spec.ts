import { mkdirSync, readFileSync, writeFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import type { Page } from "@playwright/test";
import { expect, holdClock, test } from "./fixture";
import { atSize, settle, sizes, snap } from "./snap";

const here = path.dirname(fileURLToPath(import.meta.url));
const evidence = path.resolve(here, "../../.evidence/lane/l27");

type Channel = { id: number; number: string; name: string };
type Harness = { base: string; control: string };
type Tuner = { index: number; guide?: string; viewers?: number; ours?: boolean };
type Tile = { channel: string; muted: boolean; moving: boolean; alert: string };

function harness(): Harness {
  return JSON.parse(readFileSync(path.join(here, ".run/server.json"), "utf8")) as Harness;
}

function channels(): Channel[] {
  const runtime = JSON.parse(readFileSync(path.join(here, ".run/runtime.json"), "utf8")) as { channels: Channel[] };
  return runtime.channels;
}

function channel(name: string) {
  const found = channels().find((item) => item.name === name);
  if (!found) throw new Error(`no ${name}`);
  return found;
}

async function post(url: string) {
  const res = await fetch(url, { method: "POST", signal: AbortSignal.timeout(40_000) });
  if (!res.ok && res.status !== 204) throw new Error(`${url} ${res.status} ${await res.text()}`);
}

async function openMultiview(page: Page, ids: number[], layout: "2up" | "quad", focus: number) {
  const target = `/multiview?ch=${ids.join(",")}&layout=${layout}&focus=${focus}`;
  const region = layout === "quad" ? "Quad" : "Side by side";
  await page.goto(target);
  await settle(page);
  const setup = page.getByRole("heading", { name: "Let's set up your TV" });
  const grid = page.getByRole("region", { name: region });
  await expect(setup.or(grid)).toBeVisible();
  if (await setup.isVisible()) {
    const cont = page.getByRole("button", { name: "Continue" });
    if (await cont.isVisible()) await cont.click();
    // Watch saves setup, then leaves. A new load before that save comes back here.
    await page.getByRole("button", { name: "Watch", exact: true }).click();
    await expect(setup).toBeHidden();
    await page.goto(target);
    await settle(page);
  }
  await expect(grid).toBeVisible();
}

async function tiles(page: Page): Promise<Tile[]> {
  return page.locator("video.mv-video").evaluateAll(async (videos: HTMLVideoElement[]) => {
    const from = videos.map((video) => video.currentTime);
    await new Promise((resolve) => setTimeout(resolve, 700));
    return videos.map((video, index) => {
      const alert = video.closest(".mv-tile")?.querySelector("[role='alert']")?.textContent?.replace(/\s+/g, " ").trim() || "";
      return {
        channel: video.dataset.channel || "",
        muted: video.muted,
        moving: video.videoWidth > 0 && !video.paused && video.currentTime > from[index] + 0.15,
        alert,
      };
    });
  });
}

async function ourViewers(base: string): Promise<Tuner[]> {
  const body = (await (await fetch(`${base}/api/v1/tuners`)).json()) as { tuners?: Tuner[] };
  return (body.tuners ?? [])
    .filter((tuner) => tuner.ours)
    .map((tuner) => ({ index: tuner.index, guide: tuner.guide ?? "", viewers: tuner.viewers ?? 0 }))
    .sort((a, b) => a.index - b.index);
}

function viewerKey(rows: Tuner[]) {
  return rows
    .map((row) => row.viewers ?? 0)
    .sort((a, b) => a - b)
    .join(",");
}

test.afterEach(async ({ page }) => {
  // The next test counts viewers. Closing the page does not always deliver
  // pagehide first, so the watch stays until the server drops it.
  await page
    .evaluate(() => {
      window.dispatchEvent(new PageTransitionEvent("pagehide", { persisted: false }));
    })
    .catch(() => undefined);
});

test("multiview adds a channel and swaps the one with sound", async ({ page }, info) => {
  await holdClock(page);
  await page.goto("/");
  await settle(page);
  await page.getByRole("button", { name: "Watch", exact: true }).click();
  await expect(page.getByRole("region", { name: "Player" })).toBeVisible();
  await page.getByRole("region", { name: "Player" }).press("m");

  await expect(page).toHaveURL(/\/multiview/);
  await expect(page.getByRole("listbox", { name: "Add a channel" })).toBeVisible();
  await page.getByRole("option", { name: /5\.1\s*KCTV/ }).click();
  await expect(page.getByRole("group", { name: "5.1 KCTV, sound on" })).toBeVisible();
  await expect(page.getByRole("group", { name: "4.1 WDAF", exact: true })).toBeVisible();

  await page.getByRole("group", { name: "4.1 WDAF", exact: true }).click();
  await expect(page.getByRole("group", { name: "4.1 WDAF, sound on" })).toBeVisible();
  await expect(page.getByRole("group", { name: "5.1 KCTV", exact: true })).toBeVisible();

  await page.getByRole("button", { name: "Quad", exact: true }).click();
  await expect(page.getByRole("region", { name: "Quad" })).toBeVisible();
  for (const size of sizes) {
    await atSize(page, size);
    await snap(page, `multiview-${size.name}`, info);
  }
});

test("moving the sound between equal tiles restarts neither", async ({ page }) => {
  await page.goto("/watch?channel=1");
  await settle(page);
  const setup = page.getByRole("heading", { name: "Let's set up your TV" });
  const player = page.getByRole("region", { name: "Player" });
  await expect(setup.or(player)).toBeVisible();
  if (await setup.isVisible()) {
    const cont = page.getByRole("button", { name: "Continue" });
    if (await cont.isVisible()) await cont.click();
    await page.getByRole("button", { name: "Watch", exact: true }).click();
    await expect(player).toBeVisible();
  }
  await page.goto("/multiview?ch=1,3&layout=2up&focus=1");
  const playing = () =>
    page.locator("video.mv-video").evaluateAll((videos: HTMLVideoElement[]) =>
      videos.map((video) => ({ channel: video.dataset.channel, muted: video.muted, moving: video.videoWidth > 0 && !video.paused && video.currentTime > 0.5 })),
    );
  await expect.poll(async () => (await playing()).filter((tile) => tile.moving).length, { timeout: 45_000 }).toBe(2);

  const watches: number[] = [];
  page.on("request", (req) => {
    if (req.method() === "POST" && req.url().endsWith("/api/v1/watch")) watches.push((JSON.parse(req.postData() || "{}") as { channelId: number }).channelId);
  });
  await page.locator("video.mv-video").evaluateAll((videos: HTMLVideoElement[]) => {
    const w = window as unknown as { __emptied: number };
    w.__emptied = 0;
    for (const video of videos) video.addEventListener("emptied", () => w.__emptied++);
  });

  for (const name of ["5.1 KCTV", "4.1 WDAF", "5.1 KCTV"]) {
    await page.getByRole("group", { name, exact: true }).click();
    await expect(page.getByRole("group", { name: `${name}, sound on` })).toBeVisible();
    await page.waitForTimeout(1500);
  }
  expect(watches, "a sound swap started a new watch").toEqual([]);
  expect(await page.evaluate(() => (window as unknown as { __emptied: number }).__emptied), "a sound swap emptied a tile").toBe(0);
  const tiles = await playing();
  expect(tiles.filter((tile) => tile.moving)).toHaveLength(2);
  expect(tiles.find((tile) => tile.channel === "3")?.muted).toBe(false);
  expect(tiles.find((tile) => tile.channel === "1")?.muted).toBe(true);
});

// The quad asks for its small pictures while the side-by-side encodes are
// still being let go. A tile the budget turned away then has to ask again.
test("side by side to quad plays every picture the server has room for", async ({ page }) => {
  test.skip(process.env.E2E_QUAD !== "1", "Set E2E_QUAD=1 for four channels and four pictures.");
  test.setTimeout(150_000);
  const { base } = harness();
  const wdaf = channel("WDAF");
  const kctv = channel("KCTV");
  const wdaf2 = channel("WDAF2");
  const kctv2 = channel("KCTV2");
  await expect
    .poll(async () => (await ourViewers(base)).length === 0, { timeout: 60_000, intervals: [500], message: "the previous watch let the tuners go" })
    .toBe(true);
  await openMultiview(page, [wdaf.id, kctv.id], "2up", wdaf.id);
  await expect.poll(async () => (await tiles(page)).filter((tile) => tile.moving).length, { timeout: 50_000, intervals: [800] }).toBe(2);
  const asked: string[] = [];
  page.on("response", async (res) => {
    const url = new URL(res.url());
    if (res.request().method() !== "POST" || !url.pathname.startsWith("/api/v1/watch")) return;
    const sent = res.request().postData() ?? "";
    const got = await res.text().catch(() => "");
    asked.push(`${url.pathname} ${res.status()} ${sent.slice(0, 80)} -> ${got.slice(0, 160)}`);
  });
  // A slow network: the old layout's stops land after the new tiles ask.
  await page.route("**/api/v1/watch/*/stop", async (route) => {
    await new Promise((resolve) => setTimeout(resolve, 3000));
    await route.continue();
  });
  await page.evaluate((to) => {
    window.history.pushState({ depth: 1 }, "", to);
    window.dispatchEvent(new PopStateEvent("popstate"));
  }, `/multiview?ch=${wdaf.id},${wdaf2.id},${kctv.id},${kctv2.id}&layout=quad&focus=${wdaf.id}`);
  await expect(page.getByRole("region", { name: "Quad" })).toBeVisible();
  let last: Tile[] = [];
  let saw = 0;
  await expect
    .poll(
      async () => {
        last = await tiles(page);
        const moving = last.filter((tile) => tile.moving).length;
        const budget = last.map((tile) => /can play (\d+) picture/.exec(tile.alert)?.[1]).find(Boolean);
        const room = Math.min(last.length, budget ? Number(budget) : last.length);
        saw = moving >= room ? saw + 1 : 0;
        return saw >= 3;
      },
      { timeout: 60_000, intervals: [1000], message: "every picture the budget allows plays" },
    )
    .toBe(true);
  mkdirSync(evidence, { recursive: true });
  writeFileSync(path.join(evidence, "side-to-quad.json"), JSON.stringify({ tiles: last, asked }, null, 2));
  await page.screenshot({ path: path.join(evidence, "side-to-quad.jpg"), animations: "disabled" });
});

test.describe("a restarted server", () => {
  test.setTimeout(180_000);

  for (const layout of ["2up", "quad"] as const) {
    test(`${layout === "2up" ? "side by side" : "quad"} plays again with no click`, async ({ page }) => {
      const { base, control } = harness();
      const wdaf = channel("WDAF");
      const second = channel("KCTV");
      const third = channel("WDAF2");
      const ids = layout === "2up" ? [wdaf.id, second.id] : [wdaf.id, third.id, second.id];
      const name = layout === "2up" ? "side" : "quad";
      try {
        await expect
          .poll(async () => (await ourViewers(base)).length === 0, { timeout: 60_000, intervals: [500], message: "the previous watch let the tuners go" })
          .toBe(true);
        await openMultiview(page, ids, layout, wdaf.id);
        await page.evaluate(() => {
          document.documentElement.dataset.lane = "stay";
        });
        const need = layout === "2up" ? ids.length : 2;
        let playing: Tile[] = [];
        let stable = "";
        let saw = 0;
        await expect
          .poll(
            async () => {
              const moving = (await tiles(page)).filter((tile) => tile.moving);
              const key = moving
                .map((tile) => tile.channel)
                .sort()
                .join(",");
              if (moving.length >= need && key === stable) {
                saw += 1;
                playing = moving;
                return saw >= 2;
              }
              stable = key;
              saw = 0;
              playing = moving;
              return false;
            },
            { timeout: 50_000, intervals: [800] },
          )
          .toBe(true);
        if (layout === "2up") expect(playing.map((tile) => tile.channel).sort()).toEqual(ids.map(String).sort());
        const had = playing.map((tile) => tile.channel);
        const sound = playing.find((tile) => !tile.muted);
        expect(sound, "one tile has the sound").toBeTruthy();
        if (had.includes(String(wdaf.id))) expect(sound?.channel).toBe(String(wdaf.id));
        const focusBefore = new URL(page.url()).searchParams.get("focus");
        expect(focusBefore).toBe(sound?.channel);
        let before: Tuner[] = [];
        await expect
          .poll(
            async () => {
              const rows = await ourViewers(base);
              const sum = rows.reduce((total, row) => total + (row.viewers ?? 0), 0);
              const same = before.length > 0 && viewerKey(rows) === viewerKey(before) && rows.length === before.length;
              before = rows;
              return same && sum >= had.length;
            },
            { timeout: 15_000, intervals: [400], message: "viewers settle on the tiles that are playing" },
          )
          .toBe(true);

        let watched = 0;
        page.on("request", (req) => {
          if (req.method() === "POST" && new URL(req.url()).pathname === "/api/v1/watch") watched += 1;
        });
        const restarted = Date.now();
        await post(`${control}/start`);
        mkdirSync(evidence, { recursive: true });
        const alert = page.locator(".mv-tile [role='alert']").first();
        if (await alert.isVisible().catch(() => false)) {
          await page.screenshot({ path: path.join(evidence, `${name}-restart.jpg`), animations: "disabled" });
        }

        // The buffer keeps moving after the process dies, and the message can
        // land a moment later. A tile is back once a new watch is playing.
        let recovered: Tile[] = [];
        await expect
          .poll(async () => {
            recovered = await tiles(page);
            const back = had.filter((id) => recovered.some((tile) => tile.channel === id && tile.moving && tile.alert === ""));
            const heard = recovered.find((tile) => tile.channel === sound?.channel);
            return watched >= had.length && back.length === had.length && heard?.muted === false && heard?.alert === "";
          }, { timeout: 20_000, intervals: [500], message: "every tile that had a picture is moving, and the sound tile kept it" })
          .toBe(true);
        const pictureMs = Date.now() - restarted;
        await expect(page.locator("html")).toHaveAttribute("data-lane", "stay");
        expect(new URL(page.url()).searchParams.get("focus"), "the sound did not move").toBe(focusBefore);

        let after: Tuner[] = [];
        const viewersLeft = Math.max(1_000, 30_000 - (Date.now() - restarted));
        await expect
          .poll(async () => {
            after = await ourViewers(base);
            return viewerKey(after) === viewerKey(before) && after.length === before.length;
          }, { timeout: viewersLeft, intervals: [500], message: "the same viewers are back" })
          .toBe(true);
        await page.screenshot({ path: path.join(evidence, `${name}-back.jpg`), animations: "disabled" });
        const summaryPath = path.join(evidence, "summary.json");
        const prior = (() => {
          try {
            return JSON.parse(readFileSync(summaryPath, "utf8")) as Record<string, unknown>;
          } catch {
            return {};
          }
        })();
        writeFileSync(
          summaryPath,
          JSON.stringify(
            {
              ...prior,
              [name]: {
                had,
                sound: sound?.channel,
                pictureMs,
                watches: watched,
                before,
                after,
                alerts: recovered.filter((tile) => tile.alert).map((tile) => ({ channel: tile.channel, alert: tile.alert })),
              },
            },
            null,
            2,
          ),
        );
      } finally {
        await post(`${control}/start`).catch(() => undefined);
      }
    });
  }
});

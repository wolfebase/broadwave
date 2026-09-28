// Opt-in: E2E_README=1 npm run e2e. Public README shots from the test pattern.
// E2E_SOURCE must stay unset so the picture is not a broadcast recording.
import { spawnSync } from "node:child_process";
import { mkdirSync, readFileSync, statSync, writeFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import type { Page } from "@playwright/test";
import { expect, test } from "@playwright/test";
import { holdClock } from "./fixture";

const here = path.dirname(fileURLToPath(import.meta.url));
const shots = path.resolve(here, "../../docs/screenshots/readme");
const evidence = path.resolve(here, "../../.evidence/lane/l45");
const limit = 300 * 1024;

type Channel = { id: number; number: string; name: string };
type Harness = { base: string; db: string };

const sizes = [
  { name: "1440", width: 1440, height: 900, layout: "desktop" },
  { name: "390", width: 390, height: 844, layout: "phone" },
] as const;

function harness(): Harness {
  return JSON.parse(readFileSync(path.join(here, ".run/server.json"), "utf8")) as Harness;
}

function channels(): Channel[] {
  const runtime = JSON.parse(readFileSync(path.join(here, ".run/runtime.json"), "utf8")) as { channels: Channel[] };
  return runtime.channels;
}

function sql(db: string, statement: string) {
  const result = spawnSync("sqlite3", [db, `PRAGMA busy_timeout=5000; ${statement}`], { encoding: "utf8" });
  if (result.status !== 0) throw new Error(result.stderr || result.stdout || statement);
  return result.stdout.trim();
}

// The harness seed names real teams. A public screenshot does not.
function genericTitles(db: string) {
  sql(
    db,
    `
UPDATE airings SET title='The Afternoon Game', subtitle='A Sunday game', description='The game this afternoon.' WHERE title LIKE 'NFL:%';
UPDATE airings SET title='Late Basketball', subtitle='A late tip', description='Basketball tonight.' WHERE title LIKE 'NBA:%';
`,
  );
}

function assertPublic(text: string) {
  const banned = [/\/Users\//, /broadwave-grok/, /MacBook/, /192\.168\./, /\b10\.\d{1,3}\.\d{1,3}\.\d{1,3}\b/, /Bears|Bills|Lakers|Celtics/, /Chicago|Buffalo|Los Angeles/];
  for (const re of banned) expect(text, re.source).not.toMatch(re);
}

async function visibleText(page: Page) {
  return page.evaluate(() => {
    const vh = window.innerHeight;
    const vw = window.innerWidth;
    const out: string[] = [];
    const walker = document.createTreeWalker(document.body, NodeFilter.SHOW_TEXT);
    let node = walker.nextNode();
    while (node) {
      const el = node.parentElement;
      const text = (node.textContent || "").replace(/\s+/g, " ").trim();
      node = walker.nextNode();
      if (!el || !text) continue;
      const style = getComputedStyle(el);
      if (style.visibility === "hidden" || style.display === "none" || Number(style.opacity) === 0) continue;
      const rect = el.getBoundingClientRect();
      if (rect.width < 1 || rect.height < 1) continue;
      if (rect.bottom <= 0 || rect.top >= vh || rect.right <= 0 || rect.left >= vw) continue;
      out.push(text);
    }
    return out.join("\n");
  });
}

async function save(page: Page, name: string) {
  assertPublic(await visibleText(page));
  const file = path.join(shots, name);
  let quality = 70;
  await page.screenshot({ path: file, type: "jpeg", quality, animations: "disabled", caret: "hide" });
  while (statSync(file).size > limit && quality > 30) {
    quality -= 10;
    await page.screenshot({ path: file, type: "jpeg", quality, animations: "disabled", caret: "hide" });
  }
  const size = statSync(file).size;
  expect(size, name).toBeLessThanOrEqual(limit);
  return { file: name, bytes: size, quality };
}

async function settle(page: Page) {
  await expect(page.locator("main")).toHaveAttribute("data-ready", "1");
  await page.evaluate(() => document.fonts.ready);
  const dismiss = page.getByRole("button", { name: "Not now" });
  if ((await dismiss.count()) > 0) await dismiss.first().click();
}

async function picture(page: Page, selector: string) {
  const node = page.locator(selector).first();
  await expect(node).toBeVisible();
  const start = await node.evaluate((video: HTMLVideoElement) => video.currentTime);
  await expect
    .poll(
      () =>
        node.evaluate((video: HTMLVideoElement, from: number) => {
          return video.videoWidth > 0 && !video.paused && video.currentTime > from + 0.2;
        }, start),
      { timeout: 25_000 },
    )
    .toBe(true);
}

async function wake(page: Page) {
  const stage = page.getByRole("region", { name: "Player" });
  if (!(await stage.evaluate((el) => el.classList.contains("idle")))) return;
  const box = await page.locator("video.stage-video").boundingBox();
  expect(box).toBeTruthy();
  await page.mouse.click(box!.x + box!.width / 2, box!.y + box!.height * 0.28);
  await expect(stage).not.toHaveClass(/\bidle\b/);
  await expect.poll(() => page.locator("video.stage-video").evaluate((video: HTMLVideoElement) => video.paused)).toBe(false);
}

test("readme screens from the test pattern", async ({ page, request }) => {
  test.setTimeout(280_000);
  if (process.env.E2E_SOURCE) throw new Error("E2E_SOURCE would put a broadcast in the README");
  mkdirSync(shots, { recursive: true });
  mkdirSync(evidence, { recursive: true });
  const notes: Record<string, unknown> = {};
  const errors: string[] = [];
  page.on("console", (msg) => {
    if (msg.type() === "error") errors.push(msg.text());
  });
  page.on("pageerror", (err) => errors.push(String(err)));

  const setup = await request.put("/api/v1/settings", { data: { setupComplete: "1" } });
  expect(setup.ok()).toBeTruthy();
  // The default name is the machine's hostname. A public shot uses the product name.
  const renamed = await request.patch("/api/v1/server", { data: { name: "Broadwave" } });
  expect(renamed.ok()).toBeTruthy();
  const { base, db } = harness();
  genericTitles(db);
  const lineup = channels();
  const game = lineup.find((channel) => channel.name === "KBWV");
  const news = lineup.find((channel) => channel.name === "KBWV2");
  const other = lineup.find((channel) => channel.name === "WTST");
  expect(game && news && other).toBeTruthy();

  const started = await request.post("/api/v1/recordings", {
    data: { channelId: news!.id, minutes: 30, title: "Evening News" },
  });
  notes.recording = started.ok() ? "on" : `${started.status()} ${(await started.text()).slice(0, 180)}`;
  if (started.ok()) {
    await expect
      .poll(async () => {
        const body = (await (await fetch(`${base}/api/v1/recordings`)).json()) as { recordings?: { title: string; status: string }[] };
        return (body.recordings ?? []).some((row) => row.title === "Evening News" && row.status === "recording");
      })
      .toBe(true);
    await expect
      .poll(
        async () => {
          const body = (await (await fetch(`${base}/api/v1/frames`)).json()) as { channels?: number[] };
          return (body.channels ?? []).includes(game!.id);
        },
        { timeout: 30_000 },
      )
      .toBe(true);
  }

  await holdClock(page);
  const saved: { file: string; bytes: number; quality: number }[] = [];

  for (const size of sizes) {
    await page.setViewportSize({ width: size.width, height: size.height });
    await page.goto("/");
    await settle(page);
    await expect(page.locator("html")).toHaveAttribute("data-layout", size.layout);
    await expect(page.getByRole("heading", { name: "The Afternoon Game" })).toBeVisible();
    // The blurred copy sits under the sharp frame and Playwright treats it as hidden.
    await expect(page.locator("img.art-sharp[src*='/frame'], img.nc-frame:not([hidden])").first()).toBeVisible();
    saved.push(await save(page, `home-${size.name}.jpg`));

    await page.getByRole("tab", { name: "Guide" }).click();
    await expect(page).toHaveURL(/\/guide$/);
    await expect(page.getByText("The Afternoon Game").first()).toBeVisible();
    saved.push(await save(page, `guide-${size.name}.jpg`));

    await page.goto(`/watch?channel=${game!.id}`);
    await settle(page);
    const player = page.getByRole("region", { name: "Player" });
    await expect(player).toBeVisible();
    await picture(page, "video.stage-video");
    // The video can be moving under the tuning card. Wait until that card leaves.
    await expect(page.locator(".tuning")).toHaveCount(0, { timeout: 30_000 });
    await wake(page);
    await page.getByRole("button", { name: "Channels" }).click();
    await expect(page.getByRole("listbox", { name: "Channels" })).toBeVisible();
    await expect(page.getByRole("option", { name: /The Afternoon Game/ })).toBeVisible();
    saved.push(await save(page, `player-${size.name}.jpg`));

    await page.goto(`/multiview?ch=${game!.id},${other!.id}&layout=2up&focus=${game!.id}`);
    await settle(page);
    await expect(page.getByRole("region", { name: "Side by side" })).toBeVisible();
    await picture(page, "video.mv-video");
    const tiles = await page.locator("video.mv-video").evaluateAll((videos: HTMLVideoElement[]) =>
      videos.map((video) => ({
        channel: video.dataset.channel || "",
        width: video.videoWidth,
        paused: video.paused,
        alert: video.closest(".mv-tile")?.querySelector("[role='alert']")?.textContent?.replace(/\s+/g, " ").trim() || "",
      })),
    );
    notes[`side-${size.name}`] = tiles;
    saved.push(await save(page, `side-${size.name}.jpg`));

    await page.goto("/recordings");
    await settle(page);
    await expect(page.getByRole("heading", { name: "Recordings", level: 1 })).toBeVisible();
    if (started.ok()) {
      await expect(page.getByText("Evening News").first()).toBeVisible();
      // The poster is written after the recording starts. An early shot is an empty box.
      await expect
        .poll(
          () =>
            page.locator("img.poster").first().evaluate((img: HTMLImageElement) => img.naturalWidth > 0 && img.style.visibility !== "hidden"),
          { timeout: 20_000 },
        )
        .toBe(true);
    }
    saved.push(await save(page, `recordings-${size.name}.jpg`));
  }

  notes.files = saved;
  notes.errors = errors;
  writeFileSync(path.join(evidence, "notes.json"), JSON.stringify(notes, null, 2));
});

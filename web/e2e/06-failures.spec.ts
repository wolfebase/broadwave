import { spawnSync } from "node:child_process";
import { chmodSync, mkdirSync, readFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import type { Locator, Page } from "@playwright/test";
import { expect, test } from "./fixture";
import { settle } from "./snap";

const here = path.dirname(fileURLToPath(import.meta.url));
const evidence = path.resolve(here, "../../.evidence/lane");

type Channel = { id: number; number: string; name: string };
type Harness = { base: string; db: string; config: string; admin: string; control: string };

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

function sql(db: string, statement: string) {
  const result = spawnSync("sqlite3", [db, `PRAGMA busy_timeout=5000; ${statement}`], { encoding: "utf8" });
  if (result.status !== 0) throw new Error(result.stderr || result.stdout || statement);
}

async function post(url: string) {
  const res = await fetch(url, { method: "POST", signal: AbortSignal.timeout(40_000) });
  if (!res.ok) throw new Error(`${url} ${res.status} ${await res.text()}`);
}

async function openChannel(page: Page, id: number) {
  await page.goto(`/watch?channel=${id}`);
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
  if (!page.url().includes(`channel=${id}`)) {
    await page.goto(`/watch?channel=${id}`);
    await settle(page);
  }
  await expect(player).toBeVisible();
}

async function expectPlaying(page: Page) {
  await expect
    .poll(
      () => page.locator("video.stage-video").evaluate((video: HTMLVideoElement) => video.videoWidth > 0 && !video.paused),
      { timeout: 45_000 },
    )
    .toBe(true);
}

async function mark(page: Page) {
  await page.evaluate(() => {
    document.documentElement.dataset.lane = "stay";
  });
}

async function shot(page: Page, name: string) {
  mkdirSync(evidence, { recursive: true });
  await page.screenshot({ path: path.join(evidence, name), animations: "disabled" });
}

function notice(page: Page, role: "alert" | "status", text: string | RegExp) {
  // alert and status take their accessible name from the author, not from the
  // text inside them, so the message has to be matched as content.
  return page.getByRole(role).filter({ hasText: text });
}

async function nudge(page: Page) {
  const again = page.getByRole("button", { name: "Try again" });
  // The player may recover by itself and take the button away mid-click.
  if (await again.isVisible()) await again.click({ timeout: 5_000 }).catch(() => undefined);
}

async function serverUp(base: string, control: string) {
  try {
    const res = await fetch(`${base}/api/v1/health`);
    if (res.ok) return;
  } catch {
    // The restart test stops the process. The next test brings it back.
  }
  await post(`${control}/start`);
}

test.beforeEach(async () => {
  const { base, control } = harness();
  await serverUp(base, control);
});

test("both tuners busy, then a free tuner plays without a reload", async ({ page }) => {
  const { base, admin } = harness();
  const kbwv = channel("KBWV");
  try {
    // An earlier spec's viewer holds a tuner until the server drops it as abandoned (45 s).
    await expect
      .poll(async () => {
        const res = await fetch(`${base}/api/v1/tuners`);
        const body = (await res.json()) as { tuners?: { target?: string; guide?: string; ours?: boolean }[] };
        return (body.tuners ?? []).every((tuner) => !tuner.target && !tuner.guide && !tuner.ours);
      }, { timeout: 90_000 })
      .toBe(true);
    await post(`${admin}/hold`);
    await openChannel(page, kbwv.id);
    await mark(page);
    await expect(notice(page, "alert", /tuner is busy/i)).toBeVisible();
    await expect(page.getByRole("button", { name: "Try again" })).toBeVisible();
    await shot(page, "l2-tuners-busy.jpg");
    await post(`${admin}/free`);
    await nudge(page);
    await expectPlaying(page);
    await expect(page.locator("html")).toHaveAttribute("data-lane", "stay");
    await expect(notice(page, "alert", /tuner is busy/i)).toHaveCount(0);
  } finally {
    await post(`${admin}/free`).catch(() => undefined);
  }
});

test("the server can restart mid-play and the picture comes back", async ({ page }) => {
  const { control } = harness();
  const kbwv = channel("KBWV");
  try {
    await openChannel(page, kbwv.id);
    await expectPlaying(page);
    await mark(page);
    await post(`${control}/stop`);
    await expect(notice(page, "alert", /The server stopped/)).toBeVisible({ timeout: 60_000 });
    await expect(page.getByRole("button", { name: "Try again" })).toBeVisible();
    await shot(page, "l2-server-restart.jpg");
    await post(`${control}/start`);
    await nudge(page);
    await expectPlaying(page);
    await expect(page.locator("html")).toHaveAttribute("data-lane", "stay");
    await expect(notice(page, "alert", /The server stopped/)).toHaveCount(0);
  } finally {
    await post(`${control}/start`).catch(() => undefined);
  }
});

test("a server restart under a playing picture starts it again with no click", async ({ page }) => {
  const { control } = harness();
  const kbwv = channel("KBWV");
  try {
    await openChannel(page, kbwv.id);
    await expectPlaying(page);
    await mark(page);
    let watchedAt = 0;
    page.on("request", (req) => {
      if (req.method() === "POST" && new URL(req.url()).pathname === "/api/v1/watch" && !watchedAt) watchedAt = Date.now();
    });
    // /start stops the process and launches it again: every watch it had is gone.
    await post(`${control}/start`);
    const back = Date.now();
    await expect.poll(() => watchedAt, { timeout: 30_000, message: "a new watch starts with no click" }).toBeGreaterThan(0);
    await expect
      .poll(
        () =>
          page.locator("video.stage-video").evaluate((video: HTMLVideoElement) => {
            const at = video.currentTime;
            return new Promise<boolean>((done) => setTimeout(() => done(!video.paused && video.currentTime > at + 0.5), 1000));
          }),
        { timeout: 30_000, intervals: [500] },
      )
      .toBe(true);
    console.log(`restart: new watch ${watchedAt - back} ms and picture moving ${Date.now() - back} ms after the server answered`);
    await shot(page, "j043-web-restart.jpg");
    await expect(page.locator("html")).toHaveAttribute("data-lane", "stay");
    await expect(page.getByRole("button", { name: "Try again" })).toHaveCount(0);
  } finally {
    await post(`${control}/start`).catch(() => undefined);
  }
});

test("a tuner that stops answering can be watched again", async ({ page }) => {
  const { admin } = harness();
  const kbwv = channel("KBWV");
  try {
    await openChannel(page, kbwv.id);
    await expectPlaying(page);
    await mark(page);
    await post(`${admin}/silence`);
    await expect(notice(page, "alert", /This tuner did not answer/)).toBeVisible({ timeout: 60_000 });
    await expect(page.getByRole("button", { name: "Try again" })).toBeVisible();
    await shot(page, "l2-tuner-silent.jpg");
    await post(`${admin}/answer`);
    await nudge(page);
    await expectPlaying(page);
    await expect(page.locator("html")).toHaveAttribute("data-lane", "stay");
    await expect(notice(page, "alert", /This tuner did not answer/)).toHaveCount(0);
  } finally {
    await post(`${admin}/answer`).catch(() => undefined);
  }
});

test("a channel with no listing plays, then shows the program when the guide arrives", async ({ page }) => {
  const { db } = harness();
  const wtst = channel("WTST");
  const now = Date.now();
  const start = new Date(now - 5 * 60_000).toISOString().replace(/\.\d{3}Z$/, "Z");
  const end = new Date(now + 60 * 60_000).toISOString().replace(/\.\d{3}Z$/, "Z");
  sql(db, `DELETE FROM airings WHERE channel_id = ${wtst.id};`);
  try {
    await openChannel(page, wtst.id);
    await mark(page);
    await expect(notice(page, "status", "No listing for this channel.")).toBeVisible();
    await expect(page.getByRole("button", { name: "Check for listings" })).toBeVisible();
    await expectPlaying(page);
    await shot(page, "l2-no-guide.jpg");
    sql(
      db,
      `INSERT INTO airings (channel_id, title, subtitle, description, category, starts_at, ends_at, program_id, is_live, guide_source)
       VALUES (${wtst.id}, 'Garden Hour', 'In the yard', 'A quiet hour.', 'Series', '${start}', '${end}', 'e2e-garden', 0, 'e2e');`,
    );
    await page.getByRole("button", { name: "Check for listings" }).click();
    await expect(page.getByRole("heading", { name: "Garden Hour" })).toBeVisible();
    await expect(notice(page, "status", "No listing for this channel.")).toHaveCount(0);
    await expectPlaying(page);
    await expect(page.locator("html")).toHaveAttribute("data-lane", "stay");
  } finally {
    sql(
      db,
      `INSERT INTO airings (channel_id, title, subtitle, description, category, starts_at, ends_at, program_id, is_live, guide_source)
       SELECT ${wtst.id}, 'Garden Hour', 'In the yard', 'A quiet hour.', 'Series', '${start}', '${end}', 'e2e-garden', 0, 'e2e'
       WHERE NOT EXISTS (SELECT 1 FROM airings WHERE channel_id = ${wtst.id});`,
    );
  }
});

test("a channel with no signal plays again when the signal returns", async ({ page }) => {
  const { admin } = harness();
  const wtst = channel("WTST");
  try {
    await openChannel(page, wtst.id);
    await expectPlaying(page);
    await mark(page);
    await post(`${admin}/dark?channel=${encodeURIComponent(wtst.number)}`);
    await expect(notice(page, "alert", "This channel isn't coming in. Check the antenna.")).toBeVisible({ timeout: 60_000 });
    await expect(page.getByRole("button", { name: "Try again" })).toBeVisible();
    await shot(page, "l6-no-signal.jpg");
    await post(`${admin}/light?channel=${encodeURIComponent(wtst.number)}`);
    await nudge(page);
    await expectPlaying(page);
    await expect(page.locator("html")).toHaveAttribute("data-lane", "stay");
    await expect(notice(page, "alert", "This channel isn't coming in. Check the antenna.")).toHaveCount(0);
  } finally {
    await post(`${admin}/light?channel=${encodeURIComponent(wtst.number)}`).catch(() => undefined);
  }
});

test("a multiview tile that does not start gives its sound away and can be removed", async ({ page }) => {
  const { admin, base } = harness();
  const kbwv = channel("KBWV");
  const wtst = channel("WTST");
  // A picture of 5.1 still running from the test before would play a moment,
  // and a tile that showed a picture keeps its sound.
  await expect
    .poll(async () => ((await (await fetch(`${base}/api/v1/tuners`)).json()) as { tuners?: { ours?: boolean }[] }).tuners?.some((t) => t.ours) ?? false, {
      timeout: 60_000,
      intervals: [500],
      message: "the previous watch let the tuners go",
    })
    .toBe(false);
  try {
    await openChannel(page, kbwv.id);
    await expectPlaying(page);
    await post(`${admin}/dark?channel=${encodeURIComponent(wtst.number)}`);
    await page.goto(`/multiview?ch=${kbwv.id},${wtst.id}&layout=2up&focus=${wtst.id}`);
    const dark = page.getByRole("group", { name: `${wtst.number} ${wtst.name}`, exact: true });
    await expect(dark.getByRole("alert")).toContainText("This channel isn't coming in. Check the antenna.", { timeout: 60_000 });
    await expect(dark.getByRole("button", { name: "Try again" })).toBeVisible();
    await expect(page.getByRole("group", { name: `${kbwv.number} ${kbwv.name}, sound on` })).toBeVisible();
    await expect(page).toHaveURL(new RegExp(`focus=${kbwv.id}`));
    mkdirSync(path.join(evidence, "l28"), { recursive: true });
    await page.screenshot({ path: path.join(evidence, "l28", "tile-retry.jpg"), animations: "disabled" });
    await dark.getByRole("button", { name: "Remove" }).click();
    await expect(dark).toHaveCount(0);
    await expect(page).toHaveURL(new RegExp(`ch=${kbwv.id}(&|$)`));
  } finally {
    await post(`${admin}/light?channel=${encodeURIComponent(wtst.number)}`).catch(() => undefined);
  }
});

test("a recordings folder that cannot be written works again without a reload", async ({ page }) => {
  const { base, db, config } = harness();
  const dir = path.join(config, "work", "recordings");
  mkdirSync(dir, { recursive: true });
  sql(db, `INSERT INTO settings(key, value) VALUES('watermarkGB', '0') ON CONFLICT(key) DO UPDATE SET value='0';`);
  chmodSync(dir, 0o555);
  try {
    await page.goto("/");
    await settle(page);
    const setup = page.getByRole("heading", { name: "Let's set up your TV" });
    const hero = page.getByRole("heading", { name: "NFL: Bears at Bills" });
    await expect(setup.or(hero)).toBeVisible();
    if (await setup.isVisible()) {
      const cont = page.getByRole("button", { name: "Continue" });
      if (await cont.isVisible()) await cont.click();
      await page.getByRole("button", { name: "Watch", exact: true }).click();
      await expect(setup).toHaveCount(0);
      await page.goto("/");
      await settle(page);
    }
    await mark(page);
    await expect(page.getByRole("heading", { name: "NFL: Bears at Bills" })).toBeVisible();
    await page.locator(".hero-actions").getByRole("button", { name: "Record" }).click();
    await expect(notice(page, "alert", "Broadwave can't save this recording. Check the recordings folder, then try again.")).toBeVisible();
    await shot(page, "l6-disk.jpg");
    chmodSync(dir, 0o755);
    await page.locator(".hero-actions").getByRole("button", { name: "Record" }).click();
    await expect(notice(page, "alert", /can't save this recording/)).toHaveCount(0);
    await expect
      .poll(async () => {
        const res = await fetch(`${base}/api/v1/recordings`);
        const body = (await res.json()) as { recordings?: { status?: string }[] };
        return (body.recordings ?? []).some((rec) => rec.status === "recording");
      })
      .toBe(true);
    await expect(page.locator("html")).toHaveAttribute("data-lane", "stay");
  } finally {
    chmodSync(dir, 0o755);
    sql(db, `DELETE FROM settings WHERE key = 'watermarkGB';`);
    await stopRecordings(base);
  }
});

test("a phone that loses its connection plays again when the network returns", async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  const kbwv = channel("KBWV");
  try {
    await openChannel(page, kbwv.id);
    await expectPlaying(page);
    await mark(page);
    const dropped = Date.now();
    await page.context().setOffline(true);
    await expect(notice(page, "alert", "The connection dropped. It will try again when it's back.")).toBeVisible({ timeout: 30_000 });
    const left = 20_000 - (Date.now() - dropped);
    if (left > 0) await page.waitForTimeout(left);
    await expect(notice(page, "alert", "The connection dropped. It will try again when it's back.")).toBeVisible();
    await shot(page, "l6-offline.jpg");
    await expect(page.locator("html")).toHaveAttribute("data-lane", "stay");
    await page.context().setOffline(false);
    await nudge(page);
    await expectPlaying(page);
    await expect(page.locator("html")).toHaveAttribute("data-lane", "stay");
    await expect(notice(page, "alert", /connection dropped/)).toHaveCount(0);
  } finally {
    await page.context().setOffline(false).catch(() => undefined);
  }
});

type WatchEnvelope = {
  name: string;
  status: number;
  sentence: string;
  json?: { code: string; message: string; limit?: number };
  html?: string;
};

// The sentences the server sends for these codes, plus a proxy page with no envelope.
const watchErrors: WatchEnvelope[] = [
  {
    name: "no-source",
    status: 404,
    sentence: "No source has this channel now. Check Sources in Settings.",
    json: { code: "no_source", message: "No source has this channel now. Check Sources in Settings." },
  },
  {
    name: "streams-full",
    status: 409,
    sentence: "All 2 streams from this playlist are in use. Stop one or raise the limit.",
    json: {
      code: "streams_full",
      message: "All 2 streams from this playlist are in use. Stop one or raise the limit.",
      limit: 2,
    },
  },
  {
    name: "tuner-refused",
    status: 503,
    sentence: "The tuner would not start this channel. Try again.",
    json: { code: "tuner_refused", message: "The tuner would not start this channel. Try again." },
  },
  {
    name: "internal",
    status: 500,
    sentence: "This channel did not start. The server log says why.",
    json: { code: "internal", message: "This channel did not start. The server log says why." },
  },
  {
    name: "html",
    status: 502,
    sentence: "That did not work. Try again.",
    html: "<!DOCTYPE html><html><head><title>502 Bad Gateway</title></head><body><h1>502 Bad Gateway</h1><p>nginx</p></body></html>",
  },
];

const viewports = [
  { name: "1440x900", width: 1440, height: 900, layout: "" },
  { name: "390x844", width: 390, height: 844, layout: "phone" },
  { name: "1920x1080", width: 1920, height: 1080, layout: "tv" },
] as const;

async function armWatchErrors(page: Page, channelId: number) {
  let index = 0;
  const posts: number[] = [];
  await page.route(/\/api\/v1\/watch$/, async (route) => {
    const req = route.request();
    if (req.method() !== "POST") return route.continue();
    let body: { channelId?: number } = {};
    try {
      body = req.postDataJSON() as { channelId?: number };
    } catch {
      return route.continue();
    }
    if (body.channelId !== channelId) return route.continue();
    const at = index;
    posts.push(at);
    const item = watchErrors[Math.min(at, watchErrors.length - 1)];
    if (item.html) {
      await route.fulfill({ status: item.status, contentType: "text/html; charset=utf-8", body: item.html });
      return;
    }
    await route.fulfill({ status: item.status, contentType: "application/json", body: JSON.stringify(item.json) });
  });
  return {
    posts,
    setIndex(next: number) {
      index = next;
    },
    reset() {
      index = 0;
      posts.length = 0;
    },
  };
}

async function finishSetup(page: Page, arrived: Locator) {
  // Settings land after the shell is ready, so the setup heading can show up late.
  const setup = page.getByRole("heading", { name: "Let's set up your TV" });
  await expect(setup.or(arrived)).toBeVisible();
  if (!(await setup.isVisible())) return;
  const cont = page.getByRole("button", { name: "Continue" });
  if (await cont.isVisible()) await cont.click();
  await page.getByRole("button", { name: "Watch", exact: true }).click();
  await expect(setup).toHaveCount(0);
}

async function assertSentences(
  page: Page,
  sentence: Locator,
  again: Locator,
  gate: Awaited<ReturnType<typeof armWatchErrors>>,
  file: (name: string) => string,
) {
  for (let i = 0; i < watchErrors.length; i++) {
    const item = watchErrors[i];
    await expect(sentence).toHaveText(item.sentence);
    await expect(again).toBeVisible();
    const visible = await page.locator("body").innerText();
    expect(visible).not.toContain("<html");
    expect(visible).not.toContain("502 Bad Gateway");
    expect(visible).not.toContain("nginx");
    expect(visible).not.toContain("Internal Server Error");
    expect(await sentence.innerText()).not.toContain(String(item.status));
    await page.screenshot({ path: file(item.name), animations: "disabled" });
    const before = gate.posts.length;
    gate.setIndex(i + 1);
    await again.click();
    await expect.poll(() => gate.posts.slice(before).some((n) => n === i + 1)).toBe(true);
  }
}

test("a failed watch says the sentence, and Try again asks again", async ({ page }) => {
  const kbwv = channel("KBWV");
  const gate = await armWatchErrors(page, kbwv.id);
  const dir = path.join(evidence, "l33");
  mkdirSync(dir, { recursive: true });
  for (const size of viewports) {
    await page.setViewportSize({ width: size.width, height: size.height });
    const extra = size.layout ? `&layout=${size.layout}` : "";
    const player = page.getByRole("region", { name: "Player" });
    gate.reset();
    await page.goto(`/watch?channel=${kbwv.id}${extra}`);
    await settle(page);
    await finishSetup(page, player);
    if (!page.url().includes(`channel=${kbwv.id}`) || (size.layout && !page.url().includes(`layout=${size.layout}`))) {
      gate.reset();
      await page.goto(`/watch?channel=${kbwv.id}${extra}`);
      await settle(page);
      await finishSetup(page, player);
    }
    await expect(player).toBeVisible();
    await expect(page.locator("html")).toHaveAttribute("data-layout", size.layout || "desktop");
    await mark(page);
    await assertSentences(
      page,
      page.getByRole("alert"),
      page.getByRole("button", { name: "Try again" }),
      gate,
      (name) => path.join(dir, `player-${name}-${size.name}.jpg`),
    );
    await expect(page.locator("html")).toHaveAttribute("data-lane", "stay");
  }
});

test("a multiview tile that fails to start says the sentence, and Try again asks again", async ({ page }) => {
  const kbwv = channel("KBWV");
  const wtst = channel("WTST");
  const gate = await armWatchErrors(page, wtst.id);
  const dir = path.join(evidence, "l33");
  mkdirSync(dir, { recursive: true });
  for (const size of viewports) {
    if (size.layout === "tv") {
      await page.addInitScript(() => {
        const orig = window.matchMedia.bind(window);
        window.matchMedia = (query: string) => {
          const q = String(query);
          if (q.includes("pointer") && q.includes("coarse")) {
            return {
              matches: true,
              media: q,
              onchange: null,
              addListener() {},
              removeListener() {},
              addEventListener() {},
              removeEventListener() {},
              dispatchEvent() {
                return false;
              },
            } as MediaQueryList;
          }
          return orig(q);
        };
      });
    }
    await page.setViewportSize({ width: size.width, height: size.height });
    const grid = page.getByRole("region", { name: "Side by side" });
    gate.reset();
    await page.goto(`/multiview?ch=${kbwv.id},${wtst.id}&layout=2up&focus=${wtst.id}`);
    await settle(page);
    await finishSetup(page, grid);
    if (!page.url().includes("/multiview")) {
      gate.reset();
      await page.goto(`/multiview?ch=${kbwv.id},${wtst.id}&layout=2up&focus=${wtst.id}`);
      await settle(page);
      await finishSetup(page, grid);
    }
    const want = size.layout === "tv" ? "tv" : size.width <= 760 ? "phone" : "desktop";
    await expect(page.locator("html")).toHaveAttribute("data-layout", want);
    const tile = page.getByRole("group", { name: new RegExp(`^${wtst.number} ${wtst.name}(, sound on)?$`) });
    await mark(page);
    await assertSentences(
      page,
      tile.locator(".mv-error p"),
      tile.getByRole("button", { name: "Try again" }),
      gate,
      (name) => path.join(dir, `tile-${name}-${size.name}.jpg`),
    );
    await expect(page.locator("html")).toHaveAttribute("data-lane", "stay");
    await expect(page.getByRole("group", { name: new RegExp(`^${kbwv.number} ${kbwv.name}`) })).toBeVisible();
  }
});

async function stopRecordings(base: string) {
  const res = await fetch(`${base}/api/v1/recordings`).catch(() => null);
  if (!res?.ok) return;
  const body = (await res.json()) as { recordings?: { id: number; status?: string }[] };
  for (const rec of body.recordings ?? []) {
    if (rec.status !== "recording") continue;
    await fetch(`${base}/api/v1/recordings/${rec.id}/stop`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: "{}",
    }).catch(() => undefined);
  }
}

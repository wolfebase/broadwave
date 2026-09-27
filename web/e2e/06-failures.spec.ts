import { spawnSync } from "node:child_process";
import { chmodSync, mkdirSync, readFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import type { Page } from "@playwright/test";
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
  if (await again.isVisible()) await again.click();
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
  const wdaf = channel("WDAF");
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
    await openChannel(page, wdaf.id);
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
  const wdaf = channel("WDAF");
  try {
    await openChannel(page, wdaf.id);
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

test("a tuner that stops answering can be watched again", async ({ page }) => {
  const { admin } = harness();
  const wdaf = channel("WDAF");
  try {
    await openChannel(page, wdaf.id);
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
  const kctv = channel("KCTV");
  const now = Date.now();
  const start = new Date(now - 5 * 60_000).toISOString().replace(/\.\d{3}Z$/, "Z");
  const end = new Date(now + 60 * 60_000).toISOString().replace(/\.\d{3}Z$/, "Z");
  sql(db, `DELETE FROM airings WHERE channel_id = ${kctv.id};`);
  try {
    await openChannel(page, kctv.id);
    await mark(page);
    await expect(notice(page, "status", "No listing for this channel.")).toBeVisible();
    await expect(page.getByRole("button", { name: "Check for listings" })).toBeVisible();
    await expectPlaying(page);
    await shot(page, "l2-no-guide.jpg");
    sql(
      db,
      `INSERT INTO airings (channel_id, title, subtitle, description, category, starts_at, ends_at, program_id, is_live, guide_source)
       VALUES (${kctv.id}, 'Garden Hour', 'In the yard', 'A quiet hour.', 'Series', '${start}', '${end}', 'e2e-garden', 0, 'e2e');`,
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
       SELECT ${kctv.id}, 'Garden Hour', 'In the yard', 'A quiet hour.', 'Series', '${start}', '${end}', 'e2e-garden', 0, 'e2e'
       WHERE NOT EXISTS (SELECT 1 FROM airings WHERE channel_id = ${kctv.id});`,
    );
  }
});

test("a channel with no signal plays again when the signal returns", async ({ page }) => {
  const { admin } = harness();
  const kctv = channel("KCTV");
  try {
    await openChannel(page, kctv.id);
    await expectPlaying(page);
    await mark(page);
    await post(`${admin}/dark?channel=${encodeURIComponent(kctv.number)}`);
    await expect(notice(page, "alert", "This channel isn't coming in. Check the antenna.")).toBeVisible({ timeout: 60_000 });
    await expect(page.getByRole("button", { name: "Try again" })).toBeVisible();
    await shot(page, "l6-no-signal.jpg");
    await post(`${admin}/light?channel=${encodeURIComponent(kctv.number)}`);
    await nudge(page);
    await expectPlaying(page);
    await expect(page.locator("html")).toHaveAttribute("data-lane", "stay");
    await expect(notice(page, "alert", "This channel isn't coming in. Check the antenna.")).toHaveCount(0);
  } finally {
    await post(`${admin}/light?channel=${encodeURIComponent(kctv.number)}`).catch(() => undefined);
  }
});

test("a multiview tile that does not start gives its sound away and can be removed", async ({ page }) => {
  const { admin } = harness();
  const wdaf = channel("WDAF");
  const kctv = channel("KCTV");
  try {
    await openChannel(page, wdaf.id);
    await expectPlaying(page);
    await post(`${admin}/dark?channel=${encodeURIComponent(kctv.number)}`);
    await page.goto(`/multiview?ch=${wdaf.id},${kctv.id}&layout=2up&focus=${kctv.id}`);
    const dark = page.getByRole("group", { name: `${kctv.number} ${kctv.name}`, exact: true });
    await expect(dark.getByRole("alert")).toBeVisible({ timeout: 60_000 });
    await expect(page.getByRole("group", { name: `${wdaf.number} ${wdaf.name}, sound on` })).toBeVisible();
    await expect(page).toHaveURL(new RegExp(`focus=${wdaf.id}`));
    await shot(page, "mv-refused-tile.jpg");
    await dark.getByRole("button", { name: "Remove" }).click();
    await expect(dark).toHaveCount(0);
    await expect(page).toHaveURL(new RegExp(`ch=${wdaf.id}(&|$)`));
  } finally {
    await post(`${admin}/light?channel=${encodeURIComponent(kctv.number)}`).catch(() => undefined);
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
    const hero = page.getByRole("heading", { name: "NFL: Chiefs at Bills" });
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
    await expect(page.getByRole("heading", { name: "NFL: Chiefs at Bills" })).toBeVisible();
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
  const wdaf = channel("WDAF");
  try {
    await openChannel(page, wdaf.id);
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

// One headless Chrome viewer. It uses the same keys as the on-screen remote:
// space pauses, left seeks back, down changes channel, r records.

const sleep = (ms) => new Promise((resolve) => setTimeout(resolve, ms));

export async function browse({ browser, base, channels, until, halt, onEvent, onPosition, index, quality }) {
  const context = await browser.newContext({
    baseURL: base,
    viewport: { width: 1280, height: 720 },
    reducedMotion: "reduce",
    deviceScaleFactor: 1,
  });
  const page = await context.newPage();
  await page.addInitScript((picture) => {
    // The player remembers a picture size. A local soak asks for the small
    // one so the browser shares the scripted viewers' encode.
    if (picture && picture !== "auto") {
      let current = {};
      try {
        current = JSON.parse(localStorage.getItem("ota-live") || "{}");
      } catch {
        current = {};
      }
      current.quality = picture;
      localStorage.setItem("ota-live", JSON.stringify(current));
    }
    window.__soak = [];
    window.__soakQuiet = 0;
    // A waiting event fires between segments even when the picture keeps
    // moving. A stall is the picture staying on one frame.
    let stuck = null;
    setInterval(() => {
      const video = document.querySelector("video");
      if (!video || video.paused || Date.now() < window.__soakQuiet || video.currentTime < 1 || video.readyState < 2) {
        stuck = null;
        return;
      }
      const time = video.currentTime;
      if (!stuck || Math.abs(time - stuck.time) > 0.05) {
        stuck = { time, since: Date.now(), open: false };
        return;
      }
      if (!stuck.open && Date.now() - stuck.since >= 8000) {
        stuck.open = true;
        window.__soak.push({ t: Date.now(), kind: "stall", time });
      }
    }, 1000);
  }, quality || "auto");
  const who = `browser ${index + 1}`;
  const timer = setInterval(() => {
    page
      .evaluate(() => {
        const video = document.querySelector("video");
        if (!video || video.paused || video.readyState < 2 || !Number.isFinite(video.currentTime)) return null;
        const params = new URLSearchParams(location.search);
        const channel = params.get("channel") || (params.get("ch") || "").split(",")[0];
        const channelId = Number(channel);
        if (!channelId) return null;
        return { channelId, edgePdt: video.currentTime * 1000, aligned: true };
      })
      .then((point) => {
        if (point) onPosition?.(point);
      })
      .catch(() => {});
  }, 5_000);
  let step = 0;
  try {
  while (Date.now() < until && !halt?.now) {
    const channel = channels[(step + index) % channels.length];
    const other = channels[(step + index + 1) % channels.length];
    try {
      if (step % 6 === 5 && channels.length > 1) await multiview(page, channel, other, who, onEvent);
      else await oneChannel(page, channel, who, onEvent, step);
    } catch (err) {
      onEvent({ who, kind: "error", detail: String(err?.message || err) });
      await sleep(2_000);
    }
    const fresh = await page.evaluate(() => {
      const rows = window.__soak || [];
      window.__soak = [];
      return rows;
    }).catch(() => []);
    for (const row of fresh) onEvent({ who, kind: row.kind, detail: `picture waited at ${Number(row.time).toFixed(1)}s` });
    step += 1;
  }
  } finally {
    clearInterval(timer);
    await endWatch(page);
    await context.close();
  }
}

// Playwright drops the pagehide beacon when it tears the document down, so
// the watch stays counted. Firing pagehide while the page is still up runs
// the player's own stop, and the real pagehide then sees that it already left.
// A channel that is already recording belongs to another part of the run.
// Pressing record there stops that recording, and the press meant as a stop
// starts a new one that keeps ffmpeg running after the viewers leave.
// A press that never turns recording on did not start one, so it is not
// followed by another press.
export function recordPresses({ already = false, started = false } = {}) {
  if (already) return 0;
  return started ? 2 : 1;
}

export async function endWatch(page) {
  if (!page?.evaluate) return;
  await page.evaluate(() => {
    window.dispatchEvent(new Event("pagehide"));
  }).catch(() => {});
}

async function oneChannel(page, channel, who, onEvent, step) {
  await endWatch(page);
  await page.goto(`/watch?channel=${channel.id}`, { waitUntil: "domcontentloaded" });
  await dismiss(page);
  const player = page.getByRole("region", { name: "Player" });
  await player.waitFor({ timeout: 30_000 });
  await player.focus();
  await page.mouse.move(400, 300);
  await playing(page);
  onEvent({ who, kind: "watch", detail: channel.guideNumber });
  await quiet(page, 4_000);
  await page.keyboard.press(" ");
  onEvent({ who, kind: "pause", detail: channel.guideNumber });
  await sleep(4_000);
  await player.focus();
  await page.keyboard.press(" ");
  onEvent({ who, kind: "play", detail: channel.guideNumber });
  await quiet(page, 2_000);
  await page.keyboard.press("ArrowLeft");
  onEvent({ who, kind: "seek", detail: "back 15 seconds" });
  // A seek buffers on purpose. That wait is not a stall.
  await quiet(page, 3_000);
  if (step % 3 === 0) await exerciseRecord(page, player, channel, who, onEvent);
  await player.focus();
  await page.keyboard.press("ArrowDown");
  onEvent({ who, kind: "channel", detail: "next" });
  await quiet(page, 2_000);
}

async function exerciseRecord(page, player, channel, who, onEvent) {
  const button = page.getByRole("button", { name: /record/i });
  const visible = await button.waitFor({ timeout: 10_000 }).then(() => true).catch(() => false);
  const already = visible && (await button.getAttribute("aria-pressed").catch(() => null)) === "true";
  if (recordPresses({ already, started: false }) === 0) {
    onEvent({ who, kind: "record", detail: "already recording" });
    return;
  }
  await player.focus();
  await page.keyboard.press("r");
  const started = await waitPressed(button, true);
  onEvent({
    who,
    kind: "record",
    detail: started ? channel.displayName || channel.guideNumber : "did not start",
  });
  if (recordPresses({ already: false, started }) < 2) return;
  await sleep(12_000);
  await player.focus();
  await page.keyboard.press("r");
  const stopped = await waitPressed(button, false);
  onEvent({ who, kind: "record", detail: stopped ? "stop" : "still recording" });
}

async function waitPressed(button, want) {
  const until = Date.now() + 5_000;
  while (Date.now() < until) {
    const pressed = (await button.getAttribute("aria-pressed").catch(() => null)) === "true";
    if (pressed === want) return true;
    await sleep(250);
  }
  return false;
}

async function multiview(page, channel, other, who, onEvent) {
  await endWatch(page);
  const target = `/multiview?ch=${channel.id},${other.id}&layout=2up&focus=${channel.id}`;
  await page.goto(target, { waitUntil: "domcontentloaded" });
  await dismiss(page);
  const grid = page.getByRole("region", { name: "Side by side" });
  await grid.waitFor({ timeout: 30_000 });
  onEvent({ who, kind: "multiview", detail: `${channel.guideNumber} and ${other.guideNumber}` });
  await playing(page);
  await sleep(12_000);
}

async function dismiss(page) {
  const setup = page.getByRole("heading", { name: "Let's set up your TV" });
  if (await setup.isVisible().catch(() => false)) {
    const cont = page.getByRole("button", { name: "Continue" });
    if (await cont.isVisible().catch(() => false)) await cont.click();
    const watch = page.getByRole("button", { name: "Watch", exact: true });
    if (await watch.isVisible().catch(() => false)) await watch.click();
  }
  const anyway = page.getByRole("button", { name: "Watch anyway" });
  if (await anyway.isVisible().catch(() => false)) await anyway.click();
  const retry = page.getByRole("button", { name: "Try again" });
  if (await retry.isVisible().catch(() => false)) await retry.click();
}

async function playing(page) {
  await page.waitForFunction(() => {
    const video = document.querySelector("video");
    return !!video && video.readyState >= 2 && !video.paused && video.currentTime > 0;
  }, { timeout: 45_000 });
}

async function quiet(page, ms) {
  await page.evaluate((until) => {
    window.__soakQuiet = Date.now() + until;
  }, ms);
  await sleep(ms);
}

import { mkdirSync, writeFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { expect, test } from "./fixture";
import { settle } from "./snap";

// A measurement, not a gate: a sound switch on a CPU-starved browser. It runs
// only with E2E_SLOW_SWITCH set to how the switch is made (option, next, track,
// ui from the Options panel, or panel: the panel opens and nothing switches)
// and E2E_CPU to Chrome's throttling rate.
const here = path.dirname(fileURLToPath(import.meta.url));
const evidence = path.resolve(here, "../../.evidence/f2/slow");
const mode = process.env.E2E_SLOW_SWITCH ?? "";
const rate = Number(process.env.E2E_CPU || 4);
// E2E_SWDECODE=1 decodes in software, as a runner without a GPU does.
if (process.env.E2E_SWDECODE === "1") test.use({ launchOptions: { args: ["--disable-accelerated-video-decode", "--disable-gpu"] } });

type Sample = { at: number; t: number; frames: number; dropped: number; both: string; video: string; audio: string; ready: number };
type HlsLike = {
  on: (name: string, fn: (e: string, d: Record<string, unknown>) => void) => void;
  audioTracks: { name: string; lang?: string }[];
  audioTrack: number;
  nextAudioTrack: number;
  setAudioOption: (o: { name: string; lang?: string }) => unknown;
  media?: HTMLVideoElement;
};

test("a sound switch on a slow browser", async ({ page }) => {
  test.skip(!mode || process.env.E2E_TRACKS !== "2", "measurement: E2E_SLOW_SWITCH=option|next|track|panel|ui with E2E_TRACKS=2");
  test.setTimeout(150_000);
  await page.goto("/");
  await settle(page);
  await page.getByRole("button", { name: "Watch", exact: true }).click();
  const video = page.locator("video.stage-video");
  await expect.poll(() => video.evaluate((el: HTMLVideoElement) => el.currentTime > 5 && !el.paused), { timeout: 45_000 }).toBe(true);
  await video.evaluate((el: HTMLVideoElement & { hls?: HlsLike }) => {
    const w = window as unknown as { __log: { at: number; e: string; d: string }[]; __samples: Sample[] };
    w.__log = [];
    w.__samples = [];
    const t0 = performance.now();
    const ranges = (b?: TimeRanges | null) => {
      if (!b) return "";
      const out: string[] = [];
      for (let i = 0; i < b.length; i++) out.push(`${b.start(i).toFixed(2)}-${b.end(i).toFixed(2)}`);
      return out.join(",");
    };
    const log = (e: string, d: unknown) => w.__log.push({ at: Math.round(performance.now() - t0), e, d: JSON.stringify(d)?.slice(0, 200) ?? "" });
    for (const e of ["hlsBufferFlushing", "hlsBufferFlushed", "hlsAudioTrackSwitching", "hlsAudioTrackSwitched", "hlsBufferReset", "hlsBufferCodecs", "hlsError", "hlsFragBuffered"]) {
      el.hls?.on(e, (_n, d) => {
        const frag = (d as { frag?: { type?: string; sn?: number; start?: number } }).frag;
        log(e, frag ? { type: frag.type, sn: frag.sn, start: frag.start } : { type: d.type, start: d.startOffset, end: d.endOffset, id: d.id, details: d.details, fatal: d.fatal });
      });
    }
    for (const e of ["waiting", "stalled", "seeking", "seeked", "ratechange"]) el.addEventListener(e, () => log(e, { t: el.currentTime, rate: el.playbackRate }));
    setInterval(() => {
      const q = el.getVideoPlaybackQuality();
      const ms = el.hls as unknown as { bufferController?: { tracks?: Record<string, { buffer?: SourceBuffer }> } };
      const tracks = ms?.bufferController?.tracks ?? {};
      w.__samples.push({
        at: Math.round(performance.now() - t0),
        t: el.currentTime,
        frames: q.totalVideoFrames,
        dropped: q.droppedVideoFrames,
        both: ranges(el.buffered),
        video: ranges(tracks.video?.buffer?.buffered),
        audio: ranges(tracks.audio?.buffer?.buffered),
        ready: el.readyState,
      });
    }, 250);
  });
  const cdp = await page.context().newCDPSession(page);
  // media-internals: renderer underflows, decoder resets, and splices.
  const media: { at: number; kind: string; d: string }[] = [];
  const t0 = Date.now();
  cdp.on("Media.playerMessagesLogged", (e) => e.messages.forEach((m) => media.push({ at: Date.now() - t0, kind: `msg.${m.level}`, d: m.message.slice(0, 300) })));
  cdp.on("Media.playerEventsAdded", (e) => e.events.forEach((m) => media.push({ at: Date.now() - t0, kind: "event", d: m.value.slice(0, 300) })));
  cdp.on("Media.playerErrorsRaised", (e) => e.errors.forEach((m) => media.push({ at: Date.now() - t0, kind: "error", d: JSON.stringify(m).slice(0, 300) })));
  cdp.on("Media.playerPropertiesChanged", (e) =>
    e.properties.filter((p) => /decoder|renderer|underflow|Audio|Video/i.test(p.name)).forEach((p) => media.push({ at: Date.now() - t0, kind: `prop.${p.name}`, d: p.value.slice(0, 200) })),
  );
  await cdp.send("Media.enable");
  const pageAt = await page.evaluate(() => Math.round(performance.now()));
  await cdp.send("Emulation.setCPUThrottlingRate", { rate });
  await page.waitForTimeout(12_000);
  // panel: open the Options panel and switch nothing. ui: switch from the panel, as a viewer does.
  if (mode === "panel" || mode === "ui") {
    await page.mouse.move(600, 400);
    await page.mouse.move(640, 420);
    await page.getByRole("button", { name: "Options" }).click();
    await page.getByRole("group", { name: "Audio" }).waitFor();
  }
  const switchNode = Date.now() - t0;
  const switchAt = await video.evaluate(
    (el: HTMLVideoElement & { hls?: HlsLike }, how: string) => {
      const hls = el.hls!;
      const w = window as unknown as { __log: { at: number; e: string; d: string }[]; __samples: Sample[] };
      const at = w.__samples.at(-1)?.at ?? 0;
      const target = hls.audioTracks.findIndex((t) => t.lang === "es");
      w.__log.push({ at, e: "switch", d: `${how} -> ${target}` });
      if (how === "option") hls.setAudioOption({ name: hls.audioTracks[target].name, lang: "es" });
      else if (how === "next") hls.nextAudioTrack = target;
      else if (how === "track") hls.audioTrack = target;
      return at;
    },
    mode,
  );
  if (mode === "ui") await page.getByRole("group", { name: "Audio" }).getByRole("button", { name: "Spanish" }).click();
  await page.waitForTimeout(10_000);
  await cdp.send("Emulation.setCPUThrottlingRate", { rate: 1 });
  const out = await page.evaluate(() => {
    const w = window as unknown as { __log: unknown[]; __samples: Sample[] };
    return { log: w.__log, samples: w.__samples };
  });
  const s = out.samples;
  const window6 = (from: number, to: number) => {
    const a = s.find((x) => x.at >= from)!;
    const b = [...s].reverse().find((x) => x.at <= to)!;
    return { frames: b.frames - a.frames, dropped: b.dropped - a.dropped, played: +(b.t - a.t).toFixed(2) };
  };
  const summary = { mode, rate, switchAt, cpus: (await import("node:os")).cpus().length, before: window6(switchAt - 6_000, switchAt), after: window6(switchAt, switchAt + 6_000), later: window6(switchAt + 4_000, switchAt + 10_000) };
  mkdirSync(evidence, { recursive: true });
  writeFileSync(path.join(evidence, `${mode}-${rate}x.json`), JSON.stringify({ summary, ...out, media, mediaSwitchAt: switchNode, pageAt }, null, 1));
  console.log(JSON.stringify(summary));
});

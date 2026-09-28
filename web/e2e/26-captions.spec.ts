import { mkdirSync, readFileSync, writeFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { expect, test } from "./fixture";

// Opt-in: E2E_CAPTIONS=1 with E2E_SOURCE set to a broadcast recording that
// carries CEA-608 and BROADWAVE_ENCODER=software. A libx264 encode keeps the
// 608 bytes in its SEI, and hls.js turns those into its own in-band track on
// the same video element. That track is the oracle for the server's WebVTT.
const here = path.dirname(fileURLToPath(import.meta.url));
const evidence = path.resolve(here, "../../.evidence/f1");
const hlsPath = path.resolve(here, "../node_modules/hls.js/dist/hls.min.js");
const playMs = Number(process.env.E2E_CAPTIONS_MS || 90_000);
const rendition = process.env.E2E_CAPTIONS_RENDITION || "720.aac2.broadcast";
// The oracle is late: bwdif holds the 608 bytes about 67 ms (ffmpeg on the
// same source: copy 0, scale only +17 ms, bwdif +67 ms). Ours follow the broadcast.
const medianLimitMs = 100;
const maxLimitMs = 200;
// The sample loops every 10 s or so; the same text a loop away is another cue.
const pairWindowMs = 4_000;
const inbandLabel = "INBAND-CC1";

type RawCue = { label: string; kind: string; start: number; end: number; text: string; line: string };
type Cue = { start: number; end: number; text: string; raw: string };
type Pair = { text: string; ours: number; inband: number; diffMs: number; oursEnd: number; inbandEnd: number };

function norm(text: string) {
  return text
    .replace(/<[^>]+>/g, "")
    .replace(/[‘’]/g, "'")
    .replace(/[“”]/g, '"')
    .replace(/\s+/g, " ")
    .trim();
}

function channelId() {
  const runtime = JSON.parse(readFileSync(path.join(here, ".run/runtime.json"), "utf8")) as { channels: { id: number; name: string }[] };
  // Setup leaves WDAF running at its own rendition; KCTV starts the one asked for.
  const name = process.env.E2E_CAPTIONS_CHANNEL || "KCTV";
  const found = runtime.channels.find((item) => item.name === name);
  if (!found) throw new Error(`no ${name}`);
  return found.id;
}

// hls.js writes one cue per caption row. Rows that share a start and end are one caption.
function groupRows(rows: RawCue[]): Cue[] {
  const byTime = new Map<string, RawCue[]>();
  for (const row of rows) {
    const key = `${row.start.toFixed(3)}|${row.end.toFixed(3)}`;
    const list = byTime.get(key) ?? [];
    list.push(row);
    byTime.set(key, list);
  }
  const out: Cue[] = [];
  for (const list of byTime.values()) {
    list.sort((a, b) => Number(a.line) - Number(b.line));
    const raw = list.map((row) => row.text).join("\n");
    out.push({ start: list[0].start, end: list[0].end, text: norm(raw), raw });
  }
  return out.sort((a, b) => a.start - b.start);
}

// Each .vtt repeats the caption still on screen, clipped to its own span.
// Pieces of one caption that touch are one caption; the gap between them is kept.
function joinPieces(pieces: Cue[]): { cues: Cue[]; gaps: number[] } {
  const out: Cue[] = [];
  const gaps: number[] = [];
  for (const piece of pieces) {
    const last = out.at(-1);
    if (last && last.text === piece.text && piece.start - last.end > -0.01 && piece.start - last.end < 0.1) {
      gaps.push(Math.round((piece.start - last.end) * 1000));
      last.end = Math.max(last.end, piece.end);
      continue;
    }
    out.push({ ...piece });
  }
  return { cues: out, gaps };
}

// A cue on screen across a segment boundary is written into both .vtt files.
function dedupe(rows: RawCue[]): { cues: Cue[]; duplicates: number } {
  const byStart = new Map<string, Cue>();
  let duplicates = 0;
  for (const row of rows) {
    const text = norm(row.text);
    const key = `${row.start.toFixed(3)}|${text}`;
    const had = byStart.get(key);
    if (had) {
      duplicates++;
      had.end = Math.max(had.end, row.end);
      continue;
    }
    byStart.set(key, { start: row.start, end: row.end, text, raw: row.text });
  }
  return { cues: [...byStart.values()].sort((a, b) => a.start - b.start), duplicates };
}

function median(values: number[]) {
  if (!values.length) return null;
  const sorted = [...values].sort((a, b) => a - b);
  const mid = Math.floor(sorted.length / 2);
  return sorted.length % 2 ? sorted[mid] : (sorted[mid - 1] + sorted[mid]) / 2;
}

test.describe("live captions", () => {
  test.skip(process.env.E2E_CAPTIONS !== "1", "set E2E_CAPTIONS=1 to measure caption timing");

  test("WebVTT cues land where the in-band 608 cues do", async ({ page, request }, testInfo) => {
    test.setTimeout(playMs + 180_000);
    mkdirSync(evidence, { recursive: true });
    const logs: string[] = [];
    page.on("console", (msg) => {
      if (msg.type() === "error" || msg.type() === "warning" || msg.text().startsWith("[cc]")) logs.push(`${msg.type()}: ${msg.text()}`);
    });
    page.on("pageerror", (err) => logs.push(`pageerror: ${err.message}`));

    const id = channelId();
    const watched = await request.post("/api/v1/watch", { data: { channelId: id, rendition, confirmLive: true }, timeout: 60_000 });
    expect(watched.ok(), await watched.text()).toBe(true);
    const session = (await watched.json()) as { playlist: string; rendition: string; encoder: string; boot?: string; stream: unknown };
    expect(session.rendition, JSON.stringify(session.stream)).toBe(rendition);
    const dir = session.playlist.replace(/index\.m3u8$/, "");

    // Let hls.js read the 608 bytes in the picture as well.
    let mainBody = "";
    await page.route(`**${dir}main.m3u8`, async (route) => {
      const res = await route.fetch();
      const body = await res.text();
      mainBody = body;
      await route.fulfill({ response: res, body: body.replace(",CLOSED-CAPTIONS=NONE", "") });
    });

    await page.goto("/api/v1/health");
    await page.evaluate(() => {
      document.documentElement.innerHTML = '<head></head><body style="margin:0;background:#000"><video id="v" muted playsinline autoplay style="width:1280px;height:720px"></video></body>';
    });
    await page.addScriptTag({ path: hlsPath });

    await page.evaluate(
      ({ src, label }) => {
        type HlsCtor = {
          new (config: Record<string, unknown>): {
            on(event: string, fn: (event: string, data: Record<string, unknown>) => void): void;
            loadSource(src: string): void;
            attachMedia(media: HTMLMediaElement): void;
            subtitleTrack: number;
            subtitleDisplay: boolean;
            subtitleTracks: unknown[];
          };
          Events: Record<string, string>;
        };
        const Hls = (window as unknown as { Hls: HlsCtor }).Hls;
        const video = document.getElementById("v") as HTMLVideoElement;
        const seen = new Map<string, { label: string; kind: string; start: number; end: number; text: string; line: string }>();
        const errors: unknown[] = [];
        // hls.js resets its 608 parser whenever the next load is not the next
        // whole segment, so a part-by-part (LL-HLS) load loses every caption
        // built across a part. Whole segments keep the oracle intact.
        // Native rendering holds in-band cues until a "video" buffer append,
        // and this fMP4 is one muxed "audiovideo" buffer, so they never land.
        // Both tracks are read from CUES_PARSED instead, on the same timeline.
        const hls = new Hls({ enableCEA708Captions: true, captionsTextTrack1Label: label, renderTextTracksNatively: false, lowLatencyMode: false });
        hls.on(Hls.Events.CUES_PARSED, (_e, data) => {
          const kind = String(data.type);
          const name = kind === "captions" ? label : "English CC";
          for (const cue of data.cues as VTTCue[]) {
            const key = `${name}|${cue.startTime.toFixed(4)}|${cue.endTime.toFixed(4)}|${cue.text}|${String(cue.line)}`;
            if (!seen.has(key)) seen.set(key, { label: name, kind, start: cue.startTime, end: cue.endTime, text: cue.text, line: String(cue.line) });
          }
        });
        hls.on(Hls.Events.ERROR, (_e, data) => {
          errors.push({ type: data.type, details: data.details, fatal: data.fatal, reason: String(data.reason ?? data.error ?? ""), url: (data.frag as { url?: string } | undefined)?.url ?? (data.context as { url?: string } | undefined)?.url ?? "" });
          console.log(`[cc] hls error ${String(data.type)} ${String(data.details)} fatal=${String(data.fatal)}`);
        });
        const userdata = { events: 0, samples: 0, first: [] as unknown[] };
        hls.on(Hls.Events.FRAG_PARSING_USERDATA, (_e, data) => {
          const samples = (data.samples as { pts: number; bytes?: Uint8Array; type?: number }[]) ?? [];
          userdata.events++;
          userdata.samples += samples.length;
          if (userdata.first.length < 5 && samples.length) {
            const frag = data.frag as { sn: number; start: number };
            userdata.first.push({ sn: frag.sn, fragStart: frag.start, n: samples.length, pts: samples[0].pts, lastPts: samples.at(-1)!.pts, bytes: samples[0].bytes?.length ?? 0 });
          }
        });
        const pick = () => {
          if (hls.subtitleTracks.length && hls.subtitleTrack !== 0) hls.subtitleTrack = 0;
          hls.subtitleDisplay = true;
        };
        hls.on(Hls.Events.MANIFEST_PARSED, () => {
          pick();
          void video.play();
        });
        hls.on(Hls.Events.SUBTITLE_TRACKS_UPDATED, pick);
        hls.on(Hls.Events.MEDIA_ATTACHED, pick);
        window.setInterval(pick, 1000);
        hls.loadSource(src);
        hls.attachMedia(video);
        const sweep = () => {
          for (const track of Array.from(video.textTracks)) {
            if (track.mode === "disabled") continue;
            for (const cue of Array.from(track.cues ?? [])) {
              const c = cue as VTTCue;
              const key = `${track.label}|${c.startTime.toFixed(4)}|${c.endTime.toFixed(4)}|${c.text}|${String(c.line)}`;
              if (!seen.has(key)) seen.set(key, { label: track.label, kind: track.kind, start: c.startTime, end: c.endTime, text: c.text, line: String(c.line) });
            }
          }
        };
        window.setInterval(sweep, 250);
        (window as unknown as { __cc: unknown }).__cc = {
          seen,
          errors,
          userdata,
          sweep,
          hls,
          tracks: () => Array.from(video.textTracks).map((t) => ({ label: t.label, kind: t.kind, mode: t.mode, cues: t.cues?.length ?? null })),
        };
      },
      { src: `${dir}main.m3u8`, label: inbandLabel },
    );

    await expect
      .poll(() => page.evaluate(() => { const v = document.getElementById("v") as HTMLVideoElement; return v.videoWidth > 0 && v.currentTime > 0.5; }), { timeout: 60_000 })
      .toBe(true);
    const startedAt = await page.evaluate(() => (document.getElementById("v") as HTMLVideoElement).currentTime);
    await page.waitForTimeout(playMs);

    const collected = await page.evaluate(() => {
      const cc = (window as unknown as {
        __cc: { seen: Map<string, unknown>; errors: unknown[]; userdata: unknown; sweep: () => void; tracks: () => unknown; hls: { subtitleTracks: unknown[]; subtitleTrack: number } };
      }).__cc;
      cc.sweep();
      const video = document.getElementById("v") as HTMLVideoElement;
      const q = video.getVideoPlaybackQuality();
      const buffered = [];
      for (let i = 0; i < video.buffered.length; i++) buffered.push([video.buffered.start(i), video.buffered.end(i)]);
      return {
        cues: [...cc.seen.values()],
        errors: cc.errors,
        userdata: cc.userdata,
        tracks: cc.tracks(),
        subtitleTracks: cc.hls.subtitleTracks.length,
        subtitleTrack: cc.hls.subtitleTrack,
        currentTime: video.currentTime,
        paused: video.paused,
        buffered,
        dropped: q.droppedVideoFrames,
        total: q.totalVideoFrames,
        width: video.videoWidth,
        height: video.videoHeight,
      };
    });

    const captionsM3u8 = await (await request.get(`${dir}captions.m3u8`)).text();
    const vttNames = captionsM3u8.split("\n").filter((line) => line.endsWith(".vtt"));
    const vtts: Record<string, string> = {};
    for (const name of vttNames.slice(-3, -1)) vtts[name] = await (await request.get(`${dir}${name}`)).text();
    const index = await (await request.get(`${dir}index.m3u8`)).text();

    await request.post(`/api/v1/watch/${id}/stop`, { data: { rendition, boot: session.boot ?? "" } });

    const raw = collected.cues as RawCue[];
    const oursRaw = raw.filter((cue) => cue.label !== inbandLabel);
    const inbandRaw = raw.filter((cue) => cue.label === inbandLabel);
    const { cues: pieces, duplicates } = dedupe(oursRaw);
    const { cues: ours, gaps } = joinPieces(pieces);
    const inband = groupRows(inbandRaw);

    // Paint-on text grows a character at a time: hls.js writes the final text
    // from the first character, we write each stage. Any text that another
    // text extends is paint-on and is reported apart from the pop-on pairs.
    const allTexts = new Set([...ours, ...inband].map((cue) => cue.text));
    const paintOn = (text: string) => [...allTexts].some((o) => o !== text && (o.startsWith(text) || text.startsWith(o)));
    const pairs: Pair[] = [];
    const unpaired: Cue[] = [];
    for (const cue of ours) {
      if (paintOn(cue.text)) continue;
      let best: Cue | null = null;
      for (const other of inband) {
        if (other.text !== cue.text) continue;
        if (!best || Math.abs(other.start - cue.start) < Math.abs(best.start - cue.start)) best = other;
      }
      if (best && Math.abs(best.start - cue.start) * 1000 <= pairWindowMs) {
        pairs.push({
          text: cue.text,
          ours: +cue.start.toFixed(3),
          inband: +best.start.toFixed(3),
          diffMs: Math.round((cue.start - best.start) * 1000),
          oursEnd: +cue.end.toFixed(3),
          inbandEnd: +best.end.toFixed(3),
        });
      } else {
        unpaired.push(cue);
      }
    }

    // Texts are compared where both tracks have cues, a second in from each edge.
    const lo = Math.max(ours[0]?.start ?? Infinity, inband[0]?.start ?? Infinity) + 1;
    const hi = Math.min(ours.at(-1)?.end ?? -Infinity, inband.at(-1)?.end ?? -Infinity) - 1;
    const inside = (cue: Cue) => cue.start >= lo && cue.end <= hi;
    const oursTexts = new Set(ours.filter(inside).map((cue) => cue.text));
    const inbandTexts = new Set(inband.filter(inside).map((cue) => cue.text));
    // Paint-on text grows a character at a time; the two decoders cut it at
    // different points. A text that is a prefix of one on the other side is
    // reported, not failed.
    const growing = (text: string, other: Set<string>) => [...other].some((o) => o !== text && (o.startsWith(text) || text.startsWith(o)));
    const partial = {
      ours: [...oursTexts].filter((text) => !inbandTexts.has(text) && growing(text, inbandTexts)),
      inband: [...inbandTexts].filter((text) => !oursTexts.has(text) && growing(text, oursTexts)),
    };
    const onlyOurs = [...oursTexts].filter((text) => !inbandTexts.has(text) && !partial.ours.includes(text));
    const onlyInband = [...inbandTexts].filter((text) => !oursTexts.has(text) && !partial.inband.includes(text));
    const partialPairs = ours
      .filter((cue) => paintOn(cue.text))
      .map((cue) => {
        const near = inband.filter((o) => o.text.startsWith(cue.text) || cue.text.startsWith(o.text)).sort((a, b) => Math.abs(a.start - cue.start) - Math.abs(b.start - cue.start))[0];
        return { ours: cue.text, oursStart: +cue.start.toFixed(3), inband: near?.text ?? null, inbandStart: near ? +near.start.toFixed(3) : null, diffMs: near ? Math.round((cue.start - near.start) * 1000) : null };
      });
    // Inside the shared window, an in-band caption with no WebVTT cue near it.
    const missed = inband.filter(inside).filter((cue) => !paintOn(cue.text)).filter((cue) => !pairs.some((pair) => Math.abs(pair.inband - cue.start) < 0.001 && pair.text === cue.text));

    const diffs = pairs.map((pair) => Math.abs(pair.diffMs));
    const summary = {
      rendition: session.rendition,
      encoder: session.encoder,
      startedAt,
      playMs,
      video: { currentTime: collected.currentTime, paused: collected.paused, buffered: collected.buffered, dropped: collected.dropped, total: collected.total, width: collected.width, height: collected.height },
      tracks: collected.tracks,
      subtitleTracks: collected.subtitleTracks,
      userdata: collected.userdata,
      subtitleTrack: collected.subtitleTrack,
      counts: { oursRaw: oursRaw.length, oursPieces: pieces.length, ours: ours.length, oursDuplicates: duplicates, inbandRows: inbandRaw.length, inband: inband.length, pairs: pairs.length, unpaired: unpaired.length, missed: missed.length },
      medianAbsMs: median(diffs),
      maxAbsMs: diffs.length ? Math.max(...diffs) : null,
      medianSignedMs: median(pairs.map((pair) => pair.diffMs)),
      window: [lo, hi],
      onlyOurs,
      onlyInband,
      partial,
      partialPairs,
      boundaryGapsMs: gaps,
      pairs,
      unpaired,
      missed,
      ours,
      inband,
      raw,
      hlsErrors: collected.errors,
      console: logs,
      mainM3u8: mainBody,
      indexM3u8: index,
      captionsM3u8,
      vtts,
    };
    const json = JSON.stringify(summary, null, 2);
    writeFileSync(path.join(evidence, "cues.json"), json);
    await testInfo.attach("cues.json", { body: json, contentType: "application/json" });
    await testInfo.attach("captions.m3u8", { body: captionsM3u8, contentType: "text/plain" });
    for (const [name, body] of Object.entries(vtts)) await testInfo.attach(name, { body, contentType: "text/vtt" });
    const gapCounts: Record<string, number> = {};
    for (const gap of gaps) gapCounts[gap] = (gapCounts[gap] ?? 0) + 1;
    process.stdout.write(
      `pieces joined: ${gaps.length}, gap ms histogram ${JSON.stringify(gapCounts)}\npartial: ${JSON.stringify(partialPairs.slice(0, 6))}\n` +
      `captions: ${pairs.length} pairs, median |d| ${summary.medianAbsMs} ms, max ${summary.maxAbsMs} ms, signed median ${summary.medianSignedMs} ms; ours ${ours.length} (${duplicates} dup), in-band ${inband.length}, unpaired ${unpaired.length}, missed ${missed.length}\n` +
        `signed diffs ms: ${JSON.stringify(pairs.map((pair) => pair.diffMs))}\n` +
        `only ours: ${JSON.stringify(onlyOurs)}\nonly in-band: ${JSON.stringify(onlyInband)}\n` +
        pairs
          .slice(0, 10)
          .map((pair) => `  ${JSON.stringify(pair.text)} ours ${pair.ours} in-band ${pair.inband} diff ${pair.diffMs} ms`)
          .join("\n") +
        "\n",
    );

    expect(oursRaw.length, "the WebVTT track has cues").toBeGreaterThan(0);
    expect(inband.length, "hls.js found in-band 608 cues").toBeGreaterThan(0);
    expect(pairs.length).toBeGreaterThanOrEqual(5);
    expect(summary.medianAbsMs!).toBeLessThanOrEqual(medianLimitMs);
    expect(summary.maxAbsMs!).toBeLessThanOrEqual(maxLimitMs);
    expect(onlyOurs, "texts only in the WebVTT").toEqual([]);
    expect(onlyInband, "texts only in-band").toEqual([]);
    // A caption held across segments must not blink off at the boundary.
    expect(gaps.filter((gap) => gap > 2), "gaps at segment boundaries").toEqual([]);
  });
});

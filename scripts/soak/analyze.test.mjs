import assert from "node:assert/strict";
import test from "node:test";
import {
  compareEnds,
  escapeHtml,
  noteFetch,
  noteStall,
  noteTransportFailure,
  noteWindow,
  parsePlaylist,
  passed,
  quietLeak,
  renderReport,
  runningProcesses,
  recordingIdsToStop,
  settled,
  stopRequest,
  syncSpread,
  verdicts,
} from "./analyze.mjs";

const list = [
  "#EXTM3U",
  "#EXT-X-VERSION:9",
  "#EXT-X-TARGETDURATION:2",
  "#EXT-X-MEDIA-SEQUENCE:4",
  '#EXT-X-MAP:URI="init.mp4"',
  "#EXT-X-DISCONTINUITY",
  "#EXT-X-PROGRAM-DATE-TIME:2026-10-11T18:00:00.000Z",
  "#EXTINF:2.000,",
  "seg00004.m4s",
  "#EXT-X-GAP",
  "#EXT-X-PROGRAM-DATE-TIME:2026-10-11T18:00:02.000Z",
  "#EXTINF:2.000,",
  "seg00005.m4s",
].join("\n");

test("a playlist yields the sequence, the map, and each segment's time", () => {
  const parsed = parsePlaylist(list);
  assert.equal(parsed.target, 2);
  assert.equal(parsed.sequence, 4);
  assert.equal(parsed.map, "init.mp4");
  assert.equal(parsed.segments.length, 2);
  assert.equal(parsed.segments[0].msn, 4);
  assert.equal(parsed.segments[0].discontinuity, true);
  assert.equal(parsed.segments[0].uri, "seg00004.m4s");
  assert.equal(parsed.segments[1].gap, true);
  assert.equal(parsed.segments[1].pdt, Date.parse("2026-10-11T18:00:02.000Z"));
  assert.equal(parsed.edgePdt, parsed.segments[1].pdt + 2000);
});

test("a sliding window is quiet, and a skip or a gap is a drop once", () => {
  const first = parsePlaylist("#EXT-X-MEDIA-SEQUENCE:0\n#EXTINF:2,\nseg0.m4s\n#EXTINF:2,\nseg1.m4s\n");
  const step = noteWindow(null, first);
  assert.deepEqual(step.events, []);
  const next = parsePlaylist("#EXT-X-MEDIA-SEQUENCE:1\n#EXTINF:2,\nseg1.m4s\n#EXTINF:2,\nseg2.m4s\n");
  const slid = noteWindow(step.state, next);
  assert.deepEqual(slid.events, []);
  const jumped = parsePlaylist("#EXT-X-MEDIA-SEQUENCE:6\n#EXTINF:2,\nseg6.m4s\n");
  const hole = noteWindow(slid.state, jumped);
  assert.equal(hole.events.length, 1);
  assert.equal(hole.events[0].kind, "drop");
  assert.match(hole.events[0].detail, /missed segments 3 through 5/);
  const again = noteWindow(hole.state, jumped);
  assert.deepEqual(again.events, []);

  const gapped = parsePlaylist("#EXT-X-MEDIA-SEQUENCE:7\n#EXT-X-GAP\n#EXTINF:2,\nseg7.m4s\n");
  const gap = noteWindow(again.state, gapped);
  assert.equal(gap.events.filter((event) => event.kind === "drop").length, 1);
  assert.match(gap.events[0].detail, /gap at segment 7/);
});

test("a gap already behind the edge is not counted again", () => {
  const late = noteWindow(
    { maxMsn: 1000 },
    {
      edgePdt: 3_000,
      segments: [
        { msn: 400, gap: true, discontinuity: true },
        { msn: 1001, gap: false, discontinuity: false },
      ],
    },
  );
  assert.deepEqual(late.events, []);
  assert.equal(late.state.maxMsn, 1001);
});

test("a playlist that starts its sequence over is a new timeline", () => {
  const restarted = noteWindow(
    { maxMsn: 500 },
    { edgePdt: 2_000, segments: [{ msn: 1, gap: false, discontinuity: false }] },
  );
  assert.equal(restarted.events[0].kind, "break");
  assert.match(restarted.events[0].detail, /restarted/);
  assert.equal(restarted.state.maxMsn, 1);
});

test("a discontinuity is a break, not a drop", () => {
  const parsed = parsePlaylist("#EXT-X-MEDIA-SEQUENCE:1\n#EXT-X-DISCONTINUITY\n#EXTINF:1,\none.m4s\n");
  const noted = noteWindow(null, parsed);
  assert.equal(noted.events.length, 1);
  assert.equal(noted.events[0].kind, "break");
});

test("a failed fetch is a drop and a good fetch is not", () => {
  assert.equal(noteFetch(200, "seg.m4s"), null);
  assert.deepEqual(noteFetch(404, "seg.m4s"), { kind: "drop", detail: "404 seg.m4s" });
});

test("a frozen edge stalls once, and pause or a new edge clears it", () => {
  const opts = { paused: false, quietUntil: 0, stallMs: 8000 };
  let step = noteStall(null, 1000, 0, opts);
  step = noteStall(step.state, 1000, 5000, opts);
  assert.equal(step.events.length, 0);
  step = noteStall(step.state, 1000, 8000, opts);
  assert.equal(step.events.length, 1);
  assert.equal(step.events[0].kind, "stall");
  step = noteStall(step.state, 1000, 20000, opts);
  assert.equal(step.events.length, 0);
  step = noteStall(step.state, 1000, 29000, { ...opts, paused: true });
  assert.equal(step.state.open, false);
  // The next tick is a second later, so the paused gap is not a stall.
  step = noteStall(step.state, 1000, 30000, opts);
  assert.equal(step.events.length, 0);
  step = noteStall(step.state, 5000, 31000, opts);
  step = noteStall(step.state, 5000, 40000, opts);
  assert.equal(step.events.length, 1);
  step = noteStall(step.state, 5000, 41000, { ...opts, quietUntil: 50000 });
  assert.equal(step.events.length, 0);
  assert.equal(step.state.open, false);
});

test("sync spread ignores a lone viewer and one who is seeking", () => {
  const spread = syncSpread([
    { channelId: 1, edgePdt: 1_000, aligned: true },
    { channelId: 1, edgePdt: 1_400, aligned: true },
    { channelId: 1, edgePdt: 9_000, aligned: false },
    { channelId: 2, edgePdt: 5_000, aligned: true },
  ]);
  assert.deepEqual(spread, [{ channelId: 1, n: 2, spreadMs: 400 }]);
});

test("growth that stays up fails, and a spike that returns does not", () => {
  const held = compareEnds([10, 12, 11], [80, 90, 85], { allow: 20, ratio: 1.5 });
  assert.equal(held.leaked, true);
  const back = compareEnds([10, 80, 40], [12, 11], { allow: 20, ratio: 1.5 });
  assert.equal(back.leaked, false);
  const room = compareEnds([100, 110], [130, 120], { allow: 40, ratio: 1.5 });
  assert.equal(room.leaked, false);
  assert.deepEqual(quietLeak([0, 0, 0]), { leaked: false, end: 0 });
  assert.equal(quietLeak([1, 2]).leaked, true);
  assert.equal(quietLeak([]).leaked, true);
  assert.equal(quietLeak([0, null]).leaked, true);
  assert.equal(compareEnds([], [4], { allow: 1, ratio: 2 }).leaked, true);
  assert.equal(runningProcesses({ ffmpegHub: null, ffmpegOS: 2 }), 2);
  assert.equal(runningProcesses({ ffmpegHub: null, ffmpegOS: null }), null);
  assert.equal(compareEnds([4], [], { allow: 1, ratio: 2 }).leaked, true);
});

test("the first stop still reads the quiet window", () => {
  assert.deepEqual(stopRequest(1), { halt: true, killChildren: false });
  assert.equal(stopRequest(2).killChildren, true);
});

test("the quiet window stops recordings this run started", () => {
  const before = { recordings: [{ id: 4, status: "recording" }, { id: 5, status: "complete" }] };
  const after = {
    recordings: [
      { id: 4, status: "recording" },
      { id: 9, status: "recording" },
      { id: 8, status: "complete" },
      { status: "recording" },
      { id: 0, status: "recording" },
    ],
  };
  assert.deepEqual(recordingIdsToStop({ before, after }), [9]);
  assert.deepEqual(recordingIdsToStop({ before: null, after }), [4, 9]);
  assert.deepEqual(recordingIdsToStop({ before, after: null }), []);
});

test("a quiet window is settled only when the counts were actually read", () => {
  assert.equal(settled({ viewers: 0, ffmpeg: 0, zombies: 0 }), true);
  assert.equal(settled({ viewers: 1, ffmpeg: 0, zombies: 0 }), false);
  assert.equal(settled({ viewers: 0, ffmpeg: 1, zombies: 0 }), false);
  assert.equal(settled({ viewers: 0, ffmpeg: 0, zombies: 1 }), false);
  assert.equal(settled({ viewers: null, ffmpeg: 0, zombies: 0 }), false);
  assert.equal(settled({ viewers: 0, ffmpeg: null, zombies: null }), false);
});

test("a viewer still counted after everyone left fails the run", () => {
  const rows = verdicts({
    serverDied: false,
    samples: [
      { phase: "base", goroutines: 20, fds: 10, heapAlloc: 1000, ffmpegHub: 0, ffmpegOS: 0, zombies: 0, viewers: 2 },
      { phase: "quiet", goroutines: 21, fds: 11, heapAlloc: 1100, ffmpegHub: 0, ffmpegOS: 0, zombies: 0, viewers: 4 },
    ],
    sync: [{ spreadMs: 100 }],
    events: [],
    limits: { stalls: 40, drops: 0 },
  });
  const viewers = rows.find((item) => item.name === "viewers");
  assert.equal(viewers.ok, false);
  assert.match(viewers.detail, /4 still counted/);
});

test("a transport error is a drop only after it is retried", () => {
  assert.equal(noteTransportFailure("seg00001.m4s", 0), null);
  const drop = noteTransportFailure("seg00001.m4s", 1);
  assert.equal(drop.kind, "drop");
});

test("a run that never sampled the quiet window fails", () => {
  const rows = verdicts({
    serverDied: false,
    samples: [{ phase: "base", goroutines: 20, fds: 10, heapAlloc: 1000, ffmpegHub: 0, ffmpegOS: 0, zombies: 0 }],
    sync: [],
    events: [],
    limits: { stalls: 40, drops: 0 },
  });
  for (const name of ["goroutines", "file descriptors", "heap", "ffmpeg", "zombies"]) {
    const row = rows.find((item) => item.name === name);
    assert.equal(row.ok, false, name);
  }
  const unread = verdicts({
    serverDied: false,
    samples: [
      { phase: "base", goroutines: 20, fds: 10, heapAlloc: 1000, ffmpegHub: 0, ffmpegOS: 0, zombies: 0 },
      { phase: "quiet", goroutines: 21, fds: 11, heapAlloc: 1100, ffmpegHub: null, ffmpegOS: null, zombies: null },
    ],
    sync: [],
    events: [],
    limits: { stalls: 40, drops: 0 },
  });
  for (const name of ["ffmpeg", "zombies"]) {
    assert.equal(unread.find((item) => item.name === name).ok, false, name);
  }
});

test("verdicts fail a quiet ffmpeg and a dead server, and the page escapes text", () => {
  const report = {
    serverDied: true,
    summary: "one hour",
    samples: [
      { phase: "base", goroutines: 40, fds: 20, heapAlloc: 1_000_000, ffmpegHub: 0, ffmpegOS: 0, zombies: 0 },
      { phase: "quiet", goroutines: 42, fds: 22, heapAlloc: 1_200_000, ffmpegHub: 1, ffmpegOS: 2, zombies: 0 },
    ],
    sync: [{ spreadMs: 200 }, { spreadMs: 400 }],
    events: [],
    limits: { stalls: 0, drops: 0 },
  };
  const rows = verdicts(report);
  const ffmpeg = rows.find((item) => item.name === "ffmpeg");
  const server = rows.find((item) => item.name === "server");
  const sync = rows.find((item) => item.name === "sync spread");
  assert.equal(ffmpeg.ok, false);
  assert.match(ffmpeg.detail, /2 still running/);
  assert.equal(server.ok, false);
  assert.equal(sync.ok, true);
  assert.equal(passed(rows), false);
  const html = renderReport({ ...report, verdicts: rows, summary: `<script>alert("x")</script>` });
  assert.equal(html.includes("<script>alert"), false);
  assert.match(html, /&lt;script&gt;/);
  assert.match(html, /Soak found a problem/);
  assert.equal(escapeHtml(`a&b<"c">`), "a&amp;b&lt;&quot;c&quot;&gt;");
});

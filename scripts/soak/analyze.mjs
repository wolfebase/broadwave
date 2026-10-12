// Pure soak scoring. The long run records samples and playlist observations.
// These functions decide stalls, drops, sync spread, and whether a quiet
// window still holds goroutines, files, or ffmpeg.

export function parsePlaylist(text) {
  const lines = String(text).split(/\r?\n/);
  let target = 0;
  let sequence = 0;
  let map = "";
  let msn = 0;
  let haveSeq = false;
  let pending = blank();
  const segments = [];
  for (const raw of lines) {
    const line = raw.trim();
    if (!line) continue;
    if (line.startsWith("#EXT-X-TARGETDURATION:")) target = number(line.slice(22));
    else if (line.startsWith("#EXT-X-MEDIA-SEQUENCE:")) {
      sequence = number(line.slice(22));
      if (!haveSeq) {
        msn = sequence;
        haveSeq = true;
      }
    } else if (line.startsWith("#EXT-X-MAP:")) {
      const found = line.match(/URI="([^"]+)"/);
      if (found) map = found[1];
    } else if (line === "#EXT-X-DISCONTINUITY") pending.discontinuity = true;
    else if (line === "#EXT-X-GAP") pending.gap = true;
    else if (line.startsWith("#EXT-X-PROGRAM-DATE-TIME:")) {
      const at = Date.parse(line.slice(25));
      pending.pdt = Number.isFinite(at) ? at : null;
    } else if (line.startsWith("#EXTINF:")) pending.dur = number(line.slice(8).split(",")[0]);
    else if (line.startsWith("#")) continue;
    else {
      segments.push({
        msn,
        uri: line,
        pdt: pending.pdt,
        dur: pending.dur,
        gap: pending.gap,
        discontinuity: pending.discontinuity,
      });
      msn += 1;
      pending = blank();
    }
  }
  const edge = segments.at(-1);
  const edgePdt = edge && edge.pdt != null ? edge.pdt + edge.dur * 1000 : null;
  return { target, sequence, map, segments, edgePdt };
}

function blank() {
  return { pdt: null, dur: 0, gap: false, discontinuity: false };
}

function number(raw) {
  const n = Number(raw);
  return Number.isFinite(n) ? n : 0;
}

// noteWindow reports a gap, a skipped segment number, or a discontinuity
// the first time it shows up. A window that slides forward by one is quiet.
// The live playlist keeps about 90 minutes, so a segment behind the edge
// must not be scored again on the next poll.
export function noteWindow(state, parsed) {
  const newest = parsed.segments.length ? parsed.segments.at(-1).msn : -1;
  // A rendition that starts its directory over begins again at a low number.
  if (state && state.maxMsn >= 0 && newest >= 0 && newest < state.maxMsn - 5) {
    const restarted = noteWindow(null, parsed);
    restarted.events.unshift({ kind: "break", detail: "playlist sequence restarted" });
    return restarted;
  }
  let max = state?.maxMsn ?? -1;
  const events = [];
  for (const seg of parsed.segments) {
    if (seg.msn <= max) continue;
    if (seg.gap) events.push({ kind: "drop", detail: `gap at segment ${seg.msn}` });
    if (seg.discontinuity) events.push({ kind: "break", detail: `discontinuity at segment ${seg.msn}` });
    if (max >= 0 && seg.msn > max + 1) {
      events.push({ kind: "drop", detail: `missed segments ${max + 1} through ${seg.msn - 1}` });
    }
    max = seg.msn;
  }
  return { events, state: { maxMsn: max, edgePdt: parsed.edgePdt } };
}

export function noteFetch(status, uri) {
  if (status >= 400) return { kind: "drop", detail: `${status} ${uri}` };
  return null;
}

// noteTransportFailure drops a playlist or segment only after a retry.
// One slow poll on a busy machine is not a missing segment.
export function noteTransportFailure(name, attempt) {
  if (attempt < 1) return null;
  return { kind: "drop", detail: `${name} failed` };
}

// noteStall emits one stall when the live edge stays put past stallMs.
// Pause and the grace after a channel change reset the clock.
export function noteStall(state, edgePdt, now, { paused = false, quietUntil = 0, stallMs = 8000 } = {}) {
  const prior = state || { edgePdt: null, since: now, open: false };
  if (paused || now < quietUntil || edgePdt == null) {
    return { events: [], state: { edgePdt, since: now, open: false } };
  }
  if (edgePdt !== prior.edgePdt) {
    return { events: [], state: { edgePdt, since: now, open: false } };
  }
  const since = prior.since ?? now;
  if (!prior.open && now - since >= stallMs) {
    return {
      events: [{ kind: "stall", detail: `live edge held for ${now - since} ms` }],
      state: { edgePdt, since, open: true },
    };
  }
  return { events: [], state: { edgePdt, since, open: prior.open } };
}

// syncSpread is the gap between aligned viewers on one channel.
// A paused or seeking viewer is left out.
export function syncSpread(points) {
  const groups = new Map();
  for (const point of points) {
    if (!point.aligned || point.edgePdt == null) continue;
    const list = groups.get(point.channelId) || [];
    list.push(point.edgePdt);
    groups.set(point.channelId, list);
  }
  const out = [];
  for (const [channelId, edges] of groups) {
    if (edges.length < 2) continue;
    out.push({ channelId, n: edges.length, spreadMs: Math.max(...edges) - Math.min(...edges) });
  }
  return out;
}

export function median(values) {
  if (!values.length) return null;
  const sorted = [...values].sort((a, b) => a - b);
  const mid = Math.floor(sorted.length / 2);
  if (sorted.length % 2) return sorted[mid];
  return (sorted[mid - 1] + sorted[mid]) / 2;
}

export function percentile(values, p) {
  if (!values.length) return null;
  const sorted = [...values].sort((a, b) => a - b);
  const index = Math.min(sorted.length - 1, Math.max(0, Math.ceil((p / 100) * sorted.length) - 1));
  return sorted[index];
}

// compareEnds flags a quiet window that is both much larger than the
// baseline and larger by a fixed allowance. A spike that comes back does not.
export function compareEnds(base, quiet, { allow, ratio }) {
  const b = median(base);
  const q = median(quiet);
  // No baseline or no quiet reading means the run cannot show it came back down.
  if (b == null || q == null) return { leaked: true, base: b, quiet: q };
  return { leaked: q > b + allow && q > b * ratio, base: b, quiet: q };
}

export function quietLeak(values, idle = 0) {
  if (!values.length) return { leaked: true, end: null };
  const end = values.at(-1);
  // A missing reading is not a zero. Zero would hide a process we failed to count.
  if (end == null || Number.isNaN(end)) return { leaked: true, end: null };
  return { leaked: end > idle, end };
}

// runningProcesses is the larger of the hub gauge and the operating-system
// count. Either one missing still counts the other. Both missing does not
// look like zero.
export function runningProcesses(sample) {
  const values = [sample?.ffmpegHub, sample?.ffmpegOS].filter((n) => n != null && !Number.isNaN(n));
  if (!values.length) return null;
  return Math.max(...values);
}

export function verdicts(report) {
  const limits = report.limits || {};
  const base = (report.samples || []).filter((sample) => sample.phase === "base");
  const quiet = (report.samples || []).filter((sample) => sample.phase === "quiet");
  const rows = [];
  const goroutines = compareEnds(
    base.map((sample) => sample.goroutines),
    quiet.map((sample) => sample.goroutines),
    { allow: limits.goroutineAllow ?? 40, ratio: limits.goroutineRatio ?? 1.5 },
  );
  rows.push(row("goroutines", !goroutines.leaked, ends(goroutines)));
  const fds = compareEnds(
    base.map((sample) => sample.fds).filter((n) => n != null && n >= 0),
    quiet.map((sample) => sample.fds).filter((n) => n != null && n >= 0),
    { allow: limits.fdAllow ?? 40, ratio: limits.fdRatio ?? 1.5 },
  );
  rows.push(row("file descriptors", !fds.leaked, ends(fds)));
  const heap = compareEnds(
    base.map((sample) => sample.heapAlloc),
    quiet.map((sample) => sample.heapAlloc),
    { allow: limits.heapAllow ?? 32 * 1024 * 1024, ratio: limits.heapRatio ?? 1.75 },
  );
  rows.push(row("heap", !heap.leaked, ends(heap)));
  const ffmpeg = quietLeak(quiet.map((sample) => runningProcesses(sample)));
  rows.push(row("ffmpeg", !ffmpeg.leaked, ffmpeg.end == null ? "no quiet sample" : `${ffmpeg.end} still running`));
  const zombies = quietLeak(quiet.map((sample) => sample.zombies));
  rows.push(row("zombies", !zombies.leaked, zombies.end == null ? "no quiet sample" : `${zombies.end} unreaped`));
  const viewers = quietLeak(quiet.map((sample) => sample.viewers));
  rows.push(row("viewers", !viewers.leaked, viewers.end == null ? "no quiet sample" : `${viewers.end} still counted`));
  const spreads = (report.sync || []).map((sample) => sample.spreadMs);
  const p95 = percentile(spreads, 95);
  rows.push(row("sync spread", p95 == null || p95 <= (limits.syncMs ?? 3000), p95 == null ? "no paired viewers" : `p95 ${Math.round(p95)} ms`));
  const stalls = (report.events || []).filter((event) => event.kind === "stall").length;
  const drops = (report.events || []).filter((event) => event.kind === "drop").length;
  rows.push(row("stalls", stalls <= (limits.stalls ?? 0), String(stalls)));
  rows.push(row("drops", drops <= (limits.drops ?? 0), String(drops)));
  rows.push(row("server", !report.serverDied, report.serverDied ? "exited" : "stayed up"));
  return rows;
}

function row(name, ok, detail) {
  return { name, ok, detail };
}

function ends(result) {
  if (result.base == null || result.quiet == null) return "not enough samples";
  return `baseline ${Math.round(result.base)} quiet ${Math.round(result.quiet)}`;
}

// The first stop ends the viewers and still reads the quiet window.
// A second stop kills the children, because the process may be exiting
// before that read.
export function stopRequest(hits) {
  return { halt: true, killChildren: hits >= 2 };
}

// A recording still in progress keeps an ffmpeg process. The quiet window
// stops the ones this run started. One that was already recording stays.
export function inProgressRecordingIds(body) {
  const rows = Array.isArray(body) ? body : body?.recordings;
  if (!Array.isArray(rows)) return [];
  const ids = [];
  for (const row of rows) {
    const id = row?.id;
    if (row?.status === "recording" && Number.isInteger(id) && id > 0) ids.push(id);
  }
  return ids;
}

// before is the list from before any viewer started. Null means that read
// failed. The catalog belongs to this run, so then every recording stops.
export function recordingIdsToStop({ before, after }) {
  const now = inProgressRecordingIds(after);
  if (before == null) return now;
  const prior = new Set(inProgressRecordingIds(before));
  return now.filter((id) => !prior.has(id));
}

// settled is true once nobody is counted and no encode or zombie remains.
// A missing count is not zero: the quiet window has to see the reading.
export function settled(snapshot) {
  if (snapshot?.viewers == null || snapshot?.ffmpeg == null) return false;
  if (snapshot.zombies != null && snapshot.zombies !== 0) return false;
  return snapshot.viewers === 0 && snapshot.ffmpeg === 0;
}

export function passed(rows) {
  return rows.every((item) => item.ok);
}

export function renderReport(report) {
  const rows = report.verdicts || verdicts(report);
  const ok = passed(rows);
  const title = ok ? "Soak passed" : "Soak found a problem";
  const samples = report.samples || [];
  const events = report.events || [];
  const shown = events.slice(-80);
  return `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>${escapeHtml(title)}</title>
<style>
  :root { color-scheme: dark; }
  body { margin: 0; font: 16px/1.45 ui-sans-serif, system-ui, sans-serif; background: #12141a; color: #e7e4dc; }
  main { max-width: 920px; margin: 0 auto; padding: 32px 20px 64px; }
  h1 { font-size: 32px; font-weight: 560; letter-spacing: -0.03em; margin: 0 0 8px; }
  h2 { font-size: 18px; font-weight: 560; margin: 32px 0 8px; }
  p, td, th { color: #c9c4b8; }
  .banner { display: inline-block; padding: 4px 10px; border-radius: 999px; font-size: 13px; letter-spacing: 0.04em; text-transform: uppercase; }
  .ok { background: #1c3d2e; color: #b7e4c7; }
  .bad { background: #4a2430; color: #f3c1c1; }
  table { width: 100%; border-collapse: collapse; }
  th, td { text-align: left; padding: 8px 10px; border-bottom: 1px solid #2a2e38; vertical-align: top; }
  th { font-size: 12px; letter-spacing: 0.06em; text-transform: uppercase; color: #8e897e; font-weight: 560; }
  .fail { color: #f0b4b4; }
  svg { width: 100%; height: 72px; background: #1a1d25; border-radius: 8px; }
  polyline { fill: none; stroke: #d7c4a3; stroke-width: 1.5; }
</style>
</head>
<body>
<main>
  <p class="banner ${ok ? "ok" : "bad"}">${ok ? "Passed" : "Failed"}</p>
  <h1>${escapeHtml(title)}</h1>
  <p>${escapeHtml(report.summary || "")}</p>
  <h2>Checks</h2>
  <table>
    <thead><tr><th>Check</th><th>Result</th><th>Detail</th></tr></thead>
    <tbody>
      ${rows.map((item) => `<tr><td>${escapeHtml(item.name)}</td><td class="${item.ok ? "" : "fail"}">${item.ok ? "ok" : "fail"}</td><td>${escapeHtml(item.detail)}</td></tr>`).join("\n")}
    </tbody>
  </table>
  <h2>Process</h2>
  ${chart(samples, "goroutines")}
  ${chart(samples, "heapAlloc")}
  ${chart(samples, "fds")}
  ${chart(samples, "ffmpegOS")}
  <h2>Sync spread</h2>
  <p>${escapeHtml(syncLine(report.sync || []))}</p>
  <h2>Recent events</h2>
  <table>
    <thead><tr><th>When</th><th>Who</th><th>What</th><th>Detail</th></tr></thead>
    <tbody>
      ${shown.map((event) => `<tr><td>${escapeHtml(event.at || "")}</td><td>${escapeHtml(event.who || "")}</td><td>${escapeHtml(event.kind)}</td><td>${escapeHtml(event.detail || "")}</td></tr>`).join("\n")}
    </tbody>
  </table>
</main>
</body>
</html>
`;
}

function syncLine(sync) {
  if (!sync.length) return "No two aligned viewers shared a channel.";
  const values = sync.map((sample) => sample.spreadMs);
  return `samples ${values.length}, median ${Math.round(median(values))} ms, p95 ${Math.round(percentile(values, 95))} ms, max ${Math.round(Math.max(...values))} ms`;
}

function chart(samples, key) {
  const values = samples.map((sample) => Number(sample[key])).filter((n) => Number.isFinite(n) && n >= 0);
  if (values.length < 2) return `<p>${escapeHtml(key)}: not sampled</p>`;
  const min = Math.min(...values);
  const max = Math.max(...values);
  const span = max - min || 1;
  const points = values
    .map((value, index) => {
      const x = (index / (values.length - 1)) * 600;
      const y = 68 - ((value - min) / span) * 60;
      return `${x.toFixed(1)},${y.toFixed(1)}`;
    })
    .join(" ");
  return `<p>${escapeHtml(key)} ${Math.round(min)} to ${Math.round(max)}</p><svg viewBox="0 0 600 72" role="img" aria-label="${escapeHtml(key)}"><polyline points="${points}"></polyline></svg>`;
}

export function escapeHtml(value) {
  return String(value)
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;");
}

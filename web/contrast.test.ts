import assert from "node:assert/strict";
import fs from "node:fs";
import { test } from "node:test";

// WCAG 2.1 relative luminance. The pairs below are the ones that render on a
// light frame, a white poster, or a faded category wash. Each ratio is the
// text against that background, after opacity and scrims.

const MIN = 4.5;
const WHITE: RGB = [255, 255, 255];

type RGB = [number, number, number];
type RGBA = { rgb: RGB; alpha: number };
type Stop = { alpha: number; at: number | null; unit: "%" | "px" | null };

const root = new URL(".", import.meta.url);
const read = (path: string) => fs.readFileSync(new URL(path, root), "utf8");
const base = read("src/theme/base.css");
const tokens = read("src/theme/tokens.css");
const guide = read("src/features/guide/guide.css");
const player = read("src/features/player/player.css");
const home = read("src/features/home/home.css");
const brand = read("src/theme/brand.css");
const multi = read("src/features/multiview/multiview.css");

function lin(channel: number) {
  const c = channel / 255;
  return c <= 0.04045 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4;
}

function lum(rgb: RGB) {
  return 0.2126 * lin(rgb[0]) + 0.7152 * lin(rgb[1]) + 0.0722 * lin(rgb[2]);
}

function contrast(a: RGB, b: RGB) {
  const [hi, lo] = lum(a) > lum(b) ? [lum(a), lum(b)] : [lum(b), lum(a)];
  return (hi + 0.05) / (lo + 0.05);
}

function mix(a: RGB, b: RGB, amount: number): RGB {
  return [
    a[0] * amount + b[0] * (1 - amount),
    a[1] * amount + b[1] * (1 - amount),
    a[2] * amount + b[2] * (1 - amount),
  ];
}

function over(fg: RGBA, bg: RGB): RGB {
  return mix(fg.rgb, bg, fg.alpha);
}

function atLeast(name: string, ratio: number) {
  assert.ok(ratio >= MIN, `${name} is ${ratio.toFixed(2)}:1`);
}

function rules(css: string) {
  const stripped = css.replace(/\/\*[\s\S]*?\*\//g, "");
  const found: { selector: string; body: string }[] = [];
  for (const match of stripped.matchAll(/([^{}]+)\{([^{}]*)\}/g)) {
    for (const selector of match[1].split(",")) {
      found.push({ selector: selector.trim().replace(/\s+/g, " "), body: match[2] });
    }
  }
  return found;
}

function body(css: string, selector: string) {
  const found = rules(css).find((rule) => rule.selector === selector);
  assert.ok(found, `missing ${selector}`);
  return found.body;
}

function prop(css: string, selector: string, name: string) {
  const found = body(css, selector).match(new RegExp(`${name}\\s*:\\s*([^;]+)`));
  assert.ok(found, `${selector} missing ${name}`);
  return found[1].trim();
}

function maybeProp(css: string, selector: string, name: string) {
  const found = rules(css).find((rule) => rule.selector === selector);
  return found?.body.match(new RegExp(`${name}\\s*:\\s*([^;]+)`))?.[1].trim() ?? null;
}

function token(name: string) {
  const fromBase = base.match(new RegExp(`${name}\\s*:\\s*([^;]+)`));
  const fromTokens = tokens.match(new RegExp(`${name}\\s*:\\s*([^;]+)`));
  const raw = (fromBase ?? fromTokens)?.[1]?.trim();
  assert.ok(raw, name);
  return parseColor(raw);
}

function parseColor(input: string): RGBA {
  const value = input.trim();
  const mixed = value.match(/^color-mix\(\s*in\s+srgb\s*,\s*(.*?)\s+([\d.]+)%\s*,\s*(.*)\)\s*$/);
  if (mixed) {
    const amount = Number(mixed[2]) / 100;
    const fore = parseColor(mixed[1]);
    const back = parseColor(mixed[3]);
    const alpha = fore.alpha * amount + back.alpha * (1 - amount);
    return {
      rgb: [0, 1, 2].map((index) =>
        alpha === 0 ? 0 : (fore.rgb[index] * fore.alpha * amount + back.rgb[index] * back.alpha * (1 - amount)) / alpha,
      ) as RGB,
      alpha,
    };
  }
  if (value.startsWith("var(")) {
    const name = value.match(/var\((--[\w-]+)/);
    assert.ok(name, value);
    return token(name[1]);
  }
  if (value === "transparent") return { rgb: [0, 0, 0], alpha: 0 };
  if (value.startsWith("#")) return parseHex(value);
  const rgb = value.match(/rgba?\(\s*([\d.]+)\s*,\s*([\d.]+)\s*,\s*([\d.]+)(?:\s*,\s*([\d.]+))?\s*\)/);
  assert.ok(rgb, value);
  return {
    rgb: [Number(rgb[1]), Number(rgb[2]), Number(rgb[3])],
    alpha: rgb[4] === undefined ? 1 : Number(rgb[4]),
  };
}

function parseHex(value: string): RGBA {
  let hex = value.slice(1);
  if (hex.length === 3 || hex.length === 4) hex = [...hex].map((part) => part + part).join("");
  return {
    rgb: [0, 2, 4].map((index) => Number.parseInt(hex.slice(index, index + 2), 16)) as RGB,
    alpha: hex.length >= 8 ? Number.parseInt(hex.slice(6, 8), 16) / 255 : 1,
  };
}

function argsOf(value: string, fn: string) {
  const out: string[] = [];
  let index = 0;
  while ((index = value.indexOf(`${fn}(`, index)) !== -1) {
    let depth = 0;
    for (let end = index; end < value.length; end++) {
      if (value[end] === "(") depth += 1;
      else if (value[end] === ")") {
        depth -= 1;
        if (depth === 0) {
          out.push(value.slice(index + fn.length + 1, end));
          index = end + 1;
          break;
        }
      }
    }
  }
  return out;
}

function splitArgs(value: string) {
  const out: string[] = [];
  let depth = 0;
  let start = 0;
  for (let index = 0; index < value.length; index++) {
    if (value[index] === "(") depth += 1;
    else if (value[index] === ")") depth -= 1;
    else if (value[index] === "," && depth === 0) {
      out.push(value.slice(start, index).trim());
      start = index + 1;
    }
  }
  out.push(value.slice(start).trim());
  return out;
}

function stops(gradient: string): Stop[] {
  const parts = splitArgs(gradient);
  const first = parts[0].endsWith("deg") || parts[0].startsWith("to ") ? 1 : 0;
  return parts.slice(first).map((part) => {
    const at = part.match(/(-?[\d.]+)(px|%)?\s*$/);
    const unit = at?.[2] === "%" ? "%" : at ? "px" : null;
    const color = parseColor(at && unit && part.slice(0, at.index).trim() !== "" ? part.slice(0, at.index).trim() : part);
    return { alpha: color.alpha, at: at && color !== null ? Number(at[1]) : null, unit: at ? unit : null };
  });
}

function alphaAt(list: Stop[], at: number, unit: "%" | "px") {
  const placed = list.map((stop) => ({ ...stop }));
  if (placed[0].at === null) placed[0] = { ...placed[0], at: 0, unit };
  const last = placed.length - 1;
  if (placed[last].at === null) placed[last] = { ...placed[last], at: unit === "%" ? 100 : at, unit };
  let previous = 0;
  for (let index = 0; index < placed.length; index++) {
    if (placed[index].at !== null) {
      previous = placed[index].at ?? previous;
      continue;
    }
    let next = index + 1;
    while (placed[next].at === null) next += 1;
    const gap = ((placed[next].at ?? previous) - previous) / (next - index + 1);
    placed[index] = { ...placed[index], at: previous + gap, unit };
    previous = placed[index].at ?? previous;
  }
  const ready = placed.map((stop) => ({ alpha: stop.alpha, at: stop.at ?? 0 }));
  if (at <= ready[0].at) return ready[0].alpha;
  for (let index = 1; index < ready.length; index++) {
    if (at <= ready[index].at) {
      const span = ready[index].at - ready[index - 1].at;
      const t = span === 0 ? 0 : (at - ready[index - 1].at) / span;
      return ready[index - 1].alpha + (ready[index].alpha - ready[index - 1].alpha) * t;
    }
  }
  return ready[ready.length - 1].alpha;
}

function linear(css: string, selector: string, direction: string) {
  const value = prop(css, selector, "background");
  const found = argsOf(value, "linear-gradient").find((gradient) => gradient.startsWith(direction));
  assert.ok(found, `${selector} missing ${direction} gradient`);
  return found;
}

function paint(color: RGBA, bg: RGB) {
  return contrast(over(color, bg), bg);
}

test("chip counts stay at least 4.5:1 on the chip and its hover", () => {
  const ink = parseColor(prop(base, ".chip-count", "color"));
  atLeast("chip", paint(ink, token("--color-surface1").rgb));
  atLeast("chip hover", paint(ink, token("--color-surface2").rgb));
});

test("faded guide text stays at least 4.5:1 on a category wash", () => {
  const page = token("--color-surface1").rgb;
  const surface = token("--color-surface2").rgb;
  const fade = Number(prop(guide, ".guide-cell.past", "opacity"));
  const score = parseColor(prop(guide, ".cell-score", "color"));
  const subtitle = parseColor(prop(guide, ".guide-cell.now .cell-sub", "color"));
  const fresh = parseColor(prop(guide, ".cell-new", "color"));
  for (const name of ["sports", "news", "movies", "kids", "series", "other"]) {
    const cat = token(`--cat-${name}`).rgb;
    const wash = mix(cat, surface, 0.16);
    const aired = mix(cat, wash, 0.22);
    for (const [label, ink] of [
      ["score", score],
      ["subtitle", subtitle],
      ["new", fresh],
    ] as const) {
      const fg = mix(over(ink, aired), page, fade);
      const bg = mix(aired, page, fade);
      atLeast(`${name} ${label}`, contrast(fg, bg));
    }
  }
});

test("multiview labels stay at least 4.5:1 on a white picture", () => {
  const veil = parseColor(prop(multi, ".mv-meta", "background"));
  const pill = over(veil, WHITE);
  const title = Number(prop(multi, ".mv-title", "opacity"));
  const detail = Number(prop(multi, ".mv-detail", "opacity"));
  atLeast("title", paint({ rgb: WHITE, alpha: title }, pill));
  atLeast("detail", paint({ rgb: WHITE, alpha: detail }, pill));
  atLeast("note", paint(parseColor(prop(multi, ".mv-note", "color")), pill));
  atLeast("note on black", paint(parseColor(prop(multi, ".mv-note", "color")), [0, 0, 0]));
});

test("player chrome stays at least 4.5:1 on a white frame", () => {
  const height = 1080;
  const top = stops(linear(player, ".stage-hud", "180deg"));
  const scrimAlpha = alphaAt(
    top.map((stop) => ({ ...stop, at: stop.unit === "%" ? ((stop.at ?? 0) / 100) * height : stop.at })),
    140,
    "px",
  );
  const scrim = over({ rgb: [0, 0, 0], alpha: scrimAlpha }, WHITE);
  for (const selector of [".stage-eyebrow", ".eyebrow-dim", ".stage-score"]) {
    atLeast(selector, paint(parseColor(prop(player, selector, "color")), scrim));
  }
  const glass = over(token("--color-glass-fill"), WHITE);
  for (const selector of [".time-read span", ".option-label", ".help-panel p", ".option-note"]) {
    atLeast(selector, paint(parseColor(prop(player, selector, "color")), glass));
  }
  atLeast("transport", paint(parseColor(prop(player, ".stage .text-btn", "color")), glass));
  const live = over(parseColor(prop(player, ".live-pill", "background")), scrim);
  atLeast("behind live", paint(token("--color-text"), live));
  const sync = over(parseColor(prop(player, ".sync-pill", "background")), scrim);
  atLeast("syncing", paint(parseColor(prop(player, ".sync-pill.syncing", "color")), sync));
  const locked = parseColor(prop(player, ".sync-pill.locked", "background"));
  atLeast("synced", paint(parseColor(prop(player, ".sync-pill.locked", "color")), over(locked, scrim)));
  const bar = stops(linear(player, ".mini-bar", "180deg"));
  const mini = over({ rgb: [0, 0, 0], alpha: alphaAt(bar, 32, "px") }, WHITE);
  atLeast("mini eyebrow", paint(parseColor(prop(player, ".mini-eyebrow", "color")), mini));
});

test("channel names and tuning text stay at least 4.5:1 on a white picture", () => {
  const glass = over(token("--color-glass-fill"), WHITE);
  atLeast("channel name", paint(parseColor(prop(player, ".mg-name", "color")), glass));
  atLeast("stats label", paint(parseColor(prop(player, ".info-panel dt", "color")), glass));
  // A white poster through brightness() is the bright case. No plate means the ink sits on that poster.
  const filter = prop(player, ".tuning-art", "filter");
  const bright = filter.match(/brightness\(\s*([\d.]+)\s*\)/);
  assert.ok(bright, filter);
  const art = mix(WHITE, [0, 0, 0], Number(bright[1]));
  const plateRaw = maybeProp(player, ".tuning-card", "background");
  const plate = plateRaw ? parseColor(plateRaw) : { rgb: [0, 0, 0] as RGB, alpha: 0 };
  const card = over(plate, art);
  atLeast("tuning name", paint(token("--color-text-secondary"), card));
  atLeast("tuning step", paint(parseColor(prop(player, ".tuning-step", "color")), card));
  atLeast("tuning show", paint(token("--color-text"), card));
});

test("artwork text stays at least 4.5:1 on a white poster", () => {
  const hero = alphaAt(stops(linear(home, ".hero-glow", "180deg")), 40, "%");
  const heroBg = over({ rgb: [0, 0, 0], alpha: hero }, WHITE);
  atLeast("hero secondary", paint(token("--color-text-secondary"), heroBg));
  atLeast("hero live", paint(token("--color-tally"), heroBg));
  const card = alphaAt(stops(linear(brand, ":root .now-card.has-art::before", "180deg")), 32, "%");
  const cardBg = over({ rgb: token("--color-canvas").rgb, alpha: card }, WHITE);
  atLeast("now card", paint(parseColor(prop(brand, ":root .now-card.has-art .nc-name", "color")), cardBg));
  const sheet = alphaAt(stops(linear(guide, ".ps-art::after", "180deg")), 100, "%");
  const sheetBg = over({ rgb: [0, 0, 0], alpha: sheet }, WHITE);
  atLeast("sheet name", paint(token("--color-text-secondary"), sheetBg));
  atLeast("sheet live", paint(token("--color-tally"), sheetBg));
});

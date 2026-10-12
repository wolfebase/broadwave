import assert from "node:assert/strict";
import fs from "node:fs";
import { test } from "node:test";
import { accents, applyTheme, parseAccent, parseTheme, resolveTheme } from "./src/theme/look.ts";

const MIN = 4.5;
type RGB = [number, number, number];

const read = (path: string) => fs.readFileSync(new URL(path, import.meta.url), "utf8");
const light = read("src/theme/light.css");
const base = read("src/theme/base.css");
const html = read("index.html");
const appearance = read("src/features/settings/Appearance.tsx");

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

function parseHex(value: string): RGB {
  const hex = value.replace("#", "");
  return [0, 2, 4].map((index) => Number.parseInt(hex.slice(index, index + 2), 16)) as RGB;
}

function mix(a: RGB, b: RGB, amount: number): RGB {
  return [0, 1, 2].map((index) => a[index] * amount + b[index] * (1 - amount)) as RGB;
}

function atLeast(name: string, ratio: number) {
  assert.ok(ratio >= MIN, `${name} is ${ratio.toFixed(2)}:1`);
}

function block(css: string, selector: string) {
  const escaped = selector.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
  const found = css.match(new RegExp(`${escaped}\\s*\\{([^}]*)\\}`));
  assert.ok(found, `missing ${selector}`);
  return found[1];
}

function props(body: string) {
  const out = new Map<string, string>();
  for (const part of body.split(";")) {
    const split = part.split(":");
    if (split.length < 2) continue;
    const name = split[0].trim();
    if (!name.startsWith("--") && name !== "color-scheme") continue;
    out.set(name, split.slice(1).join(":").trim());
  }
  return out;
}

function decl(body: string, name: string) {
  const found = props(body).get(name);
  assert.ok(found, name);
  return found;
}

const canvas = parseHex("#F4F6FA");
const surface1 = parseHex("#FFFFFF");
const surface2 = parseHex("#E7ECF2");
const surface3 = parseHex("#D5DCE6");
const darkSurface = parseHex("#1E2736");
const darkCanvas = parseHex("#070A10");
const surfaces = [canvas, surface1, surface2, surface3];

test("applying a theme updates the document chrome", () => {
  const content = new Map<string, string>();
  const root = { dataset: {} as Record<string, string> };
  const previous = globalThis.document;
  globalThis.document = {
    documentElement: root,
    querySelector(selector: string) {
      const name = selector.match(/name="([^"]+)"/)?.[1];
      if (!name) return null;
      return {
        setAttribute(_key: string, value: string) {
          content.set(name, value);
        },
      };
    },
  } as unknown as Document;
  try {
    applyTheme("light", "rose");
    assert.equal(root.dataset.theme, "light");
    assert.equal(root.dataset.accent, "rose");
    assert.equal(content.get("theme-color"), "#F4F6FA");
    assert.equal(content.get("color-scheme"), "light");
    assert.equal(content.get("apple-mobile-web-app-status-bar-style"), "default");
    applyTheme("dark", "blue");
    assert.equal(content.get("theme-color"), "#070A10");
    assert.equal(content.get("apple-mobile-web-app-status-bar-style"), "black-translucent");
  } finally {
    globalThis.document = previous;
  }
});

test("an unset choice is dark, and only System follows the device", () => {
  assert.equal(parseTheme(null), "dark");
  assert.equal(parseTheme("nope"), "dark");
  assert.equal(parseTheme("system"), "system");
  assert.equal(parseTheme("light"), "light");
  assert.equal(parseAccent(null), "blue");
  assert.equal(parseAccent("nope"), "blue");
  assert.equal(parseAccent("rose"), "rose");
  assert.equal(resolveTheme("system", true), "dark");
  assert.equal(resolveTheme("system", false), "light");
  assert.equal(resolveTheme("light", true), "light");
  assert.equal(resolveTheme("dark", false), "dark");
  assert.equal(accents.length, 6);
});

test("the light palette waits for a choice", () => {
  assert.doesNotMatch(light, /prefers-color-scheme/);
  assert.equal(props(block(light, 'html[data-theme="light"]')).get("color-scheme"), "light");
});

test("light text, status ink, and the default accent clear 4.5:1", () => {
  const vars = props(block(light, 'html[data-theme="light"]'));
  const text = parseHex(vars.get("--color-text") ?? "");
  const secondary = parseHex(vars.get("--color-text-secondary") ?? "");
  const tertiary = parseHex(vars.get("--color-text-tertiary") ?? "");
  const tally = parseHex(vars.get("--color-tally") ?? "");
  const success = parseHex(vars.get("--color-success") ?? "");
  const warning = parseHex(vars.get("--color-warning") ?? "");
  const link = parseHex(vars.get("--color-link") ?? "");
  const soft = vars.get("--color-tally-soft") ?? "";
  assert.match(soft, /^#[0-9A-Fa-f]{8}$/);
  const alpha = Number.parseInt(soft.slice(7, 9), 16) / 255;
  for (const bg of surfaces) {
    atLeast("text", contrast(text, bg));
    atLeast("secondary", contrast(secondary, bg));
    atLeast("tertiary", contrast(tertiary, bg));
    atLeast("tally", contrast(tally, bg));
    atLeast("success", contrast(success, bg));
    atLeast("warning", contrast(warning, bg));
    atLeast("link", contrast(link, bg));
    atLeast("tally on its wash", contrast(tally, mix(tally, bg, alpha)));
  }
  atLeast("white on tally", contrast([255, 255, 255], tally));
  atLeast("white on accent", contrast([255, 255, 255], link));
});

test("each accent clears 4.5:1 on the theme it is for", () => {
  for (const accent of accents) {
    const lightRule = block(light, `html[data-theme="light"][data-accent="${accent.id}"]`);
    const fill = parseHex(decl(lightRule, "--color-accent"));
    const on = parseHex(decl(lightRule, "--color-on-accent"));
    assert.equal(decl(lightRule, "--color-accent").toUpperCase(), accent.light);
    assert.equal(decl(lightRule, "--color-accent-soft").toUpperCase(), accent.lightSoft);
    assert.equal(decl(lightRule, "--color-link").toUpperCase(), accent.light);
    for (const bg of surfaces) atLeast(`${accent.id} link`, contrast(fill, bg));
    atLeast(`${accent.id} on accent`, contrast(on, fill));
    if (accent.id === "blue") continue;
    const darkRule = block(light, `html[data-theme="dark"][data-accent="${accent.id}"]`);
    const bright = parseHex(decl(darkRule, "--color-accent"));
    const darkOn = parseHex(decl(darkRule, "--color-on-accent"));
    assert.equal(decl(darkRule, "--color-accent").toUpperCase(), accent.dark);
    assert.equal(decl(darkRule, "--color-link").toUpperCase(), accent.dark);
    atLeast(`${accent.id} on dark card`, contrast(bright, darkSurface));
    atLeast(`${accent.id} on dark page`, contrast(bright, darkCanvas));
    atLeast(`${accent.id} label on accent`, contrast(darkOn, bright));
  }
});

test("guide ink stays at 4.5:1 on a light category wash", () => {
  const text = parseHex("#141820");
  const link = parseHex("#1D4ED8");
  const fade = 0.72;
  for (const hex of ["#2FBF71", "#3D9BFF", "#B36BFF", "#FFB020", "#5E7BA8", "#5A6474"]) {
    const wash = mix(parseHex(hex), surface2, 0.16);
    atLeast(`now ${hex}`, contrast(text, wash));
    atLeast(`new ${hex}`, contrast(link, wash));
    atLeast(`aired ${hex}`, contrast(mix(text, canvas, fade), mix(wash, canvas, fade)));
  }
  assert.match(light, /html\[data-theme="light"\] \.cell-score/);
  assert.match(light, /html\[data-theme="light"\] \.guide-cell\.now \.cell-sub/);
});

test("light page titles drop the clipped white gradient", () => {
  const rule = block(light, 'html[data-theme="light"] .page-header h1,\nhtml[data-theme="light"] .page-wrap > h1,\nhtml[data-theme="light"] .sports-page h1');
  assert.match(rule, /background:\s*none/);
  assert.match(rule, /-webkit-background-clip:\s*border-box/);
  assert.match(rule, /color:\s*var\(--color-text\)/);
});

test("hero and poster ink stays light on the dark scrim", () => {
  assert.match(block(light, 'html[data-theme="light"] .hero-num'), /rgba\(20, 24, 32, 0\.28\)/);
  assert.match(block(light, 'html[data-theme="light"] .ps-art .glass-icon,\nhtml[data-theme="light"] .ps-art .glass-icon:hover'), /#f5f7fa/);
  assert.match(block(light, 'html[data-theme="light"] .ps-art .glass-icon,\nhtml[data-theme="light"] .ps-art .glass-icon:hover'), /rgba\(0, 0, 0, 0\.72\)/);
  assert.match(block(light, "html[data-theme=\"light\"] .hero-glow"), /rgba\(0, 0, 0, 0\.9\)/);
  const scrim = mix([0, 0, 0], [255, 255, 255], 0.9);
  const thin = mix([0, 0, 0], [255, 255, 255], 0.62);
  const plate = mix([0, 0, 0], [255, 255, 255], 0.72);
  const ink = parseHex("#F5F7FA");
  atLeast("hero title", contrast(ink, scrim));
  atLeast("hero detail", contrast(parseHex("#AEB7C5"), scrim));
  atLeast("poster close", contrast(ink, plate));
  const live = block(light, 'html[data-theme="light"] .hero-cat,\nhtml[data-theme="light"] .ps-art .ch-badge-name,\nhtml[data-theme="light"] .hero .live-dot,\nhtml[data-theme="light"] .ps-art .live-dot');
  assert.match(live, /color:\s*#f5f7fa/);
  atLeast("hero live", contrast(ink, thin));
  const dot = block(light, 'html[data-theme="light"] .hero .live-dot-light,\nhtml[data-theme="light"] .ps-art .live-dot-light');
  assert.match(dot, /background:\s*#f5f7fa/);
  const poster = block(light, 'html[data-theme="light"] .ps-art::after');
  assert.match(poster, /rgba\(0, 0, 0, 0\.92\) 55%/);
  atLeast("poster name", contrast(ink, mix([0, 0, 0], [255, 255, 255], 0.92)));
});

test("the dark theme keeps today's link and status ink", () => {
  const root = block(base, ":root");
  assert.match(decl(root, "--color-link"), /color-mix\(\s*in srgb\s*,\s*var\(--color-accent\)\s+72%\s*,\s*#fff\s*\)/);
  assert.equal(decl(root, "--color-accent-ink"), "#9cbcff");
  assert.equal(decl(root, "--color-tally-ink"), "#ff8a82");
  assert.equal(decl(root, "--color-tally-soft-ink"), "#ffb3ad");
  assert.match(base, /color:\s*var\(--color-link\)/);
});

const brightCats = {
  sports: "#2FBF71",
  news: "#3D9BFF",
  movies: "#B36BFF",
  kids: "#FFB020",
  series: "#5E7BA8",
  other: "#5A6474",
} as const;

test("guide text stays at 4.5:1 after a light cell fades", () => {
  const past = block(light, 'html[data-theme="light"] .guide-cell.past:not(.dim)');
  const opacity = Number(past.match(/opacity:\s*([0-9.]+)/)?.[1]);
  assert.equal(opacity, 0.9);
  const text = parseHex("#141820");
  const secondary = parseHex("#3E4858");
  const link = parseHex("#1D4ED8");
  const page = parseHex("#FFFFFF");
  const surface = parseHex("#E7ECF2");
  const faded = (ink: RGB, bg: RGB) => contrast(mix(ink, page, opacity), mix(bg, page, opacity));
  atLeast("past subtitle", faded(secondary, surface));
  atLeast("past new", faded(link, surface));
  assert.match(block(light, 'html[data-theme="light"] .guide-cell.now .cell-new'), /color:\s*var\(--color-text\)/);
  assert.match(block(light, 'html[data-theme="light"] .guide-cell.empty .cell-frame'), /var\(--color-line\)/);
  for (const [name, hex] of Object.entries(brightCats)) {
    const cat = parseHex(hex);
    const wash = mix(cat, surface, 0.16);
    const aired = mix(cat, wash, 0.22);
    atLeast(`${name} new`, contrast(text, aired));
    atLeast(`${name} new faded`, faded(text, aired));
    const ink = parseHex(decl(block(light, `html[data-theme="light"] [data-cat="${name}"]`), "--cat-ink"));
    const bar = (bg: RGB) => {
      const ratio = contrast(ink, bg);
      assert.ok(ratio >= 3, `${name} bar is ${ratio.toFixed(2)}:1`);
    };
    bar(surface);
    bar(wash);
    bar(surface3);
    const track = mix(text, [255, 255, 255], 0.12);
    const fill = contrast(ink, track);
    assert.ok(fill >= 3, `${name} progress is ${fill.toFixed(2)}:1`);
  }
  assert.equal(block(light, 'html[data-theme="light"] .guide-cell::before').match(/opacity:\s*([0-9.]+)/)?.[1], "1");
  assert.match(block(light, 'html[data-theme="light"] .chip.on .chip-swatch'), /var\(--color-on-accent\)/);
  atLeast("selected swatch", contrast([255, 255, 255], link));
  assert.match(block(light, 'html[data-theme="light"] .progress-fill'), /var\(--cat-ink, var\(--cat-color\)\)/);
  assert.match(block(light, 'html[data-theme="light"] .hero .progress-fill,\nhtml[data-theme="light"] .now-card.has-art .progress-fill,\nhtml[data-theme="light"] .ps-art .progress-fill'), /var\(--cat-color, var\(--color-text\)\)/);
  assert.match(block(light, 'html[data-theme="light"] .rc-poster .progress-fill'), /#f5f7fa/);
  const plate = mix(parseHex("#070A10"), [255, 255, 255], 0.72);
  atLeast("poster progress", contrast(parseHex("#F5F7FA"), plate));
  const edge = parseHex("#6E7580");
  for (const [name, bg] of [["field", surface], ["page", canvas], ["card", surface1]] as const) {
    const ratio = contrast(edge, bg);
    assert.ok(ratio >= 3, `field edge on ${name} is ${ratio.toFixed(2)}:1`);
  }
  assert.match(light, /border-color:\s*#6e7580/);
  assert.match(block(light, 'html[data-theme="light"] .search-form'), /border-color:\s*#6e7580/);
  assert.match(block(light, 'html[data-theme="light"] :is(input, textarea)::placeholder'), /var\(--color-text-tertiary\)/);
  const textInk = parseHex("#141820");
  for (const [name, hex] of Object.entries(brightCats)) {
    const cat = parseHex(hex);
    const wash = mix(cat, surface, 0.16);
    const aired = mix(cat, wash, 0.1);
    const barInk = mix(parseHex(decl(block(light, `html[data-theme="light"] [data-cat="${name}"]`), "--cat-ink")), textInk, 0.85);
    const ratio = contrast(barInk, aired);
    assert.ok(ratio >= 3, `${name} now bar is ${ratio.toFixed(2)}:1`);
  }
  assert.match(block(light, 'html[data-theme="light"] .guide-cell.now::before'), /85%/);
});

test("a league name and a live edge clear the light card", () => {
  assert.match(block(light, 'html[data-theme="light"] .gc-league'), /var\(--color-success\)/);
  const success = parseHex("#0D6B3D");
  atLeast("league", contrast(success, surface1));
  const edge = mix(success, surface1, 0.75);
  for (const [name, bg] of [["card", surface1], ["page", canvas]] as const) {
    const ratio = contrast(edge, bg);
    assert.ok(ratio >= 3, `live edge on ${name} is ${ratio.toFixed(2)}:1`);
  }
  assert.match(block(light, 'html[data-theme="light"] .team-swatch'), /inset 0 0 0 1px var\(--color-line-strong\)/);
});

test("a pressed jump and the setup step follow the accent", () => {
  const pressed = block(light, 'html[data-theme="light"] .pill-btn[aria-pressed="true"],\nhtml[data-theme="light"] .setup-steps li.on button');
  assert.match(pressed, /background:\s*var\(--color-accent\)/);
  assert.match(pressed, /color:\s*var\(--color-on-accent\)/);
  assert.match(pressed, /border-color:\s*transparent/);
  const player = block(light, 'html[data-theme="light"] :is(.stage, .mv) .seg.on,\nhtml[data-theme="light"] :is(.stage, .mv) .btn.primary,\nhtml[data-theme="light"] :is(.stage, .mv) .pill-btn[aria-pressed="true"]');
  assert.match(player, /rgba\(255, 255, 255, 0\.92\)/);
  assert.match(player, /var\(--color-canvas\)/);
  assert.match(block(light, ".appearance .field > .segmented"), /justify-self:\s*start/);
});

test("an off switch shows its knob on a light track", () => {
  assert.match(block(light, 'html[data-theme="light"] .switch-row input'), /var\(--color-text\)\s+55%/);
  assert.match(block(light, 'html[data-theme="light"] .switch-row input:checked'), /var\(--color-success\)/);
  const track = mix(parseHex("#141820"), surface3, 0.55);
  const ratio = contrast([255, 255, 255], track);
  assert.ok(ratio >= 3, `switch knob is ${ratio.toFixed(2)}:1`);
});

test("the player and multiview keep the dark palette in a light page", () => {
  const stage = block(light, 'html[data-theme="light"] :is(.stage, .mv)');
  assert.equal(decl(stage, "color-scheme"), "dark");
  assert.match(stage, /color:\s*var\(--color-text\)/);
  assert.equal(decl(stage, "--color-text").toUpperCase(), "#F5F7FA");
  assert.equal(decl(stage, "--color-glass-fill").toUpperCase(), "#1A2130B8");
  assert.equal(decl(stage, "--color-surface3").toUpperCase(), "#1E2736");
  assert.equal(decl(stage, "--color-tally").toUpperCase(), "#FF3B30");
  assert.equal(decl(stage, "--color-accent").toUpperCase(), "#3D7BFF");
  assert.equal(decl(stage, "--brand-blue"), "var(--color-accent)");
  assert.match(decl(stage, "--focus-ring"), /var\(--color-canvas\)/);
  assert.match(block(light, 'html[data-theme="light"] :is(.stage, .mv) .glass-icon'), /rgba\(255, 255, 255, 0\.08\)/);
  assert.match(block(light, 'html[data-theme="light"] :is(.stage, .mv) .progress-fill'), /30%/);
  assert.match(block(light, 'html[data-theme="light"] :is(.stage, .mv) .mg-row.current .progress-fill'), /var\(--cat-ink, var\(--cat-color\)\)/);
  const glass = mix(parseHex("#1A2130"), [255, 255, 255], 0.72);
  const track = mix([255, 255, 255], glass, 0.1);
  const current = mix([255, 255, 255], glass, 0.92);
  for (const [name, hex] of Object.entries({ sports: "#2FBF71", news: "#3D9BFF", movies: "#B36BFF", kids: "#FFB020", series: "#5E7BA8", other: "#5A6474" })) {
    const fill = mix(parseHex(hex), [255, 255, 255], 0.3);
    const ratio = contrast(fill, track);
    assert.ok(ratio >= 3, `${name} player fill is ${ratio.toFixed(2)}:1`);
  }
  for (const hex of ["#268A57", "#337CC9", "#985DD9", "#9F7220", "#5E7BA8", "#5A6474"]) {
    const ratio = contrast(parseHex(hex), current);
    assert.ok(ratio >= 3, `current row fill ${hex} is ${ratio.toFixed(2)}:1`);
  }
  const selected = block(light, 'html[data-theme="light"] :is(.stage, .mv) .seg.on,\nhtml[data-theme="light"] :is(.stage, .mv) .btn.primary,\nhtml[data-theme="light"] :is(.stage, .mv) .pill-btn[aria-pressed="true"]');
  assert.match(selected, /rgba\(255, 255, 255, 0\.92\)/);
  assert.match(selected, /var\(--color-canvas\)/);
  const pill = mix([255, 255, 255], parseHex("#1A2130"), 0.92);
  atLeast("player segment", contrast(darkCanvas, pill));
  atLeast("player button", contrast(darkCanvas, parseHex("#E6EDFF")));
  assert.match(block(light, 'html[data-theme="light"] :is(.stage, .mv) .switch-row input::after'), /#fff/);
  assert.match(block(light, 'html[data-theme="light"] :is(.stage, .mv) .switch-row input:checked'), /var\(--color-success\)/);
  const knob = contrast([255, 255, 255], darkSurface);
  assert.ok(knob >= 3, `player switch knob is ${knob.toFixed(2)}:1`);
  assert.match(light, /html\[data-theme="light"\] :is\(\.stage, \.mv\) \*[\s\S]*rgba\(255, 255, 255, 0\.18\)/);
  for (const accent of accents) {
    if (accent.id === "blue") continue;
    const rule = block(light, `html[data-theme="light"][data-accent="${accent.id}"] :is(.stage, .mv)`);
    assert.equal(decl(rule, "--color-accent").toUpperCase(), accent.dark);
    assert.equal(decl(rule, "--color-link").toUpperCase(), accent.dark);
    atLeast(`${accent.id} on the player`, contrast(parseHex(accent.dark), darkSurface));
  }
});

test("the page paints the saved appearance before the app loads", () => {
  assert.match(html, /broadwave-theme/);
  assert.match(html, /broadwave-accent/);
  assert.match(html, /prefers-color-scheme: dark/);
  assert.match(html, /data-theme/);
  assert.match(html, /#F4F6FA/);
  assert.match(appearance, /role="radiogroup"/);
  assert.match(appearance, /copy\.settings\.appearance/);
  assert.match(appearance, /copy\.settings\.accent/);
});

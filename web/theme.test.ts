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
  const plate = mix([0, 0, 0], [255, 255, 255], 0.72);
  atLeast("hero title", contrast(parseHex("#F5F7FA"), scrim));
  atLeast("hero detail", contrast(parseHex("#AEB7C5"), scrim));
  atLeast("hero live", contrast(parseHex("#FF3B30"), scrim));
  atLeast("poster close", contrast(parseHex("#F5F7FA"), plate));
});

test("the dark theme keeps today's link and status ink", () => {
  const root = block(base, ":root");
  assert.match(decl(root, "--color-link"), /color-mix\(\s*in srgb\s*,\s*var\(--color-accent\)\s+72%\s*,\s*#fff\s*\)/);
  assert.equal(decl(root, "--color-accent-ink"), "#9cbcff");
  assert.equal(decl(root, "--color-tally-ink"), "#ff8a82");
  assert.equal(decl(root, "--color-tally-soft-ink"), "#ffb3ad");
  assert.match(base, /color:\s*var\(--color-link\)/);
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

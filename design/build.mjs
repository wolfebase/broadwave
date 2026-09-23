// Generates web CSS variables and the Swift theme from tokens.json.
// Usage: node design/build.mjs
import { readFileSync, writeFileSync, mkdirSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const root = join(here, "..");
const t = JSON.parse(readFileSync(join(here, "tokens.json"), "utf8"));
const header = "Generated from design/tokens.json by design/build.mjs. Do not edit.";

const kebab = (s) => s.replace(/([a-z0-9])([A-Z])/g, "$1-$2").toLowerCase();

function hexToRGBA(hex) {
  const h = hex.replace("#", "");
  const n = (i) => parseInt(h.slice(i, i + 2), 16);
  return { r: n(0), g: n(2), b: n(4), a: h.length === 8 ? n(6) / 255 : 1 };
}

function css() {
  const lines = [`/* ${header} */`, ":root {"];
  for (const [k, v] of Object.entries(t.color)) lines.push(`  --color-${kebab(k)}: ${v};`);
  for (const [k, v] of Object.entries(t.category)) lines.push(`  --cat-${k}: ${v};`);
  for (const [k, v] of Object.entries(t.radius)) lines.push(`  --radius-${k}: ${v}px;`);
  for (const [k, v] of Object.entries(t.space)) lines.push(`  --space-${k}: ${v}px;`);
  for (const [k, v] of Object.entries(t.type)) {
    lines.push(`  --type-${k}-size: ${v.size}px;`);
    lines.push(`  --type-${k}-weight: ${v.weight};`);
    lines.push(`  --type-${k}-tracking: ${v.tracking}em;`);
  }
  lines.push(`  --motion-fast: ${t.motion.fast}ms;`);
  lines.push(`  --motion-base: ${t.motion.base}ms;`);
  lines.push(`  --motion-slow: ${t.motion.slow}ms;`);
  lines.push(`  --ease-spring: cubic-bezier(0.2, 0.9, 0.25, 1.05);`);
  lines.push(`  --ease-out: cubic-bezier(0.16, 1, 0.3, 1);`);
  lines.push("}", "", 'html[data-layout="tv"] {');
  for (const [k, v] of Object.entries(t.type)) {
    lines.push(`  --type-${k}-size: ${Math.round(v.size * t.tvScale)}px;`);
  }
  lines.push("}", "");
  return lines.join("\n");
}

function swiftColor(hex) {
  const { r, g, b, a } = hexToRGBA(hex);
  const f = (x) => (x / 255).toFixed(3);
  return `Color(.sRGB, red: ${f(r)}, green: ${f(g)}, blue: ${f(b)}, opacity: ${a.toFixed(3)})`;
}

function swift() {
  const out = [`// ${header}`, "", "import SwiftUI", "", "public enum Tokens {"];
  out.push("    public enum ColorToken {");
  for (const [k, v] of Object.entries(t.color)) out.push(`        public static let ${k} = ${swiftColor(v)}`);
  out.push("    }", "", "    public enum Category {");
  for (const [k, v] of Object.entries(t.category)) out.push(`        public static let ${k} = ${swiftColor(v)}`);
  out.push("    }", "", "    public enum Radius {");
  for (const [k, v] of Object.entries(t.radius)) out.push(`        public static let ${k}: CGFloat = ${v}`);
  out.push("    }", "", "    public enum Space {");
  for (const [k, v] of Object.entries(t.space)) out.push(`        public static let s${k}: CGFloat = ${v}`);
  out.push("    }", "", "    public enum Motion {");
  out.push(`        public static let fast: Double = ${t.motion.fast / 1000}`);
  out.push(`        public static let base: Double = ${t.motion.base / 1000}`);
  out.push(`        public static let slow: Double = ${t.motion.slow / 1000}`);
  out.push(`        public static let spring = Animation.spring(response: ${t.motion.springResponse}, dampingFraction: ${t.motion.springDamping})`);
  out.push("    }", "", "    public enum TypeSize {");
  for (const [k, v] of Object.entries(t.type)) out.push(`        public static let ${k}: CGFloat = ${v.size}`);
  out.push("    }", "", `    public static let tvScale: CGFloat = ${t.tvScale}`, "}", "");
  return out.join("\n");
}

function write(path, body) {
  mkdirSync(dirname(path), { recursive: true });
  writeFileSync(path, body);
  console.log("wrote", path.replace(root + "/", ""));
}

write(join(root, "web/src/theme/tokens.css"), css());
write(join(root, "apple/Packages/OTAUI/Sources/OTAUI/Theme+Tokens.swift"), swift());

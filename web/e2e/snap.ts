import { expect, type Page, type TestInfo } from "@playwright/test";
import { existsSync, mkdirSync, readFileSync, writeFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import jpeg from "jpeg-js";
import pixelmatch from "pixelmatch";
import { PNG } from "pngjs";

const here = path.dirname(fileURLToPath(import.meta.url));
const dir = path.join(here, "snapshots");
const limit = 200 * 1024;

export const sizes = [
  { name: "390x844", width: 390, height: 844 },
  { name: "1440x900", width: 1440, height: 900 },
  { name: "1920x1080", width: 1920, height: 1080 },
] as const;

// The moving picture and the sync readout change every frame. The player test
// checks videoWidth and the shared timeline; these shots are the chrome.
const quiet = `
  .stage.idle .stage-hud, .stage.idle .stage-hud > * { opacity: 1 !important; }
  video { opacity: 0 !important; }
  img, .progress, .hero-left, .hero-next, .onnow-next, .gc-time, .gc-score,
  .eyebrow-dim, .time-read, .guide-times, .guide-cell.empty, .search-copy span,
  .ps-eyebrow, .sync-pill, .live-pill, .banner-update, .banner-home,
  .setup-finish .setup-list { visibility: hidden !important; }
  .now-flag, .now-line { display: none !important; }
`;

type RGBA = { data: Uint8Array; width: number; height: number };

function nearest(src: PNG, width: number, height: number): RGBA {
  const data = new Uint8Array(width * height * 4);
  for (let y = 0; y < height; y++) {
    const sy = Math.min(src.height - 1, Math.floor((y * src.height) / height));
    for (let x = 0; x < width; x++) {
      const sx = Math.min(src.width - 1, Math.floor((x * src.width) / width));
      const from = (sy * src.width + sx) * 4;
      const to = (y * width + x) * 4;
      data[to] = src.data[from];
      data[to + 1] = src.data[from + 1];
      data[to + 2] = src.data[from + 2];
      data[to + 3] = 255;
    }
  }
  return { data, width, height };
}

function pack(src: PNG): Buffer {
  let image: RGBA = { data: src.data, width: src.width, height: src.height };
  let quality = 60;
  let encoded = jpeg.encode(image, quality);
  while (encoded.data.length > limit && image.width > 480) {
    const width = Math.max(480, Math.round(image.width * 0.75));
    const height = Math.max(270, Math.round(image.height * 0.75));
    image = nearest(src, width, height);
    quality = 50;
    encoded = jpeg.encode(image, quality);
  }
  if (encoded.data.length > limit) throw new Error(`snapshot is ${encoded.data.length} bytes after downscale`);
  return Buffer.from(encoded.data);
}

export async function settle(page: Page) {
  await expect(page.locator("main")).toHaveAttribute("data-ready", "1");
  await page.evaluate(() => document.fonts.ready);
  const dismiss = page.getByRole("button", { name: "Not now" });
  for (let i = 0; i < 3; i++) {
    if ((await dismiss.count()) === 0) return;
    await dismiss.first().click();
  }
}

export async function snap(page: Page, name: string, info: TestInfo) {
  const dismiss = page.getByRole("button", { name: "Not now" });
  if ((await dismiss.count()) > 0) await dismiss.first().click();
  await page.addStyleTag({ content: quiet });
  const png = PNG.sync.read(await page.screenshot({ animations: "disabled", caret: "hide", scale: "css" }));
  const body = pack(png);
  mkdirSync(dir, { recursive: true });
  const file = path.join(dir, `${name}.jpg`);
  if (process.env.E2E_UPDATE_SNAPSHOTS === "1") {
    writeFileSync(file, body);
    info.annotations.push({ type: "snapshot", description: `wrote ${path.relative(here, file)} (${body.length} bytes)` });
    return;
  }
  if (!existsSync(file)) throw new Error(`missing ${path.relative(here, file)}. Re-run with E2E_UPDATE_SNAPSHOTS=1`);
  const expected = jpeg.decode(readFileSync(file), { useTArray: true, maxMemoryUsageInMB: 256 });
  const actual = jpeg.decode(body, { useTArray: true, maxMemoryUsageInMB: 256 });
  if (expected.width !== actual.width || expected.height !== actual.height) {
    throw new Error(`${name} is ${actual.width}x${actual.height}, snapshot is ${expected.width}x${expected.height}`);
  }
  const diff = pixelmatch(expected.data, actual.data, null, expected.width, expected.height, { threshold: 0.2 });
  const ratio = diff / (expected.width * expected.height);
  if (ratio > 0.02) {
    writeFileSync(path.join(here, ".run", `${name}-actual.jpg`), body);
    throw new Error(`${name} differs on ${diff} pixels (${(ratio * 100).toFixed(2)}%)`);
  }
}

export async function atSize(page: Page, size: (typeof sizes)[number]) {
  await page.setViewportSize({ width: size.width, height: size.height });
  const layout = size.width <= 760 ? "phone" : "desktop";
  await expect(page.locator("html")).toHaveAttribute("data-layout", layout, { timeout: 5_000 });
}

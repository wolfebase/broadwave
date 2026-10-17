/**
 * Whether a logo's visible pixels are mostly light, mostly dark, or mixed.
 * A white logo vanishes on the light theme and a black one on the dark theme.
 */
export type LogoTone = "light" | "dark" | "mixed";

export function toneOf(rgba: ArrayLike<number>): LogoTone | null {
  let weight = 0;
  let light = 0;
  let dark = 0;
  for (let i = 0; i + 3 < rgba.length; i += 4) {
    const a = rgba[i + 3] / 255;
    if (a < 0.2) continue;
    const y = (0.2126 * rgba[i] + 0.7152 * rgba[i + 1] + 0.0722 * rgba[i + 2]) / 255;
    weight += a;
    if (y > 0.8) light += a;
    else if (y < 0.25) dark += a;
  }
  if (weight === 0) return null;
  if (light / weight > 0.5 && light > 2 * dark) return "light";
  if (dark / weight > 0.5 && dark > 2 * light) return "dark";
  return "mixed";
}

/** Marks a loaded logo with data-tone so the theme can put a plate behind it. */
export function markLogoTone(img: HTMLImageElement) {
  try {
    const w = Math.min(img.naturalWidth, 48);
    const h = Math.min(img.naturalHeight, 48);
    if (!w || !h) return;
    const canvas = document.createElement("canvas");
    canvas.width = w;
    canvas.height = h;
    const ctx = canvas.getContext("2d", { willReadFrequently: true });
    if (!ctx) return;
    ctx.drawImage(img, 0, 0, w, h);
    const tone = toneOf(ctx.getImageData(0, 0, w, h).data);
    if (tone) img.dataset.tone = tone;
  } catch {
    // A cross-origin logo taints the canvas; it keeps no plate.
  }
}

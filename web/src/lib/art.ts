/** Mirrors guide.ArtLayout. Full-bleed only for landscape art at least 1280 px wide, within 1.25×. */
export function artLayout(width: number, height: number, slot: number): "bleed" | "composed" {
  if (width >= 1280 && height > 0 && width > height && slot > 0 && slot * 4 <= width * 5) return "bleed";
  return "composed";
}

/** Longest side to draw. Zero means the picture's own pixel size. */
export function displayEdge(native: number, slot: number): number {
  if (native <= 0 || slot <= 0) return 0;
  const limit = native + Math.floor(native / 4);
  return slot < limit ? slot : limit;
}

/** CSS pixels for one side, staying within 1.25× the picture. Zero when the size is unknown. */
export function cappedCss(native: number, slotCss: number, scale = 1): number {
  if (native <= 0 || slotCss <= 0) return 0;
  const px = Math.max(scale, 1);
  const edge = displayEdge(native, Math.round(slotCss * px));
  if (edge <= 0) return 0;
  return edge / px;
}

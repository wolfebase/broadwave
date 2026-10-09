/** Where an arrow lands in the player options. Columns wrap. Rows stop at the ends. */
export function optionArrow(rows: number[], row: number, col: number, key: string): { row: number; col: number } | null {
  if (row < 0 || row >= rows.length || rows[row] <= 0 || col < 0) return null;
  const width = rows[row];
  if (key === "ArrowRight") return { row, col: (col + 1) % width };
  if (key === "ArrowLeft") return { row, col: (col - 1 + width) % width };
  if (key !== "ArrowDown" && key !== "ArrowUp") return null;
  const next = key === "ArrowDown" ? row + 1 : row - 1;
  if (next < 0 || next >= rows.length || rows[next] <= 0) return { row, col: Math.min(col, width - 1) };
  return { row: next, col: Math.min(col, rows[next] - 1) };
}

const FOCUSABLE = "button, input, select, textarea";

/** One row per choice group, so Up and Down change setting and Left and Right change its value. */
export function optionRows(region: ParentNode | null): HTMLElement[][] {
  if (!region) return [];
  const rows: HTMLElement[][] = [];
  const seen = new Set<Element>();
  for (const block of region.querySelectorAll(".segmented, .option-actions, .sheet-actions, input[type='range']")) {
    const found = block.matches("input, select, textarea") ? [block] : [...block.querySelectorAll(FOCUSABLE)];
    const items = found.filter((el): el is HTMLElement => {
      if (!(el instanceof HTMLElement) || seen.has(el)) return false;
      if ("disabled" in el && el.disabled) return false;
      seen.add(el);
      return true;
    });
    if (items.length > 0) rows.push(items);
  }
  return rows;
}

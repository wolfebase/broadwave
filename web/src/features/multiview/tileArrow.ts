/** Next tile in reading order. Null means the arrow has left the grid. */
export function tileArrow(count: number, index: number, key: string, cols: number): number | null {
  if (count <= 0) return null;
  const step = key === "ArrowRight" ? 1 : key === "ArrowLeft" ? -1 : key === "ArrowDown" ? cols : key === "ArrowUp" ? -cols : 0;
  if (!step) return null;
  const dest = Math.max(0, Math.min(count - 1, index + step));
  return dest === index ? null : dest;
}

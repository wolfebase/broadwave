/** Next item in a menu. An index below zero is the menu just opening. */
export function menuStep(count: number, index: number, key: string): number | null {
  if (count <= 0) return null;
  const last = count - 1;
  if (index < 0) {
    if (key === "ArrowDown" || key === "ArrowRight" || key === "Home") return 0;
    if (key === "ArrowUp" || key === "ArrowLeft" || key === "End") return last;
    return null;
  }
  if (key === "Home") return 0;
  if (key === "End") return last;
  if (key === "ArrowDown" || key === "ArrowRight") return index >= last ? 0 : index + 1;
  if (key === "ArrowUp" || key === "ArrowLeft") return index <= 0 ? last : index - 1;
  return null;
}

export type MissedShowing = {
  title?: string;
  start: string;
  guideNumber?: string;
};

// missedLine is the sentence shown before a one-shot fix is confirmed.
// The clock matches formatClock. This file stays free of imports so the node test runner can load it.
export function missedLine(items: MissedShowing[]): string {
  const parts = items.map((item) => {
    const title = item.title?.trim() || "A show";
    const when = new Date(item.start).toLocaleTimeString([], { hour: "numeric", minute: "2-digit" });
    return item.guideNumber ? `${title} at ${when} on ${item.guideNumber}` : `${title} at ${when}`;
  });
  return `${joinList(parts)} will not record.`;
}

function joinList(parts: string[]): string {
  if (parts.length <= 1) return parts[0] ?? "";
  if (parts.length === 2) return `${parts[0]} and ${parts[1]}`;
  return `${parts.slice(0, -1).join(", ")}, and ${parts[parts.length - 1]}`;
}

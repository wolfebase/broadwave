export type MissedShowing = {
  title?: string;
  start: string;
  guideNumber?: string;
};

const months = ["Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"];

// missedLine is the sentence shown before a one-shot fix is confirmed.
// The date is in the sentence because two showings can share a clock inside 14 days.
// This file stays free of imports so the node test runner can load it.
export function missedLine(items: MissedShowing[]): string {
  const parts = items.map((item) => {
    const title = item.title?.trim() || "A show";
    const when = formatWhen(item.start);
    return item.guideNumber ? `${title} on ${when} on ${item.guideNumber}` : `${title} on ${when}`;
  });
  return `${joinList(parts)} will not record.`;
}

function formatWhen(start: string): string {
  const date = new Date(start);
  const clock = date.toLocaleTimeString([], { hour: "numeric", minute: "2-digit" });
  return `${months[date.getMonth()]} ${date.getDate()} at ${clock}`;
}

function joinList(parts: string[]): string {
  if (parts.length <= 1) return parts[0] ?? "";
  if (parts.length === 2) return `${parts[0]} and ${parts[1]}`;
  return `${parts.slice(0, -1).join(", ")}, and ${parts[parts.length - 1]}`;
}

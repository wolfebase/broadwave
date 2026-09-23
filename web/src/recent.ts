export type RecentChannel = { id: number; number: string; name: string };

const key = "ota-recent";

export function readRecent(): RecentChannel[] {
  try {
    const raw = JSON.parse(localStorage.getItem(key) || "[]") as unknown;
    if (!Array.isArray(raw)) return [];
    return raw
      .filter((item): item is RecentChannel => {
        if (!item || typeof item !== "object") return false;
        const row = item as RecentChannel;
        return typeof row.id === "number" && typeof row.number === "string" && typeof row.name === "string";
      })
      .slice(0, 8);
  } catch {
    return [];
  }
}

export function rememberChannel(channel: { id: number; displayNumber: string; displayName: string }) {
  const next = [
    { id: channel.id, number: channel.displayNumber, name: channel.displayName },
    ...readRecent().filter((item) => item.id !== channel.id),
  ].slice(0, 8);
  localStorage.setItem(key, JSON.stringify(next));
}

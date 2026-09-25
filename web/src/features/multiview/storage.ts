export type MvLayout = "2up" | "1+2" | "1+3" | "quad" | "pip";

export type SavedSet = { name: string; channels: number[]; layout?: MvLayout };

export const layoutChoices: { id: MvLayout; label: string }[] = [
  { id: "2up", label: "Side by side" },
  { id: "1+2", label: "One big and two" },
  { id: "1+3", label: "One big and three" },
  { id: "quad", label: "Quad" },
  { id: "pip", label: "Small over big" },
];

/** Watch together opens the layout that fits the games on now. */
export function layoutForCount(count: number): MvLayout {
  if (count >= 4) return "quad";
  if (count === 3) return "1+2";
  return "2up";
}

export function layoutLabel(layout: MvLayout): string {
  return layoutChoices.find((item) => item.id === layout)?.label ?? "Side by side";
}

const KEY = "broadwave-multiview";

type Store = { layout: MvLayout; sets: SavedSet[] };

function read(): Store {
  try {
    const raw = JSON.parse(localStorage.getItem(KEY) || "{}") as Partial<Store>;
    return { layout: isLayout(raw.layout) ? raw.layout : "2up", sets: Array.isArray(raw.sets) ? raw.sets : [] };
  } catch {
    return { layout: "2up", sets: [] };
  }
}

function write(next: Store) {
  localStorage.setItem(KEY, JSON.stringify(next));
  localStorage.setItem("broadwave-mv-layout", next.layout);
}

export function isLayout(value: unknown): value is MvLayout {
  return value === "2up" || value === "1+2" || value === "1+3" || value === "quad" || value === "pip";
}

export function savedLayout(): MvLayout {
  return read().layout;
}

export function rememberLayout(layout: MvLayout) {
  write({ ...read(), layout });
}

export function savedSets(): SavedSet[] {
  return read().sets;
}

export function saveSet(name: string, channels: number[], layout: MvLayout = layoutForCount(channels.length)) {
  const cur = read();
  const key = channels.join(",");
  const sets = [{ name, channels, layout }, ...cur.sets.filter((s) => s.channels.join(",") !== key)].slice(0, 8);
  write({ ...cur, sets });
}

/** Query strings treat + as a space, so 1+2 has to be encoded. */
export function multiviewPath(channels: number[], layout: MvLayout, focus: number, add = false) {
  const q = new URLSearchParams();
  q.set("ch", channels.join(","));
  q.set("layout", layout);
  q.set("focus", String(focus));
  if (add) q.set("add", "1");
  return `/multiview?${q}`;
}

export function layoutFromParam(raw: string | null): MvLayout | null {
  if (raw == null) return null;
  const value = raw.replaceAll(" ", "+");
  return isLayout(value) ? value : null;
}

export function slotsFor(layout: MvLayout) {
  if (layout === "1+2") return 3;
  if (layout === "1+3" || layout === "quad") return 4;
  return 2;
}

export function roomId() {
  let id = sessionStorage.getItem("broadwave-mv-room");
  if (!id) {
    id = Math.random().toString(36).slice(2, 10);
    sessionStorage.setItem("broadwave-mv-room", id);
  }
  return `multiview:${id}`;
}

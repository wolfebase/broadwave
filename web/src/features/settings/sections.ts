/** Settings is a list of sections. Each one has its own address: /settings/<id>. */
export const sectionIds = ["appearance", "tuners", "playback", "guide", "recording", "devices", "integrations", "backups", "about"] as const;

export type SectionId = (typeof sectionIds)[number];

// Older links and the pages that used to stand alone.
const aliases: Record<string, SectionId> = {
  sources: "tuners",
  lineup: "tuners",
  channels: "tuners",
  storage: "recording",
  dvr: "recording",
  share: "integrations",
  remote: "devices",
  away: "devices",
  home: "devices",
};

function known(value: string): SectionId | null {
  const id = value.trim().toLowerCase();
  if ((sectionIds as readonly string[]).includes(id)) return id as SectionId;
  return aliases[id] ?? null;
}

/**
 * The section a path opens, or null for the Settings list itself.
 * /sources and /settings#sources are the lineup's old addresses.
 */
export function sectionFromPath(path: string, hash = ""): SectionId | null {
  if (path === "/sources") return "tuners";
  if (path === "/about") return "about";
  const rest = path.replace(/^\/settings\/?/, "").split("/")[0] ?? "";
  if (rest) return known(rest);
  const anchor = hash.replace(/^#/, "");
  return anchor ? known(anchor) : null;
}

export function sectionPath(id: SectionId): string {
  return `/settings/${id}`;
}

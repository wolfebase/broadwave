export type SoundRole = "main" | "language" | "described";
export type Sound = { role: SoundRole; name: string; lang?: string };

type Rendition = { name: string; lang?: string; default: boolean; characteristics?: string };

const describesVideo = "public.accessibility.describes-video";
const order: SoundRole[] = ["main", "language", "described"];

/**
 * One sound per role from a master's audio renditions. The server marks the
 * broadcast's main mix as the default and described video by its
 * characteristic; the first other track is the second language.
 */
export function soundsOf(renditions: Rendition[]): Sound[] {
  const out: Sound[] = [];
  for (const r of renditions) {
    const role: SoundRole = r.characteristics?.includes(describesVideo) ? "described" : r.default ? "main" : "language";
    if (!out.some((s) => s.role === role)) out.push({ role, name: r.name, lang: r.lang });
  }
  return out.sort((a, b) => order.indexOf(a.role) - order.indexOf(b.role));
}

/** The sound for a role, or the main mix when the channel has none. */
export function soundFor(sounds: Sound[], role: string | undefined): Sound | undefined {
  return sounds.find((s) => s.role === (role || "main")) ?? sounds.find((s) => s.role === "main");
}

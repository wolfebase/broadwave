/** A master variant the start can prime. url may be one address or a list. */
export type PrimeCandidate = { url: string | string[]; height?: number; bitrate?: number; videoCodec?: string };

const ladderPath = /\/ladder\.m3u8(\?|$)/;

function levelURL(level: { url: string | string[] }): string {
  return Array.isArray(level.url) ? (level.url[0] ?? "") : level.url;
}

/** The level playlist to prime. A master is primed only when it has one level.
 * A ladder primes its tallest video level (height, then bitrate); audio-only
 * levels (no height and no avc1/hvc1/hev1 codec) are skipped when any video level exists. */
export function choosePrimeURL(levels: PrimeCandidate[], ladder: boolean): string | null {
  if (!ladder) {
    const urls = new Set(levels.map((level) => level.url));
    if (urls.size !== 1) return null;
    const [url] = urls;
    return typeof url === "string" ? url : null;
  }
  const urls = levels.map(levelURL).filter((url) => url !== "");
  if (urls.length === 0) return null;
  if (new Set(urls).size === 1) return urls[0];
  const video = levels.filter((level) => (level.height ?? 0) > 0 || /avc1|hvc1|hev1/i.test(level.videoCodec ?? ""));
  const pool = video.length > 0 ? video : levels;
  const ranked = [...pool].sort((a, b) => (b.height ?? 0) - (a.height ?? 0) || (b.bitrate ?? 0) - (a.bitrate ?? 0));
  return levelURL(ranked[0]) || null;
}

/** The sounds a watch switches in place, or null to watch again for another.
 * A primed master always switches in place. A ladder lists no sound tracks. */
export function primedSounds<T>(primed: { sounds: T[] } | null, ladder: boolean): T[] | null {
  if (!primed) return null;
  if (ladder && primed.sounds.length === 0) return null;
  return primed.sounds;
}

/** A ladder loaded without a prime still names a subtitle group.
 * hls.js must not play it. The player's own captions do. */
export function quietSubtitles(primed: boolean, ladder: boolean): boolean {
  return !primed && ladder;
}

/** Captions are offered for a caption master, or for a ladder whose text lists them. */
export function showCaptions(mainPlaylist: string | undefined, masterText?: string): boolean {
  if (!mainPlaylist) return false;
  if (!ladderPath.test(mainPlaylist)) return true;
  return !!masterText && masterText.includes("SUBTITLES=");
}

/** A primed body answers its first request once. On a ladder, ABR often starts
 * shorter than the rung that was primed, so any other request drops the unused
 * snapshot and a later switch loads a fresh one. The master is left for its own
 * first load. */
export function takePrimed<T>(bodies: Map<string, T>, masterURL: string, requested: string, ladder: boolean): T | undefined {
  const hit = bodies.get(requested);
  if (hit !== undefined) {
    bodies.delete(requested);
    return hit;
  }
  if (ladder) {
    for (const url of bodies.keys()) {
      if (url !== masterURL) bodies.delete(url);
    }
  }
  return undefined;
}

/** Where a live session loads. prime is the ladder or master to prime first, or
 * empty. src is what plays when nothing was primed: a ladder plays itself; a
 * master falls back to the index, which carries the main sound. */
export function playbackURL(
  session: { playlist: string; mainPlaylist?: string },
  alternates: boolean,
): { prime: string; src: string; ladder: boolean } {
  const main = session.mainPlaylist ?? "";
  if (ladderPath.test(main)) return { prime: main, src: main, ladder: true };
  if (alternates && /\/master\.m3u8(\?|$)/.test(main)) return { prime: main, src: session.playlist, ladder: false };
  return { prime: "", src: session.playlist, ladder: false };
}

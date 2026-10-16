/**
 * The picture the full player was showing when it gave the screen to
 * multiview. The tile for that channel joins the same rendition, which the
 * server still has running, instead of starting a new one cold.
 */
let held: { channelId: number; rendition: string; at: number } | null = null;

export function noteHandoff(channelId: number, rendition: string | undefined, now = Date.now()) {
  held = rendition ? { channelId, rendition, at: now } : null;
}

/** The rendition to join for this channel, once, within a few seconds of the handoff. */
export function takeHandoff(channelId: number, now = Date.now()): string {
  const h = held;
  if (!h || h.channelId !== channelId || now - h.at > 10_000) return "";
  held = null;
  return h.rendition;
}

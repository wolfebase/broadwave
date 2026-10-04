// Which channel a multiview link should play. Matches ClearBroadcast.play:
// a channel on the guide plays as itself, an encrypted 3.0 station plays its
// clear 1.0 twin, a hidden half of a pair plays as the half on the guide, and
// another tuner's copy of a channel plays as the row on the guide.

export type ClearChannel = {
  id: number;
  playsAs?: number;
  protected?: boolean;
  twinId?: number;
  sameAs?: number;
};

export type ClearChoice = { id: number; note: boolean };

export function clearBroadcast(id: number, visible: readonly { id: number }[], lineup: readonly ClearChannel[], depth = 0): ClearChoice | null {
  if (visible.some((channel) => channel.id === id)) return { id, note: false };
  const asked = lineup.find((channel) => channel.id === id);
  if (!asked) return null;
  if (asked.playsAs && visible.some((channel) => channel.id === asked.playsAs)) return { id: asked.playsAs, note: true };
  if (asked.protected !== true && asked.twinId && visible.some((channel) => channel.id === asked.twinId)) return { id: asked.twinId, note: false };
  // The shown row can itself be the hidden half of a pair.
  if (asked.sameAs && asked.sameAs !== id && depth < 2) return clearBroadcast(asked.sameAs, visible, lineup, depth + 1);
  return null;
}

/** The ids to put on screen, and the encrypted ids that became a clear twin. */
export function resolveClear(ids: readonly number[], visible: readonly { id: number }[], lineup: readonly ClearChannel[]): { ids: number[]; from: number[] } {
  const next: number[] = [];
  const from: number[] = [];
  const seen = new Set<number>();
  for (const id of ids) {
    const choice = clearBroadcast(id, visible, lineup);
    if (!choice || seen.has(choice.id)) continue;
    seen.add(choice.id);
    next.push(choice.id);
    if (choice.note) from.push(id);
  }
  return { ids: next, from };
}

/** Encrypted ids whose clear twin is still one of the channels on screen. */
export function fromStillOn(from: readonly number[], ch: readonly number[], lineup: readonly ClearChannel[]): number[] {
  return from.filter((id) => {
    const asked = lineup.find((channel) => channel.id === id);
    return asked?.playsAs != null && ch.includes(asked.playsAs);
  });
}

/** Tiles that should say the encrypted broadcast is playing as the regular one. */
export function standInIds(from: readonly number[], lineup: readonly ClearChannel[]): number[] {
  const out: number[] = [];
  for (const id of from) {
    const asked = lineup.find((channel) => channel.id === id);
    if (asked?.playsAs && !out.includes(asked.playsAs)) out.push(asked.playsAs);
  }
  return out;
}

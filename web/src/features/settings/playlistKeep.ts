export type KeepChannel = { name: string; id?: string };

/** The id when the channel has one, so two channels with one name stay apart. */
export function channelKeepKey(ch: KeepChannel): string {
  return ch.id ? ch.id : ch.name;
}

/** Comma-separated keep list for the chosen channel positions, in playlist order. */
export function playlistKeep(channels: readonly KeepChannel[], picks: readonly number[]): string {
  return [...picks]
    .filter((i) => i >= 0 && i < channels.length)
    .sort((a, b) => a - b)
    .map((i) => channelKeepKey(channels[i]))
    .join(", ");
}

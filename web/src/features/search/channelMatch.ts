type Named = { id: number; displayNumber: string; displayName: string; network?: string; twinId?: number };

/** "4-1" is typed for 4.1. */
function numberOf(q: string): string {
  return /^\d+-\d+$/.test(q) ? q.replace("-", ".") : q;
}

/**
 * Whether the guide's search field finds this channel, as server search does:
 * the number or its start ("4-1" for 4.1), the 3.0 or 1.0 twin of a channel
 * with that number, or the name or network. `q` is trimmed and lower case.
 */
export function channelMatches(channel: Named, q: string, all: readonly Named[]): boolean {
  if (!q) return true;
  const number = numberOf(q);
  if (channel.displayNumber.startsWith(number)) return true;
  if (channel.displayName.toLowerCase().includes(q)) return true;
  if (channel.network && channel.network.toLowerCase().startsWith(q)) return true;
  if (!/^\d+(\.\d+)?$/.test(number)) return false;
  return all.some((other) => other.displayNumber === number && (other.twinId === channel.id || channel.twinId === other.id));
}

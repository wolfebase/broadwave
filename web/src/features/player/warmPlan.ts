/**
 * The channels a viewer most likely goes to next, most likely first: channel
 * up, the last channel, then channel down.
 */
export function neighbourIds(order: number[], current: number, last?: number): number[] {
  const i = order.indexOf(current);
  if (i < 0 || order.length < 2) return [];
  const up = order[(i + 1) % order.length];
  const down = order[(i - 1 + order.length) % order.length];
  const out: number[] = [];
  for (const id of [up, last, down]) {
    if (id !== undefined && id !== current && order.includes(id) && !out.includes(id)) out.push(id);
  }
  return out;
}

/** The channels warmed ahead: channel up and channel down. The server keeps two guesses. */
export function warmIds(order: number[], current: number): number[] {
  const i = order.indexOf(current);
  if (i < 0 || order.length < 2) return [];
  const up = order[(i + 1) % order.length];
  const down = order[(i - 1 + order.length) % order.length];
  return up === down ? [up] : [up, down];
}

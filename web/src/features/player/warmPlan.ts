/**
 * The channels a viewer most likely goes to next, most likely first: channel
 * up, the last channel, then channel down. The server warms one at a time.
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

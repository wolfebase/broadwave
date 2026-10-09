/** A channel is missing only after this visit's lineup arrives. A saved copy can be short a channel added since. */
export function channelMissing(listed: boolean, watchId: number, ids: readonly number[]): boolean {
  return listed && watchId > 0 && ids.length > 0 && !ids.includes(watchId);
}

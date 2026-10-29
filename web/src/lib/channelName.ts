// A station's name on the air is often its call sign plus a feed suffix
// ("KCTVDT1", "WDAF-DT", "KMBC-HD", "KCPT-1"). Viewers know the call sign.
const callWithFeed = /^([KW][A-Z]{3}|[KW][A-Z]{2}|C[A-Z]{3})(?:[- ]?(?:DT|HD|LD|CD|LP)\d?|-\d)$/;

/** The channel's name as a viewer reads it: the call sign without its feed suffix. */
export function channelName(channel: { displayName: string }): string {
  const name = channel.displayName.trim();
  const match = callWithFeed.exec(name.toUpperCase());
  return match ? match[1] : name;
}

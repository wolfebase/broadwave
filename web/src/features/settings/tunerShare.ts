// Share rows are the addresses other apps paste. The playlist and the guide
// stay on this server's own port. The HDHomeRun address is that host on 8478.

export type ShareRow = {
  app: string;
  label: string;
  value: string;
  hint?: string;
};

const apps = ["Plex", "Jellyfin", "Emby", "Channels"] as const;

// bracketHost keeps an IPv6 address in one piece before the port.
// Some URL parsers already include the brackets. Others return the bare address.
export function bracketHost(hostname: string): string {
  if (hostname.includes(":") && !hostname.startsWith("[")) return `[${hostname}]`;
  return hostname;
}

export function tunerAddress(pageURL: string): string {
  return `${bracketHost(new URL(pageURL).hostname)}:8478`;
}

export function shareRows(pageURL: string, sharing: boolean, hdhr = true): ShareRow[] {
  const page = new URL(pageURL);
  const address = tunerAddress(pageURL);
  const playlist = `${page.origin}/export/lineup.m3u`;
  const guide = `${page.origin}/export/guide.xml`;
  const rows: ShareRow[] = [];
  for (const app of apps) {
    if (hdhr) {
      rows.push({
        app,
        label: "HDHomeRun",
        value: address,
        hint: sharing ? undefined : "Turn on Act as an HDHomeRun.",
      });
    }
    rows.push({ app, label: "M3U playlist", value: playlist });
    rows.push({ app, label: "XMLTV guide", value: guide });
  }
  return rows;
}

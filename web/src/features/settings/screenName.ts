import { browserName, events } from "../../lib/events.ts";

const key = "broadwave-screen-name";

/** Trim, drop control characters, and keep at most 40 characters. */
export function cleanScreenName(raw: string): string {
  let out = "";
  for (const ch of raw.trim()) {
    const code = ch.codePointAt(0) ?? 0;
    if (code < 32 || code === 127) continue;
    out += ch;
    if (out.length >= 40) break;
  }
  return out.trim();
}

/** Keeps what the viewer typed and says it again on the open socket. Blank goes back to the browser name. */
export function saveScreenName(raw: string): string {
  const name = cleanScreenName(raw);
  try {
    if (name) localStorage.setItem(key, name);
    else localStorage.removeItem(key);
  } catch {
    // This page still says the name below.
  }
  events().here();
  return name || browserName();
}

// A media link for a device that cannot carry this browser's token: a Cast receiver,
// or anything else handed a URL. An older server has no /media/link; then the caller
// sends what it sent before.

import type { MediaLink } from "../api/generated.ts";

export type LinkPoster = (path: string, token: string) => Promise<MediaLink | null>;

/** Links are asked for again when less than this is left. */
const KEEP_MS = 60 * 60 * 1000;

type Kept = { url: string; until: number };

const kept = new Map<string, Kept>();

function browserPoster(): LinkPoster {
  return async (path, token) => {
    try {
      const res = await fetch("/api/v1/media/link", {
        method: "POST",
        headers: { "Content-Type": "application/json", Authorization: "Bearer " + token },
        body: JSON.stringify({ path }),
        credentials: "omit",
        cache: "no-store",
      });
      if (!res.ok) return null;
      return (await res.json()) as MediaLink;
    } catch {
      return null;
    }
  };
}

/** An absolute URL for the link on this origin, or null when it is not a /media/ link. */
export function linkURL(origin: string, link: MediaLink | null): string | null {
  if (!link || typeof link.url !== "string" || !link.url.startsWith("/media/")) return null;
  try {
    const url = new URL(link.url, origin);
    if (url.origin !== new URL(origin).origin) return null;
    return url.toString();
  } catch {
    return null;
  }
}

/**
 * The media link for a path, absolute on origin, or undefined. Undefined with
 * no token, and when the server does not answer with a link (a 404 from an older one).
 * A link is reused until an hour before it ends. Never rejects.
 */
export async function mediaLinkFor(origin: string, path: string, token: string, post: LinkPoster = browserPoster(), now: () => number = Date.now): Promise<string | undefined> {
  if (!token) return undefined;
  const cacheKey = `${origin} ${token} ${path}`;
  const hit = kept.get(cacheKey);
  if (hit && hit.until - now() > KEEP_MS) return hit.url;
  kept.delete(cacheKey);
  let link: MediaLink | null;
  try {
    link = await post(path, token);
  } catch {
    return undefined;
  }
  const url = linkURL(origin, link);
  if (!url) return undefined;
  const until = Date.parse(link?.expiresAt ?? "");
  if (Number.isFinite(until)) kept.set(cacheKey, { url, until });
  return url;
}

/** Tests start clean. */
export function forgetMediaLinks() {
  kept.clear();
}

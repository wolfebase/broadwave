import type { Recording } from "../../types";

/** The server finds the next airing by program id or episode name. A file that is already gone can still be recorded again. */
export function recordAgainOffered(rec: Recording) {
  return Boolean(rec.health?.damaged) && rec.status !== "recording" && Boolean(rec.programId || rec.subtitle);
}

/** The name stored on the once-pass. A blank show title uses the airing's name, then the episode name. */
export function recordAgainTitle(rec: { title?: string; subtitle?: string }, airing: { title?: string; subtitle?: string }) {
  for (const value of [rec.title, airing.title, rec.subtitle, airing.subtitle]) {
    const name = (value ?? "").trim();
    if (name) return name;
  }
  return "";
}

/** Posts the once-pass and reloads the pass list. A blank name is not sent. */
export async function scheduleRecordAgain(
  rec: { title?: string; subtitle?: string },
  airing: { title?: string; subtitle?: string; channelId: number; start: string },
  add: (title: string, channelId: number, start: string) => Promise<unknown>,
  refresh: () => Promise<unknown>,
) {
  const title = recordAgainTitle(rec, airing);
  if (!title) throw new Error("title required");
  await add(title, airing.channelId, airing.start);
  await refresh();
}

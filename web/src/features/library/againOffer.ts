import type { Recording } from "../../types";

/** The server finds the next airing by program id or episode name. A file that is already gone can still be recorded again. */
export function recordAgainOffered(rec: Recording) {
  return Boolean(rec.health?.damaged) && rec.status !== "recording" && Boolean(rec.programId || rec.subtitle);
}

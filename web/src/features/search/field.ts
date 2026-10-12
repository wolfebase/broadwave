import { searchMessages } from "../../i18n/messages/search.ts";
import { bind } from "../../i18n/runtime.ts";

const t = bind(searchMessages);

/** Back and Forward show the query in the address. Letters still being typed stay in the box. */
export function searchFieldValue(urlQuery: string, typed: string, historyMove: boolean): string {
  return historyMove ? urlQuery : typed;
}

/** A record request finishes on the query that started it, or the note stays whatever the new query showed. */
export function noteAfterRecord(asked: string, current: string, title: string, existing: string): string {
  return asked === current ? t("search.recordingEvery", { title }) : existing;
}

/** Status for an empty result or a pass that was saved. Anything else is an alert. */
export function noteTone(note: string): "status" | "alert" {
  if (note === t("search.nothing") || isRecordingEvery(note)) return "status";
  return "alert";
}

// The title sits where {title} was. Pseudo repeats the whole sentence, so every copy of the title has to match.
function isRecordingEvery(note: string): boolean {
  const mark = "\u0001";
  const sample = t("search.recordingEvery", { title: mark });
  const pattern = sample.replace(/[.*+?^${}()|[\]\\]/g, "\\$&").replaceAll(mark, "(.+)");
  const found = new RegExp(`^${pattern}$`).exec(note);
  if (!found || found.length < 2) return false;
  return found.slice(1).every((title) => title === found[1]);
}

import { useState } from "react";
import { useData } from "../../app/data";
import { addPass, getRecordAgain } from "../../api";
import { libraryMessages } from "../../i18n/messages/library.ts";
import { bind, formatDateTime } from "../../i18n/runtime.ts";
import { actionName } from "../../lib/actionName";
import { formatClock } from "../../time";
import type { Recording } from "../../types";
import { scheduleRecordAgain } from "./againOffer";
import { recordingSubject } from "./model";

export { recordAgainOffered } from "./againOffer";

const t = bind(libraryMessages);

/** "Thu 7:00 PM", with the date past six days out. */
function when(start: Date) {
  const far = start.getTime() - Date.now() > 6 * 86_400_000;
  const date = formatDateTime(start, far ? { weekday: "short", month: "short", day: "numeric" } : { weekday: "short" });
  return `${date} ${formatClock(start)}`;
}

/** On a damaged recording: finds the episode's next airing and records it. */
export function RecordAgain({ rec }: { rec: Recording }) {
  const { refresh } = useData();
  const [busy, setBusy] = useState(false);
  const [note, setNote] = useState("");
  const [done, setDone] = useState(false);

  async function again() {
    setBusy(true);
    try {
      const { airing } = await getRecordAgain(rec.id);
      if (!airing) {
        setNote(t("library.noAiring"));
        return;
      }
      await scheduleRecordAgain(rec, airing, addPass, () => refresh(["passes"]));
      setDone(true);
      setNote(t("library.recordsAgain", { when: when(new Date(airing.start)) }));
    } catch {
      setNote(t("library.scheduleFailed"));
    } finally {
      setBusy(false);
    }
  }

  return (
    <>
      {done ? null : (
        <button type="button" className="btn" disabled={busy} aria-busy={busy ? true : undefined} aria-label={actionName(t("library.recordAgain"), recordingSubject(rec))} onClick={() => void again()}>
          {t("library.recordAgain")}
        </button>
      )}
      {note ? <span className="ch-tags" role="status">{note}</span> : null}
    </>
  );
}

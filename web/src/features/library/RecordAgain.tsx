import { useState } from "react";
import { addPass, getRecordAgain } from "../../api";
import { actionName } from "../../lib/actionName";
import { formatClock } from "../../time";
import type { Recording } from "../../types";
import { recordingSubject } from "./model";

/** "Thu 7:00 PM", with the date past six days out. */
function when(start: Date) {
  const far = start.getTime() - Date.now() > 6 * 86_400_000;
  return `${start.toLocaleDateString([], far ? { weekday: "short", month: "short", day: "numeric" } : { weekday: "short" })} ${formatClock(start)}`;
}

/** The server finds the next airing by program id or episode name. */
export function recordAgainOffered(rec: Recording) {
  return Boolean(rec.health?.damaged) && rec.status !== "recording" && !rec.missing && Boolean(rec.programId || rec.subtitle);
}

/** On a damaged recording: finds the episode's next airing and records it. */
export function RecordAgain({ rec }: { rec: Recording }) {
  const [busy, setBusy] = useState(false);
  const [note, setNote] = useState("");
  const [done, setDone] = useState(false);

  async function again() {
    setBusy(true);
    try {
      const { airing } = await getRecordAgain(rec.id);
      if (!airing) {
        setNote("The guide has no other airing yet.");
        return;
      }
      await addPass(rec.title, airing.channelId, airing.start);
      setDone(true);
      setNote(`Records again ${when(new Date(airing.start))}.`);
    } catch {
      setNote("Could not schedule it. Try again.");
    } finally {
      setBusy(false);
    }
  }

  return (
    <>
      {done ? null : (
        <button type="button" className="btn" disabled={busy} aria-busy={busy ? true : undefined} aria-label={actionName("Record it again", recordingSubject(rec))} onClick={() => void again()}>
          Record it again
        </button>
      )}
      {note ? <span className="ch-tags" role="status">{note}</span> : null}
    </>
  );
}

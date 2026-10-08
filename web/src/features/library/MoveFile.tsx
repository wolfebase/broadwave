import { useEffect, useRef, useState } from "react";
import { moveRecording } from "../../api";
import { useData } from "../../app/data";
import { focusRing } from "../../app/remote";
import type { Recording } from "../../types";

/** Renames a recording's file, or moves it into a folder, with the files beside it. */
export function MoveFile({ rec }: { rec: Recording }) {
  const { refresh } = useData();
  const ext = rec.file?.match(/\.[^./]+$/)?.[0] ?? "";
  const current = rec.file ? rec.file.slice(0, rec.file.length - ext.length) : "";
  const [open, setOpen] = useState(false);
  const [name, setName] = useState(current);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const field = useRef<HTMLInputElement>(null);
  const opener = useRef<HTMLButtonElement>(null);
  const wasOpen = useRef(false);
  useEffect(() => {
    if (open) focusRing(field.current);
    else if (wasOpen.current) focusRing(opener.current);
    wasOpen.current = open;
  }, [open]);

  async function save() {
    if (name.trim() === current) {
      setOpen(false);
      return;
    }
    setBusy(true);
    setError("");
    try {
      await moveRecording(rec.id, name);
      setOpen(false);
      await refresh(["recordings"]);
    } catch (err) {
      setError(err instanceof Error ? err.message : "The file could not be moved.");
    } finally {
      setBusy(false);
    }
  }

  if (!open) {
    return (
      <button
        ref={opener}
        type="button"
        className="btn"
        onClick={() => {
          setName(current);
          setError("");
          setOpen(true);
        }}
      >
        Rename file
      </button>
    );
  }
  return (
    <form
      className="sheet-actions"
      aria-label={`Rename ${rec.subtitle || rec.title}`}
      onSubmit={(event) => {
        event.preventDefault();
        void save();
      }}
      onKeyDown={(event) => {
        if (event.key === "Escape") {
          event.preventDefault();
          setOpen(false);
        }
      }}
    >
      <label className="field">
        File name in the recordings folder
        <input ref={field} value={name} spellCheck={false} disabled={busy} onChange={(event) => setName(event.target.value)} />
        <span className="hint">Use a slash for a folder, such as Show/S01E02. The files beside it move too.</span>
      </label>
      <button type="submit" className="btn primary" disabled={busy || name.trim() === ""}>
        Save
      </button>
      <button type="button" className="btn" disabled={busy} onClick={() => setOpen(false)}>
        Cancel
      </button>
      {error ? (
        <span className="hint error" role="alert">
          {error}
        </span>
      ) : null}
    </form>
  );
}

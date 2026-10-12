import { useEffect, useRef, useState } from "react";
import { moveRecording } from "../../api";
import { useData } from "../../app/data";
import { focusRing } from "../../app/remote";
import { libraryMessages } from "../../i18n/messages/library.ts";
import { bind } from "../../i18n/runtime.ts";
import { actionName } from "../../lib/actionName";
import { dismissesLayer, isTextField } from "../../lib/dialogFocus";
import type { Recording } from "../../types";
import { recordingSubject } from "./model";

const t = bind(libraryMessages);

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
      setError(err instanceof Error ? err.message : t("library.moveFailed"));
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
        aria-label={actionName(t("library.renameFile"), recordingSubject(rec))}
        onClick={() => {
          setName(current);
          setError("");
          setOpen(true);
        }}
      >
        {t("library.renameFile")}
      </button>
    );
  }
  return (
    <form
      className="sheet-actions"
      aria-label={t("library.renameNamed", { name: rec.subtitle || rec.title })}
      onSubmit={(event) => {
        event.preventDefault();
        void save();
      }}
      onKeyDown={(event) => {
        if (event.key === "Backspace" && isTextField(event.target)) return;
        if (!dismissesLayer(event.key, event.target, event.keyCode)) return;
        event.preventDefault();
        event.stopPropagation();
        setOpen(false);
      }}
    >
      <label className="field">
        {t("library.fileName")}
        <input ref={field} value={name} spellCheck={false} disabled={busy} onChange={(event) => setName(event.target.value)} />
        <span className="hint">{t("library.fileHint")}</span>
      </label>
      <button type="submit" className="btn primary" disabled={busy || name.trim() === ""}>
        {t("library.save")}
      </button>
      <button type="button" className="btn" disabled={busy} onClick={() => setOpen(false)}>
        {t("library.cancel")}
      </button>
      {error ? (
        <span className="hint error" role="alert">
          {error}
        </span>
      ) : null}
    </form>
  );
}

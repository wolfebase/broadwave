import { useRef, type ReactNode } from "react";
import { useDialogFocus } from "../../ui/useDialogFocus";

/**
 * Asks before something that cannot be undone. Cancel comes first and takes
 * focus, so OK on a remote pressed twice does not restore or remove anything.
 */
export function Confirm({
  title,
  body,
  confirm,
  cancel,
  busy,
  danger = true,
  onConfirm,
  onCancel,
}: {
  title: string;
  body: ReactNode;
  confirm: string;
  cancel: string;
  busy?: boolean;
  danger?: boolean;
  onConfirm: () => void;
  onCancel: () => void;
}) {
  const ref = useRef<HTMLDivElement>(null);
  useDialogFocus(ref, busy ? undefined : onCancel);
  return (
    <div className="set-scrim" onClick={(event) => event.target === event.currentTarget && !busy && onCancel()}>
      <div ref={ref} className="set-dialog" role="dialog" aria-modal="true" aria-labelledby="set-dialog-title" aria-describedby="set-dialog-body">
        <h2 id="set-dialog-title">{title}</h2>
        <div id="set-dialog-body" className="set-dialog-body">
          {body}
        </div>
        <div className="set-dialog-actions">
          <button type="button" className="btn" disabled={busy} onClick={onCancel}>
            {cancel}
          </button>
          <button type="button" className={danger ? "btn danger" : "btn"} disabled={busy} onClick={onConfirm}>
            {confirm}
          </button>
        </div>
      </div>
    </div>
  );
}

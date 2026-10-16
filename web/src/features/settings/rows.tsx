import { useId, type ReactNode } from "react";

/** A titled card of setting rows, like a grouped list on iPhone. */
export function Group({ title, lede, children, actions, id }: { title?: string; lede?: ReactNode; children: ReactNode; actions?: ReactNode; id?: string }) {
  const labelId = useId();
  return (
    <section className="set-group" id={id} aria-labelledby={title ? labelId : undefined}>
      {title || actions ? (
        <header className="set-group-head">
          {title ? <h3 id={labelId}>{title}</h3> : null}
          {actions ? <div className="set-group-actions">{actions}</div> : null}
        </header>
      ) : null}
      {lede ? <p className="set-group-lede">{lede}</p> : null}
      <div className="set-card">{children}</div>
    </section>
  );
}

/**
 * One setting: its name and a one-line hint on the left, the control on the right.
 * `stack` puts a wide control (a text field, a list of choices) under the name.
 */
export function Row({ label, hint, children, stack = false, labelFor }: { label: ReactNode; hint?: ReactNode; children?: ReactNode; stack?: boolean; labelFor?: string }) {
  return (
    <div className={stack ? "set-row stack" : "set-row"}>
      <div className="set-row-text">
        {labelFor ? <label htmlFor={labelFor} className="set-row-label">{label}</label> : <span className="set-row-label">{label}</span>}
        {hint ? <span className="set-row-hint">{hint}</span> : null}
      </div>
      {children != null ? <div className="set-row-control">{children}</div> : null}
    </div>
  );
}

/** An on/off switch. OK on a TV remote sends Enter, which a checkbox ignores, so Enter flips it too. */
export function Switch({ checked, onChange, label, disabled }: { checked: boolean; onChange: (next: boolean) => void; label: string; disabled?: boolean }) {
  return (
    <input
      type="checkbox"
      role="switch"
      className="set-switch"
      aria-label={label}
      checked={checked}
      disabled={disabled}
      onChange={(event) => onChange(event.target.checked)}
      onKeyDown={(event) => {
        if (event.key !== "Enter") return;
        event.preventDefault();
        onChange(!checked);
      }}
    />
  );
}

export function SwitchRow({ label, hint, checked, onChange, disabled }: { label: string; hint?: ReactNode; checked: boolean; onChange: (next: boolean) => void; disabled?: boolean }) {
  const id = useId();
  return (
    <div className="set-row switch">
      <div className="set-row-text">
        <label htmlFor={id} className="set-row-label">{label}</label>
        {hint ? <span className="set-row-hint">{hint}</span> : null}
      </div>
      <div className="set-row-control">
        <input
          id={id}
          type="checkbox"
          role="switch"
          className="set-switch"
          checked={checked}
          disabled={disabled}
          onChange={(event) => onChange(event.target.checked)}
          onKeyDown={(event) => {
            if (event.key !== "Enter") return;
            event.preventDefault();
            onChange(!checked);
          }}
        />
      </div>
    </div>
  );
}

/** A few choices side by side. A remote walks them with Left and Right. */
export function Choice<T extends string>({ label, value, options, onChange }: { label: string; value: T; options: readonly { value: T; label: string }[]; onChange: (next: T) => void }) {
  return (
    <div className="segmented set-seg" role="radiogroup" aria-label={label}>
      {options.map((option) => (
        <button
          key={option.value}
          type="button"
          role="radio"
          className={value === option.value ? "seg on" : "seg"}
          aria-checked={value === option.value}
          onClick={() => onChange(option.value)}
        >
          {option.label}
        </button>
      ))}
    </div>
  );
}

export function ChoiceRow<T extends string>({ label, hint, value, options, onChange, stack }: { label: string; hint?: ReactNode; value: T; options: readonly { value: T; label: string }[]; onChange: (next: T) => void; stack?: boolean }) {
  return (
    <Row label={label} hint={hint} stack={stack}>
      <Choice label={label} value={value} options={options} onChange={onChange} />
    </Row>
  );
}

export function SelectRow<T extends string>({ label, hint, value, options, onChange }: { label: string; hint?: ReactNode; value: T; options: readonly { value: T; label: string }[]; onChange: (next: T) => void }) {
  const id = useId();
  return (
    <Row label={label} hint={hint} labelFor={id}>
      <select id={id} className="set-select" value={value} onChange={(event) => onChange(event.target.value as T)}>
        {options.map((option) => (
          <option key={option.value} value={option.value}>
            {option.label}
          </option>
        ))}
      </select>
    </Row>
  );
}

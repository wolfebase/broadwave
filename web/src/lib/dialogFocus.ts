const TEXT_INPUT = new Set(["", "text", "search", "password", "email", "url", "tel", "number"]);

/**
 * Backspace deletes in a text field. A checkbox, a slider, and a button are not
 * text fields, so Back still leaves.
 */
export function isTextField(target: EventTarget | null): boolean {
  if (!target || typeof target !== "object") return false;
  const el = target as { tagName?: string; type?: string; isContentEditable?: boolean };
  if (el.isContentEditable) return true;
  if (el.tagName === "TEXTAREA") return true;
  if (el.tagName !== "INPUT") return false;
  return TEXT_INPUT.has((el.type || "text").toLowerCase());
}

/** Escape closes a layer. Backspace closes it too, unless it is deleting text. */
export function dismissesLayer(key: string, target: EventTarget | null): boolean {
  if (key === "Escape") return true;
  if (key === "Backspace") return !isTextField(target);
  return false;
}

/**
 * Where Tab goes inside a dialog. Null lets the browser move to the next control
 * still inside the dialog. The first and last stops wrap, and a key that arrives
 * from outside the dialog comes in.
 */
export function trapTab<T>(list: T[], active: T | null, shift: boolean): T | null {
  if (list.length === 0) return null;
  const first = list[0];
  const last = list[list.length - 1];
  const index = active == null ? -1 : list.indexOf(active);
  if (index < 0) return shift ? last : first;
  if (shift && index === 0) return last;
  if (!shift && index === list.length - 1) return first;
  return null;
}

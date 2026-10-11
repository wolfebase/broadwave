import { isTextField } from "./textField.ts";
import { tvBackKind } from "./tvKey.ts";

export { isTextField };

/** Escape closes a layer. Backspace closes it too, unless it is deleting text. */
/** The full player makes the page inert. A layer on the page must leave the player its keys. */
export function pageCovered(): boolean {
  return typeof document !== "undefined" && document.querySelector("main.content[inert]") != null;
}

export function dismissesLayer(key: string, target: EventTarget | null, code = 0): boolean {
  const kind = tvBackKind({ key, keyCode: code });
  if (kind === "escape") return true;
  if (kind === "backspace") return !isTextField(target);
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

import { isTextField } from "../lib/textField.ts";

type KeyLike = { key: string; ctrlKey: boolean; metaKey: boolean; altKey: boolean; defaultPrevented: boolean; target: EventTarget | null };

/**
 * "/" opens search from anywhere a page has the keys. A text field keeps the
 * character, and the full player and open dialogs keep their own keys.
 */
export function isSearchShortcut(event: KeyLike, busy: boolean): boolean {
  if (event.key !== "/" || event.ctrlKey || event.metaKey || event.altKey || event.defaultPrevented) return false;
  return !busy && !isTextField(event.target);
}

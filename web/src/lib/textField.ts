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

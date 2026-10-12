// LG webOS sends Back as keyCode 461. Samsung Tizen sends 10009.
// Fire TV's Back is Escape. A keyboard Backspace is the same idea on the
// other remotes, and it still deletes inside a text field.

export type KeyLike = { key: string; keyCode?: number; which?: number };

const namedBack = new Set(["Escape", "GoBack", "BrowserBack", "XF86Back", "Back"]);

function codeOf(event: KeyLike) {
  return event.keyCode || event.which || 0;
}

function unnamed(key: string) {
  return key === "Unidentified" || key === "Undefined" || key === "";
}

/** "escape" leaves a screen. "backspace" does too, except in a text field. */
export function tvBackKind(event: KeyLike): "escape" | "backspace" | null {
  if (event.key === "Backspace") return "backspace";
  if (namedBack.has(event.key)) return "escape";
  const code = codeOf(event);
  if (code === 461 || code === 10009) return "escape";
  // Fire TV's browser sometimes sends Android's Back code and no key name.
  if (unnamed(event.key) && (code === 27 || code === 4)) return "escape";
  if (unnamed(event.key) && code === 8) return "backspace";
  return null;
}

export function tvArrow(event: KeyLike): "left" | "right" | "up" | "down" | null {
  switch (event.key) {
    case "ArrowLeft":
    case "Left":
      return "left";
    case "ArrowRight":
    case "Right":
      return "right";
    case "ArrowUp":
    case "Up":
      return "up";
    case "ArrowDown":
    case "Down":
      return "down";
    default:
      break;
  }
  // A named key keeps its meaning. keyCode 37 is Left only when the browser
  // did not say which key it was.
  if (!unnamed(event.key)) return null;
  switch (codeOf(event)) {
    case 37:
      return "left";
    case 39:
      return "right";
    case 38:
      return "up";
    case 40:
      return "down";
    default:
      return null;
  }
}

const arrowName = { left: "ArrowLeft", right: "ArrowRight", up: "ArrowUp", down: "ArrowDown" } as const;

/** The key name the rest of the app already switches on. */
export function canonicalKey(event: KeyLike): string {
  const back = tvBackKind(event);
  if (back === "escape") return "Escape";
  if (back === "backspace") return "Backspace";
  const arrow = tvArrow(event);
  if (arrow) return arrowName[arrow];
  return event.key;
}

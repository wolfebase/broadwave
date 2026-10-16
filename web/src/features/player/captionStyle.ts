/** How captions look on this device. Kept per browser, like the volume. */
export type CaptionStyle = { size: "small" | "medium" | "large"; back: "none" | "box" | "solid"; color: "white" | "yellow" };

const key = "broadwave-cc-style";
export const defaultCaptionStyle: CaptionStyle = { size: "medium", back: "box", color: "white" };

export function parseCaptionStyle(raw: string | null): CaptionStyle {
  try {
    const v = JSON.parse(raw ?? "{}") as Partial<CaptionStyle>;
    return {
      size: v.size === "small" || v.size === "large" ? v.size : "medium",
      back: v.back === "none" || v.back === "solid" ? v.back : "box",
      color: v.color === "yellow" ? "yellow" : "white",
    };
  } catch {
    return { ...defaultCaptionStyle };
  }
}

export function readCaptionStyle(): CaptionStyle {
  try {
    return parseCaptionStyle(localStorage.getItem(key));
  } catch {
    return { ...defaultCaptionStyle };
  }
}

/** The page carries the style, so every player (live, recordings, multiview) draws captions the same way. */
export function applyCaptionStyle(style: CaptionStyle, root: HTMLElement = document.documentElement) {
  root.dataset.ccSize = style.size;
  root.dataset.ccBack = style.back;
  root.dataset.ccColor = style.color;
}

export function saveCaptionStyle(style: CaptionStyle) {
  try {
    localStorage.setItem(key, JSON.stringify(style));
  } catch {
    // Storage off: the style lasts as long as the page.
  }
  applyCaptionStyle(style);
}

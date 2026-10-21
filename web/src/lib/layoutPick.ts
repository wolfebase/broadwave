export type Layout = "desktop" | "tv" | "phone";

/** What the browser says about itself. Kept plain so the choice can be tested. */
export type LayoutSignals = {
  /** ?layout= on this visit, or "" */
  forced: string;
  /** a ?layout= this browser was given before, or "" */
  remembered: string;
  /** the window is phone-narrow */
  narrow: boolean;
  /** the main pointer is coarse or absent, on a wide window */
  wideCoarse: boolean;
  /** touch points the browser reports (an iPad says 5) */
  touchPoints: number;
  userAgent: string;
};

// Smart TV and set-top browsers name themselves. Tablets don't, and have touch.
const tvAgent = /\b(SmartTV|SMART-TV|Smart TV|GoogleTV|Google TV|AndroidTV|Android TV|AppleTV|Apple TV|tvOS|Tizen|Web0S|webOS|NetCast|HbbTV|BRAVIA|AFT[A-Z0-9]+|CrKey|Roku|PlayStation|Xbox|VIDAA|Viera|Philips.*TV|TV Safari)\b/i;

// Phones and tablets that are not TVs. Checked after the TV names.
const handheldAgent = /\b(iPad|iPhone|iPod|Android|Mobile)\b/;

export function isLayout(value: string): value is Layout {
  return value === "desktop" || value === "tv" || value === "phone";
}

/**
 * The 10-foot layout is for a browser on a TV: one that says so, or a wide
 * window with no fine pointer and no touch. A tablet has touch, so an iPad in
 * landscape gets the desktop layout. An explicit choice always wins.
 */
export function pickLayout(s: LayoutSignals): Layout {
  if (isLayout(s.forced)) return s.forced;
  if (isLayout(s.remembered)) return s.remembered;
  if (tvAgent.test(s.userAgent)) return "tv";
  if (s.narrow) return "phone";
  const touch = s.touchPoints > 0 || handheldAgent.test(s.userAgent);
  if (s.wideCoarse && !touch) return "tv";
  return "desktop";
}

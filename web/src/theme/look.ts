/** This device's appearance. The server does not store it. */

export const themeChoices = ["system", "light", "dark"] as const;
export type ThemeChoice = (typeof themeChoices)[number];
export type ResolvedTheme = "light" | "dark";

/** Six accents. `dark` is the bright ink on the dark surfaces. `light` is dark enough for white button text and for links on the lightest card. */
export const accents = [
  { id: "blue", label: "Blue", dark: "#3D7BFF", darkSoft: "#3D7BFF29", light: "#1D4ED8", lightSoft: "#1D4ED829" },
  { id: "teal", label: "Teal", dark: "#2DD4BF", darkSoft: "#2DD4BF29", light: "#0D6962", lightSoft: "#0D696229" },
  { id: "green", label: "Green", dark: "#34D399", darkSoft: "#34D39929", light: "#0D6B3D", lightSoft: "#0D6B3D29" },
  { id: "orange", label: "Orange", dark: "#FB923C", darkSoft: "#FB923C29", light: "#A7380A", lightSoft: "#A7380A29" },
  { id: "purple", label: "Purple", dark: "#A78BFA", darkSoft: "#A78BFA29", light: "#6D28D9", lightSoft: "#6D28D929" },
  { id: "rose", label: "Rose", dark: "#FB7185", darkSoft: "#FB718529", light: "#B8113A", lightSoft: "#B8113A29" },
] as const;

export type AccentId = (typeof accents)[number]["id"];

const themeKey = "broadwave-theme";
const accentKey = "broadwave-accent";

export const themeCanvas = { light: "#F4F6FA", dark: "#070A10" } as const;

export function parseTheme(value: string | null): ThemeChoice {
  if (value === "light" || value === "dark" || value === "system") return value;
  return "dark";
}

export function parseAccent(value: string | null): AccentId {
  const found = accents.find((item) => item.id === value);
  return found ? found.id : "blue";
}

/** System follows the device. Light and dark stay put when the device changes. */
export function resolveTheme(choice: ThemeChoice, prefersDark: boolean): ResolvedTheme {
  if (choice === "light" || choice === "dark") return choice;
  return prefersDark ? "dark" : "light";
}

function storageGet(key: string): string | null {
  try {
    return globalThis.localStorage?.getItem(key) ?? null;
  } catch {
    return null;
  }
}

function storageSet(key: string, value: string) {
  try {
    localStorage.setItem(key, value);
  } catch {
    // Private mode still paints the choice for this visit.
  }
}

export function readTheme(): ThemeChoice {
  return parseTheme(storageGet(themeKey));
}

export function readAccent(): AccentId {
  return parseAccent(storageGet(accentKey));
}

function prefersDark(): boolean {
  return window.matchMedia("(prefers-color-scheme: dark)").matches;
}

export function applyTheme(theme: ResolvedTheme, accent: AccentId) {
  const root = document.documentElement;
  root.dataset.theme = theme;
  root.dataset.accent = accent;
  document.querySelector('meta[name="color-scheme"]')?.setAttribute("content", theme);
  document.querySelector('meta[name="theme-color"]')?.setAttribute("content", themeCanvas[theme]);
  document.querySelector('meta[name="apple-mobile-web-app-status-bar-style"]')?.setAttribute("content", theme === "light" ? "default" : "black-translucent");
}

function applyStored() {
  applyTheme(resolveTheme(readTheme(), prefersDark()), readAccent());
}

export function saveTheme(choice: ThemeChoice): ThemeChoice {
  storageSet(themeKey, choice);
  applyStored();
  return choice;
}

export function saveAccent(id: AccentId): AccentId {
  storageSet(accentKey, id);
  applyStored();
  return id;
}

let listening = false;

/** Paint the saved choice, then follow the device while the choice is System. */
export function installLook() {
  applyStored();
  if (listening) return;
  listening = true;
  window.matchMedia("(prefers-color-scheme: dark)").addEventListener("change", () => {
    if (readTheme() === "system") applyStored();
  });
}

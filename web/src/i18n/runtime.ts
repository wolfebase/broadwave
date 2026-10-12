// English is the only shipped language. Pseudo is a layout check, not a language:
// it repeats a translated string so its length doubles. Clocks and dates follow the
// browser's locale and are not stretched, so a sentence that includes one is not doubled twice.

export const shippedLocale = "en";

export type LocaleMode = "en" | "pseudo";

export type PluralMessage = {
  readonly zero?: string;
  readonly one?: string;
  readonly two?: string;
  readonly few?: string;
  readonly many?: string;
  readonly other: string;
};

export type Message = string | PluralMessage;

export type Vars = Record<string, string | number>;

const plurals = new Intl.PluralRules(shippedLocale);
const relative = new Intl.RelativeTimeFormat(shippedLocale, { numeric: "always" });

let mode: LocaleMode = "en";

export function localeMode(): LocaleMode {
  return mode;
}

export function setLocaleMode(next: LocaleMode): void {
  mode = next;
  const root = typeof document !== "undefined" ? document.documentElement : undefined;
  if (root) root.dataset.locale = next;
}

function finish(text: string): string {
  return mode === "pseudo" ? text + text : text;
}

function fill(template: string, vars?: Vars): string {
  if (!vars) return template;
  return template.replace(/\{(\w+)\}/g, (whole, key: string) => (Object.hasOwn(vars, key) ? String(vars[key]) : whole));
}

/** English maps 0 to "other". A catalog `zero` line is the product's "none" sentence. */
export function translate(message: Message, vars?: Vars, count?: number): string {
  if (typeof message === "string") return finish(fill(message, vars));
  const n = count ?? 0;
  const category = n <= 0 && message.zero !== undefined ? "zero" : plurals.select(n);
  const template = message[category] ?? message.other;
  return finish(fill(template, { ...vars, count: n }));
}

export function bind<const T extends Record<string, Message>>(catalog: T) {
  function t<K extends keyof T>(id: K, ...args: T[K] extends string ? [vars?: Vars] : [count: number, vars?: Vars]): string {
    const message = catalog[id];
    if (typeof message === "string") return translate(message, args[0] as Vars | undefined);
    return translate(message, args[1] as Vars | undefined, args[0] as number);
  }
  return t;
}

export function formatTime(date: Date): string {
  return date.toLocaleTimeString([], { hour: "numeric", minute: "2-digit" });
}

export function formatDateTime(date: Date, options?: Intl.DateTimeFormatOptions): string {
  return options ? date.toLocaleString([], options) : date.toLocaleString();
}

/** A whole phrase ("5 minutes ago"). Doubled in pseudo because nothing else wraps it. */
export function formatRelative(value: number, unit: Intl.RelativeTimeFormatUnit): string {
  return finish(relative.format(value, unit));
}

/** Sunday-first short names, from Intl, matching the pass editor's day order. */
export function weekdayShortNames(): string[] {
  return [0, 1, 2, 3, 4, 5, 6].map((index) =>
    new Intl.DateTimeFormat(shippedLocale, { weekday: "short" }).format(new Date(2024, 0, 7 + index)),
  );
}

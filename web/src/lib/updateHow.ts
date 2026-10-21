import { shellMessages } from "../i18n/messages/shell.ts";
import { bind } from "../i18n/runtime.ts";

const t = bind(shellMessages);

export type InstallKind = "docker" | "unraid" | "updater" | "binary";

const kinds: InstallKind[] = ["unraid", "docker", "updater", "binary"];

/** The server may say how it was installed; older servers don't. */
export function installKind(server: unknown): InstallKind | "" {
  if (!server || typeof server !== "object") return "";
  const raw = (server as Record<string, unknown>).install;
  const value = typeof raw === "string" ? raw.toLowerCase() : "";
  if (value === "compose") return "docker";
  return (kinds as string[]).includes(value) ? (value as InstallKind) : "";
}

/** A plain sentence the server gives for its own install, when it has one. */
export function updateHint(server: unknown): string {
  if (!server || typeof server !== "object") return "";
  const raw = (server as Record<string, unknown>).updateHow;
  return typeof raw === "string" ? raw.trim() : "";
}

/**
 * How to update, in plain steps. With a known install only its own steps show;
 * otherwise each kind gets one line.
 */
export function updateSteps(kind: InstallKind | ""): { label: string; text: string }[] {
  const pick = kind ? [kind] : kinds;
  return pick.map((item) => ({ label: t(`update.${item}.label`), text: t(`update.${item}.text`) }));
}

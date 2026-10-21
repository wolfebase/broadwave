import { shellMessages } from "../i18n/messages/shell.ts";
import { bind } from "../i18n/runtime.ts";

const t = bind(shellMessages);

export type InstallKind = "docker" | "unraid" | "updater" | "binary";

const kinds: InstallKind[] = ["unraid", "docker", "updater", "binary"];

/** What the update notice may carry about this install. Older servers send neither. */
export type UpdateInstall = { install?: string; howTo?: string } | null | undefined;

/** How the server says it was installed: unraid, docker, or binary. */
export function installKind(update: UpdateInstall): InstallKind | "" {
  const value = (update?.install ?? "").toLowerCase();
  return (kinds as string[]).includes(value) ? (value as InstallKind) : "";
}

/** The server's own sentence on updating this install, when it has one. */
export function updateHint(update: UpdateInstall): string {
  return (update?.howTo ?? "").trim();
}

/**
 * How to update, in plain steps. With a known install only its own steps show;
 * otherwise each kind gets one line.
 */
export function updateSteps(kind: InstallKind | ""): { label: string; text: string }[] {
  const pick = kind ? [kind] : kinds;
  return pick.map((item) => ({ label: t(`update.${item}.label`), text: t(`update.${item}.text`) }));
}

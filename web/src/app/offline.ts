import { shellMessages } from "../i18n/messages/shell.ts";
import { bind } from "../i18n/runtime.ts";

const shell = bind(shellMessages);
const nameKey = "broadwave.serverName";

export function rememberServer(name: string | undefined) {
  try {
    if (name) localStorage.setItem(nameKey, name);
  } catch {
    // Without storage the line says Broadwave instead of the server's name.
  }
}

/** A server that did not answer reads as where it is, not as the browser's error. Null for any other failure. */
export function unreachableLine(err: unknown, saved: boolean): string | null {
  const status = (err as { status?: number } | null)?.status;
  if (!(err instanceof TypeError) && status !== 502 && status !== 503 && status !== 504) return null;
  let name: string;
  try {
    name = localStorage.getItem(nameKey) ?? "";
  } catch {
    name = "";
  }
  if (!name) return shell(saved ? "shell.unreachableSaved" : "shell.unreachableRetry");
  return shell(saved ? "shell.unreachableNamedSaved" : "shell.unreachableNamed", { name });
}

import { shellMessages } from "../i18n/messages/shell.ts";
import { bind } from "../i18n/runtime.ts";

const t = bind(shellMessages);

/** A failed action shows the server's sentence, or the catalog line when there is none. */
export function actionError(err: unknown, id: keyof typeof shellMessages): string {
  if (err instanceof Error) return err.message;
  return t(id);
}

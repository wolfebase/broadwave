import type { RecordingHealth } from "../../api/generated";
import { libraryMessages } from "../../i18n/messages/library.ts";
import { bind } from "../../i18n/runtime.ts";

const t = bind(libraryMessages);

/** A line when the finished file was damaged. Empty when it was clean or not counted yet. */
export function signalLine(health: RecordingHealth | undefined): string {
  if (!health) return "";
  const lost = Math.round(health.lostSeconds ?? 0);
  if (lost >= 1) return t("library.signalDropped", { seconds: lost });
  const times = health.continuityErrors + health.transportErrors + health.syncLosses;
  if (times <= 0) return "";
  return t("library.signalBroke", times);
}

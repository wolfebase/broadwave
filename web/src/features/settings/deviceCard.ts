import { settingsUiMessages } from "../../i18n/messages/settingsUi.ts";
import { bind } from "../../i18n/runtime.ts";

const t = bind(settingsUiMessages);

// A tuner found on the network has no source row. A playlist, a link, and a
// folder do, and they do not scan. The device itself has no kind field.
const scanKinds = new Set(["hdhomerun", "hdhr-compatible"]);

export function deviceScans(sourceKind: string | undefined): boolean {
  if (sourceKind == null) return true;
  return scanKinds.has(sourceKind);
}

export function showFirmware(version: string | undefined): boolean {
  return Boolean(version && version.trim() !== "");
}

// The tuner status lists only tuners that answer. count is every tuner the
// server knows, so a tuner that is off is said, not dropped from the total.
export function tunerLine(busy: number, answering: number, count?: number): string {
  if (answering === 0) return count ? t("settings.tunersNone", { count }) : t("settings.tunersUnavailable");
  const line = busy === 0 ? t("settings.tunersFree", { count: answering }) : t("settings.tunersBusy", { busy, answering });
  const silent = (count ?? 0) - answering;
  if (silent <= 0) return line;
  return `${line} ${t("settings.tunersSilent", silent)}`;
}

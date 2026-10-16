import { setupMessages } from "../../i18n/messages/setup.ts";
import { bind } from "../../i18n/runtime.ts";

const t = bind(setupMessages);

/** "3 channels from 2 tuners." A lineup with no tuner counts its sources. */
export function setupSummary(devices: { tunerCount: number }[], channels: number): string {
  const tuners = devices.reduce((sum, d) => sum + Math.max(0, d.tunerCount), 0);
  const from = tuners > 0 ? t("setup.tunerCount", tuners) : t("setup.sourceCount", devices.length);
  return t("setup.summaryTuners", channels, { tuners: from });
}

import { setupMessages } from "../../i18n/messages/setup.ts";
import { bind } from "../../i18n/runtime.ts";

const t = bind(setupMessages);

/** "3 channels from 2 tuners." A lineup with no tuner counts its sources. */
export function setupSummary(devices: { tunerCount: number }[], channels: number): string {
  const tuners = devices.reduce((sum, d) => sum + Math.max(0, d.tunerCount), 0);
  const from = tuners > 0 ? t("setup.tunerCount", tuners) : t("setup.sourceCount", devices.length);
  return t("setup.summaryTuners", channels, { tuners: from });
}

/** The found list's heading: an antenna tuner, a playlist or other streamed source, or both. */
export function foundHeading(devices: { tunerCount: number }[]): string {
  const tuners = devices.filter((d) => d.tunerCount > 0).length;
  const sources = devices.length - tuners;
  if (tuners > 0 && sources > 0) return t("setup.yourTunersAndSources");
  if (tuners > 0) return t("setup.yourTuner", tuners);
  return t("setup.yourSource", sources);
}

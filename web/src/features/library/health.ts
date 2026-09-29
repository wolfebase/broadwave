import type { RecordingHealth } from "../../api/generated";

/** A line when the finished file was damaged. Empty when it was clean or not counted yet. */
export function signalLine(health: RecordingHealth | undefined): string {
  if (!health) return "";
  const times = health.continuityErrors + health.transportErrors + health.syncLosses;
  if (times <= 0) return "";
  if (times === 1) return "Signal broke up once";
  return `Signal broke up ${times} times`;
}

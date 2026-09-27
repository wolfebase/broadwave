const MAX_LEAD_S = 2;

/**
 * How far past the target the next forward seek aims, in seconds. Safari takes
 * most of a second to decode up to a seek target, so a seek that aimed at the
 * target landed that far behind it and the engine seeked again every two
 * seconds. drift is the error a second after the last seek, in ms.
 */
export function nextSeekLead(lead: number, drift: number): number {
  return Math.min(MAX_LEAD_S, Math.max(0, lead - drift / 1000));
}

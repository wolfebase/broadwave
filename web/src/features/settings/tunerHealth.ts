/** The host a tuner answers on, for "It's back" and the row's facts. */
export function tunerHost(baseUrl: string | undefined): string {
  if (!baseUrl) return "";
  try {
    const url = new URL(baseUrl);
    return url.port && url.port !== "80" ? `${url.hostname}:${url.port}` : url.hostname;
  } catch {
    return baseUrl.replace(/^\w+:\/\//, "").replace(/\/.*$/, "");
  }
}

export type SignalWord = "tuners.signalStrong" | "tuners.signalGood" | "tuners.signalWeak" | "tuners.signalPoor";

/**
 * One word for how a tuned channel comes in. The tuner reports strength and
 * quality as percents; quality is what the picture shows, so it decides.
 */
export function signalWord(strength: number | undefined, quality: number | undefined): SignalWord | "" {
  if (strength == null && quality == null) return "";
  const q = quality ?? strength ?? 0;
  const s = strength ?? quality ?? 0;
  if (q >= 90 && s >= 70) return "tuners.signalStrong";
  if (q >= 75) return "tuners.signalGood";
  if (q >= 50) return "tuners.signalWeak";
  return "tuners.signalPoor";
}

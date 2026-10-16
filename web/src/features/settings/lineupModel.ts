import type { Channel, Device } from "../../types";

/** What a lineup row offers in place of a plain on/off switch. */
export type LineupState =
  | { kind: "switch"; on: boolean }
  | { kind: "pair"; choice: "atsc3" | "atsc1" | "both"; twin: string }
  | { kind: "shownAs"; number: string }
  | { kind: "encrypted"; playsAs: string }
  | { kind: "offAir" }
  | { kind: "duplicate"; of: string; tuner: string };

export type LineupRow = {
  channel: Channel;
  state: LineupState;
  /** The tuner this row comes from, by its short name. */
  tuner: string;
  /** Duplicates and off-air rows hide unless asked for. */
  extra: boolean;
};

/** "HDHomeRun FLEX 4K" reads as "FLEX 4K" in a narrow column. */
export function shortTuner(device: Device | undefined): string {
  if (!device) return "";
  const name = (device.friendlyName || device.modelNumber || device.deviceId).trim();
  const short = name.replace(/^HDHomeRun\s+/i, "");
  return short || name;
}

export function onGuide(channel: Channel): boolean {
  return channel.enabled && !channel.hidden;
}

/** One switch stands for both flags: a hidden channel and a turned-off one are both off the guide. */
export function switchPatch(on: boolean): { enabled?: boolean; hidden: boolean } {
  return on ? { enabled: true, hidden: false } : { hidden: true };
}

function pairChoice(value: string | undefined): "atsc3" | "atsc1" | "both" {
  return value === "atsc3" || value === "atsc1" ? value : "both";
}

export function lineupRows(channels: readonly Channel[], devices: readonly Device[]): LineupRow[] {
  const byId = new Map(channels.map((channel) => [channel.id, channel]));
  const deviceById = new Map(devices.map((device) => [device.deviceId, device]));
  const numberOf = (id: number | undefined) => (id != null ? (byId.get(id)?.displayNumber ?? "") : "");
  return channels.map((channel) => {
    const tuner = shortTuner(deviceById.get(channel.deviceId));
    const base = { channel, tuner };
    if (!channel.present) return { ...base, extra: true, state: { kind: "offAir" } };
    if (channel.sameAs) {
      const shown = byId.get(channel.sameAs);
      return { ...base, extra: true, state: { kind: "duplicate", of: shown?.displayNumber ?? channel.displayNumber, tuner: shortTuner(shown ? deviceById.get(shown.deviceId) : undefined) } };
    }
    if (channel.protected) return { ...base, extra: false, state: { kind: "encrypted", playsAs: numberOf(channel.playsAs) } };
    if (channel.twinId) {
      const choice = pairChoice(channel.twinChoice);
      if (channel.standard === "atsc3") return { ...base, extra: false, state: { kind: "pair", choice, twin: numberOf(channel.twinId) } };
      // The 1.0 half follows the choice made on its 3.0 twin.
      if (choice === "atsc3") return { ...base, extra: false, state: { kind: "shownAs", number: numberOf(channel.twinId) } };
    }
    return { ...base, extra: false, state: { kind: "switch", on: onGuide(channel) } };
  });
}

/** Number, name, call sign, or guide match. A number matches from its start, so 4 finds 4.1 and not 14.1. */
export function matchesLineup(channel: Channel, query: string): boolean {
  const q = query.trim().toLocaleLowerCase();
  if (!q) return true;
  if (/^[\d.-]+$/.test(q)) return channel.displayNumber.startsWith(q) || channel.guideNumber.startsWith(q);
  return [channel.displayName, channel.guideName, channel.guideKey ?? "", channel.network ?? ""].some((text) => text.toLocaleLowerCase().includes(q));
}

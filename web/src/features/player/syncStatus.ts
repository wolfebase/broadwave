export type SyncPillState = "off" | "waiting" | "syncing" | "locked";

/** The words on the sync pill. */
export function syncPillText(together: boolean, members: number): string {
  if (together) return members > 1 ? `Together · ${members}` : "Together";
  if (members > 1) return `${members} screens`;
  return "Synced";
}

/**
 * What a screen reader hears when sync changes. Waiting and syncing are not
 * written on the pill; the color is the only other signal. Off is silent until
 * sync has been on, so the first paint does not announce "Sync off".
 */
export function syncLiveText(state: SyncPillState, together: boolean, members: number, wasOn = false): string {
  if (state === "off") return wasOn ? "Sync off" : "";
  if (state === "waiting") return "Waiting to sync";
  if (state === "syncing") return members > 1 ? `Syncing ${members} screens` : "Syncing";
  return syncPillText(together, members);
}

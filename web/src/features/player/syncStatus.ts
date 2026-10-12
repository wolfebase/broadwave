import { playerUiMessages } from "../../i18n/messages/playerUi.ts";
import { bind } from "../../i18n/runtime.ts";

const t = bind(playerUiMessages);

export type SyncPillState = "off" | "waiting" | "syncing" | "locked";

/** The words on the sync pill. */
export function syncPillText(together: boolean, members: number): string {
  if (together) return members > 1 ? t("player.sync.togetherCount", { count: members }) : t("player.sync.together");
  if (members > 1) return t("player.sync.screens", members);
  return t("player.sync.synced");
}

/**
 * What a screen reader hears when sync changes. Waiting and syncing are not
 * written on the pill; the color is the only other signal. Off is silent until
 * sync has been on, so the first paint does not announce "Sync off".
 */
export function syncLiveText(state: SyncPillState, together: boolean, members: number, wasOn = false): string {
  if (state === "off") return wasOn ? t("player.sync.off") : "";
  if (state === "waiting") return t("player.sync.waiting");
  if (state === "syncing") return members > 1 ? t("player.sync.syncingScreens", members) : t("player.sync.syncing");
  return syncPillText(together, members);
}

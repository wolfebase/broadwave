import { playerUiMessages } from "../../i18n/messages/playerUi.ts";
import { bind } from "../../i18n/runtime.ts";

const t = bind(playerUiMessages);

export type SyncPillState = "off" | "waiting" | "syncing" | "locked";

/**
 * The words on the sync pill. They follow the state, so the pill never says
 * "Synced" while sync is still waiting or catching up. A screen alone in its
 * room has nothing to be in sync with, so it says sync is on, even while it
 * settles: "Syncing" on every channel start read as something going wrong.
 */
export function syncPillText(state: SyncPillState, together: boolean, members: number): string {
  if (state === "off") return t("player.sync.off");
  if (!together && members <= 1) return t("player.sync.on");
  if (state === "waiting") return t("player.sync.waitingShort");
  if (state === "syncing") return t("player.sync.syncing");
  if (together) return members > 1 ? t("player.sync.togetherCount", { count: members }) : t("player.sync.together");
  if (members > 1) return t("player.sync.screens", members);
  return t("player.sync.on");
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
  if (!together && members > 1) return t("player.sync.syncedScreens", members);
  return syncPillText(state, together, members);
}

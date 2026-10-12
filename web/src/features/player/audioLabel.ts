import { playerUiMessages } from "../../i18n/messages/playerUi.ts";
import { bind } from "../../i18n/runtime.ts";

const t = bind(playerUiMessages);

/** The sound line in the stats panel. A copied stream names the broadcast codec beside Original. */
export function streamAudioLabel(audio: string | undefined, sourceAudio?: string): string {
  if (audio === "copy") return `${t("player.original")} ${sourceAudio ?? ""}`;
  if (audio === "aac6") return t("player.audio51aac");
  if (audio === "ac3") return t("player.audio51ac3");
  return t("player.audioStereoAac");
}

/** The label of the caption track the player adds. The language code stays en. */
export function captionTrackName(): string {
  return t("player.captionsEnglish");
}

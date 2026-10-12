import { copyMessages } from "./i18n/messages/copy.ts";
import { bind } from "./i18n/runtime.ts";
import { encryptedBroadcast } from "./strings/notice.ts";

const t = bind(copyMessages);

const playerKeys = [
  ["player.key.space", "player.key.spaceDoes"],
  ["player.key.left", "player.key.leftDoes"],
  ["player.key.right", "player.key.rightDoes"],
  ["player.key.up", "player.key.upDoes"],
  ["player.key.down", "player.key.downDoes"],
  ["player.key.escape", "player.key.escapeDoes"],
  ["player.key.last", "player.key.lastDoes"],
  ["player.key.channels", "player.key.channelsDoes"],
  ["player.key.stats", "player.key.statsDoes"],
  ["player.key.side", "player.key.sideDoes"],
  ["player.key.record", "player.key.recordDoes"],
  ["player.key.captions", "player.key.captionsDoes"],
  ["player.key.theater", "player.key.theaterDoes"],
  ["player.key.help", "player.key.helpDoes"],
  ["player.key.number", "player.key.numberDoes"],
] as const;

export const copy = {
  sources: {
    get title() {
      return t("sources.title");
    },
    get lead() {
      return t("sources.lead");
    },
    get search() {
      return t("sources.search");
    },
    get look() {
      return t("sources.look");
    },
    get lookHint() {
      return t("sources.lookHint");
    },
    get lookEmpty() {
      return t("sources.lookEmpty");
    },
    get free() {
      return t("sources.free");
    },
    get freeHint() {
      return t("sources.freeHint");
    },
    get freeEmpty() {
      return t("sources.freeEmpty");
    },
    get add() {
      return t("sources.add");
    },
    get lookup() {
      return t("sources.lookup");
    },
    get address() {
      return t("sources.address");
    },
    get addressPlaceholder() {
      return t("sources.addressPlaceholder");
    },
    get addressHint() {
      return t("sources.addressHint");
    },
    get none() {
      return t("sources.none");
    },
    get tuners() {
      return t("sources.tuners");
    },
    get firmware() {
      return t("sources.firmware");
    },
    get scan() {
      return t("sources.scan");
    },
    get scanning() {
      return t("sources.scanning");
    },
    get remove() {
      return t("sources.remove");
    },
    offline: (when: string) => (when ? t("sources.offlineSeen", { when }) : t("sources.offline")),
    streams: (used: number, limit: number) => t("sources.streams", { used, limit }),
    removeLabel: (name: string) => t("sources.removeLabel", { name }),
    removeConfirm: (name: string) => t("sources.removeConfirm", { name }),
    get channels() {
      return t("sources.channels");
    },
    get enabled() {
      return t("sources.enabled");
    },
    get hidden() {
      return t("sources.hidden");
    },
    get name() {
      return t("sources.name");
    },
    get number() {
      return t("sources.number");
    },
    get match() {
      return t("sources.match");
    },
    get matchHint() {
      return t("sources.matchHint");
    },
    get atsc3() {
      return t("sources.atsc3");
    },
    encrypted: (atsc3: boolean) => (atsc3 ? t("sources.encryptedAtsc3") : t("sources.encryptedCopy")),
    playsAs: (name: string) => t("sources.playsAs", { name }),
    get twinShow() {
      return t("sources.twinShow");
    },
    twinChoices: {
      get atsc3() {
        return t("sources.twin.atsc3");
      },
      atsc1: (name: string) => (name ? t("sources.twin.atsc1Named", { name }) : t("sources.twin.atsc1")),
      get both() {
        return t("sources.twin.both");
      },
    },
    twinOf: (name: string) => t("sources.twinOf", { name }),
    get groups() {
      return t("sources.groups");
    },
    get groupsHint() {
      return t("sources.groupsHint");
    },
    get file() {
      return t("sources.file");
    },
    get pick() {
      return t("sources.pick");
    },
  },
  library: {
    get title() {
      return t("library.title");
    },
    get body() {
      return t("library.body");
    },
    get gone() {
      return t("library.gone");
    },
    get removeGone() {
      return t("library.removeGone");
    },
    get findCommercials() {
      return t("library.findCommercials");
    },
    get findingCommercials() {
      return t("library.findingCommercials");
    },
    foundBreaks: (count: number) => t("library.foundBreaks", count),
    get findFailed() {
      return t("library.findFailed");
    },
  },
  schedule: {
    get title() {
      return t("schedule.title");
    },
  },
  settings: {
    get title() {
      return t("settings.title");
    },
    get appearance() {
      return t("settings.appearance");
    },
    get appearanceHint() {
      return t("settings.appearanceHint");
    },
    get accent() {
      return t("settings.accent");
    },
    get accentHint() {
      return t("settings.accentHint");
    },
    get screenName() {
      return t("settings.screenName");
    },
    get screenNameHint() {
      return t("settings.screenNameHint");
    },
    get liveDelay() {
      return t("settings.liveDelay");
    },
    get liveDelayHint() {
      return t("settings.liveDelayHint");
    },
    get adaptive() {
      return t("settings.adaptive");
    },
    get adaptiveHint() {
      return t("settings.adaptiveHint");
    },
    get layout() {
      return t("settings.layout");
    },
    get layoutHint() {
      return t("settings.layoutHint");
    },
    get recordings() {
      return t("settings.recordings");
    },
    get recordingsHint() {
      return t("settings.recordingsHint");
    },
    get byShow() {
      return t("settings.byShow");
    },
    get byShowEmpty() {
      return t("settings.byShowEmpty");
    },
    get byShowFailed() {
      return t("settings.byShowFailed");
    },
    get nfo() {
      return t("settings.nfo");
    },
    get nfoHint() {
      return t("settings.nfoHint");
    },
    get reserve() {
      return t("settings.reserve");
    },
    get reserveHint() {
      return t("settings.reserveHint");
    },
    get buffer() {
      return t("settings.buffer");
    },
    get bufferHint() {
      return t("settings.bufferHint");
    },
    get backup() {
      return t("settings.backup");
    },
    get backups() {
      return t("settings.backups");
    },
    get backupHint() {
      return t("settings.backupHint");
    },
    get backupEmpty() {
      return t("settings.backupEmpty");
    },
    get backupFailed() {
      return t("settings.backupFailed");
    },
    get backupDownload() {
      return t("settings.backupDownload");
    },
    get backupNightly() {
      return t("settings.backupNightly");
    },
    get backupWeekly() {
      return t("settings.backupWeekly");
    },
    get backupUpdate() {
      return t("settings.backupUpdate");
    },
    get restore() {
      return t("settings.restore");
    },
    get restoreConfirm() {
      return t("settings.restoreConfirm");
    },
    get restoreFailed() {
      return t("settings.restoreFailed");
    },
    get support() {
      return t("settings.support");
    },
    get supportHint() {
      return t("settings.supportHint");
    },
    get guideAccount() {
      return t("settings.guideAccount");
    },
    get guideAccountHint() {
      return t("settings.guideAccountHint");
    },
    get guideLineup() {
      return t("settings.guideLineup");
    },
    get guideAddress() {
      return t("settings.guideAddress");
    },
    get guideAddressHint() {
      return t("settings.guideAddressHint");
    },
    get movieArt() {
      return t("settings.movieArt");
    },
    get movieArtHint() {
      return t("settings.movieArtHint");
    },
    get hideScores() {
      return t("settings.hideScores");
    },
    get hideScoresHint() {
      return t("settings.hideScoresHint");
    },
    get scoresFrom() {
      return t("settings.scoresFrom");
    },
    get scoresFromSportsDB() {
      return t("settings.scoresFromSportsDB");
    },
    get liveScores() {
      return t("settings.liveScores");
    },
    get liveScoresHint() {
      return t("settings.liveScoresHint");
    },
    get sportsdbKey() {
      return t("settings.sportsdbKey");
    },
    get sportsdbKeyHint() {
      return t("settings.sportsdbKeyHint");
    },
    get updates() {
      return t("settings.updates");
    },
    get updatesHint() {
      return t("settings.updatesHint");
    },
    get passwordTitle() {
      return t("settings.passwordTitle");
    },
    get passwordBody() {
      return t("settings.passwordBody");
    },
    get profile() {
      return t("settings.profile");
    },
  },
  player: {
    get encrypted() {
      return encryptedBroadcast();
    },
    get stats() {
      return t("player.stats");
    },
    get delayApple() {
      return t("player.delayApple");
    },
    get playing() {
      return t("player.playing");
    },
    get source() {
      return t("player.source");
    },
    get output() {
      return t("player.output");
    },
    get tuner() {
      return t("player.tuner");
    },
    get bitrate() {
      return t("player.bitrate");
    },
    get dropped() {
      return t("player.dropped");
    },
    get buffer() {
      return t("player.buffer");
    },
    get behind() {
      return t("player.behind");
    },
    get drift() {
      return t("player.drift");
    },
    get rendition() {
      return t("player.rendition");
    },
    get encoder() {
      return t("player.encoder");
    },
    get sound() {
      return t("player.sound");
    },
    get waiting() {
      return t("player.waiting");
    },
    get helpTitle() {
      return t("player.helpTitle");
    },
    get close() {
      return t("player.close");
    },
    get sleep() {
      return t("player.sleep");
    },
    get sleep30() {
      return t("player.sleep30");
    },
    get sleep60() {
      return t("player.sleep60");
    },
    get sleep90() {
      return t("player.sleep90");
    },
    get sleepHint() {
      return t("player.sleepHint");
    },
    get volume() {
      return t("player.volume");
    },
    get keys() {
      return playerKeys.map(([name, does]) => [t(name), t(does)] as [string, string]);
    },
  },
};

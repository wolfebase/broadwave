import affiliationsBody from "../../../api/fixtures/affiliations.json";
import airingsBody from "../../../api/fixtures/airings.json";
import backupRestoreBody from "../../../api/fixtures/backup-restore.json";
import channelsBody from "../../../api/fixtures/channels.json";
import devicesBody from "../../../api/fixtures/devices.json";
import discoverBody from "../../../api/fixtures/discover.json";
import eventsBody from "../../../api/fixtures/events.json";
import freeAddBody from "../../../api/fixtures/free-add.json";
import freeBody from "../../../api/fixtures/free.json";
import guideRefreshBody from "../../../api/fixtures/guide-refresh.json";
import homeBody from "../../../api/fixtures/home.json";
import lookBody from "../../../api/fixtures/look.json";
import markerCreateBody from "../../../api/fixtures/marker-create.json";
import markerDeleteBody from "../../../api/fixtures/marker-delete.json";
import markersBody from "../../../api/fixtures/markers.json";
import multiviewBody from "../../../api/fixtures/multiview.json";
import passCreateBody from "../../../api/fixtures/pass-create.json";
import passDeleteBody from "../../../api/fixtures/pass-delete.json";
import passesBody from "../../../api/fixtures/passes.json";
import recordingCreateBody from "../../../api/fixtures/recording-create.json";
import recordingDeleteBody from "../../../api/fixtures/recording-delete.json";
import recordingDetectBody from "../../../api/fixtures/recording-detect.json";
import recordingPlayBody from "../../../api/fixtures/recording-play.json";
import recordingProgressBody from "../../../api/fixtures/recording-progress.json";
import recordingStopBody from "../../../api/fixtures/recording-stop.json";
import recordingWatchedBody from "../../../api/fixtures/recording-watched.json";
import recordingsBody from "../../../api/fixtures/recordings.json";
import scanStatusBody from "../../../api/fixtures/scan-status.json";
import scanBody from "../../../api/fixtures/scan.json";
import scheduleSkipBody from "../../../api/fixtures/schedule-skip.json";
import serverRenameBody from "../../../api/fixtures/server-rename.json";
import serverBody from "../../../api/fixtures/server.json";
import settingsSaveBody from "../../../api/fixtures/settings-save.json";
import settingsBody from "../../../api/fixtures/settings.json";
import setupFinishDoneBody from "../../../api/fixtures/setup-finish-done.json";
import setupFinishPostBody from "../../../api/fixtures/setup-finish-post.json";
import setupFinishBody from "../../../api/fixtures/setup-finish.json";
import signalsCheckBody from "../../../api/fixtures/signals-check.json";
import signalsBody from "../../../api/fixtures/signals.json";
import starBody from "../../../api/fixtures/star.json";
import teamUnfollowBody from "../../../api/fixtures/team-unfollow.json";
import teamsBody from "../../../api/fixtures/teams.json";
import tunersBody from "../../../api/fixtures/tuners.json";
import virtualCreateBody from "../../../api/fixtures/virtual-create.json";
import watchStopBody from "../../../api/fixtures/watch-stop.json";
import watchBody from "../../../api/fixtures/watch.json";
import wsActivityBody from "../../../api/fixtures/ws-activity.json";
import wsLiveBody from "../../../api/fixtures/ws-live.json";
import wsSourcesBody from "../../../api/fixtures/ws-sources.json";
import xtreamBody from "../../../api/fixtures/xtream.json";
import type {
  Airing,
  Channel,
  ChannelSignal,
  Device,
  Event,
  Marker,
  MultiviewPlan,
  Pass,
  Recording,
  ServerInfo,
  Settings,
  SetupFinish,
  Source,
  TeamFollow,
  Tuner,
  VirtualChannel,
} from "./generated";

// Assigning the golden responses to the generated types fails the build when a
// server change and a client type disagree.
export function contractFixtures(): number {
  const channels: Channel[] = channelsBody.channels;
  const airings: Airing[] = airingsBody.airings;
  const recordings: Recording[] = recordingsBody.recordings;
  const devices: Device[] = devicesBody.devices;
  const passes: Pass[] = passesBody.passes;
  const teams: TeamFollow[] = teamsBody.teams;
  const events: Event[] = eventsBody.events;
  const markers: Marker[] = markersBody.markers;
  const signals: ChannelSignal[] = signalsBody.channels;
  const tuners: Tuner[] = tunersBody.tuners;
  const server: ServerInfo = serverBody;
  const pictureMode = settingsBody.pictureMode;
  if (pictureMode !== "broadcast" && pictureMode !== "smooth" && pictureMode !== "film") {
    throw new Error("picture mode");
  }
  const settings: Settings = { ...settingsBody, pictureMode };
  const savedPicture = settingsSaveBody.pictureMode;
  if (savedPicture !== "broadcast" && savedPicture !== "smooth" && savedPicture !== "film") {
    throw new Error("picture mode");
  }
  const savedSettings: Settings = { ...settingsSaveBody, pictureMode: savedPicture };
  const plan: MultiviewPlan = multiviewBody;
  const discover: { devices: Device[]; found: number } = discoverBody;
  const look: { found: { kind: string; name: string; addr: string; id?: string }[] } = lookBody;
  const home: {
    places: { id: string; group: string; kind: string; name: string; addr?: string; action: string; detail?: string }[];
    tunerAddress: string;
    sharing: boolean;
  } = homeBody;
  const calls: Record<string, string> = affiliationsBody.calls;
  const starred: { starred: { id: number; guideName?: string; displayNumber?: string; network: string }[] } = starBody;
  const free: {
    found: { kind: string; name: string; addr: string; playlist: string; guide: string }[];
    guide: string;
  } = freeBody;
  const freeAdd: { id: number; kind: string; name: string; url: string; xmltvUrl: string; message: string } = freeAddBody;
  const xtream: Source = xtreamBody;
  const watch: { code: string; message: string } = watchBody;
  const stopped: { ok: boolean } = watchStopBody;
  const setup: SetupFinish = setupFinishBody;
  const setupPost: SetupFinish = setupFinishPostBody;
  const setupDone: SetupFinish = setupFinishDoneBody;
  const signalsCheck: { running: boolean; message?: string } = signalsCheckBody;
  const scan: { scanning: boolean } = scanBody;
  const scanStatus: { scanning: boolean; found: number } = scanStatusBody;
  const sourcesFound: { type: string; data: { found: number } } = wsSourcesBody;
  const liveChanged: { type: string; data: null } = wsLiveBody;
  const activity: { type: string; data: Event } = wsActivityBody;
  const renamed: ServerInfo = serverRenameBody;
  const refreshed: { airings: number } = guideRefreshBody;
  const skipped: { ok: boolean } = scheduleSkipBody;
  const progress: { position: number } = recordingProgressBody;
  const watchedFlag: { ok: boolean; watched: boolean } = recordingWatchedBody;
  const played: {
    playlist: string;
    recording: Recording;
    markers: Marker[];
    position: number;
    growing: boolean;
  } = recordingPlayBody;
  const detected: { markers: Marker[] } = recordingDetectBody;
  const createdMarker: Marker = markerCreateBody;
  const deletedMarker: { ok: boolean } = markerDeleteBody;
  const recordError: { code: string; message: string } = recordingCreateBody;
  const recordStopped: { ok: boolean } = recordingStopBody;
  const recordDeleted: { ok: boolean } = recordingDeleteBody;
  const createdPasses: Pass[] = passCreateBody.passes;
  const deletedPasses: Pass[] = passDeleteBody.passes;
  const unfollowed: TeamFollow[] = teamUnfollowBody.teams;
  const createdVirtual: VirtualChannel = virtualCreateBody;
  const restored: { ok: boolean } = backupRestoreBody;
  return (
    channels.length +
    airings.length +
    recordings.length +
    devices.length +
    passes.length +
    teams.length +
    events.length +
    markers.length +
    signals.length +
    tuners.length +
    server.apiVersion +
    (server.minAppVersion === "1.0" ? 1 : 0) +
    plan.tunersNeeded +
    (settings.layout ? 1 : 0) +
    discover.devices.length +
    discover.found +
    look.found.length +
    home.places.length +
    home.tunerAddress.length +
    (home.sharing ? 1 : 0) +
    Object.keys(calls).length +
    starred.starred.length +
    free.found.length +
    free.guide.length +
    freeAdd.id +
    xtream.id +
    watch.code.length +
    (stopped.ok ? 1 : 0) +
    setup.steps.length +
    (setupPost.running ? 1 : 0) +
    (setupDone.channelId ?? 0) +
    (signalsCheck.running ? 1 : 0) +
    (scan.scanning ? 1 : 0) +
    scanStatus.found +
    sourcesFound.data.found +
    (liveChanged.data === null ? 1 : 0) +
    activity.data.id +
    renamed.name.length +
    (savedSettings.hideScores === "1" ? 1 : 0) +
    refreshed.airings +
    (skipped.ok ? 1 : 0) +
    progress.position +
    (watchedFlag.watched ? 1 : 0) +
    played.markers.length +
    played.recording.id +
    (played.growing ? 1 : 0) +
    detected.markers.length +
    createdMarker.id +
    (deletedMarker.ok ? 1 : 0) +
    recordError.code.length +
    (recordStopped.ok ? 1 : 0) +
    (recordDeleted.ok ? 1 : 0) +
    createdPasses.length +
    deletedPasses.length +
    unfollowed.length +
    createdVirtual.recordings.length +
    (restored.ok ? 1 : 0)
  );
}

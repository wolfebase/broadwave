import affiliationsBody from "../../../api/fixtures/affiliations.json";
import airingsBody from "../../../api/fixtures/airings.json";
import channelsBody from "../../../api/fixtures/channels.json";
import devicesBody from "../../../api/fixtures/devices.json";
import discoverBody from "../../../api/fixtures/discover.json";
import eventsBody from "../../../api/fixtures/events.json";
import freeAddBody from "../../../api/fixtures/free-add.json";
import freeBody from "../../../api/fixtures/free.json";
import homeBody from "../../../api/fixtures/home.json";
import lookBody from "../../../api/fixtures/look.json";
import markersBody from "../../../api/fixtures/markers.json";
import multiviewBody from "../../../api/fixtures/multiview.json";
import passesBody from "../../../api/fixtures/passes.json";
import recordingsBody from "../../../api/fixtures/recordings.json";
import scanStatusBody from "../../../api/fixtures/scan-status.json";
import scanBody from "../../../api/fixtures/scan.json";
import serverBody from "../../../api/fixtures/server.json";
import settingsBody from "../../../api/fixtures/settings.json";
import setupFinishDoneBody from "../../../api/fixtures/setup-finish-done.json";
import setupFinishPostBody from "../../../api/fixtures/setup-finish-post.json";
import setupFinishBody from "../../../api/fixtures/setup-finish.json";
import signalsCheckBody from "../../../api/fixtures/signals-check.json";
import signalsBody from "../../../api/fixtures/signals.json";
import starBody from "../../../api/fixtures/star.json";
import teamsBody from "../../../api/fixtures/teams.json";
import tunersBody from "../../../api/fixtures/tuners.json";
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
    activity.data.id
  );
}

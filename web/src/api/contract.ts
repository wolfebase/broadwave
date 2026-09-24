import airingsBody from "../../../api/fixtures/airings.json";
import channelsBody from "../../../api/fixtures/channels.json";
import devicesBody from "../../../api/fixtures/devices.json";
import eventsBody from "../../../api/fixtures/events.json";
import markersBody from "../../../api/fixtures/markers.json";
import multiviewBody from "../../../api/fixtures/multiview.json";
import passesBody from "../../../api/fixtures/passes.json";
import recordingsBody from "../../../api/fixtures/recordings.json";
import serverBody from "../../../api/fixtures/server.json";
import settingsBody from "../../../api/fixtures/settings.json";
import signalsBody from "../../../api/fixtures/signals.json";
import teamsBody from "../../../api/fixtures/teams.json";
import tunersBody from "../../../api/fixtures/tuners.json";
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
    (settings.layout ? 1 : 0)
  );
}

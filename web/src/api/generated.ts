// Generated from api/openapi.yaml. Do not edit.

export type Airing = {
  id: number;
  channelId: number;
  title: string;
  subtitle?: string;
  description?: string;
  category?: string;
  programId?: string;
  new?: boolean;
  imageUrl?: string;
  imageWidth?: number;
  imageHeight?: number;
  season?: number;
  episode?: number;
  episodeLabel?: string;
  originalAir?: string;
  seriesId?: string;
  live?: boolean;
  premiere?: boolean;
  finale?: boolean;
  rating?: string;
  cast?: string;
  gameId?: string;
  guideSource?: string;
  guideNumber?: string;
  channelName?: string;
  start: string;
  end: string;
};

export type Caps = {
  platform: string;
  video: string[];
  audio: string[];
  maxHeight?: number;
  network?: string;
};

export type Channel = {
  id: number;
  deviceId: string;
  guideNumber: string;
  guideName: string;
  displayNumber: string;
  displayName: string;
  videoCodec?: string;
  audioCodec?: string;
  hd: boolean;
  favorite: boolean;
  enabled: boolean;
  hidden: boolean;
  present: boolean;
  guideKey?: string;
  artUrl?: string;
  artWidth?: number;
  artHeight?: number;
  network?: string;
};

export type ChannelPatch = {
  favorite?: boolean;
  enabled?: boolean;
  hidden?: boolean;
  customName?: string;
  customNumber?: string;
  guideKey?: string;
};

export type ChannelSignal = {
  channelId: number;
  number: string;
  name: string;
  frequencyHz?: number;
  strength?: number;
  quality?: number;
  symbol?: number;
  verdict?: string;
  tip?: string;
  live?: boolean;
  checkedAt?: string;
};

export type Device = {
  deviceId: string;
  friendlyName: string;
  modelNumber?: string;
  firmwareName?: string;
  firmwareVersion?: string;
  upgradeAvailable?: string;
  baseUrl: string;
  lineupUrl?: string;
  tunerCount: number;
  priority?: number;
  lastSeen?: string;
  note?: string;
};

export type DeviceHealth = {
  deviceId: string;
  model: string;
  firmwareVersion: string;
  tuners: TunerLock[];
  error?: string;
};

export type Event = {
  id: number;
  at: string;
  kind: string;
  message: string;
};

export type Game = {
  id: string;
  league: string;
  name: string;
  shortName?: string;
  start: string;
  state: string;
  completed?: boolean;
  detail?: string;
  clock?: string;
  period?: number;
  broadcasts?: string[];
  teams?: SportsTeam[];
};

export type Marker = {
  id: number;
  recordingId?: number;
  start: number;
  end: number;
};

export type MultiviewPlanBlocked = {
  channelId: number;
  holders: string[];
  reason: string;
};

export type MultiviewPlanPlayable = {
  channelId: number;
  frequencyHz: number;
  shared: boolean;
};

export type MultiviewPlan = {
  playable: MultiviewPlanPlayable[];
  blocked: MultiviewPlanBlocked[];
  tunersNeeded: number;
  tunersFree: number;
  note?: string;
};

export type Pass = {
  id: number;
  title: string;
  channelId?: number;
  kind?: string;
  padBefore?: number;
  padAfter?: number;
  priority?: number;
  episodes?: string;
  keepMode?: string;
  keepCount?: number;
  limitCount?: number;
  rerecord?: boolean;
  commercials?: boolean;
  timeStart?: string;
  timeEnd?: string;
  matchKind?: string;
};

export type PassList = {
  passes: Pass[];
};

export type PictureMode = "broadcast" | "smooth" | "film";

export type PlannedAiring = {
  passId: number;
  airing: Airing;
  priority: number;
  padBefore: number;
  padAfter: number;
  conflict: boolean;
  skipped: boolean;
  reason?: string;
};

export type Recording = {
  id: number;
  channelId: number;
  guideNumber: string;
  title: string;
  subtitle?: string;
  description?: string;
  category?: string;
  programId?: string;
  gameId?: string;
  status: string;
  error?: string;
  startedAt: string;
  endsAt?: string;
  endedAt?: string;
  bytes?: number;
  position?: number;
  durationSec?: number;
  watched?: number;
};

export type RoomState = {
  room: string;
  channelId?: number;
  mode: string;
  anchorServer: number;
  anchorMedia: number;
  rate: number;
  latency: string;
  version: number;
  members: number;
};

export type SearchAiring = {
  cast?: string;
  category?: string;
  channelId: number;
  channelName?: string;
  description?: string;
  end: string;
  episode?: number;
  episodeLabel?: string;
  finale?: boolean;
  gameId?: string;
  guideNumber?: string;
  guideSource?: string;
  id: number;
  imageHeight?: number;
  imageUrl?: string;
  imageWidth?: number;
  live?: boolean;
  new?: boolean;
  originalAir?: string;
  premiere?: boolean;
  programId?: string;
  rating?: string;
  season?: number;
  seriesId?: string;
  start: string;
  subtitle?: string;
  title: string;
};

export type ServerInfo = {
  id: string;
  name: string;
  version: string;
  apiVersion: number;
  encoder?: string;
  tunerCount?: number;
  features: string[];
  update?: ServerUpdate;
};

export type ServerUpdate = {
  version: string;
  notesUrl: string;
  message: string;
};

export type Settings = {
  layout?: string;
  recordingsPath?: string;
  profile?: string;
  audio?: string;
  encoder?: string;
  watermarkGB?: string;
  pictureMode?: PictureMode;
  autoplay?: string;
  hdhrEmulate?: string;
  setupComplete?: string;
  needsSetup?: string;
  lastGuidePull?: string;
  nextGuidePull?: string;
  lastManualGuidePull?: string;
  hideScores?: string;
  checkUpdates?: string;
  sdUser?: string;
  sdPassword?: string;
  sdLineup?: string;
  sdPasswordSet?: string;
  guideUrl?: string;
  tmdbKey?: string;
  tmdbKeySet?: string;
};

export type SetupFinish = {
  running: boolean;
  ready?: string;
  channelId?: number;
  steps: SetupStep[];
};

export type SetupStep = {
  id: string;
  title: string;
  state: string;
  detail?: string;
};

export type Slot = {
  recordingId: number;
  title: string;
  start: string;
  end: string;
};

export type Source = {
  id: number;
  kind: string;
  name: string;
  url?: string;
  xmltvUrl?: string;
  enabled: boolean;
  stableKey?: string;
  priority?: number;
  tunerCount?: number;
  streamLimit?: number;
  streamFormat?: string;
  hasGuide?: boolean;
  needsTuner?: boolean;
  refresh?: string;
  lastRefresh?: string;
  health?: string;
  streamsInUse?: number;
  deviceId?: string;
};

export type SportsTeam = {
  name: string;
  short?: string;
  abbr?: string;
  score?: string;
  home?: boolean;
  color?: string;
  altColor?: string;
  logo?: string;
};

export type StreamInfo = {
  rendition: string;
  video: string;
  audio: string;
  mode?: PictureMode;
  reason: string;
  sourceVideo?: string;
  sourceAudio?: string;
  encoder?: string;
  scan?: string;
  sourceWidth?: number;
  sourceHeight?: number;
  sourceFps?: string;
  outputWidth?: number;
  outputHeight?: number;
  outputFps?: string;
  bitrate?: string;
  decode?: string;
};

export type TeamFollow = {
  id?: number;
  name: string;
  short?: string;
  abbr?: string;
  league?: string;
  logo?: string;
  color?: string;
  record?: boolean;
};

export type TeamList = {
  teams: TeamFollow[];
};

export type Tuner = {
  index: number;
  guide?: string;
  name?: string;
  target?: string;
  ours: boolean;
  strength?: number;
  quality?: number;
  symbol?: number;
  viewers?: number;
};

export type TunerLock = {
  index: number;
  locked: boolean;
};

export type VirtualChannel = {
  id: number;
  number: string;
  name: string;
  orderMode?: string;
  ruleTitle?: string;
  recordings: number[];
};

export type WatchSession = {
  channelId: number;
  playlist: string;
  rendition: string;
  stream: StreamInfo;
  profile?: string;
  audio?: string;
  encoder: string;
  picture?: PictureMode;
  videoMode?: string;
  shared: boolean;
  viewers: number;
  frequencyHz?: number;
  program?: number;
  hints?: string[];
  tuners?: Tuner[];
};

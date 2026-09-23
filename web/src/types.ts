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
};

export type Device = {
  friendlyName: string;
  modelNumber: string;
  firmwareName: string;
  firmwareVersion: string;
  upgradeAvailable?: string;
  deviceId: string;
  baseUrl: string;
  lineupUrl: string;
  tunerCount: number;
  priority: number;
  lastSeen: string;
};

export type Settings = {
  layout: "auto" | "desktop" | "tv" | "phone";
  recordingsPath: string;
  profile: "transparent" | "balanced" | "saver";
  audio: "stereo" | "surround";
  watermarkGB: string;
  pictureMode: "broadcast" | "smooth" | "film";
  autoplay: string;
  hdhrEmulate: string;
  setupComplete?: string;
  needsSetup?: "0" | "1";
};

export type StorageInfo = {
  freeBytes: number;
  totalBytes: number;
  watermarkGB: number;
};

export type Airing = {
  id: number;
  channelId: number;
  title: string;
  subtitle?: string;
  description?: string;
  category?: string;
  programId?: string;
  new?: boolean;
  start: string;
  end: string;
};

export type Pass = {
  id: number;
  title: string;
  channelId: number;
  kind?: string;
  padBefore: number;
  padAfter: number;
  priority: number;
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

export type Recording = {
  id: number;
  channelId: number;
  guideNumber: string;
  title: string;
  status: string;
  error?: string;
  startedAt: string;
  endsAt?: string;
  endedAt?: string;
  bytes?: number;
  position?: number;
  durationSec?: number;
  subtitle?: string;
  description?: string;
  category?: string;
  programId?: string;
  watched?: number;
};

export type Caps = {
  platform: "web" | "ios" | "tvos" | "ipados" | "macos";
  video: string[];
  audio: string[];
  maxHeight?: number;
  network?: "lan" | "wifi" | "cellular" | "remote";
};

export type Prefs = {
  quality?: "auto" | "original" | "high" | "medium" | "saver";
  audio?: "auto" | "surround" | "stereo";
  picture?: "broadcast" | "smooth" | "film";
};

export type StreamInfo = {
  rendition: string;
  video: string;
  audio: string;
  mode?: string;
  reason: string;
  sourceVideo?: string;
  sourceAudio?: string;
  encoder?: string;
};

export type ServerInfo = {
  id: string;
  name: string;
  version: string;
  apiVersion: number;
  encoder?: string;
  tunerCount?: number;
  features: string[];
};

export type WatchSession = {
  channelId: number;
  playlist: string;
  rendition: string;
  stream: StreamInfo;
  profile: string;
  audio: string;
  encoder: string;
  picture?: string;
  videoMode: string;
  shared: boolean;
  viewers: number;
  frequencyHz: number;
  program: number;
  hints: string[];
  tuners?: { index: number; guide?: string; name?: string; target?: string; ours: boolean; strength?: number; quality?: number; symbol?: number }[];
};

export type VirtualChannel = {
  id: number;
  number: string;
  name: string;
  recordings: number[];
  orderMode?: string;
  ruleTitle?: string;
};

export type TunerStatus = {
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

export type PlannedAiring = {
  passId: number;
  priority: number;
  padBefore: number;
  padAfter: number;
  conflict: boolean;
  skipped: boolean;
  reason?: string;
  airing: Airing;
};

export type ChannelPatch = {
  favorite?: boolean;
  enabled?: boolean;
  hidden?: boolean;
  customName?: string;
  customNumber?: string;
};

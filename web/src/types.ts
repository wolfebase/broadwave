export type {
  Airing,
  Caps,
  Channel,
  ChannelPatch,
  Device,
  MultiviewPlan,
  Pass,
  PlannedAiring,
  Recording,
  SearchAiring,
  ServerInfo,
  Settings,
  TeamFollow,
  Tuner,
  VirtualChannel,
  WatchSession,
} from "./api/generated";

import type { Tuner } from "./api/generated";

export type TunerStatus = Tuner;

export type StorageInfo = {
  freeBytes: number;
  totalBytes: number;
  watermarkGB: number;
};

export type Prefs = {
  quality?: "auto" | "original" | "high" | "medium" | "saver" | "tile" | "360";
  audio?: "auto" | "surround" | "stereo" | "none";
  picture?: "broadcast" | "smooth" | "film";
  track?: "main" | "language" | "described";
  even?: boolean;
};

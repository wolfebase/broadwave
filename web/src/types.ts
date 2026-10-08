export type {
  Airing,
  Caps,
  CatalogBackup,
  Channel,
  ChannelPatch,
  Device,
  DeviceHealth,
  FrameList,
  GameAlert,
  MultiviewPlan,
  NewPass,
  Pass,
  PassPreview,
  PlannedAiring,
  Recording,
  SearchAiring,
  ServerInfo,
  Settings,
  StorageKeep,
  StorageShow,
  StorageShows,
  TeamFollow,
  Tuner,
  VirtualChannel,
  WatchSession,
} from "./api/generated";

import type { Tuner } from "./api/generated";

export type TunerStatus = Tuner;

export type StorageInfo = {
  path?: string;
  freeBytes: number;
  totalBytes: number;
  watermarkGB: number;
};

export type Prefs = {
  quality?: "auto" | "original" | "high" | "medium" | "saver" | "tile" | "360" | "focus";
  audio?: "auto" | "surround" | "stereo" | "none";
  picture?: "broadcast" | "smooth" | "film";
  track?: "main" | "language" | "described";
  even?: boolean;
};

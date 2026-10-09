import { useEffect, useState } from "react";
import { authHeaders } from "../../lib/deviceToken";
import type { TunerStatus } from "../../types";

export type Diagnostics = {
  version: string;
  os: string;
  server?: { id: string; name: string };
  tuners?: TunerStatus[];
  tunerError?: string;
  encoder?: { name: string; hardware: boolean; deinterlace?: string; ffmpeg: string; ac4?: boolean; line?: string; height?: number; focus?: string; tiles?: number };
  storage?: { Free: number; Total: number };
  guide?: { channels: number; channelsWithListings: number; airings: number; listingsUntil?: string; nextRefresh?: string; lastRefresh?: string; lastError?: string; lastErrorAt?: string };
  relay?: { channelId: number; guideNumber: string; name: string; tuner: number; recording: boolean; exports: number; fieldOrder?: string; renditions: { key: string; viewers: number }[]; buffer?: { minutes: number; bytes: number; state: "on" | "off" | "full" } }[];
  starts?: ChannelStart[];
  connectedApps?: number;
  recentActivity?: { id: number; at: string; kind: string; message: string }[];
  doctor?: { id: string; message: string }[];
  feeds?: { channelId: number; guideNumber: string; name: string; viewers: number; ffmpeg: number; recording?: boolean; exports?: number }[];
  logs?: string[];
};

/** A picture a watch started: seconds from the ask to its first segment. tune is absent when the frequency was already tuned. */
export type ChannelStart = { at: string; channelId: number; guideNumber: string; rendition: string; seconds: number; tune?: number; keyframe: number; encoder: number; segment: number };

/** Diagnostics, refreshed every few seconds while mounted. */
export function useDiagnostics(key?: unknown, every = 5000): Diagnostics | null {
  const [diag, setDiag] = useState<Diagnostics | null>(null);
  useEffect(() => {
    let dead = false;
    const load = () =>
      fetch("/api/v1/diagnostics", { headers: authHeaders() })
        .then((r) => r.json() as Promise<Diagnostics>)
        .then((d) => !dead && setDiag(d))
        .catch(() => undefined);
    void load();
    const id = window.setInterval(load, every);
    return () => {
      dead = true;
      window.clearInterval(id);
    };
  }, [key, every]);
  return diag;
}

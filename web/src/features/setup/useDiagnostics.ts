import { useEffect, useState } from "react";
import type { TunerStatus } from "../../types";

export type Diagnostics = {
  version: string;
  os: string;
  server?: { id: string; name: string };
  tuners?: TunerStatus[];
  tunerError?: string;
  encoder?: { name: string; hardware: boolean; deinterlace?: string; ffmpeg: string };
  storage?: { Free: number; Total: number };
  guide?: { channels: number; channelsWithListings: number; airings: number; listingsUntil?: string; nextRefresh?: string; lastRefresh?: string };
  relay?: { channelId: number; guideNumber: string; name: string; tuner: number; recording: boolean; exports: number; fieldOrder?: string; renditions: { key: string; viewers: number }[] }[];
  connectedApps?: number;
  recentActivity?: { id: number; at: string; kind: string; message: string }[];
};

/** Diagnostics, refreshed every few seconds while mounted. */
export function useDiagnostics(key?: unknown, every = 5000): Diagnostics | null {
  const [diag, setDiag] = useState<Diagnostics | null>(null);
  useEffect(() => {
    let dead = false;
    const load = () =>
      fetch("/api/v1/diagnostics")
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

import { useEffect, useState } from "react";
import { getAirings } from "../../api";
import type { Airing } from "../../types";

/** The guide holds grid fields only. A sheet reads the full listing for its channel and time. */
export function useAiringDetails(airing: Airing | undefined): Airing | undefined {
  const [full, setFull] = useState<Airing | undefined>(undefined);
  const id = airing?.id;
  const channelId = airing?.channelId;
  const start = airing?.start;
  const end = airing?.end;
  useEffect(() => {
    if (id === undefined || channelId === undefined || !start || !end) return;
    let live = true;
    getAirings({ from: start, to: end, channels: String(channelId) })
      .then((r) => {
        const hit = r.airings.find((a) => a.id === id);
        if (live && hit) setFull(hit);
      })
      .catch(() => undefined);
    return () => {
      live = false;
    };
  }, [id, channelId, start, end]);
  if (!airing) return undefined;
  return full && full.id === airing.id ? { ...airing, ...full } : airing;
}

import { createContext, lazy, Suspense, useCallback, useContext, useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { clearBroadcast } from "../features/multiview/clear";
import { copy } from "../strings";
import type { Channel } from "../types";
import { useData } from "./data";
import { inAppDepth, navigate, useRoute } from "./router";

type Player = {
  channel: Channel | null;
  mode: "full" | "mini";
  open: (channel: Channel) => void;
  close: () => void;
};

const LivePlayer = lazy(() => import("../features/player/LivePlayer").then((m) => ({ default: m.LivePlayer })));

const Ctx = createContext<Player | null>(null);

export function usePlayer(): Player {
  const v = useContext(Ctx);
  if (!v) throw new Error("usePlayer outside PlayerProvider");
  return v;
}

/**
 * Live TV keeps playing while you browse: leaving the full player docks it as a
 * mini player, and one video element carries playback between the two.
 */
export function PlayerProvider({ children }: { children: ReactNode }) {
  const { channels, allChannels, ready } = useData();
  const { path, params } = useRoute();
  const [channel, setChannel] = useState<Channel | null>(null);
  const back = useRef("/guide");
  // The page under the player. Back to browsing steps back in history only when it is not setup.
  const under = useRef("");
  const watchId = path === "/watch" ? Number(params.get("channel") || 0) : 0;
  // A link can name a row the guide does not show: an encrypted 3.0 station,
  // the hidden half of a pair, or another tuner's copy. It plays as the row on
  // the guide (below). A row with no such stand-in, such as one the viewer hid,
  // plays as itself.
  const choice = ready && watchId ? clearBroadcast(watchId, channels, allChannels) : null;
  const fromList = ready && watchId ? (channels.find((c) => c.id === watchId) ?? (choice ? null : allChannels.find((c) => c.id === watchId) ?? null)) : null;
  const playing = channel && (!watchId || channel.id === watchId) ? channel : fromList;
  const mode: "full" | "mini" = watchId && playing && playing.id === watchId ? "full" : "mini";

  // An encrypted 3.0 channel plays its clear 1.0 twin, and the player says why
  // for a few seconds. A link to a channel that is not in the lineup goes to the guide.
  const unknown = ready && watchId > 0 && allChannels.length > 0 && !allChannels.some((c) => c.id === watchId);
  const playsId = choice?.id ?? 0;
  const playsNote = choice?.note ?? false;
  useEffect(() => {
    if (unknown) navigate("/guide", true);
    else if (playsId && playsId !== watchId) navigate(`/watch?channel=${playsId}${playsNote ? `&from=${watchId}` : ""}`, true);
  }, [playsId, playsNote, watchId, unknown]);
  const from = watchId ? Number(params.get("from") || 0) : 0;
  const standIn = from && allChannels.some((c) => c.id === from && c.playsAs === watchId) ? `${from}-${watchId}` : "";
  const [noted, setNoted] = useState("");
  const [notedFor, setNotedFor] = useState(standIn);
  // Leaving the player forgets the note, so the next visit says why again.
  if (standIn !== notedFor) {
    setNotedFor(standIn);
    setNoted("");
  }
  useEffect(() => {
    if (!standIn) return;
    const t = window.setTimeout(() => setNoted(standIn), 10_000);
    return () => window.clearTimeout(t);
  }, [standIn]);

  useEffect(() => {
    // Setup is done once you watch, so leaving the player never goes back to it.
    if (path === "/watch" || path === "/multiview") return;
    under.current = path;
    if (path !== "/setup") back.current = path + (params.toString() ? `?${params}` : "");
  }, [path, params]);

  const open = useCallback((c: Channel) => {
    const twin = c.playsAs ? allChannels.find((t) => t.id === c.playsAs) : undefined;
    setChannel(twin ?? c);
    navigate(twin ? `/watch?channel=${twin.id}&from=${c.id}` : `/watch?channel=${c.id}`, path === "/watch");
  }, [path, allChannels]);

  const close = useCallback(() => {
    setChannel(null);
    if (path === "/watch") navigate(back.current || "/guide");
  }, [path]);

  const value = useMemo(() => ({ channel: playing, mode, open, close }), [playing, mode, open, close]);

  return (
    <Ctx.Provider value={value}>
      {children}
      {playing && path !== "/multiview" ? (
        <Suspense fallback={null}>
        <LivePlayer
          key="live"
          channel={playing}
          mode={mode}
          notice={standIn && standIn !== noted && mode === "full" ? copy.player.encrypted : undefined}
          onChannel={open}
          onMinimize={() => {
            if (inAppDepth() > 0 && under.current !== "/setup") window.history.back();
            else navigate(back.current || "/guide");
          }}
          onExpand={() => navigate(`/watch?channel=${playing.id}`)}
          onClose={close}
        />
        </Suspense>
      ) : null}
    </Ctx.Provider>
  );
}

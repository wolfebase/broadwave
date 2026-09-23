import { createContext, lazy, Suspense, useCallback, useContext, useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import type { Channel } from "../types";
import { useData } from "./data";
import { navigate, useRoute } from "./router";

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
  const { channels, ready } = useData();
  const { path, params } = useRoute();
  const [channel, setChannel] = useState<Channel | null>(null);
  const back = useRef("/guide");
  const watchId = path === "/watch" ? Number(params.get("channel") || 0) : 0;
  const fromList = ready && watchId ? channels.find((c) => c.id === watchId) ?? null : null;
  const playing = channel && (!watchId || channel.id === watchId) ? channel : fromList;
  const mode: "full" | "mini" = watchId && playing && playing.id === watchId ? "full" : "mini";

  useEffect(() => {
    if (path !== "/watch" && path !== "/multiview") back.current = path + (params.toString() ? `?${params}` : "");
  }, [path, params]);

  const open = useCallback((c: Channel) => {
    setChannel(c);
    navigate(`/watch?channel=${c.id}`, path === "/watch");
  }, [path]);

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
          onChannel={open}
          onMinimize={() => navigate(back.current || "/guide")}
          onExpand={() => navigate(`/watch?channel=${playing.id}`)}
          onClose={close}
        />
        </Suspense>
      ) : null}
    </Ctx.Provider>
  );
}

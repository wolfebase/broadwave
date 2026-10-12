import { useEffect, useRef, useState } from "react";
import { readToken } from "../lib/deviceToken";
import { readLiveDelay } from "../lib/events";
import { CastIcon } from "../ui/icons";
import { castControl, castLoad, castOutgoing } from "./protocol";
import { ensureSender, requestCast, type CastLink } from "./sender";

/** Cast in the web player. Hidden until this server has an application id and a playlist. */
export function CastButton({
  appId,
  channelId,
  playlist,
  room,
  title,
  subtitle,
}: {
  appId: string;
  channelId: number;
  playlist: string;
  room: string;
  title: string;
  subtitle: string;
}) {
  const [link, setLink] = useState<CastLink | null>(null);
  const [error, setError] = useState("");
  const busy = useRef(false);
  const alive = useRef(true);

  useEffect(() => {
    alive.current = true;
    return () => {
      alive.current = false;
    };
  }, []);

  useEffect(() => {
    if (!link || !castOutgoing(appId, playlist, true)) return;
    const token = readToken();
    link.send(castLoad({ channelId, playlist, room, latency: readLiveDelay(), token: token || undefined, title, subtitle }));
  }, [link, appId, channelId, playlist, room, title, subtitle]);

  const mode = castControl(appId, playlist, link !== null);
  if (mode === "hidden") return null;

  async function start() {
    if (busy.current) return;
    busy.current = true;
    setError("");
    try {
      const ready = await ensureSender();
      if (!ready) {
        setError("Cast didn't load.");
        return;
      }
      const next = await requestCast(appId, undefined, () => {
        if (alive.current) setLink(null);
      });
      if (!next) return;
      setError("");
      setLink(next);
    } catch (err) {
      setError(err instanceof Error && err.message ? err.message : "No Cast devices found.");
    } finally {
      busy.current = false;
    }
  }

  function stop() {
    if (!link) return;
    link.send({ type: "stop" });
    link.end();
    setLink(null);
    setError("");
  }

  const casting = mode === "casting";
  return (
    <>
      <button type="button" className={casting ? "glass-icon on" : "glass-icon"} aria-pressed={casting} aria-label={casting ? "Stop casting" : "Cast"} onClick={() => (casting ? stop() : void start())}>
        <CastIcon />
      </button>
      {error ? (
        <span className="cast-note" role="status">
          {error}
        </span>
      ) : null}
    </>
  );
}

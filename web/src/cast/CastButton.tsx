import { useEffect, useRef, useState } from "react";
import { readToken } from "../lib/deviceToken";
import { readLiveDelay } from "../lib/events";
import { CastIcon } from "../ui/icons";
import { castAvailable, castLoad } from "./protocol";
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

  useEffect(() => {
    if (!link) return;
    const token = readToken();
    link.send(castLoad({ channelId, playlist, room, latency: readLiveDelay(), token: token || undefined, title, subtitle }));
  }, [link, channelId, playlist, room, title, subtitle]);

  if (!castAvailable(appId, playlist)) return null;

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
      const next = await requestCast(appId);
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

  const casting = link !== null;
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

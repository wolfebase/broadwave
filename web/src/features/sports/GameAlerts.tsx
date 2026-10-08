import { useCallback, useEffect, useRef, useState } from "react";
import { useData } from "../../app/data";
import { usePlayer } from "../../app/player";
import { events } from "../../lib/events";
import type { GameAlert } from "../../types";

type Seen = GameAlert & { at: number };

/** How long an alert stays worth showing. A close finish is over soon. */
const freshFor = 10 * 60_000;

/** Ids this page has shown. A server restart sends a close finish again. */
const shown = new Set<string>();

/**
 * Game alerts from the server (game.alert): a followed team's game starting
 * or a close finish, with Watch. They wait while the player is up, like the
 * home notice, and one older than ten minutes is dropped.
 */
export function GameAlerts({ held }: { held: boolean }) {
  const [queue, setQueue] = useState<Seen[]>([]);
  useEffect(() => {
    const off = events().on("game.alert", (data) => {
      const alert = data as GameAlert;
      if (!alert?.id || !alert.channelId || shown.has(alert.id)) return;
      shown.add(alert.id);
      setQueue((cur) => [...cur, { ...alert, at: Date.now() }]);
    });
    return () => {
      off();
    };
  }, []);
  const dismiss = useCallback(() => setQueue((cur) => cur.slice(1)), []);
  const current = queue[0];
  const [stale, setStale] = useState("");
  useEffect(() => {
    if (held || !current) return;
    // 12 s once it can be seen, as the home notice; a stale one goes at once.
    let timer = 0;
    const arm = () => {
      window.clearTimeout(timer);
      const left = current.at + freshFor - Date.now();
      if (left <= 0) {
        setStale(current.id);
        timer = window.setTimeout(dismiss, 0);
      } else if (document.visibilityState === "visible") {
        timer = window.setTimeout(dismiss, Math.min(12_000, left));
      }
    };
    arm();
    document.addEventListener("visibilitychange", arm);
    return () => {
      window.clearTimeout(timer);
      document.removeEventListener("visibilitychange", arm);
    };
  }, [current, held, dismiss]);
  if (held || !current || stale === current.id) return null;
  return <GameNotice key={current.id} alert={current} onDismiss={dismiss} />;
}

function GameNotice({ alert, onDismiss }: { alert: Seen; onDismiss: () => void }) {
  const { channels, allChannels } = useData();
  const player = usePlayer();
  const ref = useRef<HTMLDivElement>(null);
  const channel = channels.find((c) => c.id === alert.channelId) ?? allChannels.find((c) => c.id === alert.channelId);
  useEffect(() => {
    // It floats over the page. A control under it that takes focus wins.
    const onFocus = (event: FocusEvent) => {
      const box = ref.current?.getBoundingClientRect();
      const target = event.target;
      if (!box || !(target instanceof HTMLElement) || ref.current?.contains(target)) return;
      const r = target.getBoundingClientRect();
      if (r.left < box.right && box.left < r.right && r.top < box.bottom && box.top < r.bottom) onDismiss();
    };
    document.addEventListener("focusin", onFocus);
    return () => document.removeEventListener("focusin", onFocus);
  }, [onDismiss]);
  return (
    <div ref={ref} className="banner-home" role="status" data-testid="game-alert">
      <p>
        {alert.text}
        {alert.detail ? <> · {alert.detail}</> : null}
      </p>
      {channel ? (
        <button
          type="button"
          className="btn small"
          onClick={() => {
            onDismiss();
            player.open(channel);
          }}
        >
          Watch {alert.channel}
        </button>
      ) : null}
      <button type="button" className="btn small ghost" onClick={onDismiss}>
        Not now
      </button>
    </div>
  );
}

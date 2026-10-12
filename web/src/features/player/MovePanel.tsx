import { useCallback, useEffect, useRef, useState, type KeyboardEvent } from "react";
import { useData } from "../../app/data";
import { focusRing } from "../../app/remote";
import { canonicalKey } from "../../lib/tvKey";
import { playerUiMessages } from "../../i18n/messages/playerUi.ts";
import { bind } from "../../i18n/runtime.ts";
import { browserName, screenId } from "../../lib/events";
import type { Channel, Screen } from "../../types";
import { shownOutage } from "./outage";
import { getScreens, sendToScreen, startedOn } from "./screens";

const t = bind(playerUiMessages);

/** Sends this channel to another open screen. That screen joins the channel's room, so it picks up at the same moment. */
export function MovePanel({ channel, onMoved, onClose }: { channel: Channel; onMoved: (name: string) => void; onClose: () => void }) {
  const { channels, allChannels } = useData();
  const ref = useRef<HTMLDivElement>(null);
  const [screens, setScreens] = useState<Screen[] | null>(null);
  const [error, setError] = useState("");
  const [sending, setSending] = useState("");
  const [status, setStatus] = useState("");
  const open = useRef(true);
  // Only the newest look may land. Strict mode starts two.
  const loadN = useRef(0);

  const load = useCallback(() => {
    const mine = ++loadN.current;
    getScreens()
      .then(({ screens: list }) => {
        if (!open.current || mine !== loadN.current) return;
        setScreens(list.filter((s) => s.id !== screenId()));
      })
      .catch((err: Error) => {
        if (!open.current || mine !== loadN.current) return;
        setError(err.message || t("player.outage.requestFailed"));
      });
  }, []);

  useEffect(() => {
    open.current = true;
    focusRing(ref.current);
    load();
    return () => {
      open.current = false;
    };
  }, [load]);

  useEffect(() => {
    // The list arrives after the panel took focus. Move onto its first screen.
    if (!screens?.length || document.activeElement !== ref.current) return;
    focusRing(ref.current?.querySelector<HTMLElement>("li button"));
  }, [screens]);

  async function send(screen: Screen) {
    if (sending) return;
    // The rows go disabled; the panel keeps the keys.
    focusRing(ref.current);
    setSending(screen.id);
    setError("");
    try {
      await sendToScreen(screen.id, channel.id, browserName());
      // A sleeping phone keeps its socket open for a while and takes the event
      // without playing. Stop here only once the other screen is on the channel.
      setStatus(t("player.startingOn", { name: screen.name }));
      if (await startedOn(screen.id, channel.id, () => open.current)) {
        onMoved(screen.name);
        return;
      }
      if (open.current) setError(t("player.didntStart", { name: screen.name }));
    } catch (err) {
      if (open.current) setError((err as Error).message || t("player.outage.requestFailed"));
    }
    if (!open.current) return;
    setStatus("");
    setSending("");
    load();
  }

  function watching(screen: Screen) {
    if (!screen.channelId) return "";
    const on = channels.find((c) => c.id === screen.channelId) ?? allChannels.find((c) => c.id === screen.channelId);
    return on ? t("player.watchingNumber", { number: on.displayNumber }) : "";
  }

  function onKey(event: KeyboardEvent) {
    const k = canonicalKey(event);
    if (k === "Escape" || k === "Backspace") {
      event.preventDefault();
      event.stopPropagation();
      onClose();
      return;
    }
    if (k !== "ArrowUp" && k !== "ArrowDown" && k !== "ArrowLeft" && k !== "ArrowRight") return;
    // The player's arrows change channel and seek. Here they walk the list.
    event.preventDefault();
    event.stopPropagation();
    const list = Array.from(ref.current?.querySelectorAll<HTMLElement>("button:not(:disabled)") ?? []);
    if (list.length === 0 || k === "ArrowLeft" || k === "ArrowRight") return;
    const at = list.findIndex((el) => el === document.activeElement);
    const next = at < 0 ? 0 : Math.min(list.length - 1, Math.max(0, at + (k === "ArrowDown" ? 1 : -1)));
    focusRing(list[next]);
  }

  return (
    <div ref={ref} className="info-panel glass" role="dialog" aria-labelledby="move-panel-title" tabIndex={-1} onKeyDown={onKey}>
      <h3 id="move-panel-title">{t("player.move")}</h3>
      {screens === null && !error ? <p className="dim">{t("player.looking")}</p> : null}
      {screens?.length === 0 ? <p className="dim">{t("player.openAnother")}</p> : null}
      {screens?.length ? (
        <ul className="together-list" aria-label={t("player.screens")}>
          {screens.map((s) => {
            const line = watching(s);
            return (
              <li key={s.id}>
                <button type="button" className={sending === s.id ? "text-btn on" : "text-btn"} disabled={Boolean(sending)} aria-busy={sending === s.id || undefined} onClick={() => void send(s)}>
                  <span className="mg-body">
                    <span className="mg-title">{s.name}</span>
                    {line ? <span className="mg-name">{line}</span> : null}
                  </span>
                </button>
              </li>
            );
          })}
        </ul>
      ) : null}
      {status ? <p role="status">{status}</p> : null}
      {error ? <p role="alert">{shownOutage(error)}</p> : null}
      <div className="option-actions">
        <button type="button" className="btn small ghost" onClick={onClose}>
          {t("player.cancel")}
        </button>
      </div>
    </div>
  );
}

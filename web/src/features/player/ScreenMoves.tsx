import { useEffect, useRef, useState } from "react";
import { useData } from "../../app/data";
import { usePlayer } from "../../app/player";
import { useRoute } from "../../app/router";
import { events } from "../../lib/events";
import { onMoved, sentHere } from "./moved";

/**
 * Channels another screen sends here (screen.watch) play at once, with who
 * sent them over the picture. A screen that sent its channel away says where.
 */
export function ScreenMoves() {
  const { channels, allChannels, ready, settings } = useData();
  const player = usePlayer();
  const { path } = useRoute();
  // A channel must not pull a screen out of setup.
  const setup = path === "/setup" || (ready && settings.needsSetup === "1");
  const latest = useRef({ channels, allChannels, open: player.open, setup });
  useEffect(() => {
    latest.current = { channels, allChannels, open: player.open, setup };
  });
  useEffect(() => {
    const off = events().on("screen.watch", (data) => {
      const { channelId, from } = (data ?? {}) as { channelId?: number; from?: string };
      const { channels: list, allChannels: all, open, setup: busy } = latest.current;
      if (busy) return;
      const channel = list.find((c) => c.id === channelId) ?? all.find((c) => c.id === channelId);
      if (!channel) return;
      const name = from?.trim();
      // The player plays an encrypted channel's clear twin; the note goes with what plays.
      if (name) sentHere({ id: channel.playsAs && all.some((c) => c.id === channel.playsAs) ? channel.playsAs : channel.id, text: `From ${name}` });
      open(channel);
    });
    return () => {
      off();
    };
  }, []);

  const [sentTo, setSentTo] = useState("");
  useEffect(() => onMoved(setSentTo), []);
  useEffect(() => {
    if (!sentTo) return;
    const t = window.setTimeout(() => setSentTo(""), 5_000);
    return () => window.clearTimeout(t);
  }, [sentTo]);
  if (!sentTo) return null;
  return (
    <div className="banner-home" role="status">
      <p>Playing on {sentTo}</p>
    </div>
  );
}

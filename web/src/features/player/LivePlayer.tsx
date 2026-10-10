import { useEffect, useMemo, useRef, useState, type KeyboardEvent as ReactKey, type RefObject } from "react";
import { useData } from "../../app/data";
import { useLayout } from "../../app/layout";
import { focusRing } from "../../app/remote";
import { navigate } from "../../app/router";
import { airingAt, categoryOf, minutesLeft, progress } from "../../lib/guide";
import { channelNumberContinues, typedChannel } from "../../lib/remote";
import type { SyncStatus } from "../../lib/sync";
import { events, saveLiveDelay, type LiveDelay } from "../../lib/events";
import { readZoom, saveZoom, type PictureMode, type Zoom } from "../../picture";
import { copy } from "../../strings";
import type { Channel } from "../../types";
import { saveSound, sleepDue, sleepSentence, sleepUntilFrom } from "./extras";
import { ChevronIcon, InfoIcon, ListIcon, RecordIcon, SideBySideIcon, SyncIcon } from "../../ui/icons";
import { Progress } from "../../ui/primitives";
import { isLayout, multiviewPath } from "../multiview/storage";
import { useScoreMap } from "../sports/scores";
import { movedTo, onSentHere, type SentNote } from "./moved";
import { MovePanel } from "./MovePanel";
import { listingNote } from "./outage";
import { soundFor } from "./sounds";
import { recordingHoldingStart, startOverChoice } from "./startOver";
import { Stage } from "./Stage";
import { groupRoom, peopleSentence, personLabel, useGroup } from "./together";
import { useLiveStream } from "./useLiveStream";

type Quality = "auto" | "original" | "high" | "medium" | "saver";
type Sound = "auto" | "surround" | "stereo";
type Track = "main" | "language" | "described";
type Options = { quality: Quality; audio: Sound; track: Track; even: boolean; sync: boolean; captions: boolean };

function readOptions(): Options {
  try {
    return { quality: "auto", audio: "auto", track: "main", even: false, sync: true, captions: false, ...JSON.parse(localStorage.getItem("ota-live") || "{}") };
  } catch {
    return { quality: "auto", audio: "auto", track: "main", even: false, sync: true, captions: false };
  }
}

const qualityLabels: Record<Options["quality"], string> = { auto: "Auto", original: "Original", high: "High", medium: "Medium", saver: "Data saver" };
const delayLabels: Record<LiveDelay, string> = { lowest: "Lowest", balanced: "Balanced", stable: "Stable" };

export function LivePlayer({
  channel,
  mode,
  onChannel,
  onMinimize,
  onClose,
  onExpand,
  notice,
}: {
  channel: Channel;
  mode: "full" | "mini";
  /** A short note over the picture, such as why another channel is playing. */
  notice?: string;
  onChannel: (channel: Channel) => void;
  onMinimize: () => void;
  onClose: () => void;
  onExpand: () => void;
}) {
  const { channels, index, now, recordings, settings, saveSettings, record, stopRecord, refresh } = useData();
  const layout = useLayout();
  const videoRef = useRef<HTMLVideoElement>(null);
  const rootRef = useRef<HTMLElement>(null);
  const [opts, setOpts] = useState<Options>(readOptions);
  const [picture, setPicture] = useState<PictureMode>(settings.pictureMode || "broadcast");
  // Watching together is this channel, this visit: a new channel or a reload
  // starts alone instead of opening a group of one there.
  const [togetherOn, setTogetherOn] = useState<number | null>(null);
  const together = opts.sync && togetherOn === channel.id;
  const room = together ? groupRoom(channel.id) : `channel:${channel.id}`;
  const group = useGroup(channel.id);
  const stream = useLiveStream(videoRef, {
    channelId: channel.id,
    quality: opts.quality,
    audio: opts.audio,
    track: opts.track,
    even: opts.even,
    picture,
    room,
    sync: opts.sync,
    profile: mode === "mini" ? "tile" : layout,
    audible: true,
    remember: channel,
    captions: opts.captions,
    alternates: true,
  });
  const session = stream.session;
  const error = stream.error;
  const sync: SyncStatus = stream.syncStatus;
  const [panel, setPanel] = useState<"none" | "guide" | "info" | "sync" | "help" | "move">("none");
  const moveButton = useRef<HTMLButtonElement>(null);
  // Options comes back when the move panel closes; focus goes to the button that opened it.
  const backToMove = useRef(false);
  useEffect(() => {
    if (panel !== "none" || !backToMove.current) return;
    backToMove.current = false;
    focusRing(moveButton.current);
  }, [panel]);
  const [listing, setListing] = useState({ id: channel.id, checks: 0, checking: false });
  if (listing.id !== channel.id) setListing({ id: channel.id, checks: 0, checking: false });
  const playback = usePlaybackStats(videoRef, panel === "info");
  const [behind, setBehind] = useState(0);
  // The broadcast time the live window starts at, for Start over.
  const [windowFrom, setWindowFrom] = useState<number | null>(null);
  const [span, setSpan] = useState({ at: 0, len: 1 });
  const matchedRow = Math.max(0, channels.findIndex((c) => c.id === channel.id));
  const [guideRow, setGuideRow] = useState(matchedRow);
  const [rowFor, setRowFor] = useState(`${channel.id}:${channels.map((c) => c.id).join(",")}`);
  const rowKey = `${channel.id}:${channels.map((c) => c.id).join(",")}`;
  if (rowFor !== rowKey) {
    setRowFor(rowKey);
    setGuideRow(matchedRow);
  }
  // A row the viewer rests on is the likely next channel. Its picture starts
  // now when that costs nothing, so the change that follows is instant.
  const warm = stream.warm;
  const restingOn = panel === "guide" ? channels[guideRow]?.id : undefined;
  useEffect(() => {
    if (!restingOn || restingOn === channel.id) return;
    const t = window.setTimeout(() => warm(restingOn), 300);
    return () => window.clearTimeout(t);
  }, [restingOn, channel.id, warm]);
  const [zoom, setZoom] = useState<Zoom>(readZoom);
  // Only a pick made on this channel explains itself: a room someone else
  // started at balanced is not a refusal.
  const [asked, setAsked] = useState<{ channel: number; delay: LiveDelay } | null>(null);
  const [sleepUntil, setSleepUntil] = useState<number | null>(null);
  const [sleepFor, setSleepFor] = useState(0);
  const [theater, setTheater] = useState(false);
  const typed = useRef("");
  const typedTimer = useRef(0);
  const [entry, setEntry] = useState("");
  const previous = useRef<Channel | null>(null);
  const shown = useRef(channel.id);
  useEffect(() => {
    if (shown.current === channel.id) return;
    previous.current = channels.find((c) => c.id === shown.current) ?? previous.current;
    shown.current = channel.id;
  }, [channel.id, channels]);

  const airing = airingAt(index, channel.id, now);
  // A master switches sound without a new picture.
  const tuning = useFirstFrame(videoRef, `${channel.id}:${opts.quality}:${opts.audio}:${stream.sounds ? "" : opts.track}:${opts.even}:${picture}`);
  // Who sent this channel here. Its 6 s start once the picture moves; a cold tune would use them up.
  const [sent, setSent] = useState<SentNote | null>(null);
  useEffect(() => onSentHere(setSent), []);
  const sentText = sent?.id === channel.id ? sent.text : undefined;
  // Leaving the channel drops it. The note can land a render before its channel does.
  const [sentOn, setSentOn] = useState(channel.id);
  if (sentOn !== channel.id) {
    setSentOn(channel.id);
    if (sent && sent.id !== channel.id) setSent(null);
  }
  useEffect(() => {
    if (tuning || !sentText) return;
    const t = window.setTimeout(() => setSent(null), 6_000);
    return () => window.clearTimeout(t);
  }, [tuning, sentText]);
  const scores = useScoreMap();
  const active = recordings.find((r) => r.status === "recording" && r.channelId === channel.id);
  const { mediaNow } = stream;
  const held = airing ? recordingHoldingStart(airing, recordings) : undefined;
  const startOverFrom = startOverChoice(airing ? Date.parse(airing.start) : undefined, windowFrom, held ? Date.parse(held.startedAt) : undefined);

  useEffect(() => localStorage.setItem("ota-live", JSON.stringify(opts)), [opts]);

  useEffect(() => {
    const video = videoRef.current;
    if (!video) return;
    const tick = () => {
      const end = video.seekable.length ? video.seekable.end(video.seekable.length - 1) : video.currentTime;
      const start = video.seekable.length ? video.seekable.start(0) : 0;
      setBehind(Math.max(0, end - video.currentTime));
      setSpan({ at: Math.max(0, video.currentTime - start), len: Math.max(1, end - start) });
      const media = mediaNow();
      setWindowFrom(media === null || !video.seekable.length ? null : Math.round((media - (video.currentTime - start) * 1000) / 1000) * 1000);
    };
    // A paused video fires no timeupdate, and live moves on without it.
    const paused = window.setInterval(() => {
      if (video.paused) tick();
    }, 1000);
    video.addEventListener("timeupdate", tick);
    video.addEventListener("seeked", tick);
    return () => {
      window.clearInterval(paused);
      video.removeEventListener("timeupdate", tick);
      video.removeEventListener("seeked", tick);
    };
  }, [channel.id, mediaNow]);

  useEffect(() => {
    if (!sleepUntil) return;
    const id = window.setInterval(() => {
      if (sleepDue(sleepUntil, Date.now())) onClose();
    }, 1000);
    return () => window.clearInterval(id);
  }, [sleepUntil, onClose]);

  useEffect(() => {
    const root = document.documentElement;
    if (theater && layout === "desktop") root.dataset.theater = "1";
    else delete root.dataset.theater;
    return () => {
      delete root.dataset.theater;
    };
  }, [theater, layout]);

  useEffect(() => {
    if (mode !== "full") return;
    const root = rootRef.current;
    if (!root) return;
    // On a TV the chrome takes the keys until the viewer moves to the picture.
    // A channel change must not pull focus off a control they are already on.
    if (layout === "tv") {
      const active = document.activeElement;
      if (active instanceof HTMLElement && root.contains(active)) return;
      const channelsBtn = root.querySelector<HTMLElement>("button[aria-label='Channels']");
      focusRing(channelsBtn ?? root);
      return;
    }
    root.focus();
  }, [channel.id, mode, layout]);

  // The selected row keeps the keys and stays in view. A row the pointer
  // rests on is selected too, but it must not take focus from the keys.
  const hoverRow = useRef(false);
  useEffect(() => {
    if (panel !== "guide") return;
    const row = document.querySelectorAll<HTMLElement>(".mini-guide [role='option']")[guideRow];
    if (hoverRow.current) {
      hoverRow.current = false;
      return;
    }
    if (layout === "tv") focusRing(row);
    else row?.focus();
  }, [panel, guideRow, layout]);

  function openGuide() {
    hoverRow.current = false;
    setGuideRow(matchedRow);
    setPanel("guide");
  }

  useEffect(() => () => window.clearTimeout(typedTimer.current), []);

  function detachSync() {
    if (!opts.sync || together) return;
    // The engine seeks forward on its next tick. Stop it before the playhead
    // moves, or a rewind is put back before React turns sync off.
    stream.releaseSync();
    setOpts((o) => ({ ...o, sync: false }));
  }

  function jump(delta: number) {
    const video = videoRef.current;
    if (!video) return;
    if (together) {
      seekTogether(video.currentTime + delta);
      return;
    }
    detachSync();
    video.currentTime = Math.max(0, video.currentTime + delta);
  }

  // Moves the group to this screen's time t, kept inside this screen's
  // playlist window: a frame no screen holds would leave every screen waiting.
  function seekTogether(t: number) {
    const video = videoRef.current;
    const media = stream.mediaNow();
    if (!video || !media || !video.seekable.length) return;
    const first = Math.min(video.seekable.start(0) + 1, video.currentTime);
    stream.command("seek", media + (Math.max(first, t) - video.currentTime) * 1000);
  }

  // A drag on the scrubber is one seek for the group, sent when it rests.
  const [scrubAt, setScrubAt] = useState<number | null>(null);
  const scrubTimer = useRef(0);
  useEffect(() => () => window.clearTimeout(scrubTimer.current), []);
  function scrubTogether(value: number) {
    setScrubAt(value);
    window.clearTimeout(scrubTimer.current);
    scrubTimer.current = window.setTimeout(() => {
      const video = videoRef.current;
      if (video?.seekable.length) seekTogether(video.seekable.start(0) + value);
      scrubTimer.current = window.setTimeout(() => setScrubAt(null), 1000);
    }, 250);
  }

  function togglePlay() {
    const video = videoRef.current;
    if (!video) return;
    if (together) {
      stream.command(video.paused ? "play" : "pause");
      return;
    }
    if (!video.paused) detachSync();
    if (video.paused) void video.play();
    else video.pause();
  }

  function goLive() {
    const video = videoRef.current;
    if (!video) return;
    if (together) return stream.command("live");
    if (!opts.sync) return setOpts((o) => ({ ...o, sync: true }));
    if (video.seekable.length) video.currentTime = video.seekable.end(video.seekable.length - 1) - 10;
    void video.play();
  }

  // Plays the show on now from its start: in the live window when it still
  // holds it, else from a recording of this showing.
  function startOver() {
    if (!airing) return;
    if (startOverFrom === "recording" && held) {
      navigate(`/play?recording=${held.id}`);
      return;
    }
    const video = videoRef.current;
    const media = stream.mediaNow();
    if (startOverFrom !== "live" || !video || media === null || !video.seekable.length) return;
    // Read before detaching: the sync engine holds the broadcast clock.
    const target = Math.max(video.seekable.start(0) + 0.5, video.currentTime + (Date.parse(airing.start) - media) / 1000);
    if (together) return seekTogether(target);
    detachSync();
    video.currentTime = target;
    void video.play();
  }

  function step(dir: number) {
    const i = channels.findIndex((c) => c.id === channel.id);
    const next = channels[(i + dir + channels.length) % channels.length];
    if (next) onChannel(next);
  }

  async function toggleRecord() {
    if (active) await stopRecord(active.id);
    else await record(channel, airing?.title || channel.displayName);
  }

  function tuneTyped(number: string) {
    typed.current = "";
    setEntry("");
    const hit = typedChannel(channels, number, true);
    if (hit && hit.id !== channel.id) onChannel(hit);
  }

  function lastChannel() {
    const prior = previous.current;
    if (!prior || prior.id === channel.id) return;
    onChannel(prior);
  }

  function onKey(event: ReactKey) {
    const k = event.key;
    const target = event.target;
    if (target instanceof HTMLInputElement || target instanceof HTMLTextAreaElement || (target instanceof HTMLElement && target.isContentEditable)) {
      if (k === "Escape") {
        target.blur();
        event.preventDefault();
      }
      return;
    }
    if (panel === "help") {
      // A modal keeps the player's keys, but Enter, Space, and Tab belong to its Close button.
      if (k === "Enter" || k === " " || k === "Tab") return;
      if (k === "Escape" || k === "Backspace" || k === "?") setPanel("none");
      event.preventDefault();
      return;
    }
    if (k === "Backspace" && typed.current) {
      event.preventDefault();
      window.clearTimeout(typedTimer.current);
      const next = typed.current.slice(0, -1);
      typed.current = next;
      setEntry(next);
      if (next) typedTimer.current = window.setTimeout(() => tuneTyped(typed.current), 1500);
      return;
    }
    // Wait for the whole number: tuning on each digit would take a tuner for 4.1 on the way to 41.1.
    // "51" is 5.1. The dot can be skipped; the echo stays up for 1.5 s.
    if (/^[0-9.]$/.test(k)) {
      event.preventDefault();
      const next = channelNumberContinues(channels, typed.current + k) ? typed.current + k : k;
      window.clearTimeout(typedTimer.current);
      if (typedChannel(channels, next, false)) return tuneTyped(next);
      typed.current = next;
      setEntry(next);
      typedTimer.current = window.setTimeout(() => tuneTyped(typed.current), 1500);
      return;
    }
    if (k === "Enter" && typed.current) {
      event.preventDefault();
      window.clearTimeout(typedTimer.current);
      tuneTyped(typed.current);
      return;
    }
    if (panel === "guide") {
      const backToChannels = () => {
        // The list is about to unmount with the focus in it. On a TV the button
        // (also named Channels) keeps the keys; elsewhere the picture does.
        if (layout === "tv") focusRing(document.querySelector<HTMLElement>(".stage:not(.mini) button[aria-label='Channels']"));
        else rootRef.current?.focus();
      };
      if (k === "Escape" || k === "Backspace" || k === "g") {
        setPanel("none");
        backToChannels();
      } else if (k === "ArrowDown") setGuideRow((r) => Math.min(channels.length - 1, r + 1));
      else if (k === "ArrowUp") setGuideRow((r) => Math.max(0, r - 1));
      else if (k === "Enter" && channels[guideRow]) {
        setPanel("none");
        onChannel(channels[guideRow]);
        backToChannels();
      } else return;
      event.preventDefault();
      return;
    }
    // On a TV the arrows walk the page, and the chrome when the stage has them.
    // The stage still seeks and changes channel when it has the keys. A mini
    // player must not take those keys from the page underneath it.
    const arrows = k === "ArrowLeft" || k === "ArrowRight" || k === "ArrowUp" || k === "ArrowDown";
    if (layout === "tv" && arrows && (!(target instanceof Element) || !target.closest(".stage") || target.closest(".stage-hud"))) return;
    const actions: Record<string, () => void> = {
      Escape: () => (panel !== "none" ? setPanel("none") : onMinimize()),
      // A TV remote's Back arrives as Backspace, so it leaves the player like Escape.
      Backspace: () => (panel !== "none" ? setPanel("none") : onMinimize()),
      " ": togglePlay,
      ArrowLeft: () => jump(-15),
      ArrowRight: () => jump(30),
      ArrowUp: () => step(-1),
      ArrowDown: () => step(1),
      g: openGuide,
      m: () => {
        const stored = localStorage.getItem("broadwave-mv-layout");
        navigate(multiviewPath([channel.id], isLayout(stored) ? stored : "2up", channel.id, true));
      },
      i: () => setPanel((p) => (p === "info" ? "none" : "info")),
      r: () => void toggleRecord(),
      l: lastChannel,
      t: () => setTheater((on) => !on),
      "?": () => setPanel("help"),
      c: () => setOpts((o) => ({ ...o, captions: !o.captions })),
    };
    const fn = actions[k] ?? actions[k.toLowerCase()];
    if (!fn) return;
    // A held C would flip twice, and Ctrl+C is Copy.
    if (k.toLowerCase() === "c" && (event.repeat || event.metaKey || event.ctrlKey || event.altKey)) return;
    event.preventDefault();
    fn();
  }

  // Nothing else on the page takes focus under the full player. A key that
  // lands on the page itself (focus lost to a remount or a click on nothing)
  // is the player's, or every key would go dead.
  const keyRef = useRef(onKey);
  useEffect(() => {
    keyRef.current = onKey;
  });
  useEffect(() => {
    if (mode !== "full") return;
    const onStray = (event: KeyboardEvent) => {
      if (event.defaultPrevented || (event.target !== document.body && event.target !== document.documentElement)) return;
      const root = rootRef.current;
      if (!root) return;
      if (layout === "tv") focusRing(root);
      else root.focus();
      keyRef.current(event as unknown as ReactKey);
    };
    window.addEventListener("keydown", onStray);
    return () => window.removeEventListener("keydown", onStray);
  }, [mode, layout]);

  const liveLabel = livePillLabel(opts.sync && sync.state !== "off", behind);
  const title = airing?.title || channel.displayName;
  const eyebrow = useMemo(
    () => (
      <>
        <span className="eyebrow-num">{channel.displayNumber}</span> {channel.displayName}
        {airing ? <span className="eyebrow-dim"> · {minutesLeft(airing, now)}</span> : null}
      </>
    ),
    [channel, airing, now],
  );

  const syncBadge =
    opts.sync && sync.state !== "off" ? (
      <button type="button" className={`sync-pill ${sync.state}`} onClick={() => setPanel((p) => (p === "sync" ? "none" : "sync"))} aria-label="Whole-Home Sync">
        <SyncIcon />
        {together ? (sync.members > 1 ? `Together · ${sync.members}` : "Together") : sync.members > 1 ? `${sync.members} screens` : "Synced"}
      </button>
    ) : null;

  return (
    <Stage
      videoRef={videoRef}
      rootRef={rootRef}
      mode={mode}
      onKeyDown={onKey}
      videoClass={zoom === "fit" ? "stage-video" : `stage-video ${zoom}`}
      eyebrow={eyebrow}
      title={title}
      score={airing?.gameId ? scores.get(airing.gameId) : undefined}
      onBack={onMinimize}
      backLabel="Back to browsing"
      onExpand={onExpand}
      onClose={onClose}
      liveLabel={liveLabel}
      onLive={goLive}
      position={scrubAt ?? span.at}
      duration={span.len}
      onSeek={(value) => {
        const video = videoRef.current;
        if (!video || !video.seekable.length) return;
        if (together) return scrubTogether(value);
        detachSync();
        video.currentTime = video.seekable.start(0) + value;
      }}
      onJump={jump}
      onTogglePlay={togglePlay}
      onSound={() => {
        const video = videoRef.current;
        if (video) saveSound(video);
      }}
      error={error}
      errorAction={
        stream.needsConfirm ? (
          <button type="button" className="btn small" onClick={stream.confirm}>
            Watch anyway
          </button>
        ) : error ? (
          <button type="button" className="btn small" onClick={stream.retry}>
            Try again
          </button>
        ) : null
      }
      note={error ? undefined : stream.reconnecting ? "Reconnecting…" : notice ? notice : sentText ? sentText : !airing ? listingNote(listing.checks > 0) : undefined}
      noteAction={
        !error && !stream.reconnecting && !notice && !sentText && !airing ? (
          <button
            type="button"
            className="btn small"
            disabled={listing.checking}
            onClick={() => {
              const id = channel.id;
              setListing((row) => ({ ...row, checking: true }));
              void refresh(["airings"]).finally(() => {
                setListing((row) => (row.id === id ? { id, checks: row.checks + 1, checking: false } : row));
              });
            }}
          >
            {listing.checking ? "Checking…" : "Check for listings"}
          </button>
        ) : null
      }
      badge={syncBadge}
      moreAside={panel === "move"}
      loading={tuning ? <TuningCard channel={channel} show={airing?.title} art={airing?.imageUrl ? `/media/art/airing/${airing.id}?w=960` : ""} mini={mode === "mini"} /> : null}
      tools={
        <>
          {layout === "phone" ? (
            <span className="channel-step">
              <button type="button" className="glass-icon" aria-label="Previous channel" onClick={() => step(-1)}>
                <ChevronIcon style={{ transform: "rotate(-90deg)" }} />
              </button>
              <button type="button" className="glass-icon" aria-label="Next channel" onClick={() => step(1)}>
                <ChevronIcon style={{ transform: "rotate(90deg)" }} />
              </button>
            </span>
          ) : null}
          <button type="button" className={panel === "guide" ? "glass-icon on" : "glass-icon"} onClick={() => (panel === "guide" ? setPanel("none") : openGuide())} aria-label="Channels">
            <ListIcon />
          </button>
          <button
            type="button"
            className="glass-icon"
            aria-label="Side by side"
            onClick={() => {
              const stored = localStorage.getItem("broadwave-mv-layout");
              navigate(multiviewPath([channel.id], isLayout(stored) ? stored : "2up", channel.id, true));
            }}
          >
            <SideBySideIcon />
          </button>
          {startOverFrom ? (
            <button type="button" className="text-btn" onClick={startOver}>
              Start over
            </button>
          ) : null}
          <button type="button" className={active ? "record-btn on" : "record-btn"} onClick={() => void toggleRecord()} aria-pressed={!!active}>
            <RecordIcon />
            {active ? "Recording" : "Record"}
          </button>
          <button type="button" className={panel === "info" ? "glass-icon on" : "glass-icon"} onClick={() => setPanel((p) => (p === "info" ? "none" : "info"))} aria-label={copy.player.stats}>
            <InfoIcon />
          </button>
        </>
      }
      more={
        <div className="options-grid">
          <OptionRow label="Quality" value={opts.quality} options={Object.keys(qualityLabels) as Options["quality"][]} labels={qualityLabels} onChange={(quality) => setOpts((o) => ({ ...o, quality }))} />
          <OptionRow label="Sound" value={opts.audio} options={["auto", "surround", "stereo"]} labels={{ auto: "Auto", surround: "Surround", stereo: "Stereo" }} onChange={(audio) => setOpts((o) => ({ ...o, audio }))} />
          {stream.sounds ? (
            <OptionRow
              label="Audio"
              value={soundFor(stream.sounds, opts.track)?.role ?? "main"}
              options={stream.sounds.map((s) => s.role)}
              labels={Object.fromEntries(stream.sounds.map((s) => [s.role, s.name]))}
              onChange={(track) => setOpts((o) => ({ ...o, track }))}
            />
          ) : (
            <OptionRow label="Audio" value={opts.track} options={["main", "language", "described"]} labels={{ main: "Main", language: "Second language", described: "Described video" }} onChange={(track) => setOpts((o) => ({ ...o, track }))} />
          )}
          {stream.canCaption ? (
            <OptionRow
              label="Captions"
              value={opts.captions ? "on" : "off"}
              options={["off", "on"]}
              labels={{ off: "Off", on: "On" }}
              pressed
              onChange={(v) => setOpts((o) => ({ ...o, captions: v === "on" }))}
            />
          ) : null}
          <OptionRow label="Even volume" value={opts.even ? "on" : "off"} options={["off", "on"]} labels={{ off: "Off", on: "On" }} onChange={(v) => setOpts((o) => ({ ...o, even: v === "on" }))} />
          <OptionRow
            label="Motion"
            value={picture}
            options={["broadcast", "smooth", "film"]}
            labels={{ broadcast: "Broadcast 60", smooth: "Smooth", film: "Film 24" }}
            onChange={(p) => {
              setPicture(p);
              void saveSettings({ pictureMode: p });
            }}
          />
          {sync.room ? (
            <OptionRow
              label="Live delay"
              value={sync.room.latency}
              options={["lowest", "balanced", "stable"]}
              labels={delayLabels}
              onChange={(latency) => {
                setAsked({ channel: channel.id, delay: latency });
                saveLiveDelay(latency);
                events().command(room, "latency", { latency });
              }}
            />
          ) : null}
          {asked?.channel === channel.id && asked.delay === "lowest" && sync.room && sync.room.latency !== "lowest" ? (
            <p className="option-note" role="status">
              {copy.player.delayApple}
            </p>
          ) : null}
          <OptionRow
            label="Picture"
            value={zoom}
            options={["fit", "fill", "zoom"]}
            labels={{ fit: "Fit", fill: "Fill", zoom: "Zoom" }}
            onChange={(z) => {
              setZoom(z);
              saveZoom(z);
            }}
          />
          <OptionRow
            label={copy.player.sleep}
            value={sleepFor ? String(sleepFor) : "off"}
            options={["off", "30", "60", "90"]}
            labels={{ off: "Off", "30": "30 min", "60": "1 hr", "90": "90 min" }}
            onChange={(v) => {
              const minutes = v === "off" ? 0 : Number(v);
              setSleepFor(minutes);
              setSleepUntil(sleepUntilFrom(minutes, Date.now()));
            }}
          />
          <VolumeRow videoRef={videoRef} />
          {sleepFor ? (
            <p className="option-note" role="status">
              {sleepSentence(sleepFor)}
            </p>
          ) : null}
          <div className="option-actions">
            <button type="button" className="text-btn" onClick={() => setPanel((p) => (p === "info" ? "none" : "info"))}>
              {copy.player.stats}
            </button>
            {together ? null : (
              // The other screen would join the channel at live, not this group's moment.
              <button ref={moveButton} type="button" className={panel === "move" ? "text-btn on" : "text-btn"} onClick={() => setPanel((p) => (p === "move" ? "none" : "move"))} aria-expanded={panel === "move"}>
                Move to another screen
              </button>
            )}
          </div>
        </div>
      }
    >
      {entry ? (
        <p className="typed-number glass" role="status" aria-label={`Channel ${entry}`}>
          {entry}
        </p>
      ) : null}
      {panel === "guide" ? (
        <div className="mini-guide glass" role="listbox" aria-label="Channels">
          {channels.map((c, i) => {
            const a = airingAt(index, c.id, now);
            return (
              <button
                key={c.id}
                type="button"
                role="option"
                aria-selected={i === guideRow}
                className={c.id === channel.id ? "mg-row current" : i === guideRow ? "mg-row on" : "mg-row"}
                onMouseEnter={() => {
                  if (i === guideRow) return;
                  hoverRow.current = true;
                  setGuideRow(i);
                }}
                onClick={() => {
                  setPanel("none");
                  onChannel(c);
                }}
              >
                <span className="mg-num">{c.displayNumber}</span>
                <span className="mg-body">
                  <span className="mg-name">{c.displayName}</span>
                  <span className="mg-title">{a?.title ?? "No listing"}</span>
                  <Progress value={progress(a, now)} category={categoryOf(a)} />
                </span>
              </button>
            );
          })}
        </div>
      ) : null}
      {panel === "info" ? (
        <div className="info-panel glass" role="dialog" aria-label={copy.player.stats}>
          <h3>{copy.player.stats}</h3>
          {session ? (
            <dl>
              <dt>{copy.player.playing}</dt>
              <dd>{session.stream.reason}</dd>
              <dt>{copy.player.source}</dt>
              <dd>{sourceLine(session.stream)}</dd>
              <dt>{copy.player.output}</dt>
              <dd>{outputLine(session.stream, playback)}</dd>
              <dt>{copy.player.bitrate}</dt>
              <dd>{bitrateText(session.stream.bitrate) || copy.player.waiting}</dd>
              <dt>{copy.player.dropped}</dt>
              <dd>{playback.total > 0 ? `${playback.dropped} of ${playback.total}` : "0"}</dd>
              <dt>{copy.player.buffer}</dt>
              <dd>{`${playback.buffer.toFixed(1)}s`}</dd>
              <dt>{copy.player.behind}</dt>
              <dd>{formatBehind(behind)}</dd>
              <dt>{copy.player.drift}</dt>
              <dd>{playback.drift == null ? copy.player.waiting : `${Math.round(playback.drift)} ms`}</dd>
              <dt>{copy.player.rendition}</dt>
              <dd>{session.rendition || copy.player.waiting}</dd>
              <dt>{copy.player.encoder}</dt>
              <dd>{session.encoder || session.stream.encoder || copy.player.waiting}</dd>
              <dt>{copy.player.sound}</dt>
              <dd>{session.stream.audio === "copy" ? `Original ${session.stream.sourceAudio ?? ""}` : session.stream.audio === "aac6" ? "5.1 AAC" : session.stream.audio === "ac3" ? "5.1 AC-3" : "Stereo AAC"}</dd>
              <dt>{copy.player.tuner}</dt>
              <dd>{session.shared ? `Shared · ${session.viewers} watching` : "This screen only"}</dd>
            </dl>
          ) : (
            <p>Tuning…</p>
          )}
        </div>
      ) : null}
      {panel === "help" ? <HelpDialog onClose={() => setPanel("none")} /> : null}
      {panel === "move" ? (
        <MovePanel
          channel={channel}
          onMoved={(name) => {
            setPanel("none");
            movedTo(name);
            onClose();
          }}
          onClose={() => {
            backToMove.current = true;
            setPanel("none");
          }}
        />
      ) : null}
      {panel === "sync" ? (
        <div className="info-panel glass" role="dialog" aria-labelledby="sync-panel-title">
          <h3 id="sync-panel-title">{together ? "Watching together" : "Whole-Home Sync"}</h3>
          {together ? (
            <>
              <p className="dim">Pause, rewind, and Live move every screen watching together.</p>
              <ul className="together-list" aria-label="Screens watching together">
                {(sync.room?.people ?? []).map((p, i) => (
                  <li key={`${p.name}:${p.kind}:${i}`}>{personLabel(p)}</li>
                ))}
              </ul>
              <div className="option-actions">
                <button type="button" className="btn" onClick={() => setTogetherOn(null)}>
                  Leave
                </button>
              </div>
            </>
          ) : (
            <>
              <p className="dim">Every screen on this channel shows the same moment{sync.members > 1 ? ` — ${sync.members} screens right now` : ""}.</p>
              <label className="switch-row">
                <input type="checkbox" checked={opts.sync} onChange={(e) => setOpts((o) => ({ ...o, sync: e.target.checked }))} />
                <span>Sync with other screens</span>
              </label>
              <p className="dim">
                {group?.people?.length
                  ? `${peopleSentence(group.people)} ${group.people.length === 1 ? "is" : "are"} watching together.`
                  : "Watch together: pause and rewind for every screen that joins."}
              </p>
              <div className="option-actions">
                <button
                  type="button"
                  className="btn"
                  onClick={() => {
                    setTogetherOn(channel.id);
                    setOpts((o) => ({ ...o, sync: true }));
                  }}
                >
                  {group?.people?.length ? "Join" : "Watch together"}
                </button>
              </div>
            </>
          )}
        </div>
      ) : null}
    </Stage>
  );
}

function OptionRow<T extends string>({
  label,
  value,
  options,
  labels,
  onChange,
  pressed,
}: {
  label: string;
  value: T | string;
  options: T[];
  labels: Record<string, string>;
  onChange: (v: T) => void;
  pressed?: boolean;
}) {
  return (
    <div className="option-row">
      <span className="option-label">{label}</span>
      <div className="segmented" role="group" aria-label={label}>
        {options.map((o) => (
          <button key={o} type="button" className={value === o ? "seg on" : "seg"} aria-pressed={pressed ? value === o : undefined} onClick={() => onChange(o)}>
            {labels[o] ?? o}
          </button>
        ))}
      </div>
    </div>
  );
}

type PictureStats = { width: number; height: number; dropped: number; total: number; buffer: number; fps: number; drift: number | null };

function usePlaybackStats(videoRef: RefObject<HTMLVideoElement | null>, on: boolean): PictureStats {
  const [stats, setStats] = useState<PictureStats>({ width: 0, height: 0, dropped: 0, total: 0, buffer: 0, fps: 0, drift: null });
  const prev = useRef({ frames: 0, at: 0 });
  useEffect(() => {
    if (!on) return;
    const id = window.setInterval(() => {
      const video = videoRef.current;
      if (!video) return;
      const quality = video.getVideoPlaybackQuality?.();
      const dropped = quality?.droppedVideoFrames ?? 0;
      const total = quality?.totalVideoFrames ?? 0;
      const buffer = bufferedAhead(video);
      const now = performance.now();
      let fps = 0;
      if (prev.current.at > 0 && now - prev.current.at > 400 && total >= prev.current.frames) {
        fps = ((total - prev.current.frames) * 1000) / (now - prev.current.at);
      }
      prev.current = { frames: total, at: now };
      const raw = video.dataset.syncDrift;
      const drift = raw == null || raw === "" || Number.isNaN(Number(raw)) ? null : Number(raw);
      setStats({ width: video.videoWidth, height: video.videoHeight, dropped, total, buffer, fps, drift });
    }, 1000);
    return () => window.clearInterval(id);
  }, [videoRef, on]);
  return stats;
}

function VolumeRow({ videoRef }: { videoRef: RefObject<HTMLVideoElement | null> }) {
  const [level, setLevel] = useState(1);
  useEffect(() => {
    const video = videoRef.current;
    if (video) setLevel(video.volume);
  }, [videoRef]);
  return (
    <label className="option-row">
      <span className="option-label">{copy.player.volume}</span>
      <input
        className="volume"
        type="range"
        min={0}
        max={1}
        step={0.05}
        value={level}
        aria-label={copy.player.volume}
        onChange={(event) => {
          const next = Number(event.target.value);
          setLevel(next);
          const video = videoRef.current;
          if (!video) return;
          video.volume = next;
          saveSound(video);
        }}
      />
    </label>
  );
}

function HelpDialog({ onClose }: { onClose: () => void }) {
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const root = ref.current;
    const prev = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    const items = () =>
      [...(root?.querySelectorAll<HTMLElement>("button, a[href], input, select, textarea") ?? [])].filter((el) => !el.hidden && !el.hasAttribute("disabled"));
    focusRing(items()[0] ?? root);
    const onKey = (event: KeyboardEvent) => {
      if (event.key !== "Tab") return;
      const list = items();
      if (list.length === 0) {
        event.preventDefault();
        event.stopPropagation();
        return;
      }
      const first = list[0];
      const last = list[list.length - 1];
      const active = document.activeElement;
      if (event.shiftKey && (active === first || !root?.contains(active))) {
        event.preventDefault();
        event.stopPropagation();
        focusRing(last);
      } else if (!event.shiftKey && (active === last || !root?.contains(active))) {
        event.preventDefault();
        event.stopPropagation();
        focusRing(first);
      }
    };
    window.addEventListener("keydown", onKey, true);
    return () => {
      window.removeEventListener("keydown", onKey, true);
      focusRing(prev);
    };
  }, []);
  return (
    <div ref={ref} className="info-panel help-panel glass" role="dialog" aria-modal="true" aria-label={copy.player.helpTitle} tabIndex={-1}>
      <h3>{copy.player.helpTitle}</h3>
      <ul className="key-list">
        {copy.player.keys.map(([key, what]) => (
          <li key={key}>
            <span>{key}</span>
            <span>{what}</span>
          </li>
        ))}
      </ul>
      <p>{copy.player.sleepHint}</p>
      <button type="button" className="btn small" onClick={onClose}>
        {copy.player.close}
      </button>
    </div>
  );
}

function sourceLine(stream: { sourceVideo?: string; sourceWidth?: number; sourceHeight?: number; scan?: string; sourceFps?: string }) {
  return joinFacts([stream.sourceVideo, sizeText(stream.sourceWidth, stream.sourceHeight), scanWord(stream.scan), stream.sourceFps]);
}

function outputLine(stream: { outputWidth?: number; outputHeight?: number; outputFps?: string; decode?: string; video?: string }, picture: PictureStats) {
  const width = picture.width || stream.outputWidth;
  const height = picture.height || stream.outputHeight;
  const fps = stream.outputFps || (picture.fps > 1 ? picture.fps.toFixed(2) : "");
  const decode = stream.decode === "gpu" ? "GPU decode" : stream.decode === "cpu" ? "CPU decode" : stream.video === "copy" ? "Direct" : "";
  return joinFacts([sizeText(width, height), fps, decode]);
}

function sizeText(width?: number, height?: number) {
  return width && height ? `${width}×${height}` : "";
}

function scanWord(scan?: string) {
  if (scan === "progressive") return "Progressive";
  if (scan === "interlaced") return "Interlaced";
  if (scan === "film") return "Film";
  return "";
}

function joinFacts(parts: Array<string | undefined>) {
  return parts.filter(Boolean).join(" · ") || copy.player.waiting;
}

function bitrateText(rate?: string) {
  if (!rate) return "";
  if (rate.endsWith("M")) return `${rate.slice(0, -1)} Mb/s`;
  if (rate.endsWith("k")) return `${rate.slice(0, -1)} kb/s`;
  return rate;
}

function bufferedAhead(video: HTMLVideoElement) {
  const t = video.currentTime;
  let ahead = 0;
  for (let i = 0; i < video.buffered.length; i++) {
    const start = video.buffered.start(i);
    const end = video.buffered.end(i);
    if (start <= t + 0.05 && end >= t) {
      ahead = Math.max(ahead, end - t);
    }
  }
  return Math.max(0, ahead);
}

/** "Live" while lined up. Far enough behind, the pill says how far and is the way back. */
export function livePillLabel(synced: boolean, behindSeconds: number): string {
  if (synced || behindSeconds < 14) return "Live";
  return `${formatBehind(behindSeconds)} behind`;
}

function formatBehind(seconds: number) {
  const s = Math.round(seconds);
  if (s < 60) return `${s}s`;
  const m = Math.floor(s / 60);
  return `${m}m ${s % 60}s`;
}

/**
 * True from a channel or setting change until the picture is moving. The first
 * frame alone is not enough: sync can hold it while the room catches up.
 */
function useFirstFrame(videoRef: RefObject<HTMLVideoElement | null>, key: string) {
  const [readyKey, setReadyKey] = useState("");
  useEffect(() => {
    const video = videoRef.current;
    if (!video) return;
    let from = -1;
    const onTime = () => {
      if (video.paused || video.readyState < 3) return;
      if (from < 0) from = video.currentTime;
      else if (video.currentTime - from >= 0.3) setReadyKey(key);
    };
    video.addEventListener("timeupdate", onTime);
    return () => video.removeEventListener("timeupdate", onTime);
  }, [videoRef, key]);
  return readyKey !== key;
}

const tuneSteps = [
  { at: 0, text: "Tuning the antenna" },
  { at: 3000, text: "Starting the picture" },
  { at: 7000, text: "Lining up with live" },
  { at: 18000, text: "Still tuning. A weak signal can take longer" },
];

function TuningCard({ channel, show, art, mini }: { channel: Channel; show?: string; art: string; mini: boolean }) {
  const [elapsed, setElapsed] = useState(0);
  useEffect(() => {
    const started = performance.now();
    const id = window.setInterval(() => setElapsed(performance.now() - started), 500);
    return () => window.clearInterval(id);
  }, [channel.id]);
  const step = [...tuneSteps].reverse().find((s) => elapsed >= s.at) ?? tuneSteps[0];
  return (
    <div className={mini ? "tuning mini" : "tuning"} role="status" aria-live="polite">
      {art ? <span className="tuning-art" aria-hidden="true" style={{ backgroundImage: `url(${art})` }} /> : null}
      <div className="tuning-card">
        <p className="tuning-num">
          {channel.displayNumber} <span>{channel.displayName}</span>
        </p>
        {show && show !== channel.displayName ? <p className="tuning-show">{show}</p> : null}
        {mini ? null : (
          <>
            <span className="tuning-bar" aria-hidden="true" />
            <p className="tuning-step">{step.text}…</p>
          </>
        )}
      </div>
    </div>
  );
}

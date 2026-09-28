import { useEffect, useRef } from "react";
import { useData } from "../../app/data";
import { usePlayer } from "../../app/player";
import { focusRing } from "../../app/remote";
import { navigate } from "../../app/router";
import { categoryLabel, categoryOf, guideSourceLine, isRecording, minutesLeft, progress, recordingKeys, spanLabel, dayLabel } from "../../lib/guide";
import type { Airing, Channel } from "../../types";
import { CloseIcon, PlayIcon, RecordIcon, StarIcon } from "../../ui/icons";
import { ArtFrame } from "../../ui/ArtFrame";
import { ChannelBadge, LiveDot, Progress } from "../../ui/primitives";

export function ProgramSheet({ channel, airing, onClose, onWatch }: { channel: Channel; airing?: Airing; onClose: () => void; onWatch: (c: Channel) => void }) {
  const { now, planned, recordings, passes, record, recordSeries, recordOnce, removePass, favorite, stopRecord } = useData();
  const player = usePlayer();
  const ref = useRef<HTMLDivElement>(null);
  const cat = categoryOf(airing);
  const onNow = airing ? Date.parse(airing.start) <= now && Date.parse(airing.end) > now : true;
  const rec = airing ? isRecording(recordingKeys(planned, recordings), airing, now) : null;
  const active = recordings.find((r) => r.status === "recording" && r.channelId === channel.id);
  const hasPass = airing ? passes.some((p) => p.kind !== "once" && p.title.toLowerCase() === airing.title.toLowerCase()) : false;
  const once = airing
    ? passes.find((p) => p.kind === "once" && p.channelId === channel.id && p.airingStart && Date.parse(p.airingStart) === Date.parse(airing.start))
    : undefined;
  const upcoming = airing ? Date.parse(airing.start) > now : false;

  useEffect(() => {
    const root = ref.current;
    const prev = document.activeElement as HTMLElement | null;
    const items = () =>
      [...(root?.querySelectorAll<HTMLElement>("button, a[href], input, select, textarea") ?? [])].filter((el) => !el.hidden && !el.hasAttribute("disabled"));
    const list = items();
    focusRing(list.find((el) => el.classList.contains("primary")) ?? list[0]);
    const onKey = (e: globalThis.KeyboardEvent) => {
      if (e.key === "Escape" || e.key === "Backspace") {
        e.preventDefault();
        onClose();
        return;
      }
      if (e.key !== "Tab" || !root) return;
      const list = items();
      if (list.length === 0) {
        e.preventDefault();
        return;
      }
      const first = list[0];
      const last = list[list.length - 1];
      const active = document.activeElement;
      if (e.shiftKey && (active === first || !root.contains(active))) {
        e.preventDefault();
        focusRing(last);
      } else if (!e.shiftKey && (active === last || !root.contains(active))) {
        e.preventDefault();
        focusRing(first);
      }
    };
    window.addEventListener("keydown", onKey);
    return () => {
      window.removeEventListener("keydown", onKey);
      focusRing(prev);
    };
  }, [onClose]);

  return (
    <div className="sheet-layer" onClick={onClose}>
      <div ref={ref} className="program-sheet glass" data-cat={cat} role="dialog" aria-modal="true" aria-label={airing?.title ?? channel.displayName} onClick={(e) => e.stopPropagation()}>
        <div className="ps-art" data-cat={cat}>
          {airing?.imageUrl ? <ArtFrame src={`/media/art/airing/${airing.id}?w=640`} width={airing.imageWidth} height={airing.imageHeight} /> : null}
          <button type="button" className="glass-icon ps-close" onClick={onClose} aria-label="Close">
            <CloseIcon />
          </button>
          <div className="ps-art-meta">
            <ChannelBadge channel={channel} size="lg" />
            {onNow && airing ? <LiveDot /> : null}
          </div>
        </div>
        <div className="ps-body">
          <p className="ps-eyebrow">
            {airing ? `${categoryLabel[cat]} · ${dayLabel(airing.start, now)} · ${spanLabel(airing)}` : "No listing"}
          </p>
          <h2 className="ps-title">{airing?.title ?? channel.displayName}</h2>
          {airing?.subtitle ? <p className="ps-sub">{airing.subtitle}</p> : null}
          {onNow && airing ? (
            <div className="ps-progress">
              <Progress value={progress(airing, now)} category={cat} />
              <span>{minutesLeft(airing, now)}</span>
            </div>
          ) : null}
          <div className="ps-tags">
            {airing?.new ? <span className="tag new">New</span> : null}
            {airing?.live ? <span className="tag">Live</span> : null}
            {airing?.premiere ? <span className="tag">Premiere</span> : null}
            {airing?.finale ? <span className="tag">Finale</span> : null}
            {airing?.rating ? <span className="tag">{airing.rating}</span> : null}
            {airing?.episodeLabel ? <span className="tag">{airing.episodeLabel}</span> : null}
            {channel.hd ? <span className="tag">HD</span> : null}
            {rec === "recording" ? <span className="tag rec">Recording</span> : rec === "scheduled" ? <span className="tag rec">Will record</span> : null}
          </div>
          {airing?.originalAir ? <p className="ps-sub">First aired {airing.originalAir}</p> : null}
          {airing?.cast ? <p className="ps-sub">{airing.cast}</p> : null}
          {airing?.description ? <p className="ps-desc">{airing.description}</p> : null}
          {guideSourceLine(airing?.guideSource) ? <p className="ps-sub">{guideSourceLine(airing?.guideSource)}</p> : null}
          <div className="ps-actions">
            {onNow ? (
              <button type="button" className="btn primary" onClick={() => onWatch(channel)}>
                <PlayIcon /> Watch
              </button>
            ) : null}
            {onNow ? (
              <button
                type="button"
                className="btn"
                onClick={() => {
                  const current = player.channel?.id;
                  const ids = current && current !== channel.id ? [current, channel.id] : [channel.id];
                  navigate(`/multiview?ch=${ids.join(",")}&layout=2up&focus=${channel.id}${ids.length < 2 ? "&add=1" : ""}`);
                  onClose();
                }}
              >
                Watch together
              </button>
            ) : null}
            {airing && onNow ? (
              active ? (
                <button type="button" className="btn" onClick={() => void stopRecord(active.id)}>
                  <RecordIcon className="tally" /> Stop recording
                </button>
              ) : (
                <button type="button" className="btn" onClick={() => void record(channel, airing.title)}>
                  <RecordIcon className="tally" /> Record
                </button>
              )
            ) : null}
            {airing && upcoming && !hasPass ? (
              once ? (
                <button type="button" className="btn" onClick={() => void removePass(once.id)}>
                  <RecordIcon className="tally" /> Don't record
                </button>
              ) : rec !== "scheduled" ? (
                <button type="button" className="btn" onClick={() => void recordOnce(airing, channel)}>
                  <RecordIcon className="tally" /> Record
                </button>
              ) : null
            ) : null}
            {airing ? (
              <button type="button" className="btn" disabled={hasPass} onClick={() => void recordSeries(airing.title, channel)}>
                {hasPass ? "Series is recording" : cat === "sports" ? "Record every airing" : "Record series"}
              </button>
            ) : null}
            <button type="button" className="btn ghost" onClick={() => void favorite(channel)} aria-pressed={channel.favorite}>
              <StarIcon filled={channel.favorite} /> {channel.favorite ? "Favorite" : "Add favorite"}
            </button>
          </div>
        </div>
      </div>
    </div>
  );
}

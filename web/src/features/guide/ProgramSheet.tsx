import { useEffect, useRef } from "react";
import { useData } from "../../app/data";
import { categoryLabel, categoryOf, isRecording, minutesLeft, progress, recordingKeys, spanLabel, dayLabel } from "../../lib/guide";
import type { Airing, Channel } from "../../types";
import { CloseIcon, PlayIcon, RecordIcon, StarIcon } from "../../ui/icons";
import { ChannelBadge, LiveDot, Progress } from "../../ui/primitives";

export function ProgramSheet({ channel, airing, onClose, onWatch }: { channel: Channel; airing?: Airing; onClose: () => void; onWatch: (c: Channel) => void }) {
  const { now, planned, recordings, passes, record, recordSeries, favorite, stopRecord } = useData();
  const ref = useRef<HTMLDivElement>(null);
  const cat = categoryOf(airing);
  const onNow = airing ? Date.parse(airing.start) <= now && Date.parse(airing.end) > now : true;
  const rec = airing ? isRecording(recordingKeys(planned, recordings), airing, now) : null;
  const active = recordings.find((r) => r.status === "recording" && r.channelId === channel.id);
  const hasPass = airing ? passes.some((p) => p.title.toLowerCase() === airing.title.toLowerCase()) : false;

  useEffect(() => {
    const prev = document.activeElement as HTMLElement | null;
    ref.current?.querySelector<HTMLElement>("button.primary, button")?.focus();
    const onKey = (e: globalThis.KeyboardEvent) => {
      if (e.key === "Escape") onClose();
    };
    window.addEventListener("keydown", onKey);
    return () => {
      window.removeEventListener("keydown", onKey);
      prev?.focus();
    };
  }, [onClose]);

  return (
    <div className="sheet-layer" onClick={onClose}>
      <div ref={ref} className="program-sheet glass" data-cat={cat} role="dialog" aria-modal="true" aria-label={airing?.title ?? channel.displayName} onClick={(e) => e.stopPropagation()}>
        <div className="ps-art" data-cat={cat}>
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
            {channel.hd ? <span className="tag">HD</span> : null}
            {rec === "recording" ? <span className="tag rec">Recording</span> : rec === "scheduled" ? <span className="tag rec">Will record</span> : null}
          </div>
          {airing?.description ? <p className="ps-desc">{airing.description}</p> : null}
          <div className="ps-actions">
            {onNow ? (
              <button type="button" className="btn primary" onClick={() => onWatch(channel)}>
                <PlayIcon /> Watch
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

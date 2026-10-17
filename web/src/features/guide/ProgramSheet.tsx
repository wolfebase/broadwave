import { useEffect, useRef, useState, type MouseEvent } from "react";
import { guideUiMessages } from "../../i18n/messages/guideUi.ts";
import { bind } from "../../i18n/runtime.ts";
import { useData } from "../../app/data";
import { usePlayer } from "../../app/player";
import { useDialogFocus } from "../../ui/useDialogFocus";
import { navigate } from "../../app/router";
import { useLayout } from "../../app/layout";
import { actionName } from "../../lib/actionName";
import { categoryLabel, categoryOf, isRecording, liveChannel, minutesLeft, progress, recordingKeys, spanLabel, dayLabel } from "../../lib/guide";
import type { Airing, Channel } from "../../types";
import { CloseIcon, PlayIcon, RecordIcon, StarIcon } from "../../ui/icons";
import { ArtFrame } from "../../ui/ArtFrame";
import { ChannelBadge, LiveDot, Progress } from "../../ui/primitives";
import { useAiringDetails } from "./useAiringDetails";

const t = bind(guideUiMessages);

type Step = "record" | "series";

/**
 * On a TV a remote's Enter opens the sheet with Record focused, so a second or held
 * Enter would record. There the first press asks, and a second press records once the
 * key was let go in between. A pointer click is deliberate and needs no release.
 */
function useConfirmStep(ask: boolean) {
  const [armed, setArmed] = useState<Step | null>(null);
  const released = useRef(true);
  useEffect(() => {
    if (!armed) return;
    released.current = false;
    const up = (event: KeyboardEvent) => {
      if (event.key === "Enter" || event.key === " ") released.current = true;
    };
    window.addEventListener("keyup", up, true);
    const timer = window.setTimeout(() => setArmed(null), 8000);
    return () => {
      window.removeEventListener("keyup", up, true);
      window.clearTimeout(timer);
    };
  }, [armed]);
  const press = (step: Step, event: MouseEvent, act: () => void) => {
    if (!ask) return act();
    if (armed === step && (released.current || event.detail > 0)) {
      setArmed(null);
      act();
      return;
    }
    setArmed(step);
  };
  return { armed, press, disarm: () => setArmed(null) };
}

export function ProgramSheet({ channel, airing: listed, onClose, onWatch }: { channel: Channel; airing?: Airing; onClose: () => void; onWatch: (c: Channel) => void }) {
  const airing = useAiringDetails(listed);
  const { now, planned, recordings, passes, record, recordSeries, recordOnce, removePass, favorite, stopRecord, allChannels } = useData();
  const shown = liveChannel(allChannels, channel);
  const player = usePlayer();
  const ref = useRef<HTMLDivElement>(null);
  useDialogFocus(ref, onClose);
  const cat = categoryOf(airing);
  const onNow = airing ? Date.parse(airing.start) <= now && Date.parse(airing.end) > now : true;
  const rec = airing ? isRecording(recordingKeys(planned, recordings), airing, now) : null;
  const active = recordings.find((r) => r.status === "recording" && r.channelId === channel.id);
  const hasPass = airing ? passes.some((p) => p.kind !== "once" && p.title.toLowerCase() === airing.title.toLowerCase()) : false;
  const once = airing
    ? passes.find((p) => p.kind === "once" && p.channelId === channel.id && p.airingStart && Date.parse(p.airingStart) === Date.parse(airing.start))
    : undefined;
  const upcoming = airing ? Date.parse(airing.start) > now : false;
  const subject = airing?.title ?? channel.displayName;
  const named = (action: string) => actionName(action, subject);
  const confirm = useConfirmStep(useLayout() === "tv");
  const recordLabel = confirm.armed === "record" ? t("guideUi.confirmRecord") : t("guideUi.record");
  const seriesLabel = hasPass
    ? t("guideUi.seriesRecording")
    : confirm.armed === "series"
      ? cat === "sports"
        ? t("guideUi.confirmEvery")
        : t("guideUi.confirmSeries")
      : cat === "sports"
        ? t("guideUi.recordEvery")
        : t("guideUi.recordSeries");

  return (
    <div className="sheet-layer" onClick={onClose}>
      <div ref={ref} className="program-sheet glass" data-cat={cat} role="dialog" aria-modal="true" aria-label={airing?.title ?? channel.displayName} onClick={(e) => e.stopPropagation()}>
        <div className="ps-art" data-cat={cat}>
          {airing?.imageUrl ? <ArtFrame src={`/media/art/airing/${airing.id}?w=640`} width={airing.imageWidth} height={airing.imageHeight} /> : null}
          <button type="button" className="glass-icon ps-close" onClick={onClose} aria-label={t("guideUi.close")}>
            <CloseIcon />
          </button>
          {/* Over a picture the badge covered the art. It moves under it. */}
          {airing?.imageUrl ? null : (
            <div className="ps-art-meta">
              <ChannelBadge channel={channel} size="lg" />
              {onNow && airing ? <LiveDot /> : null}
            </div>
          )}
        </div>
        <div className="ps-body">
          {airing?.imageUrl ? (
            <div className="ps-channel">
              <ChannelBadge channel={channel} />
              {onNow ? <LiveDot /> : null}
            </div>
          ) : null}
          <p className="ps-eyebrow">
            {airing ? `${categoryLabel[cat]} · ${dayLabel(airing.start, now)} · ${spanLabel(airing)}` : t("guideUi.noListing")}
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
            {airing?.new ? <span className="tag new">{t("guideUi.new")}</span> : null}
            {airing?.live ? <span className="tag">{t("guideUi.live")}</span> : null}
            {airing?.premiere ? <span className="tag">{t("guideUi.premiere")}</span> : null}
            {airing?.finale ? <span className="tag">{t("guideUi.finale")}</span> : null}
            {airing?.rating ? <span className="tag">{airing.rating}</span> : null}
            {airing?.episodeLabel ? <span className="tag">{airing.episodeLabel}</span> : null}
            {rec === "recording" ? <span className="tag rec">{t("guideUi.recording")}</span> : rec === "scheduled" ? <span className="tag rec">{t("guideUi.willRecord")}</span> : null}
          </div>
          {airing?.originalAir ? <p className="ps-sub">{t("guideUi.firstAired", { date: airing.originalAir })}</p> : null}
          {airing?.cast ? <p className="ps-sub">{airing.cast}</p> : null}
          {airing?.description && airing.description.trim() !== airing.subtitle?.trim() ? <p className="ps-desc">{airing.description}</p> : null}
          <div className="ps-actions">
            {onNow ? (
              <button type="button" className="btn primary" onClick={() => onWatch(shown)}>
                <PlayIcon /> {t("guideUi.watch")}
              </button>
            ) : null}
            {onNow ? (
              <button
                type="button"
                className="btn"
                aria-label={named(t("guideUi.watchTogether"))}
                onClick={() => {
                  const current = player.channel?.id;
                  const ids = current && current !== channel.id ? [current, channel.id] : [channel.id];
                  navigate(`/multiview?ch=${ids.join(",")}&layout=2up&focus=${channel.id}${ids.length < 2 ? "&add=1" : ""}`);
                  onClose();
                }}
              >
                {t("guideUi.watchTogether")}
              </button>
            ) : null}
            {airing && onNow ? (
              active ? (
                <button type="button" className="btn" aria-label={named(t("guideUi.stopRecording"))} onClick={() => void stopRecord(active.id)}>
                  <RecordIcon className="tally" /> {t("guideUi.stopRecording")}
                </button>
              ) : (
                <button
                  type="button"
                  className="btn"
                  aria-label={named(recordLabel)}
                  onBlur={confirm.disarm}
                  onClick={(event) => confirm.press("record", event, () => void record(channel, airing.title))}
                >
                  <RecordIcon className="tally" /> {recordLabel}
                </button>
              )
            ) : null}
            {airing && upcoming && !hasPass ? (
              once ? (
                <button type="button" className="btn" aria-label={named(t("guideUi.dontRecord"))} onClick={() => void removePass(once.id)}>
                  <RecordIcon className="tally" /> {t("guideUi.dontRecord")}
                </button>
              ) : rec !== "scheduled" ? (
                <button
                  type="button"
                  className="btn primary"
                  aria-label={named(recordLabel)}
                  onBlur={confirm.disarm}
                  onClick={(event) => confirm.press("record", event, () => void recordOnce(airing, channel))}
                >
                  <RecordIcon className="tally" /> {recordLabel}
                </button>
              ) : null
            ) : null}
            {airing ? (
              <button
                type="button"
                className="btn"
                disabled={hasPass}
                aria-label={named(seriesLabel)}
                onBlur={confirm.disarm}
                onClick={(event) => confirm.press("series", event, () => void recordSeries(airing.title, channel.id))}
              >
                {seriesLabel}
              </button>
            ) : null}
            {/* The star is the channel's, not the show's, so it says so. */}
            <button type="button" className="btn ghost" onClick={() => void favorite(shown)} aria-pressed={shown.favorite} aria-label={t("guideUi.favoriteChannel", { channel: shown.displayName })}>
              <StarIcon filled={shown.favorite} /> {t("guideUi.favoriteChannel", { channel: shown.displayName })}
            </button>
          </div>
        </div>
      </div>
    </div>
  );
}

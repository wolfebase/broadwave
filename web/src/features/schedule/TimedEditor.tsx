import { useEffect, useState } from "react";
import type { Channel, NewPass, Pass, PassPreview } from "../../types";
import { addSeriesPass, previewPass, updatePass } from "../../api";
import { scheduleMessages } from "../../i18n/messages/schedule.ts";
import { bind, weekdayShortNames } from "../../i18n/runtime.ts";
import { copy } from "../../strings";
import { PreviewLines, clockLabel } from "./PreviewLines";
import { durationPresets, maxMinutes, newTimedDraft, timedBody, timedDraftOf, timedEnd, timedProblem, type TimedDraft } from "./manual";

const t = bind(scheduleMessages);

const problems = {
  channel: "schedule.needChannel",
  time: "schedule.needTime",
  duration: "schedule.needDuration",
  date: "schedule.needDate",
  days: "schedule.needDays",
} as const;

/**
 * Record a channel at a set time, listed or not: once on a date, or on the
 * days picked. The plan below shows what it records and what it pushes out,
 * as it does for a series.
 */
export function TimedEditor({
  pass,
  channels,
  start,
  onDone,
}: {
  pass?: Pass;
  channels: Channel[];
  start?: { channelId: number; start?: Date; minutes?: number };
  onDone: (saved: boolean) => void;
}) {
  const [draft, setDraft] = useState<TimedDraft>(() =>
    pass ? timedDraftOf(pass, new Date()) : newTimedDraft(new Date(), start?.channelId ?? 0, start?.start, start?.minutes),
  );
  const [custom, setCustom] = useState(() => !durationPresets.some((m) => m === draft.duration));
  const [preview, setPreview] = useState<PassPreview | null>(null);
  const [previewNote, setPreviewNote] = useState("");
  const [error, setError] = useState("");
  const [saving, setSaving] = useState(false);
  const problem = timedProblem(draft);
  const body = timedBody(draft, Boolean(pass));
  const key = JSON.stringify(body);
  useEffect(() => {
    // The plan below is hidden until the draft is whole.
    if (problem) return;
    const abort = new AbortController();
    const timer = window.setTimeout(() => {
      previewPass({ ...(body as NewPass), id: pass?.id }, abort.signal)
        .then((res) => {
          setPreview(res);
          setPreviewNote("");
        })
        .catch((err: unknown) => {
          if (abort.signal.aborted) return;
          setPreview(null);
          setPreviewNote(err instanceof Error ? err.message : "");
        });
    }, 350);
    return () => {
      window.clearTimeout(timer);
      abort.abort();
    };
    // body is rebuilt each render; key holds its value.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [key, pass?.id, problem]);

  function set<K extends keyof TimedDraft>(field: K, value: TimedDraft[K]) {
    setDraft((d) => ({ ...d, [field]: value }));
  }

  async function save() {
    if (problem) {
      setError(t(problems[problem]));
      return;
    }
    setSaving(true);
    setError("");
    try {
      if (pass) await updatePass(pass.id, body as Partial<Pass>);
      else await addSeriesPass(body as NewPass);
      onDone(true);
    } catch (err) {
      setError(err instanceof Error ? err.message : t("schedule.saveFailed"));
    } finally {
      setSaving(false);
    }
  }

  const id = pass?.id ?? "timed";
  const dayNames = weekdayShortNames();
  const shown = channels.filter((ch) => !ch.hidden || ch.id === draft.channelId);
  const picked = channels.find((ch) => ch.id === draft.channelId);
  const label = pass && picked ? t("schedule.timedNamed", { channel: `${picked.displayNumber} ${picked.displayName}` }) : t("schedule.recordTime");
  const end = timedEnd(draft.timeStart, draft.duration);
  return (
    <form
      className="pass-editor timed-editor"
      aria-label={pass ? t("schedule.editNamed", { title: label }) : label}
      onSubmit={(event) => {
        event.preventDefault();
        void save();
      }}
    >
      <div className="pe-row">
        <label className="pe-label" htmlFor={`pe-channel-${id}`}>
          {t("schedule.channel")}
        </label>
        <div className="pe-control">
          <select id={`pe-channel-${id}`} value={draft.channelId} autoFocus={!pass && !draft.channelId} onChange={(e) => set("channelId", Number(e.target.value))}>
            <option value={0} disabled>
              {t("schedule.pickChannel")}
            </option>
            {shown.map((ch) => (
              <option key={ch.id} value={ch.id}>
                {ch.displayNumber} {ch.displayName}
              </option>
            ))}
          </select>
        </div>
      </div>
      <div className="pe-row">
        <span className="pe-label" id={`pe-repeat-${id}`}>
          {t("schedule.repeat")}
        </span>
        <div className="pe-control">
          <div className="segmented" role="group" aria-labelledby={`pe-repeat-${id}`}>
            <button type="button" className={draft.repeat ? "seg" : "seg on"} aria-pressed={!draft.repeat} onClick={() => set("repeat", false)}>
              {t("schedule.once")}
            </button>
            <button
              type="button"
              className={draft.repeat ? "seg on" : "seg"}
              aria-pressed={draft.repeat}
              onClick={() =>
                setDraft((d) => ({
                  ...d,
                  repeat: true,
                  // Repeat starts on the day the date was on.
                  days: d.days.length ? d.days : [new Date(`${d.date}T12:00`).getDay()].filter((n) => !Number.isNaN(n)),
                }))
              }
            >
              {t("schedule.onDays")}
            </button>
          </div>
        </div>
      </div>
      {draft.repeat ? (
        <div className="pe-row pass-days" role="group" aria-labelledby={`pe-days-${id}`}>
          <span className="pe-label" id={`pe-days-${id}`}>
            {t("schedule.days")}
          </span>
          <div className="pe-control guide-chips pe-days">
            {dayNames.map((name, day) => {
              const on = draft.days.includes(day);
              return (
                <button
                  key={name}
                  type="button"
                  className={`chip${on ? " on" : ""}`}
                  aria-pressed={on}
                  onClick={() => set("days", on ? draft.days.filter((d) => d !== day) : [...draft.days, day].sort())}
                >
                  {name}
                </button>
              );
            })}
          </div>
        </div>
      ) : (
        <div className="pe-row">
          <label className="pe-label" htmlFor={`pe-date-${id}`}>
            {t("schedule.date")}
          </label>
          <div className="pe-control">
            <input id={`pe-date-${id}`} type="date" value={draft.date} onChange={(e) => set("date", e.target.value)} />
          </div>
        </div>
      )}
      <div className="pe-row">
        <label className="pe-label" htmlFor={`pe-start-${id}`}>
          {t("schedule.startTime")}
        </label>
        <div className="pe-control pe-inline">
          <input id={`pe-start-${id}`} type="time" value={draft.timeStart} onChange={(e) => set("timeStart", e.target.value)} />
          {problem !== "time" && problem !== "duration" ? (
            <span className="pe-note">{t(end.nextDay ? "schedule.endsNextDay" : "schedule.endsAt", { end: clockLabel(end.clock) })}</span>
          ) : null}
        </div>
      </div>
      <div className="pe-row">
        <span className="pe-label" id={`pe-length-${id}`}>
          {t("schedule.length")}
        </span>
        <div className="pe-control pe-inline">
          <div className="segmented timed-lengths" role="group" aria-labelledby={`pe-length-${id}`}>
            {durationPresets.map((m) => {
              const on = !custom && draft.duration === m;
              return (
                <button
                  key={m}
                  type="button"
                  className={on ? "seg on" : "seg"}
                  aria-pressed={on}
                  onClick={() => {
                    setCustom(false);
                    set("duration", m);
                  }}
                >
                  {m % 60 === 0 ? t("schedule.hours", m / 60) : t("schedule.minutes", { count: m })}
                </button>
              );
            })}
            <button type="button" className={custom ? "seg on" : "seg"} aria-pressed={custom} onClick={() => setCustom(true)}>
              {t("schedule.custom")}
            </button>
          </div>
          {custom ? (
            <label className="pe-unit">
              <input
                type="number"
                min={1}
                max={maxMinutes}
                aria-label={t("schedule.lengthMinutes")}
                value={draft.duration || ""}
                onChange={(e) => set("duration", Math.round(Number(e.target.value)) || 0)}
              />
              <span>{t("schedule.minUnit")}</span>
            </label>
          ) : null}
        </div>
      </div>
      <div className="pe-row">
        <label className="pe-label" htmlFor={`pe-title-${id}`}>
          {t("schedule.timedTitle")}
        </label>
        <div className="pe-control">
          <input
            id={`pe-title-${id}`}
            type="text"
            className="timed-title"
            placeholder={t("schedule.timedTitlePlaceholder")}
            value={draft.title}
            onChange={(e) => set("title", e.target.value)}
          />
        </div>
      </div>
      <div className="pe-row" role="group" aria-labelledby={`pe-pad-${id}`}>
        <span className="pe-label" id={`pe-pad-${id}`}>
          {t("schedule.padding")}
        </span>
        <div className="pe-control pe-inline">
          <label className="pe-unit">
            <span>{t("schedule.start")}</span>
            <input type="number" min={0} max={30} aria-label={t("schedule.startEarlyLabel")} value={draft.padBefore} onChange={(e) => set("padBefore", clamp(e.target.value, 0, 30))} />
            <span>{t("schedule.minEarly")}</span>
          </label>
          <label className="pe-unit">
            <span>{t("schedule.end")}</span>
            <input type="number" min={0} max={30} aria-label={t("schedule.endLateLabel")} value={draft.padAfter} onChange={(e) => set("padAfter", clamp(e.target.value, 0, 30))} />
            <span>{t("schedule.minLate")}</span>
          </label>
        </div>
      </div>
      {draft.repeat ? (
        <>
          <div className="pe-row">
            <span className="pe-label" id={`pe-keep-${id}`}>
              {t("schedule.keep")}
            </span>
            <div className="pe-control pe-inline">
              <div className="segmented" role="group" aria-labelledby={`pe-keep-${id}`}>
                {(["all", "unwatched", "last"] as const).map((mode) => (
                  <button key={mode} type="button" className={draft.keepMode === mode ? "seg on" : "seg"} aria-pressed={draft.keepMode === mode} onClick={() => set("keepMode", mode)}>
                    {mode === "all" ? t("schedule.keepAll") : mode === "unwatched" ? t("schedule.keepUnwatched") : t("schedule.keepNewest")}
                  </button>
                ))}
              </div>
              {draft.keepMode === "last" ? (
                <label className="pe-unit">
                  <input type="number" min={1} max={99} aria-label={t("schedule.keepNewestCount")} value={draft.keepCount} onChange={(e) => set("keepCount", clamp(e.target.value, 1, 99))} />
                  <span>{t("schedule.recordings")}</span>
                </label>
              ) : null}
            </div>
          </div>
          <div className="pe-row">
            <span className="pe-label" id={`pe-limit-${id}`}>
              {t("schedule.limit")}
            </span>
            <div className="pe-control pe-inline">
              <div className="segmented" role="group" aria-labelledby={`pe-limit-${id}`}>
                <button type="button" className={draft.limitCount === 0 ? "seg on" : "seg"} aria-pressed={draft.limitCount === 0} onClick={() => set("limitCount", 0)}>
                  {t("schedule.noLimitChoice")}
                </button>
                <button type="button" className={draft.limitCount > 0 ? "seg on" : "seg"} aria-pressed={draft.limitCount > 0} onClick={() => set("limitCount", draft.limitCount || 5)}>
                  {t("schedule.limitAt")}
                </button>
              </div>
              {draft.limitCount > 0 ? (
                <label className="pe-unit">
                  <input type="number" min={1} max={99} aria-label={t("schedule.limitCount")} value={draft.limitCount} onChange={(e) => set("limitCount", clamp(e.target.value, 1, 99))} />
                  <span>{t("schedule.unwatchedUnit")}</span>
                </label>
              ) : null}
            </div>
          </div>
        </>
      ) : null}
      <div className="pe-row" role="group" aria-labelledby={`pe-opts-${id}`}>
        <span className="pe-label" id={`pe-opts-${id}`}>
          {t("schedule.options")}
        </span>
        <div className="pe-control pe-switches">
          <label className="switch-row">
            <input type="checkbox" checked={draft.commercials} onChange={(e) => set("commercials", e.target.checked)} />
            <span>{copy.library.findCommercials}</span>
          </label>
        </div>
      </div>
      {problem ? null : <PreviewLines preview={preview} note={previewNote} channels={channels} timed />}
      {preview && preview.utcOffset !== -new Date().getTimezoneOffset() * 60 ? <p className="hint">{t("schedule.serverClock", { zone: preview.timeZone })}</p> : null}
      {error ? (
        <p className="hint error" role="alert">
          {error}
        </p>
      ) : null}
      <div className="pe-actions">
        <button type="submit" className="btn primary" disabled={saving}>
          {pass ? t("schedule.save") : t("schedule.addTimed")}
        </button>
        <button type="button" className="btn ghost" onClick={() => onDone(false)}>
          {t("schedule.cancel")}
        </button>
      </div>
    </form>
  );
}

function clamp(value: string, min: number, max: number) {
  const n = Math.round(Number(value));
  if (!Number.isFinite(n)) return min;
  return Math.min(max, Math.max(min, n));
}

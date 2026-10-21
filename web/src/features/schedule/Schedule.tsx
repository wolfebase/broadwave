import { useEffect, useMemo, useRef, useState } from "react";
import type { PlannedAiring, Pass, Recording } from "../../types";
import { fixSchedule, getEvents, getSchedule, stopRecording } from "../../api";
import { useData } from "../../app/data";
import { focusRing } from "../../app/remote";
import { navigate, useRoute } from "../../app/router";
import { scheduleMessages } from "../../i18n/messages/schedule.ts";
import { bind } from "../../i18n/runtime.ts";
import { actionName } from "../../lib/actionName";
import { dismissesLayer, pageCovered } from "../../lib/dialogFocus";
import { formatClock } from "../../time";
import { Passes } from "./Passes";
import { timedRequest } from "./manual";
import { takeScheduleRead, takeScheduleWrite } from "./plan";
import { activityRows, dayOf, notRecording } from "./view";
import "../manage.css";
import "./schedule.css";

const t = bind(scheduleMessages);

// Activity can run to hundreds of lines. The first screenful is what anyone reads.
const firstActivity = 12;

type Event = { id: number; at: string; kind: string; message: string };

export function Schedule({
  recordings,
  passes,
  onStop,
  onPasses,
}: {
  recordings: Recording[];
  passes: Pass[];
  onStop: () => void;
  onPasses: () => void;
}) {
  const { channels } = useData();
  // /schedule?record=<channel> comes from the guide: open Record a time for that channel.
  const { params } = useRoute();
  const ask = params.get("record") ? params.toString() : "";
  const request = useMemo(() => (ask ? timedRequest(new URLSearchParams(ask)) : null), [ask]);
  const active = recordings.filter((rec) => rec.status === "recording");
  const [items, setItems] = useState<PlannedAiring[]>([]);
  const [tunerCount, setTunerCount] = useState(2);
  const [events, setEvents] = useState<Event[]>([]);
  const [everything, setEverything] = useState(false);
  const [more, setMore] = useState(false);
  const [fixing, setFixing] = useState("");
  const [note, setNote] = useState("");
  const planN = useRef(0);
  const writeN = useRef(0);
  useEffect(() => {
    const mine = ++planN.current;
    const writeAt = writeN.current;
    void getSchedule()
      .then((res) => {
        const next = takeScheduleRead(mine, planN.current, writeAt, writeN.current, res.items, res.tunerCount);
        if (!next) return;
        setItems(next.items);
        setTunerCount(next.tunerCount);
      })
      .catch(() => undefined);
    return () => {
      planN.current += 1;
    };
  }, [passes, recordings]);
  useEffect(() => {
    let cancel = false;
    void getEvents()
      .then((res) => {
        if (!cancel) setEvents(res.events);
      })
      .catch(() => undefined);
    return () => {
      cancel = true;
    };
  }, [passes, recordings]);
  async function recordLater(item: PlannedAiring) {
    const alt = item.suggestion;
    if (!alt || fixing) return;
    const mine = ++writeN.current;
    planN.current += 1;
    setFixing("1");
    setNote("");
    try {
      const res = await fixSchedule({
        passId: item.passId,
        channelId: item.airing.channelId,
        start: item.airing.start,
        suggestionChannelId: alt.channelId,
        suggestionStart: alt.start,
      });
      const next = takeScheduleWrite(mine, writeN.current, res.items, res.tunerCount);
      if (next) {
        setItems(next.items);
        setTunerCount(next.tunerCount);
        onPasses();
      }
    } catch (err) {
      // The later airing may have stopped fitting; show what fits now.
      const fresh = await getSchedule().catch(() => undefined);
      const next = takeScheduleWrite(mine, writeN.current, fresh?.items ?? null, fresh?.tunerCount ?? tunerCount);
      if (next) {
        setItems(next.items);
        setTunerCount(next.tunerCount);
      }
      if (mine === writeN.current) setNote(err instanceof Error ? err.message : t("schedule.couldNot"));
    } finally {
      if (mine === writeN.current) setFixing("");
    }
  }
  const number = (item: PlannedAiring) => item.airing.guideNumber || channels.find((ch) => ch.id === item.airing.channelId)?.displayNumber || "";
  const upcoming = notRecording(items, active);
  const { rows: activity, hidden } = activityRows(events, everything);
  const listed = more ? activity : activity.slice(0, firstActivity);
  const now = new Date();
  return (
    <section className="page schedule">
      <section className="sched-section" aria-labelledby="sched-coming">
        <h2 id="sched-coming" className="section-title">
          {t("schedule.comingUp")}
        </h2>
        {upcoming.length === 0 && active.length === 0 ? (
          <div className="sched-empty">
            <p>{passes.length === 0 ? t("schedule.nothingSet") : t("schedule.noMatch")}</p>
            <button type="button" className="btn small" onClick={() => navigate("/guide")}>
              {t("schedule.openGuide")}
            </button>
          </div>
        ) : (
          <ul className="sched-list">
            {active.map((rec) => (
              <LiveRow key={`rec-${rec.id}`} rec={rec} onStop={onStop} />
            ))}
            {upcoming.map((item) => {
              const start = new Date(item.airing.start);
              const window = recordWindow(item);
              const ch = number(item);
              return (
                <li key={`${item.passId}-${item.airing.id}`} className={item.skipped ? "sched-row skipped" : "sched-row"}>
                  <span className="sched-when">
                    <span className="sched-day">{dayName(start, now)}</span>
                    <span className="sched-clock">{formatClock(start)}</span>
                  </span>
                  <span className="sched-main">
                    <span className="sched-title">
                      {item.manual ? <span className="tag pass-kind" data-kind="manual">{t("schedule.timedTag")}</span> : null}
                      {item.manual ? " " : null}
                      {item.airing.title}
                    </span>
                    <span className="sched-sub">
                      {item.skipped ? (
                        <>
                          <span className="tag sched-skip">{t("schedule.skippedTag")}</span> {item.reason || t("schedule.lowerPriority", tunerCount)}
                        </>
                      ) : ch ? (
                        t("schedule.recordsWindow", { number: ch, start: formatClock(window.start), end: formatClock(window.end) })
                      ) : (
                        t("schedule.recordsWindowNoNumber", { start: formatClock(window.start), end: formatClock(window.end) })
                      )}
                    </span>
                  </span>
                  {item.suggestion ? (
                    <span className="schedule-suggest">
                      <span>
                        {item.suggestion.guideNumber
                          ? t("schedule.laterOnChannel", { when: formatDay(new Date(item.suggestion.start), now), number: item.suggestion.guideNumber })
                          : t("schedule.laterOn", { when: formatDay(new Date(item.suggestion.start), now) })}
                      </span>
                      <button
                        type="button"
                        className="btn small schedule-fix"
                        disabled={fixing !== ""}
                        aria-label={actionName(t("schedule.recordLater"), item.airing.title)}
                        onClick={() => void recordLater(item)}
                      >
                        {t("schedule.recordLater")}
                      </button>
                    </span>
                  ) : null}
                </li>
              );
            })}
          </ul>
        )}
        {note ? (
          <p className="hint" role="alert">
            {note}
          </p>
        ) : null}
        {items.some((item) => item.conflict) ? <p className="hint">{t("schedule.conflictHint")}</p> : null}
      </section>
      <Passes passes={passes} onPasses={onPasses} request={request} onRequestDone={() => (ask ? navigate("/schedule", true) : undefined)} />
      <section className="sched-section" aria-labelledby="sched-activity">
        <div className="sched-head">
          <h2 id="sched-activity" className="section-title">
            {t("schedule.activity")}
          </h2>
          {hidden > 0 || everything ? (
            <button type="button" className="chip sched-finds" aria-pressed={everything} onClick={() => setEverything((on) => !on)}>
              {everything ? t("schedule.hideFinds") : t("schedule.showFinds", hidden)}
            </button>
          ) : null}
        </div>
        {activity.length === 0 ? (
          <p className="sched-empty">{t("schedule.activityQuiet")}</p>
        ) : (
          <ul className="sched-activity">
            {listed.map((event) => {
              const at = new Date(event.at);
              const day = dayOf(at, now);
              return (
                <li key={event.id} data-kind={event.kind}>
                  <time dateTime={event.at}>{day === "today" ? formatClock(at) : formatDay(at, now)}</time>
                  <span>{event.message}</span>
                </li>
              );
            })}
          </ul>
        )}
        {activity.length > listed.length ? (
          <button type="button" className="btn small ghost sched-more" onClick={() => setMore(true)}>
            {t("schedule.showMore", { count: activity.length - listed.length })}
          </button>
        ) : null}
      </section>
    </section>
  );
}

/** A recording on now. Stop asks first: the button sits where a quick tap lands. */
function LiveRow({ rec, onStop }: { rec: Recording; onStop: () => void }) {
  const [asking, setAsking] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const keepRef = useRef<HTMLButtonElement>(null);
  const stopRef = useRef<HTMLButtonElement>(null);
  const was = useRef(false);
  useEffect(() => {
    if (asking) focusRing(keepRef.current);
    else if (was.current) focusRing(stopRef.current);
    was.current = asking;
  }, [asking]);
  useEffect(() => {
    if (!asking) return;
    const onKey = (event: KeyboardEvent) => {
      if (event.metaKey || event.ctrlKey || event.altKey) return;
      if (!dismissesLayer(event.key, event.target, event.keyCode) || pageCovered()) return;
      event.preventDefault();
      event.stopPropagation();
      setAsking(false);
    };
    window.addEventListener("keydown", onKey, true);
    return () => window.removeEventListener("keydown", onKey, true);
  }, [asking]);
  const start = new Date(rec.startedAt);
  const end = rec.endsAt ? new Date(rec.endsAt) : null;
  const name = rec.subtitle ? `${rec.title} · ${rec.subtitle}` : rec.title;
  async function stop() {
    setBusy(true);
    setError("");
    try {
      await stopRecording(rec.id);
      setAsking(false);
      onStop();
    } catch {
      setError(t("schedule.stopFailed"));
    } finally {
      setBusy(false);
    }
  }
  return (
    <li className="sched-row live">
      <span className="sched-when">
        <span className="live-dot">
          <span className="live-dot-light" aria-hidden="true" />
          {t("schedule.recordingNow")}
        </span>
      </span>
      <span className="sched-main">
        <span className="sched-title">{name}</span>
        <span className="sched-sub">
          {end ? t("schedule.recordingUntil", { number: rec.guideNumber, end: formatClock(end) }) : t("schedule.recordingSince", { number: rec.guideNumber, start: formatClock(start) })}
        </span>
      </span>
      {asking ? (
        <span className="sched-actions sched-ask" role="group" aria-label={t("schedule.stopAsk", { title: rec.title })}>
          <span className="sched-ask-text">
            {t("schedule.stopAsk", { title: rec.title })} <span className="codec">{t("schedule.stopKept")}</span>
          </span>
          <button ref={keepRef} type="button" className="btn small" disabled={busy} onClick={() => setAsking(false)}>
            {t("schedule.keepRecording")}
          </button>
          <button type="button" className="btn small danger" disabled={busy} aria-label={actionName(t("schedule.stopNow"), rec.title)} onClick={() => void stop()}>
            {t("schedule.stopNow")}
          </button>
          {error ? (
            <span className="hint error" role="alert">
              {error}
            </span>
          ) : null}
        </span>
      ) : (
        <span className="sched-actions">
          <button type="button" className="btn small" aria-label={actionName(t("schedule.watch"), rec.title)} onClick={() => navigate(`/play?recording=${rec.id}`)}>
            {t("schedule.watch")}
          </button>
          <button ref={stopRef} type="button" className="btn small ghost" aria-label={actionName(t("schedule.stop"), rec.title)} onClick={() => setAsking(true)}>
            {t("schedule.stop")}
          </button>
        </span>
      )}
    </li>
  );
}

function dayName(date: Date, now: Date) {
  const day = dayOf(date, now);
  if (day === "today") return t("schedule.today");
  if (day === "tomorrow") return t("schedule.tomorrow");
  if (day === "yesterday") return t("schedule.yesterday");
  return date.toLocaleDateString([], { weekday: "short", month: "short", day: "numeric" });
}

function formatDay(date: Date, now: Date) {
  return `${dayName(date, now)}, ${formatClock(date)}`;
}

function recordWindow(item: PlannedAiring) {
  return {
    start: new Date(new Date(item.airing.start).getTime() - (item.padBefore || 0) * 60_000),
    end: new Date(new Date(item.airing.end).getTime() + (item.padAfter || 0) * 60_000),
  };
}

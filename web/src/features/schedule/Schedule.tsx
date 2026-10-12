import { useEffect, useRef, useState } from "react";
import type { Pass, PlannedAiring, Recording } from "../../types";
import { fixSchedule, getEvents, getSchedule, stopRecording } from "../../api";
import { scheduleMessages } from "../../i18n/messages/schedule.ts";
import { bind } from "../../i18n/runtime.ts";
import { actionName } from "../../lib/actionName";
import { copy } from "../../strings";
import { formatClock } from "../../time";
import { Passes } from "./Passes";
import { takeScheduleRead, takeScheduleWrite } from "./plan";
import "./schedule.css";

const t = bind(scheduleMessages);

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
  const active = recordings.filter((rec) => rec.status === "recording");
  const [items, setItems] = useState<PlannedAiring[]>([]);
  const [tunerCount, setTunerCount] = useState(2);
  const [events, setEvents] = useState<{ id: number; at: string; message: string }[]>([]);
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
  return (
    <section className="page">
      <div className="page-head">
        <h2>{copy.schedule.title}</h2>
      </div>
      <h3 className="section-title">{t("schedule.comingUp")}</h3>
      {items.length === 0 ? (
        <p className="empty">{t("schedule.noMatch")}</p>
      ) : (
        <ul className="source-list">
          {items.map((item) => {
            return (
              <li key={`${item.passId}-${item.airing.id}`} className="source-row">
                <span className="ch-num">{formatDay(new Date(item.airing.start))}</span>
                <span>{item.airing.title}</span>
                <span className="codec">{item.skipped ? item.reason || t("schedule.lowerPriority", tunerCount) : t("schedule.willRecord", { start: formatClock(recordWindow(item).start), end: formatClock(recordWindow(item).end) })}</span>
                {item.suggestion ? (
                  <span className="schedule-suggest">
                    <span>
                      {item.suggestion.guideNumber
                        ? t("schedule.laterOnChannel", { when: formatDay(new Date(item.suggestion.start)), number: item.suggestion.guideNumber })
                        : t("schedule.laterOn", { when: formatDay(new Date(item.suggestion.start)) })}
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
      {note ? <p className="hint" role="alert">{note}</p> : null}
      {items.some((item) => item.conflict) ? (
        <p className="hint">{t("schedule.conflictHint")}</p>
      ) : null}
      <Passes passes={passes} onPasses={onPasses} />
      <h3 className="section-title">{t("schedule.activity")}</h3>
      {events.length === 0 ? (
        <p className="empty">{t("schedule.activityEmpty")}</p>
      ) : (
        <ul className="source-list">
          {events.map((event) => (
            <li key={event.id} className="source-row">
              <span className="ch-num">{formatClock(new Date(event.at))}</span>
              <span>{event.message}</span>
            </li>
          ))}
        </ul>
      )}
      {active.length === 0 ? (
        <article className="quiet-card wide">
          <p>{t("schedule.nothingRecording")}</p>
        </article>
      ) : (
        <ul className="source-list">
          {active.map((rec) => (
            <li key={rec.id} className="source-row">
              <span className="ch-num">{rec.guideNumber}</span>
              <span>{rec.title}</span>
              <button
                type="button"
                className="btn"
                aria-label={actionName(t("schedule.stop"), rec.title)}
                onClick={() => void stopRecording(rec.id).then(onStop)}
              >
                {t("schedule.stop")}
              </button>
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}

function formatDay(date: Date) {
  const months = ["Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"];
  return `${months[date.getMonth()]} ${date.getDate()}, ${formatClock(date)}`;
}

function recordWindow(item: PlannedAiring) {
  return {
    start: new Date(new Date(item.airing.start).getTime() - (item.padBefore || 0) * 60_000),
    end: new Date(new Date(item.airing.end).getTime() + (item.padAfter || 0) * 60_000),
  };
}

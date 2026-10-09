import { useEffect, useRef, useState } from "react";
import type { Pass, PlannedAiring, Recording } from "../../types";
import { fixSchedule, getEvents, getSchedule, stopRecording } from "../../api";
import { copy } from "../../strings";
import { formatClock } from "../../time";
import { Passes } from "./Passes";
import { takeScheduleRead, takeScheduleWrite } from "./plan";
import "./schedule.css";
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
      if (mine === writeN.current) setNote(err instanceof Error ? err.message : "That airing could not be scheduled.");
    } finally {
      if (mine === writeN.current) setFixing("");
    }
  }
  return (
    <section className="page">
      <div className="page-head">
        <h2>{copy.schedule.title}</h2>
      </div>
      <h3 className="section-title">Coming up</h3>
      {items.length === 0 ? (
        <p className="empty">No pass matches an airing in the guide.</p>
      ) : (
        <ul className="source-list">
          {items.map((item) => {
            return (
              <li key={`${item.passId}-${item.airing.id}`} className="source-row">
                <span className="ch-num">{formatDay(new Date(item.airing.start))}</span>
                <span>{item.airing.title}</span>
                <span className="codec">{item.skipped ? item.reason || (tunerCount === 1 ? "Lower priority · 1 tuner" : `Lower priority · ${tunerCount} tuners`) : `Will record ${formatClock(recordWindow(item).start)}–${formatClock(recordWindow(item).end)}`}</span>
                {item.suggestion ? (
                  <span className="schedule-suggest">
                    <span>
                      Later on {formatDay(new Date(item.suggestion.start))}
                      {item.suggestion.guideNumber ? ` on ${item.suggestion.guideNumber}` : ""}.
                    </span>
                    <button
                      type="button"
                      className="btn small schedule-fix"
                      disabled={fixing !== ""}
                      onClick={() => void recordLater(item)}
                    >
                      Record the later airing
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
        <p className="hint">A skipped show can move to a later airing when one fits. Otherwise move its pass up the list.</p>
      ) : null}
      <Passes passes={passes} onPasses={onPasses} />
      <h3 className="section-title">Activity</h3>
      {events.length === 0 ? (
        <p className="empty">Guide updates and recordings will be listed here.</p>
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
          <p>Nothing is recording right now. A pass above starts the next matching airing on its own.</p>
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
                onClick={() => void stopRecording(rec.id).then(onStop)}
              >
                Stop
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

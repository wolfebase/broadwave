import { useEffect, useState } from "react";
import type { Pass, PlannedAiring, Recording } from "../../types";
import { deletePass, getEvents, getSchedule, stopRecording, updatePass } from "../../api";
import { copy } from "../../strings";
import { formatClock } from "../../time";
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
  useEffect(() => {
    let cancel = false;
    void getSchedule()
      .then((res) => {
        if (!cancel) {
          setItems(res.items);
          setTunerCount(res.tunerCount);
        }
      })
      .catch(() => undefined);
    return () => {
      cancel = true;
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
  return (
    <section className="page">
      <div className="page-head">
        <h2>{copy.schedule.title}</h2>
      </div>
      <h3 className="section-title">Coming up</h3>
      {items.length === 0 ? (
        <p className="empty">No series pass matches an airing in the guide.</p>
      ) : (
        <ul className="source-list">
          {items.map((item) => (
            <li key={`${item.passId}-${item.airing.id}`} className="source-row">
              <span className="ch-num">{formatClock(new Date(item.airing.start))}</span>
              <span>{item.airing.title}</span>
              <span className="codec">{item.skipped ? item.reason || `Lower priority · ${tunerCount} tuners` : `Will record ${formatClock(recordWindow(item).start)}–${formatClock(recordWindow(item).end)}`}</span>
            </li>
          ))}
        </ul>
      )}
      {items.some((item) => item.skipped) ? (
        <p className="hint">More shows overlap than the HDHomeRun has tuners. The lower priority pass waits. Raise priority on the one you want.</p>
      ) : null}
      <h3 className="section-title">Series passes</h3>
      {passes.length === 0 ? <p className="empty">A series pass records the next airing of a title. Set one from the guide.</p> : (
        <ul className="source-list">
          {passes.map((pass) => (
            <li key={pass.id} className="source-row">
              <span>{pass.title}</span>
              <PadFields
                pass={pass}
                onSave={(patch) => void updatePass(pass.id, patch).then(onPasses)}
              />
              <button type="button" className="btn" onClick={() => void deletePass(pass.id).then(onPasses)}>
                Remove
              </button>
            </li>
          ))}
        </ul>
      )}
      <p className="hint">Early starts the tuner before the listing. After keeps it through the credits. A new pass uses 1 minute early and 2 minutes after. A higher priority number keeps the tuner when two passes overlap.</p>
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
          <p>Nothing is recording right now. A series pass above starts the next matching airing on its own.</p>
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

function recordWindow(item: PlannedAiring) {
  return {
    start: new Date(new Date(item.airing.start).getTime() - (item.padBefore || 0) * 60_000),
    end: new Date(new Date(item.airing.end).getTime() + (item.padAfter || 0) * 60_000),
  };
}

function PadFields({ pass, onSave }: { pass: Pass; onSave: (patch: Partial<Pass>) => void }) {
  const [before, setBefore] = useState(String(pass.padBefore ?? 0));
  const [after, setAfter] = useState(String(pass.padAfter ?? 0));
  const [priority, setPriority] = useState(String(pass.priority ?? 0));
  const [episodes, setEpisodes] = useState(pass.episodes || "all");
  const [keepMode, setKeepMode] = useState(pass.keepMode || "all");
  const [commercials, setCommercials] = useState(pass.commercials !== false);
  useEffect(() => {
    setBefore(String(pass.padBefore ?? 0));
    setAfter(String(pass.padAfter ?? 0));
    setPriority(String(pass.priority ?? 0));
    setEpisodes(pass.episodes || "all");
    setKeepMode(pass.keepMode || "all");
    setCommercials(pass.commercials !== false);
  }, [pass]);
  function commit(extra: Partial<Pass> = {}) {
    const padBefore = clampMinutes(before);
    const padAfter = clampMinutes(after);
    const rank = clampPriority(priority);
    setBefore(String(padBefore));
    setAfter(String(padAfter));
    setPriority(String(rank));
    onSave({ padBefore, padAfter, priority: rank, episodes, keepMode, commercials, ...extra });
  }
  return (
    <span className="pad">
      <label>
        Early
        <input
          type="number"
          min={0}
          max={30}
          aria-label={`${pass.title} minutes early`}
          value={before}
          onChange={(event) => setBefore(event.target.value)}
          onBlur={() => commit()}
        />
      </label>
      <label>
        After
        <input
          type="number"
          min={0}
          max={30}
          aria-label={`${pass.title} minutes after`}
          value={after}
          onChange={(event) => setAfter(event.target.value)}
          onBlur={() => commit()}
        />
      </label>
      <label>
        Priority
        <input
          type="number"
          min={0}
          max={100}
          aria-label={`${pass.title} priority`}
          value={priority}
          onChange={(event) => setPriority(event.target.value)}
          onBlur={() => commit()}
        />
      </label>
      <label>
        Episodes
        <select aria-label={`${pass.title} episodes`} value={episodes} onChange={(event) => { setEpisodes(event.target.value); commit({ episodes: event.target.value }); }}>
          <option value="all">All</option>
          <option value="new">New only</option>
        </select>
      </label>
      <label>
        Keep
        <select aria-label={`${pass.title} keep`} value={keepMode} onChange={(event) => { setKeepMode(event.target.value); commit({ keepMode: event.target.value }); }}>
          <option value="all">All</option>
          <option value="unwatched">Unwatched</option>
          <option value="last">Last few</option>
        </select>
      </label>
      <label>
        Commercials
        <input type="checkbox" aria-label={`${pass.title} commercials`} checked={commercials} onChange={(event) => { setCommercials(event.target.checked); commit({ commercials: event.target.checked }); }} />
      </label>
    </span>
  );
}

function clampPriority(value: string) {
  const n = Math.round(Number(value));
  if (!Number.isFinite(n) || n < 0) return 0;
  if (n > 100) return 100;
  return n;
}

function clampMinutes(value: string) {
  const n = Math.round(Number(value));
  if (!Number.isFinite(n) || n < 0) return 0;
  if (n > 30) return 30;
  return n;
}

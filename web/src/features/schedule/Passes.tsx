import { useEffect, useRef, useState } from "react";
import type { Channel, NewPass, Pass, PassPreview } from "../../types";
import { addSeriesPass, deletePass, orderPasses, previewPass, updatePass } from "../../api";
import { useData } from "../../app/data";
import { focusRing } from "../../app/remote";
import { formatClock } from "../../time";

const dayNames = ["Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"];

type Draft = Required<Omit<NewPass, "airingStart" | "priority">>;
type Body = NewPass & { rename?: string };

function draftOf(pass?: Pass): Draft {
  return {
    title: pass?.title ?? "",
    matchKind: pass?.matchKind === "contains" || pass?.matchKind === "category" ? pass.matchKind : "title",
    channelId: pass?.channelId ?? 0,
    padBefore: pass?.padBefore ?? 1,
    padAfter: pass?.padAfter ?? 2,
    episodes: pass?.episodes || "all",
    keepMode: pass?.keepMode || "all",
    // The server keeps one when "the newest" has no count.
    keepCount: pass ? pass.keepCount || 1 : 5,
    limitCount: pass?.limitCount ?? 0,
    rerecord: pass?.rerecord ?? false,
    commercials: pass?.commercials !== false,
    timeStart: pass?.timeStart ?? "",
    timeEnd: pass?.timeEnd ?? "",
    days: pass?.days ?? [],
  };
}

/** What the server takes for this kind of pass: a once pass keeps its airing, a team pass its team. A saved series pass changes title by rename. */
function payload(draft: Draft, pass?: Pass): Body {
  if (pass?.kind === "once") {
    return { title: pass.title, padBefore: draft.padBefore, padAfter: draft.padAfter, commercials: draft.commercials };
  }
  const out: Body = { ...draft, title: draft.title.trim(), keepCount: draft.keepMode === "last" ? draft.keepCount : 0 };
  if (pass?.kind === "team") {
    out.title = pass.title;
    delete out.matchKind;
  } else if (pass) {
    out.rename = out.title;
  }
  return out;
}

export function Passes({ passes, onPasses }: { passes: Pass[]; onPasses: () => void }) {
  const { channels } = useData();
  // A drag reorders here first; a new list from the server replaces it.
  const [local, setLocal] = useState<{ from: Pass[]; ids: number[] } | null>(null);
  const order = local && local.from === passes ? local.ids : passes.map((p) => p.id);
  const setOrder = (ids: number[]) => setLocal({ from: passes, ids });
  const [editing, setEditing] = useState<number | "new" | null>(null);
  const [dragId, setDragId] = useState<number | null>(null);
  const [note, setNote] = useState("");
  const dragFrom = useRef<number[]>([]);
  const dropped = useRef(false);
  const queue = useRef<Promise<void>>(Promise.resolve());
  const focusAfter = useRef<{ id: number; up: boolean } | null>(null);
  const [removing, setRemoving] = useState<number | null>(null);
  const [removeError, setRemoveError] = useState("");
  const [removeBusy, setRemoveBusy] = useState(false);
  const cancelRef = useRef<HTMLButtonElement>(null);
  const restoreRemove = useRef<number | null>(null);
  // A moved row keeps focus on its button, or on the other one at an end of the list.
  useEffect(() => {
    const want = focusAfter.current;
    if (!want) return;
    focusAfter.current = null;
    const pick = (up: boolean) => document.querySelector<HTMLButtonElement>(`[data-pass="${want.id}"][data-move="${up ? "up" : "down"}"]`);
    const button = pick(want.up);
    (button && !button.disabled ? button : pick(!want.up))?.focus();
  });
  // The ask puts focus on Cancel. Leaving it returns to that row's Remove.
  useEffect(() => {
    if (removing != null) {
      focusRing(cancelRef.current);
      return;
    }
    const id = restoreRemove.current;
    if (id == null) return;
    restoreRemove.current = null;
    focusRing(document.querySelector<HTMLButtonElement>(`[data-pass-remove="${id}"]`));
  }, [removing]);
  // Escape and Backspace cancel the ask. They would otherwise leave the page.
  useEffect(() => {
    if (removing == null) return;
    const onKey = (event: KeyboardEvent) => {
      if (event.key !== "Escape" && event.key !== "Backspace") return;
      if (event.metaKey || event.ctrlKey || event.altKey) return;
      const target = event.target;
      if (target instanceof HTMLElement && (target.tagName === "INPUT" || target.tagName === "TEXTAREA" || target.isContentEditable)) return;
      event.preventDefault();
      restoreRemove.current = removing;
      setRemoving(null);
      setRemoveError("");
    };
    window.addEventListener("keydown", onKey, true);
    return () => window.removeEventListener("keydown", onKey, true);
  }, [removing]);

  function askRemove(id: number) {
    setEditing(null);
    setRemoveError("");
    setRemoving(id);
  }

  function cancelRemove() {
    if (removing != null) restoreRemove.current = removing;
    setRemoving(null);
    setRemoveError("");
  }

  async function confirmRemove(id: number) {
    setRemoveBusy(true);
    setRemoveError("");
    try {
      await deletePass(id);
      restoreRemove.current = null;
      setRemoving(null);
      onPasses();
    } catch (err) {
      setRemoveError(err instanceof Error ? err.message : "The pass could not be removed.");
    } finally {
      setRemoveBusy(false);
    }
  }
  const byId = new Map(passes.map((p) => [p.id, p]));
  const rows = order.map((id) => byId.get(id)).filter((p): p is Pass => Boolean(p));

  // Orders are sent one at a time, so quick presses land in the order made.
  function saveOrder(ids: number[]) {
    setNote("");
    queue.current = queue.current
      .then(() => orderPasses(ids))
      .then(
        () => undefined,
        (err: unknown) => setNote(err instanceof Error ? err.message : "The order could not be saved."),
      );
    void queue.current.then(onPasses);
  }

  function move(id: number, by: number) {
    const at = order.indexOf(id);
    const to = at + by;
    if (at < 0 || to < 0 || to >= order.length) return;
    const next = [...order];
    next.splice(at, 1);
    next.splice(to, 0, id);
    focusAfter.current = { id, up: by < 0 };
    setOrder(next);
    saveOrder(next);
  }

  function dragOver(overId: number) {
    if (dragId === null || dragId === overId) return;
    const next = order.filter((id) => id !== dragId);
    const at = next.indexOf(overId);
    const from = order.indexOf(dragId);
    // Dropping on a row below puts the pass under it; above, over it.
    next.splice(from <= order.indexOf(overId) ? at + 1 : at, 0, dragId);
    setOrder(next);
  }

  function dragEnd() {
    setDragId(null);
    if (!dropped.current) {
      // Let go outside the list, or Escape: put it back.
      setLocal(null);
      return;
    }
    if (dragFrom.current.join(",") !== order.join(",")) saveOrder(order);
  }

  return (
    <>
      <div className="passes-head">
        <h3 className="section-title">Passes</h3>
        {editing === "new" ? null : (
          <button type="button" className="btn small" onClick={() => setEditing("new")}>
            New pass
          </button>
        )}
      </div>
      {editing === "new" ? (
        <PassEditor
          channels={channels}
          onDone={(saved) => {
            setEditing(null);
            if (saved) onPasses();
          }}
        />
      ) : null}
      {rows.length === 0 && editing !== "new" ? (
        <p className="empty">A pass records every airing that matches: a title, words in a title, or a category. Set one here or from the guide.</p>
      ) : (
        <ol className="source-list pass-list">
          {rows.map((pass, index) => (
            <li
              key={pass.id}
              className={`source-row pass-row${dragId === pass.id ? " dragging" : ""}`}
              draggable={editing === null && removing !== pass.id}
              onDragStart={(event) => {
                event.dataTransfer.effectAllowed = "move";
                event.dataTransfer.setData("text/plain", String(pass.id));
                dragFrom.current = order;
                dropped.current = false;
                setDragId(pass.id);
              }}
              onDragOver={(event) => {
                if (dragId === null) return;
                event.preventDefault();
                dragOver(pass.id);
              }}
              onDrop={(event) => {
                event.preventDefault();
                dropped.current = true;
              }}
              onDragEnd={dragEnd}
            >
              <span className="pass-rank" aria-hidden="true">{index + 1}</span>
              <span className="pass-text">
                <span className="pass-title">{passLabel(pass)}</span>
                <span className="codec">{passDetails(pass, channels)}</span>
              </span>
              {removing === pass.id ? (
                <span className="pad pass-actions">
                  <span>Remove {passLabel(pass)}?</span>
                  <span className="codec">Recordings it made stay.</span>
                  <button type="button" className="btn small" disabled={removeBusy} onClick={() => void confirmRemove(pass.id)}>
                    Remove
                  </button>
                  <button ref={cancelRef} type="button" className="btn small" disabled={removeBusy} onClick={cancelRemove}>
                    Cancel
                  </button>
                  {removeError ? (
                    <span className="hint error" role="alert">
                      {removeError}
                    </span>
                  ) : null}
                </span>
              ) : (
                <span className="pad pass-actions">
                  <button type="button" className="btn small" data-pass={pass.id} data-move="up" aria-label={`Move ${pass.title} up`} disabled={index === 0} onClick={() => move(pass.id, -1)}>
                    Up
                  </button>
                  <button type="button" className="btn small" data-pass={pass.id} data-move="down" aria-label={`Move ${pass.title} down`} disabled={index === rows.length - 1} onClick={() => move(pass.id, 1)}>
                    Down
                  </button>
                  <button type="button" className="btn small" aria-label={`${editing === pass.id ? "Close" : "Edit"} ${pass.title}`} aria-expanded={editing === pass.id} onClick={() => setEditing(editing === pass.id ? null : pass.id)}>
                    {editing === pass.id ? "Close" : "Edit"}
                  </button>
                  <button type="button" className="btn small" data-pass-remove={pass.id} aria-label={`Remove ${pass.title}`} onClick={() => askRemove(pass.id)}>
                    Remove
                  </button>
                </span>
              )}
              {editing === pass.id ? (
                <PassEditor
                  key={pass.id}
                  pass={pass}
                  channels={channels}
                  onDone={(saved) => {
                    setEditing(null);
                    if (saved) onPasses();
                  }}
                />
              ) : null}
            </li>
          ))}
        </ol>
      )}
      {note ? <p className="hint" role="alert">{note}</p> : null}
      <p className="hint">When two passes need the same tuner, the one higher in the list records. Drag a pass or use Up and Down to change the order.</p>
    </>
  );
}

function PassEditor({ pass, channels, onDone }: { pass?: Pass; channels: Channel[]; onDone: (saved: boolean) => void }) {
  const [draft, setDraft] = useState<Draft>(() => draftOf(pass));
  const [preview, setPreview] = useState<PassPreview | null>(null);
  const [previewNote, setPreviewNote] = useState("");
  const [error, setError] = useState("");
  const [saving, setSaving] = useState(false);
  const once = pass?.kind === "once";
  const team = pass?.kind === "team";
  const body = payload(draft, pass);
  const key = JSON.stringify(body);
  useEffect(() => {
    if (!body.title) return;
    const abort = new AbortController();
    const timer = window.setTimeout(() => {
      previewPass({ ...body, id: pass?.id }, abort.signal)
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
  }, [key, pass?.id]);

  function set<K extends keyof Draft>(field: K, value: Draft[K]) {
    setDraft((d) => ({ ...d, [field]: value }));
  }

  async function save() {
    if (!draft.title.trim() && !once && !team) {
      setError("Name a title, words, or a category.");
      return;
    }
    setSaving(true);
    setError("");
    try {
      if (pass) await updatePass(pass.id, body);
      else await addSeriesPass(body);
      onDone(true);
    } catch (err) {
      setError(err instanceof Error ? err.message : "The pass could not be saved.");
    } finally {
      setSaving(false);
    }
  }

  const label = pass ? `${pass.title}` : "New pass";
  const shown = channels.filter((ch) => !ch.hidden || ch.id === draft.channelId);
  return (
    <form
      className="pass-editor"
      aria-label={pass ? `Edit ${label}` : label}
      onSubmit={(event) => {
        event.preventDefault();
        void save();
      }}
    >
      {once || team ? null : (
        <div className="pad">
          <label>
            Match
            <select value={draft.matchKind} onChange={(e) => set("matchKind", e.target.value as Draft["matchKind"])}>
              <option value="title">Title is</option>
              <option value="contains">Title contains</option>
              <option value="category">Category</option>
            </select>
          </label>
          <label className="pass-name">
            {draft.matchKind === "category" ? "Category" : draft.matchKind === "contains" ? "Words" : "Title"}
            <input
              type="text"
              placeholder={draft.matchKind === "category" ? "Sports" : draft.matchKind === "contains" ? "Chiefs" : "Jeopardy!"}
              value={draft.title}
              autoFocus={!pass}
              onChange={(e) => set("title", e.target.value)}
            />
          </label>
        </div>
      )}
      {once ? null : (
        <>
          <div className="pad">
            <label>
              Channel
              <select value={draft.channelId} onChange={(e) => set("channelId", Number(e.target.value))}>
                <option value={0}>Any channel</option>
                {shown.map((ch) => (
                  <option key={ch.id} value={ch.id}>
                    {ch.displayNumber} {ch.displayName}
                  </option>
                ))}
              </select>
            </label>
            {team ? null : (
              <label>
                Episodes
                <select value={draft.episodes} onChange={(e) => set("episodes", e.target.value as Draft["episodes"])}>
                  <option value="all">All</option>
                  <option value="new">New only</option>
                </select>
              </label>
            )}
          </div>
          <fieldset className="pad pass-days">
            <legend>Days</legend>
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
            <span className="codec">{draft.days.length === 0 ? "Every day" : ""}</span>
          </fieldset>
          <div className="pad">
            <label>
              From
              <input type="time" value={draft.timeStart} onChange={(e) => set("timeStart", e.target.value)} />
            </label>
            <label>
              Until
              <input type="time" value={draft.timeEnd} onChange={(e) => set("timeEnd", e.target.value)} />
            </label>
            {draft.timeStart || draft.timeEnd ? (
              <button type="button" className="btn small" onClick={() => setDraft((d) => ({ ...d, timeStart: "", timeEnd: "" }))}>
                Any time
              </button>
            ) : (
              <span className="codec">Any time</span>
            )}
          </div>
          <div className="pad">
            <label>
              Keep
              <select value={draft.keepMode} onChange={(e) => set("keepMode", e.target.value as Draft["keepMode"])}>
                <option value="all">All</option>
                <option value="unwatched">Unwatched</option>
                <option value="last">The newest</option>
              </select>
            </label>
            {draft.keepMode === "last" ? (
              <label>
                How many
                <input type="number" min={1} max={99} value={draft.keepCount} onChange={(e) => set("keepCount", clamp(e.target.value, 1, 99))} />
              </label>
            ) : null}
            <label>
              Stop at
              <input type="number" min={0} max={99} aria-describedby={`stop-${pass?.id ?? "new"}`} value={draft.limitCount} onChange={(e) => set("limitCount", clamp(e.target.value, 0, 99))} />
            </label>
            <span className="codec" id={`stop-${pass?.id ?? "new"}`}>unwatched{draft.limitCount === 0 ? " (no limit)" : ""}</span>
            {team ? null : (
              <label>
                <input type="checkbox" checked={draft.rerecord} onChange={(e) => set("rerecord", e.target.checked)} />
                Record again after a delete
              </label>
            )}
          </div>
        </>
      )}
      <div className="pad">
        <label>
          Early
          <input type="number" min={0} max={30} value={draft.padBefore} onChange={(e) => set("padBefore", clamp(e.target.value, 0, 30))} />
        </label>
        <label>
          After
          <input type="number" min={0} max={30} value={draft.padAfter} onChange={(e) => set("padAfter", clamp(e.target.value, 0, 30))} />
        </label>
        <label>
          <input type="checkbox" checked={draft.commercials} onChange={(e) => set("commercials", e.target.checked)} />
          Find commercials
        </label>
      </div>
      {body.title ? <PreviewLines preview={preview} note={previewNote} channels={channels} /> : null}
      {preview && !once && preview.utcOffset !== -new Date().getTimezoneOffset() * 60 ? (
        <p className="hint">Days and times are on the server's clock ({preview.timeZone}).</p>
      ) : null}
      {error ? <p className="hint error" role="alert">{error}</p> : null}
      <div className="pad">
        <button type="submit" className="btn primary small" disabled={saving}>
          {pass ? "Save" : "Add pass"}
        </button>
        <button type="button" className="btn small" onClick={() => onDone(false)}>
          Cancel
        </button>
      </div>
    </form>
  );
}

function PreviewLines({ preview, note, channels }: { preview: PassPreview | null; note: string; channels: Channel[] }) {
  if (note) return <p className="hint" role="status">{note}</p>;
  if (!preview) return null;
  const records = preview.items.filter((item) => !item.skipped);
  const number = (id: number) => channels.find((ch) => ch.id === id)?.displayNumber ?? "";
  let head = "Nothing in the guide matches in the next 2 weeks.";
  if (preview.items.length > 0) {
    if (records.length === preview.items.length) head = `Records ${count(records.length, "airing")} in the next 2 weeks.`;
    else if (records.length === 0) head = `Matches ${count(preview.items.length, "airing")} in the next 2 weeks, but none will record.`;
    else head = `Records ${records.length} of ${count(preview.items.length, "airing")} in the next 2 weeks.`;
  }
  return (
    <div className="pass-preview" role="status" aria-live="polite">
      <p>{head}</p>
      {preview.items.length > 0 ? (
        <ul className="marker-list">
          {preview.items.slice(0, 5).map((item) => (
            <li key={`${item.airing.channelId}-${item.airing.start}`} className="codec">
              {formatDay(item.airing.start)} · {number(item.airing.channelId)} {item.airing.title}
              {item.skipped ? ` · ${item.conflict ? "Skipped: another pass has the tuner" : item.reason || "Skipped"}` : ""}
            </li>
          ))}
          {preview.items.length > 5 ? <li className="codec">and {preview.items.length - 5} more</li> : null}
        </ul>
      ) : null}
      {preview.bumps.length > 0 ? (
        <>
          <p>This pass would stop these from recording:</p>
          <ul className="marker-list">
            {preview.bumps.map((item) => (
              <li key={`${item.passId}-${item.airing.channelId}-${item.airing.start}`} className="codec">
                {formatDay(item.airing.start)} · {number(item.airing.channelId)} {item.airing.title}
              </li>
            ))}
          </ul>
        </>
      ) : null}
    </div>
  );
}

export function passLabel(pass: Pass) {
  switch (pass.kind === "once" || pass.kind === "team" ? pass.kind : pass.matchKind) {
    case "once":
      return pass.airingStart ? `${pass.title} · ${formatDay(pass.airingStart)} only` : pass.title;
    case "team":
      return `${pass.title} games`;
    case "contains":
      return `Titles with “${pass.title}”`;
    case "category":
      return `${pass.title} (category)`;
    default:
      return pass.title;
  }
}

export function passDetails(pass: Pass, channels: Channel[]) {
  const parts: string[] = [];
  const ch = pass.channelId ? channels.find((c) => c.id === pass.channelId) : undefined;
  if (pass.kind !== "once") parts.push(ch ? `${ch.displayNumber} ${ch.displayName}` : "Any channel");
  if (pass.days?.length) parts.push(daysLabel(pass.days));
  if (pass.timeStart && pass.timeEnd) parts.push(`${clockLabel(pass.timeStart)}–${clockLabel(pass.timeEnd)}`);
  if (pass.episodes === "new") parts.push("New only");
  if (pass.keepMode === "unwatched") parts.push("Keeps unwatched");
  if (pass.keepMode === "last") parts.push(`Keeps the newest ${pass.keepCount || 1}`);
  if (pass.limitCount) parts.push(`Stops at ${pass.limitCount} unwatched`);
  parts.push(`${pass.padBefore ?? 0} min early, ${pass.padAfter ?? 0} after`);
  return parts.join(" · ");
}

export function daysLabel(days: number[]) {
  const set = [...days].sort().join("");
  if (set === "12345") return "Weekdays";
  if (set === "06") return "Weekends";
  if (set === "0123456") return "Every day";
  return [...days].sort().map((d) => dayNames[d]).join(", ");
}

function clockLabel(hhmm: string) {
  const [h, m] = hhmm.split(":").map(Number);
  const d = new Date();
  d.setHours(h, m, 0, 0);
  return formatClock(d);
}

function formatDay(iso: string) {
  const date = new Date(iso);
  return `${dayNames[date.getDay()]} ${date.getMonth() + 1}/${date.getDate()}, ${formatClock(date)}`;
}

function count(n: number, word: string) {
  return `${n} ${word}${n === 1 ? "" : "s"}`;
}

function clamp(value: string, min: number, max: number) {
  const n = Math.round(Number(value));
  if (!Number.isFinite(n)) return min;
  return Math.min(max, Math.max(min, n));
}

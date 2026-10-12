import { useEffect, useRef, useState } from "react";
import type { Channel, NewPass, Pass, PassPreview } from "../../types";
import { addSeriesPass, deletePass, orderPasses, previewPass, updatePass } from "../../api";
import { useData } from "../../app/data";
import { focusRing } from "../../app/remote";
import { scheduleMessages } from "../../i18n/messages/schedule.ts";
import { bind, weekdayShortNames } from "../../i18n/runtime.ts";
import { dismissesLayer, pageCovered } from "../../lib/dialogFocus";
import { copy } from "../../strings";
import { formatClock } from "../../time";
import { reloadAfterOrder } from "./plan";

const t = bind(scheduleMessages);

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
  const orderN = useRef(0);
  const focusAfter = useRef<{ id: number; up: boolean } | null>(null);
  const [removing, setRemoving] = useState<number | null>(null);
  const [removeError, setRemoveError] = useState("");
  const [removeBusy, setRemoveBusy] = useState(false);
  const cancelRef = useRef<HTMLButtonElement>(null);
  const restoreRemove = useRef<number | null>(null);
  const editingWas = useRef<number | "new" | null>(null);
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
      if (event.metaKey || event.ctrlKey || event.altKey) return;
      if (!dismissesLayer(event.key, event.target, event.keyCode)) return;
      event.preventDefault();
      restoreRemove.current = removing;
      setRemoving(null);
      setRemoveError("");
    };
    window.addEventListener("keydown", onKey, true);
    return () => window.removeEventListener("keydown", onKey, true);
  }, [removing]);
  // Closing the editor puts focus back on New pass or that pass's Edit.
  useEffect(() => {
    const previous = editingWas.current;
    editingWas.current = editing;
    if (editing != null || previous == null) return;
    const el =
      previous === "new"
        ? document.querySelector<HTMLButtonElement>("[data-pass-new]")
        : document.querySelector<HTMLButtonElement>(`[data-pass-edit="${previous}"]`);
    focusRing(el);
  }, [editing]);
  // Escape closes the editor. A title field would otherwise only blur.
  useEffect(() => {
    if (editing == null) return;
    const onKey = (event: KeyboardEvent) => {
      if (event.metaKey || event.ctrlKey || event.altKey) return;
      if (!dismissesLayer(event.key, event.target, event.keyCode) || pageCovered()) return;
      event.preventDefault();
      event.stopPropagation();
      setEditing(null);
    };
    window.addEventListener("keydown", onKey, true);
    return () => window.removeEventListener("keydown", onKey, true);
  }, [editing]);

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
      setRemoveError(err instanceof Error ? err.message : t("schedule.removeFailed"));
    } finally {
      setRemoveBusy(false);
    }
  }
  const byId = new Map(passes.map((p) => [p.id, p]));
  const rows = order.map((id) => byId.get(id)).filter((p): p is Pass => Boolean(p));

  // Orders are sent one at a time, so quick presses land in the order made.
  // The list is read back only after the last one, or the earlier read snaps the rows backward.
  function saveOrder(ids: number[]) {
    const mine = ++orderN.current;
    setNote("");
    queue.current = queue.current
      .then(() => orderPasses(ids))
      .then(
        () => undefined,
        (err: unknown) => setNote(err instanceof Error ? err.message : t("schedule.orderFailed")),
      );
    void queue.current.then(() => {
      if (reloadAfterOrder(mine, orderN.current)) onPasses();
    });
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
        <h3 className="section-title">{t("schedule.passes")}</h3>
        {editing === "new" ? null : (
          <button type="button" className="btn small" data-pass-new onClick={() => setEditing("new")}>
            {t("schedule.newPass")}
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
        <p className="empty">{t("schedule.passesEmpty")}</p>
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
                  <span>{t("schedule.removeAsk", { name: passLabel(pass) })}</span>
                  <span className="codec">{t("schedule.recordingsStay")}</span>
                  <button type="button" className="btn small" disabled={removeBusy} aria-label={t("schedule.removeNamed", { name: passLabel(pass) })} onClick={() => void confirmRemove(pass.id)}>
                    {t("schedule.remove")}
                  </button>
                  <button ref={cancelRef} type="button" className="btn small" disabled={removeBusy} onClick={cancelRemove}>
                    {t("schedule.cancel")}
                  </button>
                  {removeError ? (
                    <span className="hint error" role="alert">
                      {removeError}
                    </span>
                  ) : null}
                </span>
              ) : (
                <span className="pad pass-actions">
                  <button type="button" className="btn small" data-pass={pass.id} data-move="up" aria-label={t("schedule.moveUp", { title: pass.title })} disabled={index === 0} onClick={() => move(pass.id, -1)}>
                    {t("schedule.up")}
                  </button>
                  <button type="button" className="btn small" data-pass={pass.id} data-move="down" aria-label={t("schedule.moveDown", { title: pass.title })} disabled={index === rows.length - 1} onClick={() => move(pass.id, 1)}>
                    {t("schedule.down")}
                  </button>
                  <button type="button" className="btn small" data-pass-edit={pass.id} aria-label={t(editing === pass.id ? "schedule.closeNamed" : "schedule.editNamed", { title: pass.title })} aria-expanded={editing === pass.id} onClick={() => setEditing(editing === pass.id ? null : pass.id)}>
                    {editing === pass.id ? t("schedule.close") : t("schedule.edit")}
                  </button>
                  <button type="button" className="btn small" data-pass-remove={pass.id} aria-label={t("schedule.removeNamed", { name: pass.title })} onClick={() => askRemove(pass.id)}>
                    {t("schedule.remove")}
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
      <p className="hint">{t("schedule.priorityHint")}</p>
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
      setError(t("schedule.nameNeeded"));
      return;
    }
    setSaving(true);
    setError("");
    try {
      if (pass) await updatePass(pass.id, body);
      else await addSeriesPass(body);
      onDone(true);
    } catch (err) {
      setError(err instanceof Error ? err.message : t("schedule.saveFailed"));
    } finally {
      setSaving(false);
    }
  }

  const dayNames = weekdayShortNames();
  const label = pass ? pass.title : t("schedule.newPass");
  const shown = channels.filter((ch) => !ch.hidden || ch.id === draft.channelId);
  return (
    <form
      className="pass-editor"
      aria-label={pass ? t("schedule.editNamed", { title: label }) : label}
      onSubmit={(event) => {
        event.preventDefault();
        void save();
      }}
    >
      {once || team ? null : (
        <div className="pad">
          <label>
            {t("schedule.match")}
            <select value={draft.matchKind} onChange={(e) => set("matchKind", e.target.value as Draft["matchKind"])}>
              <option value="title">{t("schedule.titleIs")}</option>
              <option value="contains">{t("schedule.titleContains")}</option>
              <option value="category">{t("schedule.category")}</option>
            </select>
          </label>
          <label className="pass-name">
            {draft.matchKind === "category" ? t("schedule.category") : draft.matchKind === "contains" ? t("schedule.words") : t("schedule.titleField")}
            <input
              type="text"
              placeholder={draft.matchKind === "category" ? t("schedule.placeholderCategory") : draft.matchKind === "contains" ? t("schedule.placeholderWords") : t("schedule.placeholderTitle")}
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
              {t("schedule.channel")}
              <select value={draft.channelId} onChange={(e) => set("channelId", Number(e.target.value))}>
                <option value={0}>{t("schedule.anyChannel")}</option>
                {shown.map((ch) => (
                  <option key={ch.id} value={ch.id}>
                    {ch.displayNumber} {ch.displayName}
                  </option>
                ))}
              </select>
            </label>
            {team ? null : (
              <label>
                {t("schedule.episodes")}
                <select value={draft.episodes} onChange={(e) => set("episodes", e.target.value as Draft["episodes"])}>
                  <option value="all">{t("schedule.all")}</option>
                  <option value="new">{t("schedule.newOnly")}</option>
                </select>
              </label>
            )}
          </div>
          <fieldset className="pad pass-days">
            <legend>{t("schedule.days")}</legend>
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
            <span className="codec">{draft.days.length === 0 ? t("schedule.everyDay") : ""}</span>
          </fieldset>
          <div className="pad">
            <label>
              {t("schedule.from")}
              <input type="time" value={draft.timeStart} onChange={(e) => set("timeStart", e.target.value)} />
            </label>
            <label>
              {t("schedule.until")}
              <input type="time" value={draft.timeEnd} onChange={(e) => set("timeEnd", e.target.value)} />
            </label>
            {draft.timeStart || draft.timeEnd ? (
              <button type="button" className="btn small" onClick={() => setDraft((d) => ({ ...d, timeStart: "", timeEnd: "" }))}>
                {t("schedule.anyTime")}
              </button>
            ) : (
              <span className="codec">{t("schedule.anyTime")}</span>
            )}
          </div>
          <div className="pad">
            <label>
              {t("schedule.keep")}
              <select value={draft.keepMode} onChange={(e) => set("keepMode", e.target.value as Draft["keepMode"])}>
                <option value="all">{t("schedule.all")}</option>
                <option value="unwatched">{t("schedule.unwatched")}</option>
                <option value="last">{t("schedule.theNewest")}</option>
              </select>
            </label>
            {draft.keepMode === "last" ? (
              <label>
                {t("schedule.howMany")}
                <input type="number" min={1} max={99} value={draft.keepCount} onChange={(e) => set("keepCount", clamp(e.target.value, 1, 99))} />
              </label>
            ) : null}
            <label>
              {t("schedule.stopAt")}
              <input type="number" min={0} max={99} aria-describedby={`stop-${pass?.id ?? "new"}`} value={draft.limitCount} onChange={(e) => set("limitCount", clamp(e.target.value, 0, 99))} />
            </label>
            <span className="codec" id={`stop-${pass?.id ?? "new"}`}>{t("schedule.unwatchedLimit")}{draft.limitCount === 0 ? t("schedule.noLimit") : ""}</span>
            {team ? null : (
              <label>
                <input type="checkbox" checked={draft.rerecord} onChange={(e) => set("rerecord", e.target.checked)} />
                {t("schedule.recordAgainAfterDelete")}
              </label>
            )}
          </div>
        </>
      )}
      <div className="pad">
        <label>
          {t("schedule.early")}
          <input type="number" min={0} max={30} value={draft.padBefore} onChange={(e) => set("padBefore", clamp(e.target.value, 0, 30))} />
        </label>
        <label>
          {t("schedule.after")}
          <input type="number" min={0} max={30} value={draft.padAfter} onChange={(e) => set("padAfter", clamp(e.target.value, 0, 30))} />
        </label>
        <label>
          <input type="checkbox" checked={draft.commercials} onChange={(e) => set("commercials", e.target.checked)} />
          {copy.library.findCommercials}
        </label>
      </div>
      {body.title ? <PreviewLines preview={preview} note={previewNote} channels={channels} /> : null}
      {preview && !once && preview.utcOffset !== -new Date().getTimezoneOffset() * 60 ? (
        <p className="hint">{t("schedule.serverClock", { zone: preview.timeZone })}</p>
      ) : null}
      {error ? <p className="hint error" role="alert">{error}</p> : null}
      <div className="pad">
        <button type="submit" className="btn primary small" disabled={saving}>
          {pass ? t("schedule.save") : t("schedule.addPass")}
        </button>
        <button type="button" className="btn small" onClick={() => onDone(false)}>
          {t("schedule.cancel")}
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
  let head = t("schedule.previewNone");
  if (preview.items.length > 0) {
    if (records.length === preview.items.length) head = t("schedule.previewRecords", records.length);
    else if (records.length === 0) head = t("schedule.previewNoneRecord", preview.items.length);
    else head = t("schedule.previewSome", preview.items.length, { done: records.length });
  }
  return (
    <div className="pass-preview" role="status" aria-live="polite">
      <p>{head}</p>
      {preview.items.length > 0 ? (
        <ul className="marker-list">
          {preview.items.slice(0, 5).map((item) => (
            <li key={`${item.airing.channelId}-${item.airing.start}`} className="codec">
              {formatDay(item.airing.start)} · {number(item.airing.channelId)} {item.airing.title}
              {item.skipped ? ` · ${item.conflict ? t("schedule.skippedTuner") : item.reason || t("schedule.skipped")}` : ""}
            </li>
          ))}
          {preview.items.length > 5 ? <li className="codec">{t("schedule.andMore", { count: preview.items.length - 5 })}</li> : null}
        </ul>
      ) : null}
      {preview.bumps.length > 0 ? (
        <>
          <p>{t("schedule.previewBump")}</p>
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
      return pass.airingStart ? t("schedule.onceLabel", { title: pass.title, when: formatDay(pass.airingStart) }) : pass.title;
    case "team":
      return t("schedule.teamLabel", { title: pass.title });
    case "contains":
      return t("schedule.containsLabel", { title: pass.title });
    case "category":
      return t("schedule.categoryLabel", { title: pass.title });
    default:
      return pass.title;
  }
}

export function passDetails(pass: Pass, channels: Channel[]) {
  const parts: string[] = [];
  const ch = pass.channelId ? channels.find((c) => c.id === pass.channelId) : undefined;
  if (pass.kind !== "once") parts.push(ch ? `${ch.displayNumber} ${ch.displayName}` : t("schedule.anyChannel"));
  if (pass.days?.length) parts.push(daysLabel(pass.days));
  if (pass.timeStart && pass.timeEnd) parts.push(`${clockLabel(pass.timeStart)}–${clockLabel(pass.timeEnd)}`);
  if (pass.episodes === "new") parts.push(t("schedule.newOnly"));
  if (pass.keepMode === "unwatched") parts.push(t("schedule.keepsUnwatched"));
  if (pass.keepMode === "last") parts.push(t("schedule.keepsNewest", { count: pass.keepCount || 1 }));
  if (pass.limitCount) parts.push(t("schedule.stopsAt", { count: pass.limitCount }));
  parts.push(t("schedule.pad", { before: pass.padBefore ?? 0, after: pass.padAfter ?? 0 }));
  return parts.join(" · ");
}

export function daysLabel(days: number[]) {
  const names = weekdayShortNames();
  const set = [...days].sort().join("");
  if (set === "12345") return t("schedule.weekdays");
  if (set === "06") return t("schedule.weekends");
  if (set === "0123456") return t("schedule.everyDay");
  return [...days].sort().map((d) => names[d]).join(", ");
}

function clockLabel(hhmm: string) {
  const [h, m] = hhmm.split(":").map(Number);
  const d = new Date();
  d.setHours(h, m, 0, 0);
  return formatClock(d);
}

function formatDay(iso: string) {
  const date = new Date(iso);
  return `${weekdayShortNames()[date.getDay()]} ${date.getMonth() + 1}/${date.getDate()}, ${formatClock(date)}`;
}

function clamp(value: string, min: number, max: number) {
  const n = Math.round(Number(value));
  if (!Number.isFinite(n)) return min;
  return Math.min(max, Math.max(min, n));
}

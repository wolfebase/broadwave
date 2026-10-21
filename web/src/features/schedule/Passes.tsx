import { useEffect, useRef, useState } from "react";
import type { Channel, NewPass, Pass, PassPreview } from "../../types";
import { addSeriesPass, deletePass, orderPasses, previewPass, updatePass } from "../../api";
import { useData } from "../../app/data";
import { focusRing } from "../../app/remote";
import { scheduleMessages } from "../../i18n/messages/schedule.ts";
import { bind, weekdayShortNames } from "../../i18n/runtime.ts";
import { dismissesLayer, pageCovered } from "../../lib/dialogFocus";
import { copy } from "../../strings";
import { PreviewLines, clockLabel, formatDay } from "./PreviewLines";
import { reloadAfterOrder } from "./plan";
import { TimedEditor } from "./TimedEditor";
import { timedEnd } from "./manual";

const t = bind(scheduleMessages);

type Draft = Required<Omit<NewPass, "airingStart" | "priority" | "kind" | "duration" | "date">>;
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

export type TimedStart = { channelId: number; start?: Date; minutes?: number };

export function Passes({ passes, onPasses, request, onRequestDone }: { passes: Pass[]; onPasses: () => void; request?: TimedStart | null; onRequestDone?: () => void }) {
  const { channels } = useData();
  // A drag reorders here first; a new list from the server replaces it.
  const [local, setLocal] = useState<{ from: Pass[]; ids: number[] } | null>(null);
  const order = local && local.from === passes ? local.ids : passes.map((p) => p.id);
  const setOrder = (ids: number[]) => setLocal({ from: passes, ids });
  // "timed" is a new timed recording; a guide link opens one for its channel.
  const [editing, setEditing] = useState<number | "new" | "timed" | null>(() => (request ? "timed" : null));
  const [timedFrom, setTimedFrom] = useState<TimedStart | null>(request ?? null);
  // A new link from a channel opens the editor again, for that channel.
  const [lastRequest, setLastRequest] = useState(request);
  if (request !== lastRequest) {
    setLastRequest(request);
    if (request) {
      setTimedFrom(request);
      setEditing("timed");
    }
  }
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
  const editingWas = useRef<number | "new" | "timed" | null>(null);
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
      previous === "new" || previous === "timed"
        ? document.querySelector<HTMLButtonElement>(previous === "new" ? "[data-pass-new]" : "[data-pass-timed]")
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
  // Buttons name a rule by its title; an untitled timed recording by its channel.
  const nameOf = (pass: Pass) => (pass.kind === "manual" ? passLabel(pass, channels) : pass.title);
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
    <section className="sched-section" aria-labelledby="sched-passes">
      <div className="sched-head passes-head">
        <h2 id="sched-passes" className="section-title">
          {t("schedule.passes")}
        </h2>
        {editing === "new" || editing === "timed" ? null : (
          <span className="passes-adds">
            <button type="button" className="btn small" data-pass-new onClick={() => setEditing("new")}>
              {t("schedule.newPass")}
            </button>
            <button
              type="button"
              className="btn small"
              data-pass-timed
              onClick={() => {
                setTimedFrom(null);
                setEditing("timed");
              }}
            >
              {t("schedule.recordTime")}
            </button>
          </span>
        )}
      </div>
      {editing === "timed" ? (
        <TimedEditor
          key={timedFrom ? `${timedFrom.channelId}-${timedFrom.start?.getTime() ?? ""}` : "timed"}
          channels={channels}
          start={timedFrom ?? undefined}
          onDone={(saved) => {
            setEditing(null);
            setTimedFrom(null);
            onRequestDone?.();
            if (saved) onPasses();
          }}
        />
      ) : null}
      {editing === "new" ? (
        <PassEditor
          channels={channels}
          onDone={(saved) => {
            setEditing(null);
            if (saved) onPasses();
          }}
        />
      ) : null}
      {rows.length === 0 && editing !== "new" && editing !== "timed" ? (
        <p className="sched-empty">{t("schedule.passesEmpty")}</p>
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
                <span className="pass-title">
                  <span className="tag pass-kind" data-kind={pass.kind || "series"}>
                    {passKind(pass)}
                  </span>{" "}
                  {passLabel(pass, channels)}
                </span>
                <span className="codec">{passDetails(pass, channels)}</span>
              </span>
              {removing === pass.id ? (
                <span className="pad pass-actions">
                  <span>{t("schedule.removeAsk", { name: passLabel(pass, channels) })}</span>
                  <span className="codec">{t("schedule.recordingsStay")}</span>
                  <button type="button" className="btn small" disabled={removeBusy} aria-label={t("schedule.removeNamed", { name: passLabel(pass, channels) })} onClick={() => void confirmRemove(pass.id)}>
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
                  <button type="button" className="btn small" data-pass={pass.id} data-move="up" aria-label={t("schedule.moveUp", { title: nameOf(pass) })} disabled={index === 0} onClick={() => move(pass.id, -1)}>
                    {t("schedule.up")}
                  </button>
                  <button type="button" className="btn small" data-pass={pass.id} data-move="down" aria-label={t("schedule.moveDown", { title: nameOf(pass) })} disabled={index === rows.length - 1} onClick={() => move(pass.id, 1)}>
                    {t("schedule.down")}
                  </button>
                  <button type="button" className="btn small" data-pass-edit={pass.id} aria-label={t(editing === pass.id ? "schedule.closeNamed" : "schedule.editNamed", { title: nameOf(pass) })} aria-expanded={editing === pass.id} onClick={() => setEditing(editing === pass.id ? null : pass.id)}>
                    {editing === pass.id ? t("schedule.close") : t("schedule.edit")}
                  </button>
                  <button type="button" className="btn small" data-pass-remove={pass.id} aria-label={t("schedule.removeNamed", { name: nameOf(pass) })} onClick={() => askRemove(pass.id)}>
                    {t("schedule.remove")}
                  </button>
                </span>
              )}
              {editing === pass.id && pass.kind === "manual" ? (
                <TimedEditor
                  key={pass.id}
                  pass={pass}
                  channels={channels}
                  onDone={(saved) => {
                    setEditing(null);
                    if (saved) onPasses();
                  }}
                />
              ) : editing === pass.id ? (
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
      {rows.length > 1 ? <p className="hint">{t("schedule.priorityHint")}</p> : null}
    </section>
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
  const id = pass?.id ?? "new";
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
        <div className="pe-row">
          <span className="pe-label" id={`pe-match-${id}`}>
            {t("schedule.match")}
          </span>
          <div className="pe-control">
            <div className="segmented" role="group" aria-labelledby={`pe-match-${id}`}>
              {(["title", "contains", "category"] as const).map((kind) => (
                <button key={kind} type="button" className={draft.matchKind === kind ? "seg on" : "seg"} aria-pressed={draft.matchKind === kind} onClick={() => set("matchKind", kind)}>
                  {kind === "title" ? t("schedule.titleIs") : kind === "contains" ? t("schedule.titleHas") : t("schedule.category")}
                </button>
              ))}
            </div>
            <label className="pe-field pass-name">
              <span className="sr-only">{draft.matchKind === "category" ? t("schedule.category") : draft.matchKind === "contains" ? t("schedule.words") : t("schedule.titleField")}</span>
              <input
                type="text"
                placeholder={draft.matchKind === "category" ? t("schedule.placeholderCategory") : draft.matchKind === "contains" ? t("schedule.placeholderWords") : t("schedule.placeholderTitle")}
                value={draft.title}
                autoFocus={!pass}
                onChange={(e) => set("title", e.target.value)}
              />
            </label>
          </div>
        </div>
      )}
      {once ? null : (
        <>
          <div className="pe-row">
            <label className="pe-label" htmlFor={`pe-channel-${id}`}>
              {t("schedule.channel")}
            </label>
            <div className="pe-control">
              <select id={`pe-channel-${id}`} value={draft.channelId} onChange={(e) => set("channelId", Number(e.target.value))}>
                <option value={0}>{t("schedule.anyChannel")}</option>
                {shown.map((ch) => (
                  <option key={ch.id} value={ch.id}>
                    {ch.displayNumber} {ch.displayName}
                  </option>
                ))}
              </select>
            </div>
          </div>
          {team ? null : (
            <div className="pe-row">
              <span className="pe-label" id={`pe-episodes-${id}`}>
                {t("schedule.episodes")}
              </span>
              <div className="pe-control">
                <div className="segmented" role="group" aria-labelledby={`pe-episodes-${id}`}>
                  {(["all", "new"] as const).map((kind) => (
                    <button key={kind} type="button" className={draft.episodes === kind ? "seg on" : "seg"} aria-pressed={draft.episodes === kind} onClick={() => set("episodes", kind)}>
                      {kind === "all" ? t("schedule.all") : t("schedule.newOnly")}
                    </button>
                  ))}
                </div>
              </div>
            </div>
          )}
          <div className="pe-row pass-days" role="group" aria-labelledby={`pe-days-${id}`}>
            <span className="pe-label" id={`pe-days-${id}`}>{t("schedule.days")}</span>
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
              {draft.days.length === 0 ? <span className="pe-note">{t("schedule.everyDay")}</span> : null}
            </div>
          </div>
          <div className="pe-row" role="group" aria-labelledby={`pe-time-${id}`}>
            <span className="pe-label" id={`pe-time-${id}`}>{t("schedule.time")}</span>
            <div className="pe-control pe-inline">
              <label className="pe-unit">
                <span>{t("schedule.from")}</span>
                <input type="time" value={draft.timeStart} onChange={(e) => set("timeStart", e.target.value)} />
              </label>
              <label className="pe-unit">
                <span>{t("schedule.until")}</span>
                <input type="time" value={draft.timeEnd} onChange={(e) => set("timeEnd", e.target.value)} />
              </label>
              {draft.timeStart || draft.timeEnd ? (
                <button type="button" className="btn small ghost" onClick={() => setDraft((d) => ({ ...d, timeStart: "", timeEnd: "" }))}>
                  {t("schedule.anyTime")}
                </button>
              ) : (
                <span className="pe-note">{t("schedule.anyTime")}</span>
              )}
            </div>
          </div>
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
                  <input type="number" min={1} max={99} aria-label={t("schedule.limitCount")} aria-describedby={`stop-${id}`} value={draft.limitCount} onChange={(e) => set("limitCount", clamp(e.target.value, 1, 99))} />
                  <span>{t("schedule.unwatchedUnit")}</span>
                </label>
              ) : null}
              {draft.limitCount > 0 ? (
                <span className="pe-note pe-wide" id={`stop-${id}`}>
                  {t("schedule.limitHint")}
                </span>
              ) : null}
            </div>
          </div>
        </>
      )}
      <div className="pe-row" role="group" aria-labelledby={`pe-pad-${id}`}>
        <span className="pe-label" id={`pe-pad-${id}`}>{t("schedule.padding")}</span>
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
      <div className="pe-row" role="group" aria-labelledby={`pe-opts-${id}`}>
        <span className="pe-label" id={`pe-opts-${id}`}>{t("schedule.options")}</span>
        <div className="pe-control pe-switches">
          <label className="switch-row">
            <input type="checkbox" checked={draft.commercials} onChange={(e) => set("commercials", e.target.checked)} />
            <span>{copy.library.findCommercials}</span>
          </label>
          {once || team ? null : (
            <label className="switch-row">
              <input type="checkbox" checked={draft.rerecord} onChange={(e) => set("rerecord", e.target.checked)} />
              <span>{t("schedule.recordAgainAfterDelete")}</span>
            </label>
          )}
        </div>
      </div>
      {body.title ? <PreviewLines preview={preview} note={previewNote} channels={channels} /> : null}
      {preview && !once && preview.utcOffset !== -new Date().getTimezoneOffset() * 60 ? (
        <p className="hint">{t("schedule.serverClock", { zone: preview.timeZone })}</p>
      ) : null}
      {error ? <p className="hint error" role="alert">{error}</p> : null}
      <div className="pe-actions">
        <button type="submit" className="btn primary" disabled={saving}>
          {pass ? t("schedule.save") : t("schedule.addPass")}
        </button>
        <button type="button" className="btn ghost" onClick={() => onDone(false)}>
          {t("schedule.cancel")}
        </button>
      </div>
    </form>
  );
}

/** The kind of rule in plain words: a series, one airing, a set time, or a team's games. */
export function passKind(pass: Pass) {
  switch (pass.kind) {
    case "once":
      return t("schedule.kindOnce");
    case "manual":
      return t("schedule.kindTimed");
    case "team":
      return t("schedule.kindGames");
    default:
      return t("schedule.kindSeries");
  }
}

export function passLabel(pass: Pass, channels: Channel[] = []): string {
  if (pass.kind === "manual") {
    // Untitled, it records under the guide's title or the channel's name.
    const ch = channels.find((c) => c.id === pass.channelId);
    return pass.title || (ch ? `${ch.displayNumber} ${ch.displayName}` : t("schedule.kindTimed"));
  }
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
  // Two one-time recordings of one title differ by channel and time, so both show.
  // An untitled timed recording is already named by its channel.
  if (ch && !(pass.kind === "manual" && !pass.title)) parts.push(`${ch.displayNumber} ${ch.displayName}`);
  else if (pass.kind !== "once" && pass.kind !== "manual") parts.push(t("schedule.anyChannel"));
  if (pass.kind === "manual" && pass.timeStart && pass.duration) {
    if (pass.days?.length) parts.push(daysLabel(pass.days));
    else if (pass.date) parts.push(dateLabel(pass.date));
    const end = timedEnd(pass.timeStart, pass.duration).clock;
    parts.push(t("schedule.timedSpan", { start: clockLabel(pass.timeStart), end: clockLabel(end) }));
  } else {
    if (pass.days?.length) parts.push(daysLabel(pass.days));
    if (pass.timeStart && pass.timeEnd) parts.push(`${clockLabel(pass.timeStart)}–${clockLabel(pass.timeEnd)}`);
  }
  if (pass.episodes === "new") parts.push(t("schedule.newOnly"));
  if (pass.keepMode === "unwatched") parts.push(t("schedule.keepsUnwatched"));
  if (pass.keepMode === "last") parts.push(t("schedule.keepsNewest", { count: pass.keepCount || 1 }));
  if (pass.limitCount) parts.push(t("schedule.stopsAt", { count: pass.limitCount }));
  const before = pass.padBefore ?? 0;
  const after = pass.padAfter ?? 0;
  if (before && after) parts.push(t("schedule.padLine", { before, after }));
  else if (before) parts.push(t("schedule.padStart", { before }));
  else if (after) parts.push(t("schedule.padEnd", { after }));
  return parts.join(" · ");
}

/** A YYYY-MM-DD day as "Tue 10/21". */
function dateLabel(date: string) {
  const [y, m, d] = date.split("-").map(Number);
  const day = new Date(y, m - 1, d);
  return `${weekdayShortNames()[day.getDay()]} ${m}/${d}`;
}

export function daysLabel(days: number[]) {
  const names = weekdayShortNames();
  const set = [...days].sort().join("");
  if (set === "12345") return t("schedule.weekdays");
  if (set === "06") return t("schedule.weekends");
  if (set === "0123456") return t("schedule.everyDay");
  return [...days].sort().map((d) => names[d]).join(", ");
}

function clamp(value: string, min: number, max: number) {
  const n = Math.round(Number(value));
  if (!Number.isFinite(n)) return min;
  return Math.min(max, Math.max(min, n));
}

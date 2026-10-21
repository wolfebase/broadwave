/**
 * A timed recording is a manual pass: a channel from a start time for some
 * minutes, once on a date or on chosen weekdays. Times are on the server's
 * clock, which the editor treats as this device's unless the preview says
 * the zones differ.
 */

export type TimedDraft = {
  title: string;
  channelId: number;
  repeat: boolean;
  date: string;
  days: number[];
  timeStart: string;
  duration: number;
  padBefore: number;
  padAfter: number;
  keepMode: "all" | "unwatched" | "last";
  keepCount: number;
  limitCount: number;
  commercials: boolean;
};

export type TimedPass = {
  title: string;
  channelId?: number;
  kind?: string;
  timeStart?: string;
  duration?: number;
  date?: string;
  days?: number[];
  padBefore?: number;
  padAfter?: number;
  keepMode?: string;
  keepCount?: number;
  limitCount?: number;
  commercials?: boolean;
};

export type TimedBody = {
  kind: "manual";
  title: string;
  channelId: number;
  timeStart: string;
  duration: number;
  date?: string;
  days: number[];
  padBefore: number;
  padAfter: number;
  keepMode: "all" | "unwatched" | "last";
  keepCount: number;
  limitCount: number;
  commercials: boolean;
  rename?: string;
};

/** Lengths offered as one press. Anything else is typed. */
export const durationPresets = [30, 60, 90, 120] as const;

export const maxMinutes = 480;

const pad2 = (n: number) => String(n).padStart(2, "0");

export function dateText(d: Date): string {
  return `${d.getFullYear()}-${pad2(d.getMonth() + 1)}-${pad2(d.getDate())}`;
}

export function clockText(d: Date): string {
  return `${pad2(d.getHours())}:${pad2(d.getMinutes())}`;
}

/**
 * A new draft. From a show in the guide it starts with that show and lasts
 * as long; otherwise it starts at the next five minutes for an hour.
 */
export function newTimedDraft(now: Date, channelId = 0, start?: Date, minutes?: number): TimedDraft {
  let at = start && !Number.isNaN(start.getTime()) && start.getTime() > now.getTime() - 60_000 ? new Date(start) : null;
  if (!at) {
    at = new Date(now);
    at.setSeconds(0, 0);
    at.setMinutes(Math.ceil((at.getMinutes() + 1) / 5) * 5);
  }
  const length = minutes && minutes > 0 ? Math.min(maxMinutes, Math.round(minutes)) : 60;
  return {
    title: "",
    channelId,
    repeat: false,
    date: dateText(at),
    days: [],
    timeStart: clockText(at),
    duration: length,
    padBefore: 0,
    padAfter: 0,
    keepMode: "all",
    keepCount: 5,
    limitCount: 0,
    commercials: true,
  };
}

/** A saved pass, as the editor holds it. A repeating pass has no date; one is offered if the viewer switches to once. */
export function timedDraftOf(pass: TimedPass, now: Date): TimedDraft {
  const base = newTimedDraft(now, pass.channelId ?? 0);
  const repeat = (pass.days?.length ?? 0) > 0;
  const keep = pass.keepMode === "unwatched" || pass.keepMode === "last" ? pass.keepMode : "all";
  return {
    ...base,
    title: pass.title ?? "",
    repeat,
    date: pass.date || base.date,
    days: repeat ? [...(pass.days ?? [])].sort() : [],
    timeStart: pass.timeStart || base.timeStart,
    duration: pass.duration || base.duration,
    padBefore: pass.padBefore ?? 0,
    padAfter: pass.padAfter ?? 0,
    keepMode: keep,
    keepCount: pass.keepCount || 1,
    limitCount: pass.limitCount ?? 0,
    commercials: pass.commercials !== false,
  };
}

/** What POST /passes or PATCH /passes/{id} takes. A saved pass changes its title by rename. */
export function timedBody(draft: TimedDraft, saved: boolean): TimedBody {
  const body: TimedBody = {
    kind: "manual",
    title: draft.title.trim(),
    channelId: draft.channelId,
    timeStart: draft.timeStart,
    duration: draft.duration,
    days: draft.repeat ? draft.days : [],
    padBefore: draft.padBefore,
    padAfter: draft.padAfter,
    keepMode: draft.keepMode,
    keepCount: draft.keepMode === "last" ? draft.keepCount : 0,
    limitCount: draft.limitCount,
    commercials: draft.commercials,
  };
  if (!draft.repeat) body.date = draft.date;
  if (saved) body.rename = body.title;
  return body;
}

/** What is missing before the server is asked: "" when the draft can be previewed and saved. */
export function timedProblem(draft: TimedDraft): "" | "channel" | "time" | "duration" | "date" | "days" {
  if (!draft.channelId) return "channel";
  if (!/^\d\d:\d\d$/.test(draft.timeStart)) return "time";
  if (!Number.isInteger(draft.duration) || draft.duration < 1 || draft.duration > maxMinutes) return "duration";
  if (draft.repeat ? draft.days.length === 0 : !/^\d{4}-\d\d-\d\d$/.test(draft.date)) return draft.repeat ? "days" : "date";
  return "";
}

/** The end on the clock, HH:MM, and whether it falls on a later day. */
export function timedEnd(timeStart: string, duration: number): { clock: string; nextDay: boolean } {
  const [h, m] = timeStart.split(":").map(Number);
  const total = (h || 0) * 60 + (m || 0) + Math.max(0, duration || 0);
  return { clock: `${pad2(Math.floor(total / 60) % 24)}:${pad2(total % 60)}`, nextDay: total >= 24 * 60 };
}

/** The open request from a channel: /schedule?record=<channel id>[&start=<ISO>&minutes=<n>]. */
export function timedRequest(params: URLSearchParams): { channelId: number; start?: Date; minutes?: number } | null {
  const channelId = Number(params.get("record") || 0);
  if (!Number.isInteger(channelId) || channelId <= 0) return null;
  const start = params.get("start");
  const minutes = Number(params.get("minutes") || 0);
  return {
    channelId,
    start: start ? new Date(start) : undefined,
    minutes: Number.isFinite(minutes) && minutes > 0 ? minutes : undefined,
  };
}

/** The link the guide opens: a channel, and a show's start and length when there is one. */
export function timedLink(channelId: number, airing?: { start: string; end: string }): string {
  const params = new URLSearchParams({ record: String(channelId) });
  if (airing) {
    params.set("start", airing.start);
    params.set("minutes", String(Math.round((Date.parse(airing.end) - Date.parse(airing.start)) / 60_000)));
  }
  return `/schedule?${params.toString()}`;
}

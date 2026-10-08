import type { Recording } from "../../types";
import { watched } from "./resume.ts";

export { continueWatching, watched } from "./resume.ts";

export type Sort = "newest" | "oldest" | "title" | "largest";
export type Kind = "all" | "shows" | "movies" | "sports";

export type Season = { season: number; items: Recording[] };
export type Show = {
  title: string;
  items: Recording[];
  seasons: Season[];
  unwatched: number;
  bytes: number;
  newest: number;
};

export function isMovie(rec: Recording) {
  return (rec.category || "").toLowerCase().includes("movie");
}

export function isSports(rec: Recording) {
  return Boolean(rec.gameId) || (rec.category || "").toLowerCase().includes("sport");
}

function kindOf(rec: Recording): Exclude<Kind, "all"> {
  if (isMovie(rec)) return "movies";
  if (isSports(rec)) return "sports";
  return "shows";
}

/** "S2 E5", "E5", the guide's own label, or "". */
export function episodeTag(rec: Recording) {
  if (rec.season && rec.episode) return `S${rec.season} E${rec.episode}`;
  if (rec.episode) return `E${rec.episode}`;
  return rec.episodeLabel ?? "";
}

function compare(sort: Sort) {
  return (a: Recording, b: Recording) => {
    switch (sort) {
      case "oldest":
        return airOrder(a, b) || a.id - b.id;
      case "title":
        return (a.subtitle || a.title).localeCompare(b.subtitle || b.title) || b.id - a.id;
      case "largest":
        return (b.bytes ?? 0) - (a.bytes ?? 0) || b.id - a.id;
      default:
        return airOrder(b, a) || b.id - a.id;
    }
  };
}

/** Season, then episode, then the day it was recorded. */
function airOrder(a: Recording, b: Recording) {
  return (a.season ?? 0) - (b.season ?? 0) || (a.episode ?? 0) - (b.episode ?? 0) || (Date.parse(a.startedAt) || 0) - (Date.parse(b.startedAt) || 0);
}

/** The recording of the same show that airs after this one, or none after the last. */
export function nextEpisode(recordings: Recording[], current: Recording) {
  const show = current.title.trim().toLocaleLowerCase();
  return recordings
    .filter((rec) => rec.id !== current.id && rec.status !== "recording" && !rec.missing && rec.title.trim().toLocaleLowerCase() === show)
    .sort((a, b) => airOrder(a, b) || a.id - b.id)
    .find((rec) => (airOrder(rec, current) || rec.id - current.id) > 0);
}

export function filterRecordings(recordings: Recording[], kind: Kind, unwatchedOnly: boolean) {
  return recordings.filter((rec) => (kind === "all" || kindOf(rec) === kind) && (!unwatchedOnly || !watched(rec)));
}

/** Movies stay single; everything else groups by title, then by season when the guide gave one. */
export function buildLibrary(recordings: Recording[], sort: Sort) {
  const by = compare(sort);
  const movies = recordings.filter(isMovie).sort(by);
  const map = new Map<string, Recording[]>();
  for (const rec of recordings) {
    if (isMovie(rec)) continue;
    const key = rec.title.trim().toLocaleLowerCase();
    map.set(key, [...(map.get(key) ?? []), rec]);
  }
  const shows: Show[] = [...map.values()].map((items) => {
    items.sort(by);
    const numbers = [...new Set(items.map((rec) => rec.season ?? 0))];
    const seasons = numbers
      .map((season) => ({ season, items: items.filter((rec) => (rec.season ?? 0) === season) }))
      .sort((a, b) => (a.season === 0 ? 1 : b.season === 0 ? -1 : sort === "oldest" ? a.season - b.season : b.season - a.season));
    return {
      title: items[0].title,
      items,
      seasons,
      unwatched: items.filter((rec) => !watched(rec)).length,
      bytes: items.reduce((sum, rec) => sum + (rec.bytes ?? 0), 0),
      newest: Math.max(...items.map((rec) => Date.parse(rec.startedAt) || 0)),
    };
  });
  shows.sort((a, b) => {
    switch (sort) {
      case "oldest":
        return a.newest - b.newest;
      case "title":
        return a.title.localeCompare(b.title);
      case "largest":
        return b.bytes - a.bytes;
      default:
        return b.newest - a.newest;
    }
  });
  return { shows, movies };
}

export function totalBytes(recordings: Recording[]) {
  return recordings.reduce((sum, rec) => sum + (rec.bytes ?? 0), 0);
}

import { useEffect, useMemo, useState } from "react";
import { followTeam, getTeams } from "../../api";
import { useData } from "../../app/data";
import { usePlayer } from "../../app/player";
import { navigate } from "../../app/router";
import { categoryOf, dayLabel, isRecording, minutesLeft, progress, recordingKeys, spanLabel } from "../../lib/guide";
import type { Airing, Channel, TeamFollow } from "../../types";
import { PlayIcon, RecordIcon } from "../../ui/icons";
import { ChannelBadge, Chip, Empty, LiveDot, Progress, RecDot } from "../../ui/primitives";
import { useScoreMap } from "./scores";
import "./sports.css";

type Range = "live" | "today" | "week";

/** Splits "Chiefs at Bills" or "Lakers vs. Celtics" into a matchup. */
export function matchup(a: Airing): [string, string] | null {
  const text = a.subtitle && / (at|vs\.?|@) /i.test(a.subtitle) ? a.subtitle : a.title;
  const m = text.match(/^(.*?)\s+(?:at|vs\.?|@)\s+(.*)$/i);
  if (!m) return null;
  const clean = (s: string) => s.replace(/^.*?:\s*/, "").trim();
  return [clean(m[1]), clean(m[2])];
}

function league(a: Airing): string {
  const t = `${a.title} ${a.category ?? ""}`;
  const known = ["NFL", "NBA", "MLB", "NHL", "MLS", "WNBA", "NASCAR", "PGA", "UFC", "College Football", "College Basketball", "Soccer", "Golf", "Tennis", "Olympics"];
  return known.find((k) => new RegExp(`\\b${k}\\b`, "i").test(t)) ?? a.title.split(":")[0];
}

export function Sports() {
  const { channels, index, now, planned, recordings, record, recordSeries, passes } = useData();
  const player = usePlayer();
  const [range, setRange] = useState<Range>("today");
  const [leagueFilter, setLeague] = useState("all");
  const [teams, setTeams] = useState<TeamFollow[]>([]);
  const scores = useScoreMap();
  useEffect(() => {
    getTeams().then((r) => setTeams(r.teams)).catch(() => setTeams([]));
  }, []);
  const keys = useMemo(() => recordingKeys(planned, recordings), [planned, recordings]);

  const all = useMemo(() => {
    const out: { channel: Channel; airing: Airing }[] = [];
    for (const channel of channels) {
      for (const a of index.get(channel.id) ?? []) {
        if (Date.parse(a.end) > now && categoryOf(a) === "sports") out.push({ channel, airing: a });
      }
    }
    return out.sort((a, b) => a.airing.start.localeCompare(b.airing.start));
  }, [channels, index, now]);

  const leagues = useMemo(() => Array.from(new Set(all.map((x) => league(x.airing)))).slice(0, 10), [all]);
  const liveGames = all.filter((x) => Date.parse(x.airing.start) <= now);

  const items = all.filter(({ airing }) => {
    const s = Date.parse(airing.start);
    if (leagueFilter !== "all" && league(airing) !== leagueFilter) return false;
    if (range === "live") return s <= now;
    if (range === "today") return new Date(s).toDateString() === new Date(now).toDateString() || s <= now;
    return true;
  });

  const days = new Map<string, typeof items>();
  for (const it of items) {
    const key = Date.parse(it.airing.start) <= now ? "Live now" : dayLabel(it.airing.start, now);
    days.set(key, [...(days.get(key) ?? []), it]);
  }

  return (
    <div className="sports-page">
      <header className="page-header">
        <h1>Sports</h1>
        <div className="guide-chips">
          <Chip on={range === "live"} onClick={() => setRange("live")}>Live now</Chip>
          <Chip on={range === "today"} onClick={() => setRange("today")}>Today</Chip>
          <Chip on={range === "week"} onClick={() => setRange("week")}>Coming up</Chip>
        </div>
        {liveGames.length >= 2 ? (
          <button
            type="button"
            className="btn primary"
            onClick={() => {
              const ids = liveGames.slice(0, 4).map((g) => g.channel.id);
              const layout = ids.length >= 4 ? "quad" : ids.length === 3 ? "1+2" : "2up";
              navigate(`/multiview?ch=${ids.join(",")}&layout=${layout}&focus=${ids[0]}`);
            }}
          >
            Watch together
          </button>
        ) : null}
      </header>
      {leagues.length > 1 ? (
        <div className="guide-chips league-chips">
          <Chip on={leagueFilter === "all"} onClick={() => setLeague("all")}>Everything</Chip>
          {leagues.map((l) => (
            <Chip key={l} on={leagueFilter === l} onClick={() => setLeague(l)}>{l}</Chip>
          ))}
        </div>
      ) : null}
      {items.length === 0 ? <Empty title={range === "live" ? "No games on right now" : "No games in the guide"}>Sports on your channels show up here as soon as they're listed.</Empty> : null}
      {[...days.entries()].map(([day, list]) => (
        <section key={day} className="sports-day">
          <h2 className="sports-day-title">{day}</h2>
          <div className="sports-grid">
            {list.map(({ channel, airing }) => {
              const liveNow = Date.parse(airing.start) <= now;
              const sides = matchup(airing);
              const rec = isRecording(keys, airing, now);
              const passed = passes.some((p) => p.title.toLowerCase() === airing.title.toLowerCase());
              return (
                <article key={airing.id} className={liveNow ? "game-card live" : "game-card"}>
                  <div className="gc-head">
                    <span className="gc-league">{league(airing)}</span>
                    {liveNow ? <LiveDot /> : <span className="gc-time">{spanLabel(airing)}</span>}
                    {rec ? <RecDot scheduled={rec === "scheduled"} /> : null}
                  </div>
                  {sides ? (
                    <div className="gc-matchup">
                      <span className="team">{sides[0]}</span>
                      <span className="at">at</span>
                      <span className="team">{sides[1]}</span>
                    </div>
                  ) : (
                    <h3 className="gc-title">{airing.subtitle || airing.title}</h3>
                  )}
                  {airing.gameId && scores.get(airing.gameId) ? <p className="gc-score">{scores.get(airing.gameId)}</p> : null}
                  {liveNow ? (
                    <div className="gc-progress">
                      <Progress value={progress(airing, now)} category="sports" />
                      <span>{minutesLeft(airing, now)}</span>
                    </div>
                  ) : null}
                  <div className="gc-foot">
                    <ChannelBadge channel={channel} size="sm" />
                    <span className="gc-actions">
                      {liveNow ? (
                        <button type="button" className="btn primary small" onClick={() => player.open(channel)}>
                          <PlayIcon /> Watch
                        </button>
                      ) : null}
                      {liveNow && !rec ? (
                        <button type="button" className="btn small" onClick={() => void record(channel, airing.title)} aria-label="Record">
                          <RecordIcon className="tally" />
                        </button>
                      ) : null}
                      {!passed ? (
                        <button type="button" className="btn small ghost" onClick={() => void recordSeries(airing.title, channel)}>
                          Record all
                        </button>
                      ) : null}
                      {sides?.map((side) => {
                        const mine = teams.find((team) => team.short?.toLowerCase() === side.toLowerCase() || team.name.toLowerCase() === side.toLowerCase());
                        if (mine?.record) return <span key={side} className="dim">Every {side} game</span>;
                        if (mine) {
                          return (
                            <button key={side} type="button" className="btn small ghost" onClick={() => void followTeam({ ...mine, record: true }).then((r) => setTeams(r.teams))}>
                              Record every {side} game
                            </button>
                          );
                        }
                        return (
                          <button key={side} type="button" className="btn small ghost" onClick={() => void followTeam({ name: side, short: side, league: league(airing) }).then((r) => setTeams(r.teams))}>
                            Follow {side}
                          </button>
                        );
                      })}
                    </span>
                  </div>
                </article>
              );
            })}
          </div>
        </section>
      ))}
    </div>
  );
}

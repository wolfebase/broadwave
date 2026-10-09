import { useEffect, useMemo, useRef, useState } from "react";
import { followTeam, getTeams } from "../../api";
import { takeTeams } from "./follow";
import { useData } from "../../app/data";
import { usePlayer } from "../../app/player";
import { navigate } from "../../app/router";
import { categoryOf, dayLabel, isRecording, minutesLeft, progress, recordingKeys, spanLabel } from "../../lib/guide";
import type { Airing, Channel, TeamFollow } from "../../types";
import { PlayIcon, RecordIcon } from "../../ui/icons";
import { ArtFrame } from "../../ui/ArtFrame";
import { ChannelBadge, Chip, Empty, LiveDot, Progress, RecDot } from "../../ui/primitives";
import { layoutForCount, multiviewPath } from "../multiview/storage";
import { teamScoreShown } from "./board";
import { scoreLine, useScoreboard, type ScoreGame, type ScoreTeam } from "./scores";
import "./sports.css";

type Range = "live" | "today" | "week";

function Matchup({ names, game }: { names: [string, string]; game?: ScoreGame }) {
  const home = game?.teams?.find((team) => team.home);
  const away = game?.teams?.find((team) => team !== home);
  const line = scoreLine(game);
  return (
    <>
      <div className="gc-matchup">
        <TeamMark name={names[0]} team={away} state={game?.state} />
        <span className="at">at</span>
        <TeamMark name={names[1]} team={home} state={game?.state} />
      </div>
      {game?.state === "in" && (game.detail || game.clock) ? <p className="gc-score">{game.detail || game.clock}</p> : null}
      {line && game?.state !== "in" ? <p className="gc-score">{line}</p> : null}
    </>
  );
}

function TeamMark({ name, team, state }: { name: string; team?: ScoreTeam; state?: string }) {
  return (
    <span className="team">
      {team?.logo ? <img className="team-logo" alt="" src={team.logo} /> : team?.color ? <span className="team-swatch" style={{ background: team.color }} /> : null}
      <span>{team?.short || name}</span>
      {teamScoreShown(team?.score, state) ? <span className="team-score">{team?.score}</span> : null}
    </span>
  );
}

/** Splits "Bears at Bills" or "Lakers vs. Celtics" into a matchup. */
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
  const teamN = useRef(0);
  const board = useScoreboard();
  useEffect(() => {
    const mine = ++teamN.current;
    getTeams()
      .then((r) => {
        const next = takeTeams(mine, teamN.current, r.teams);
        if (next) setTeams(next);
      })
      .catch(() => {
        if (mine === teamN.current) setTeams([]);
      });
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
              const layout = layoutForCount(ids.length);
              navigate(multiviewPath(ids, layout, ids[0]));
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
              const passed = passes.some((p) => p.kind !== "once" && p.title.toLowerCase() === airing.title.toLowerCase());
              return (
                <article key={airing.id} className={liveNow ? "game-card live" : "game-card"}>
                  {airing.imageUrl ? (
                    <div className="gc-art">
                      <ArtFrame src={`/media/art/airing/${airing.id}?w=640`} width={airing.imageWidth} height={airing.imageHeight} />
                    </div>
                  ) : null}
                  <div className="gc-head">
                    <span className="gc-league">{league(airing)}</span>
                    {liveNow ? <LiveDot /> : <span className="gc-time">{spanLabel(airing)}</span>}
                    {rec ? <RecDot scheduled={rec === "scheduled"} /> : null}
                  </div>
                  {sides ? (
                    <Matchup names={sides} game={board.find((game) => game.id === airing.gameId)} />
                  ) : (
                    <h3 className="gc-title">{airing.subtitle || airing.title}</h3>
                  )}
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
                            <button
                              key={side}
                              type="button"
                              className="btn small ghost"
                              onClick={() => {
                                const mineN = ++teamN.current;
                                void followTeam({ ...mine, record: true }).then((r) => {
                                  const next = takeTeams(mineN, teamN.current, r.teams);
                                  if (next) setTeams(next);
                                });
                              }}
                            >
                              Record every {side} game
                            </button>
                          );
                        }
                        return (
                          <button
                            key={side}
                            type="button"
                            className="btn small ghost"
                            onClick={() => {
                              const mineN = ++teamN.current;
                              void followTeam({ name: side, short: side, league: league(airing) }).then((r) => {
                                const next = takeTeams(mineN, teamN.current, r.teams);
                                if (next) setTeams(next);
                              });
                            }}
                          >
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

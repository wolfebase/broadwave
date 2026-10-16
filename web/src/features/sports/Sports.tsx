import { useEffect, useMemo, useRef, useState } from "react";
import { sportsMessages } from "../../i18n/messages/sports.ts";
import { bind } from "../../i18n/runtime.ts";
import { followTeam, getTeams } from "../../api";
import { takeTeams } from "./follow";
import { useData } from "../../app/data";
import { usePlayer } from "../../app/player";
import { navigate } from "../../app/router";
import { focusRing } from "../../app/remote";
import { useLayout } from "../../app/layout";
import { actionName } from "../../lib/actionName";
import { categoryOf, dayLabel, isRecording, minutesLeft, progress, recordingKeys, spanLabel } from "../../lib/guide";
import type { Airing, Channel, TeamFollow } from "../../types";
import { PlayIcon, RecordIcon } from "../../ui/icons";
import { ArtFrame } from "../../ui/ArtFrame";
import { ChannelBadge, Chip, Empty, LiveDot, Progress, RecDot } from "../../ui/primitives";
import { layoutForCount, multiviewPath } from "../multiview/storage";
import { teamScoreShown } from "./board";
import { cardLabel, leagueLabel, matchup } from "./matchup";
import { isFiction, isGame, leagueOf, playsIn } from "./kind";
import { scoreLine, useScoreboard, type ScoreGame, type ScoreTeam } from "./scores";
import "../manage.css";
import "./sports.css";

const t = bind(sportsMessages);

type Range = "live" | "today" | "week";

function Matchup({ names, game }: { names: [string, string]; game?: ScoreGame }) {
  const home = game?.teams?.find((team) => team.home);
  const away = game?.teams?.find((team) => team !== home);
  const line = scoreLine(game);
  return (
    <>
      <div className="gc-matchup">
        <TeamMark name={names[0]} team={away} state={game?.state} />
        <span className="at">{t("sports.at")}</span>
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

type Item = { channel: Channel; airing: Airing };
type Card = Item & { also: Channel[] };

export function Sports() {
  const { channels, index, now, planned, recordings, record, recordSeries, passes } = useData();
  const player = usePlayer();
  const [range, setRange] = useState<Range>("today");
  const [filter, setFilter] = useState("all");
  const [teams, setTeams] = useState<TeamFollow[]>([]);
  const teamN = useRef(0);
  const pageRef = useRef<HTMLDivElement>(null);
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
    const out: Item[] = [];
    for (const channel of channels) {
      for (const a of index.get(channel.id) ?? []) {
        if (Date.parse(a.end) > now && categoryOf(a) === "sports" && !isFiction(a)) out.push({ channel, airing: a });
      }
    }
    return out.sort((a, b) => a.airing.start.localeCompare(b.airing.start));
  }, [channels, index, now]);

  const gameOf = (a: Airing) => (a.gameId ? board.find((game) => game.id === a.gameId) : undefined);
  const leagueFor = (a: Airing) => leagueOf(a, gameOf(a)?.league);
  const games = all.filter(({ airing }) => isGame(airing));
  // Chips come from every game ahead, so switching Today and Coming up does not move them.
  const leagues = Array.from(new Set(games.map(({ airing }) => leagueFor(airing)).filter(Boolean))).slice(0, 12);
  const followed = teams.filter((team) => games.some(({ airing }) => playsIn(matchup(airing), team)));
  const filters = [...leagues.map((l) => `l:${l}`), ...followed.map((team) => `t:${team.name}`)];
  const shownFilter = filters.includes(filter) ? filter : "all";
  const liveGames = games.filter((x) => Date.parse(x.airing.start) <= now);

  const inRange = ({ airing }: Item) => {
    const s = Date.parse(airing.start);
    if (range === "live") return s <= now;
    if (range === "today") return new Date(s).toDateString() === new Date(now).toDateString() || s <= now;
    return true;
  };
  const matches = ({ airing }: Item) => {
    if (shownFilter === "all") return true;
    if (shownFilter.startsWith("l:")) return leagueFor(airing) === shownFilter.slice(2);
    const team = followed.find((item) => item.name === shownFilter.slice(2));
    return team ? playsIn(matchup(airing), team) : true;
  };
  const items = games.filter((x) => inRange(x) && matches(x));
  const shows = shownFilter === "all" ? all.filter((x) => !isGame(x.airing) && inRange(x)) : [];

  // One game on two channels (a 3.0 station and its 1.0 twin, or a sister station) is one card.
  const cards: Card[] = [];
  const seen = new Map<string, Card>();
  for (const it of items) {
    const key = `${it.airing.gameId || `${it.airing.title}|${it.airing.subtitle ?? ""}`}|${it.airing.start}`;
    const same = seen.get(key);
    if (same) same.also.push(it.channel);
    else {
      const card = { ...it, also: [] };
      seen.set(key, card);
      cards.push(card);
    }
  }
  const days = new Map<string, Card[]>();
  for (const it of cards) {
    const key = Date.parse(it.airing.start) <= now ? t("sports.liveNow") : dayLabel(it.airing.start, now);
    days.set(key, [...(days.get(key) ?? []), it]);
  }

  const layout = useLayout();
  const firstKey = items[0]?.airing.id ?? -1;
  useEffect(() => {
    // On a TV the remote starts on the page, not on the tab it came from.
    if (layout !== "tv") return;
    const frame = window.requestAnimationFrame(() => {
      const active = document.activeElement;
      if (active instanceof HTMLElement && active !== document.body && !active.closest(".topbar")) return;
      const root = pageRef.current;
      const target = root?.querySelector<HTMLElement>(".game-card button:not([disabled])") ?? root?.querySelector<HTMLElement>(".chip.on");
      focusRing(target);
    });
    return () => window.cancelAnimationFrame(frame);
  }, [layout, firstKey]);

  const recordAll = (channel: Channel, airing: Airing) => {
    const passed = passes.some((p) => p.kind !== "once" && p.title.toLowerCase() === airing.title.toLowerCase());
    if (passed) return null;
    return (
      <button type="button" className="btn small ghost" aria-label={actionName(t("sports.recordAll"), cardLabel(airing))} onClick={() => void recordSeries(airing.title, channel)}>
        {t("sports.recordAll")}
      </button>
    );
  };

  return (
    <div className="sports-page" ref={pageRef}>
      <header className="page-header">
        <h1>{t("sports.title")}</h1>
        <div className="guide-chips sports-range" role="group" aria-label={t("sports.title")}>
          <Chip on={range === "live"} onClick={() => setRange("live")}>{t("sports.liveNow")}</Chip>
          <Chip on={range === "today"} onClick={() => setRange("today")}>{t("sports.today")}</Chip>
          <Chip on={range === "week"} onClick={() => setRange("week")}>{t("sports.comingUp")}</Chip>
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
            {t("sports.watchTogether")}
          </button>
        ) : null}
      </header>
      {filters.length > 1 ? (
        <div className="guide-chips league-chips" role="group" aria-label={t("sports.filters")}>
          <Chip on={shownFilter === "all"} onClick={() => setFilter("all")}>{t("sports.all")}</Chip>
          {leagues.map((l) => (
            <Chip key={l} on={shownFilter === `l:${l}`} onClick={() => setFilter(`l:${l}`)}>{leagueLabel(l)}</Chip>
          ))}
          {followed.map((team) => (
            <Chip key={team.name} on={shownFilter === `t:${team.name}`} onClick={() => setFilter(`t:${team.name}`)}>
              {team.logo ? <img className="chip-logo" alt="" src={team.logo} /> : null}
              {team.short || team.name}
            </Chip>
          ))}
        </div>
      ) : null}
      {items.length === 0 && (shows.length > 0 || (range !== "week" && games.some(matches))) ? (
        <div className="sports-none">
          <p>{range === "live" ? t("sports.noLive") : range === "today" ? t("sports.noGamesToday") : t("sports.noGames")}</p>
          {range !== "week" && games.some(matches) ? (
            <button type="button" className="btn small" onClick={() => setRange("week")}>
              {t("sports.seeComingUp")}
            </button>
          ) : null}
        </div>
      ) : items.length === 0 ? (
        <Empty title={range === "live" ? t("sports.noLive") : t("sports.noGames")}>{t("sports.empty")}</Empty>
      ) : null}
      {[...days.entries()].map(([day, list]) => (
        <section key={day} className="sports-day">
          <h2 className="sports-day-title">{day}</h2>
          <div className="sports-grid">
            {list.map(({ channel, airing, also }) => {
              const liveNow = Date.parse(airing.start) <= now;
              const sides = matchup(airing);
              const rec = isRecording(keys, airing, now);
              const league = leagueFor(airing);
              return (
                <article key={airing.id} className={liveNow ? "game-card live" : "game-card"}>
                  {airing.imageUrl ? (
                    <div className="gc-art">
                      <ArtFrame src={`/media/art/airing/${airing.id}?w=640`} width={airing.imageWidth} height={airing.imageHeight} />
                    </div>
                  ) : null}
                  <div className="gc-head">
                    <span className="gc-league">{league ? leagueLabel(league) : airing.title}</span>
                    {liveNow ? <LiveDot /> : <span className="gc-time">{spanLabel(airing)}</span>}
                    {rec ? <RecDot scheduled={rec === "scheduled"} /> : null}
                  </div>
                  {sides ? (
                    <Matchup names={sides} game={gameOf(airing)} />
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
                    <span className="gc-channels">
                      <ChannelBadge channel={channel} size="sm" />
                      {also.map((other) => (
                        <ChannelBadge key={other.id} channel={other} size="sm" />
                      ))}
                    </span>
                    <span className="gc-actions">
                      {liveNow ? (
                        <button type="button" className="btn primary small" aria-label={actionName(t("sports.watch"), cardLabel(airing))} onClick={() => player.open(channel)}>
                          <PlayIcon /> {t("sports.watch")}
                        </button>
                      ) : null}
                      {liveNow && !rec ? (
                        <button type="button" className="btn small" onClick={() => void record(channel, airing.title)} aria-label={actionName(t("sports.record"), cardLabel(airing))}>
                          <RecordIcon className="tally" />
                        </button>
                      ) : null}
                      {recordAll(channel, airing)}
                      {sides?.map((side) => {
                        const mine = teams.find((team) => team.short?.toLowerCase() === side.toLowerCase() || team.name.toLowerCase() === side.toLowerCase());
                        if (mine?.record) return <span key={side} className="dim gc-every">{t("sports.everyGame", { side })}</span>;
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
                              {t("sports.recordEveryGame", { side })}
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
                              void followTeam({ name: side, short: side, league: league || undefined }).then((r) => {
                                const next = takeTeams(mineN, teamN.current, r.teams);
                                if (next) setTeams(next);
                              });
                            }}
                          >
                            {t("sports.follow", { side })}
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
      {shows.length > 0 ? (
        <section className="sports-shows" aria-labelledby="sports-shows-title">
          <h2 id="sports-shows-title" className="sports-shows-title">{t("sports.shows")}</h2>
          <ul className="show-rows">
            {shows.map(({ channel, airing }) => {
              const liveNow = Date.parse(airing.start) <= now;
              const rec = isRecording(keys, airing, now);
              return (
                <li key={airing.id} className="show-row">
                  <span className="sr-when">
                    {liveNow ? (
                      <LiveDot />
                    ) : (
                      <>
                        {range === "week" ? <span className="sr-day">{dayLabel(airing.start, now)}</span> : null}
                        <span className="gc-time">{spanLabel(airing)}</span>
                      </>
                    )}
                  </span>
                  <span className="sr-name">
                    {rec ? <RecDot scheduled={rec === "scheduled"} /> : null}
                    <strong>{airing.title}</strong>
                    {airing.subtitle ? <span className="dim"> · {airing.subtitle}</span> : null}
                  </span>
                  <ChannelBadge channel={channel} size="sm" />
                  <span className="sr-actions">
                    {liveNow ? (
                      <button type="button" className="btn small" aria-label={actionName(t("sports.watch"), airing.title)} onClick={() => player.open(channel)}>
                        {t("sports.watch")}
                      </button>
                    ) : null}
                    {recordAll(channel, airing)}
                  </span>
                </li>
              );
            })}
          </ul>
        </section>
      ) : null}
    </div>
  );
}

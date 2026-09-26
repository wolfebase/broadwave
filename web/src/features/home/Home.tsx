import { useEffect, useMemo, useState } from "react";
import { getTeams } from "../../api";
import { useData } from "../../app/data";
import { usePlayer } from "../../app/player";
import { navigate } from "../../app/router";
import { airingAt, categoryLabel, categoryOf, dayLabel, minutesLeft, nextAfter, progress, timeLabel, type Category } from "../../lib/guide";
import type { Airing, Channel, Recording, TeamFollow } from "../../types";
import { PlayIcon, RecordIcon } from "../../ui/icons";
import { isLayout, layoutForCount, layoutLabel, multiviewPath, savedSets } from "../multiview/storage";
import { ArtFrame } from "../../ui/ArtFrame";
import { LiveFrame } from "../../ui/LiveFrame";
import { ChannelBadge, Empty, LiveDot, Progress, SectionHeader } from "../../ui/primitives";
import "./home.css";

type Live = { channel: Channel; airing?: Airing; cat: Category };

function mentions(text: string, name: string) {
  const escaped = name.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
  return new RegExp(`\\b${escaped}\\b`, "i").test(text);
}

export function Home() {
  const { channels, index, now, recordings, ready, settled, record } = useData();
  const player = usePlayer();
  const [teams, setTeams] = useState<TeamFollow[]>([]);
  useEffect(() => {
    getTeams().then((r) => setTeams(r.teams)).catch(() => setTeams([]));
  }, []);

  const live: Live[] = useMemo(
    () =>
      channels.map((channel) => {
        const airing = airingAt(index, channel.id, now);
        return { channel, airing, cat: categoryOf(airing) };
      }),
    [channels, index, now],
  );

  const hero = useMemo(() => {
    const score = (l: Live) => (l.cat === "sports" ? 4 : 0) + (l.channel.favorite ? 2 : 0) + (l.airing ? 1 : 0) + (l.channel.hd ? 0.5 : 0);
    return [...live].sort((a, b) => score(b) - score(a))[0];
  }, [live]);

  const onNow = useMemo(() => [...live].filter((l) => l.airing).sort((a, b) => Number(b.channel.favorite) - Number(a.channel.favorite)), [live]);

  const yours = useMemo(() => {
    const names = teams.flatMap((team) => [team.short, team.name].filter((name): name is string => !!name && name.length >= 4));
    if (names.length === 0) return [];
    const out: { channel: Channel; airing: Airing }[] = [];
    const until = now + 36 * 3600_000;
    for (const channel of channels) {
      for (const airing of index.get(channel.id) ?? []) {
        const start = Date.parse(airing.start);
        if (Date.parse(airing.end) <= now || start > until) continue;
        const text = `${airing.title} ${airing.subtitle ?? ""}`;
        if (names.some((name) => mentions(text, name))) out.push({ channel, airing });
      }
    }
    return out.sort((a, b) => a.airing.start.localeCompare(b.airing.start)).slice(0, 12);
  }, [teams, channels, index, now]);

  const sports = useMemo(() => {
    const out: { channel: Channel; airing: Airing }[] = [];
    const until = now + 48 * 3600_000;
    for (const channel of channels) {
      for (const a of index.get(channel.id) ?? []) {
        const s = Date.parse(a.start);
        if (Date.parse(a.end) > now && s < until && categoryOf(a) === "sports") out.push({ channel, airing: a });
      }
    }
    return out.sort((a, b) => a.airing.start.localeCompare(b.airing.start)).slice(0, 16);
  }, [channels, index, now]);

  const tonight = useMemo(() => {
    const d = new Date(now);
    d.setHours(20, 0, 0, 0);
    if (d.getTime() + 3 * 3600_000 < now) d.setDate(d.getDate() + 1);
    const at = Math.max(now, d.getTime()) + 60_000;
    return channels
      .map((channel) => ({ channel, airing: airingAt(index, channel.id, at) }))
      .filter((x): x is { channel: Channel; airing: Airing } => !!x.airing && categoryOf(x.airing) !== "other")
      .slice(0, 14);
  }, [channels, index, now]);

  const recordingNow = recordings.filter((r) => r.status === "recording");
  const resume = recordings.filter((r) => r.status !== "recording" && (r.position ?? 0) > 30 && !r.watched).slice(0, 10);
  const recent = recordings.filter((r) => r.status !== "recording" && !resume.includes(r)).slice(0, 12);

  if (!settled && channels.length === 0) {
    return (
      <div className="home" aria-busy="true">
        <div className="skel hero-skel" />
        <div className="skel-row">
          <div className="skel" />
          <div className="skel" />
          <div className="skel" />
        </div>
      </div>
    );
  }

  if (ready && settled && channels.length === 0) {
    return (
      <div className="home">
        <Empty title="Let's find your tuner" action={<button className="btn primary" onClick={() => navigate("/settings#sources")}>Set up</button>}>
          Connect an HDHomeRun to your network, then search for it.
        </Empty>
      </div>
    );
  }

  return (
    <div className="home">
      {hero ? (
        <section className="hero" data-cat={hero.cat}>
          {hero.airing?.imageUrl ? (
            <ArtFrame src={`/media/art/airing/${hero.airing.id}?w=960`} width={hero.airing.imageWidth} height={hero.airing.imageHeight} />
          ) : (
            <ArtFrame src={`/api/v1/channels/${hero.channel.id}/frame?w=1280`} width={1280} height={720} />
          )}
          <div className="hero-glow" aria-hidden="true" />
          <div className="hero-num" aria-hidden="true">
            {hero.channel.displayNumber}
          </div>
          <div className="hero-body">
            <div className="hero-meta">
              <LiveDot label="Live now" />
              {hero.cat !== "other" ? <span className="hero-cat">{categoryLabel[hero.cat]}</span> : null}
            </div>
            <h1 className="hero-title">{hero.airing?.title ?? hero.channel.displayName}</h1>
            {hero.airing?.subtitle ? <p className="hero-sub">{hero.airing.subtitle}</p> : null}
            <div className="hero-line">
              <ChannelBadge channel={hero.channel} />
              {hero.airing ? (
                <>
                  <Progress value={progress(hero.airing, now)} category={hero.cat} />
                  <span className="hero-left">{minutesLeft(hero.airing, now)}</span>
                </>
              ) : null}
            </div>
            <div className="hero-actions">
              <button type="button" className="btn primary big" onClick={() => player.open(hero.channel)}>
                <PlayIcon /> Watch
              </button>
              {hero.airing ? (
                <button type="button" className="btn big" onClick={() => void record(hero.channel, hero.airing!.title)}>
                  <RecordIcon className="tally" /> Record
                </button>
              ) : null}
              {(() => {
                const n = nextAfter(index, hero.channel.id, hero.airing ? Date.parse(hero.airing.end) : now);
                return n ? (
                  <span className="hero-next">
                    Next at {timeLabel(n.start)} · {n.title}
                  </span>
                ) : null;
              })()}
            </div>
          </div>
        </section>
      ) : null}

      {recordingNow.length > 0 ? (
        <div className="rec-strip">
          {recordingNow.map((r) => (
            <span key={r.id} className="rec-chip">
              <span className="live-dot-light" aria-hidden="true" /> Recording {r.title}
            </span>
          ))}
        </div>
      ) : null}

      <Shelf title="On now" action={<button className="text-btn" onClick={() => navigate("/guide")}>Guide</button>}>
        {onNow.length === 0 && !settled ? (
          <div className="skel-row" aria-hidden="true">
            <div className="skel" />
            <div className="skel" />
          </div>
        ) : null}
        {onNow.map((l) => (
          <button key={l.channel.id} type="button" className={`now-card${l.airing?.imageUrl ? " has-art" : ""}`} data-cat={l.cat} onClick={() => player.open(l.channel)}>
            {l.airing?.imageUrl ? (
              <span className="nc-art" aria-hidden="true" style={{ backgroundImage: `url(/media/art/airing/${l.airing.id}?w=640)` }} />
            ) : null}
            <LiveFrame id={l.channel.id} className="nc-frame" />
            <span className="nc-top">
              <span className="nc-num">{l.channel.displayNumber}</span>
              <span className="nc-name">{l.channel.displayName}</span>
            </span>
            <span className="nc-title">{l.airing?.title}</span>
            <span className="nc-foot">
              <Progress value={progress(l.airing, now)} category={l.cat} />
              <span>{l.airing ? minutesLeft(l.airing, now) : ""}</span>
            </span>
          </button>
        ))}
      </Shelf>

      {yours.length > 0 ? (
        <Shelf title="Your teams" action={<button className="text-btn" onClick={() => navigate("/sports")}>Sports</button>}>
          {yours.map(({ channel, airing }) => {
            const liveNow = Date.parse(airing.start) <= now;
            return (
              <button key={airing.id} type="button" className="sport-card" onClick={() => (liveNow ? player.open(channel) : navigate("/sports"))}>
                <span className="sc-when">{liveNow ? <LiveDot /> : `${dayLabel(airing.start, now)} · ${timeLabel(airing.start)}`}</span>
                <span className="sc-title">{airing.title}</span>
                <ChannelBadge channel={channel} size="sm" />
              </button>
            );
          })}
        </Shelf>
      ) : null}

      {sports.length > 0 ? (
        <Shelf
          title="Sports"
          action={
            <>
              {sports.filter((s) => Date.parse(s.airing.start) <= now).length >= 2 ? (
                <button
                  type="button"
                  className="text-btn"
                  onClick={() => {
                    const ids = sports.filter((s) => Date.parse(s.airing.start) <= now).slice(0, 4).map((s) => s.channel.id);
                    const layout = layoutForCount(ids.length);
                    navigate(multiviewPath(ids, layout, ids[0]));
                  }}
                >
                  Watch together
                </button>
              ) : null}
              <button className="text-btn" onClick={() => navigate("/sports")}>All sports</button>
            </>
          }
        >
          {sports.map(({ channel, airing }) => {
            const liveNow = Date.parse(airing.start) <= now;
            return (
              <button key={airing.id} type="button" className="sport-card" onClick={() => (liveNow ? player.open(channel) : navigate("/sports"))}>
                <span className="sc-when">{liveNow ? <LiveDot /> : `${dayLabel(airing.start, now)} · ${timeLabel(airing.start)}`}</span>
                <span className="sc-title">{airing.title}</span>
                {airing.subtitle ? <span className="sc-sub">{airing.subtitle}</span> : null}
                <ChannelBadge channel={channel} size="sm" />
              </button>
            );
          })}
        </Shelf>
      ) : null}

      {savedSets().length > 0 ? (
        <Shelf title="Saved sets">
          {savedSets().map((set) => {
            const layout = set.layout && isLayout(set.layout) ? set.layout : layoutForCount(set.channels.length);
            return (
              <button key={set.channels.join(",")} type="button" className="now-card" onClick={() => navigate(multiviewPath(set.channels, layout, set.channels[0]))}>
                <span className="nc-title">{set.name}</span>
                <span className="nc-foot dim">{layoutLabel(layout)}</span>
              </button>
            );
          })}
        </Shelf>
      ) : null}

      {resume.length > 0 ? (
        <Shelf title="Continue watching">
          {resume.map((r) => (
            <RecordingCard key={r.id} rec={r} />
          ))}
        </Shelf>
      ) : null}

      {tonight.length > 0 ? (
        <Shelf title="Tonight">
          {tonight.map(({ channel, airing }) => (
            <button key={airing.id} type="button" className="now-card tonight" data-cat={categoryOf(airing)} onClick={() => navigate("/guide")}>
              <span className="nc-top">
                <span className="nc-num">{channel.displayNumber}</span>
                <span className="nc-name">{timeLabel(airing.start)}</span>
              </span>
              <span className="nc-title">{airing.title}</span>
              <span className="nc-foot dim">{categoryLabel[categoryOf(airing)]}</span>
            </button>
          ))}
        </Shelf>
      ) : null}

      {recent.length > 0 ? (
        <Shelf title="Recently recorded" action={<button className="text-btn" onClick={() => navigate("/recordings")}>Recordings</button>}>
          {recent.map((r) => (
            <RecordingCard key={r.id} rec={r} />
          ))}
        </Shelf>
      ) : null}
    </div>
  );
}

function Shelf({ title, action, children }: { title: string; action?: React.ReactNode; children: React.ReactNode }) {
  return (
    <section className="shelf">
      <SectionHeader title={title} action={action} />
      <div className="shelf-row">{children}</div>
    </section>
  );
}

function RecordingCard({ rec }: { rec: Recording }) {
  const pct = rec.durationSec && rec.position ? rec.position / rec.durationSec : 0;
  return (
    <button type="button" className="rec-card" onClick={() => navigate(`/play?recording=${rec.id}`)}>
      <span className="rc-poster">
        <img
          src={`/media/poster/${rec.id}`}
          alt=""
          loading="lazy"
          onError={(e) => {
            const img = e.currentTarget;
            if (img.dataset.fallback) {
              img.style.visibility = "hidden";
              return;
            }
            img.dataset.fallback = "1";
            img.src = `/media/art/channel/${rec.channelId}?w=320`;
          }}
        />
        {pct > 0 ? <Progress value={pct} /> : null}
      </span>
      <span className="rc-title">{rec.title}</span>
      <span className="rc-sub">{rec.subtitle || new Date(rec.startedAt).toLocaleDateString([], { month: "short", day: "numeric" })}</span>
    </button>
  );
}

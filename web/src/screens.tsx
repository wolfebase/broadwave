import { useEffect, useState } from "react";
import type { Airing, Channel, Device, Pass, PlannedAiring, Recording, Settings, StorageInfo, TunerStatus } from "./types";
import { addSource, deletePass, getEvents, getSchedule, getTuners, stopRecording, updatePass } from "./api";
import { readRecent } from "./recent";
import { copy } from "./strings";
import { formatClock } from "./time";

export function Home({
  channels,
  airings,
  recordings,
  storage,
  now,
  onWatch,
  onResume,
}: {
  channels: Channel[];
  airings: Airing[];
  recordings: Recording[];
  storage: StorageInfo | null;
  now: Date;
  onWatch: (channel: Channel) => void;
  onResume: (recording: Recording) => void;
}) {
  const favorites = channels.filter((channel) => channel.favorite);
  const hero = favorites[0] ?? channels[0];
  const heroTitle = hero ? currentTitle(airings, hero.id) : "";
  const active = recordings.filter((recording) => recording.status === "recording");
  const resume = recordings.filter((recording) => recording.status !== "recording" && (recording.position ?? 0) > 1 && !watched(recording));
  const recent = readRecent()
    .map((item) => channels.find((channel) => channel.id === item.id))
    .filter((channel): channel is Channel => Boolean(channel));
  return (
    <section className="page">
      <div className="page-head">
        <div>
          <p className="kicker">{copy.home.kicker}</p>
          <h2>{formatClock(now)}</h2>
        </div>
      </div>
      {hero ? (
        <article className="hero">
          <div className="hero-num">{hero.displayNumber}</div>
          <div>
            <h3>{hero.displayName}</h3>
            <p>{heroTitle || copy.home.listings}</p>
            <button type="button" className="btn primary hero-watch" onClick={() => onWatch(hero)}>
              Watch
            </button>
          </div>
        </article>
      ) : (
        <p className="empty">{copy.sources.none}</p>
      )}
      <h3 className="section-title">{copy.home.favorites}</h3>
      {favorites.length === 0 ? (
        <p className="empty">{copy.home.noFavorites}</p>
      ) : (
        <div className="fav-row">
          {favorites.map((channel) => (
            <button key={channel.id} type="button" className="fav-chip" onClick={() => onWatch(channel)}>
              <span className="ch-num">{channel.displayNumber}</span>
              <span>{channel.displayName}</span>
            </button>
          ))}
        </div>
      )}
      {recent.length > 0 ? (
        <>
          <h3 className="section-title">Recent</h3>
          <div className="fav-row">
            {recent.map((channel) => (
              <button key={channel.id} type="button" className="fav-chip" onClick={() => onWatch(channel)}>
                <span className="ch-num">{channel.displayNumber}</span>
                <span>{channel.displayName}</span>
              </button>
            ))}
          </div>
        </>
      ) : null}
      <RecordingSoon channels={channels} now={now} />
      <UpNext channels={favorites.length > 0 ? favorites : channels.slice(0, 4)} airings={airings} now={now} onWatch={onWatch} />
      <OnLater channels={channels} airings={airings} now={now} onWatch={onWatch} />
      <div className="split">
        <article className="quiet-card">
          <h3>{active.length > 0 ? active[0].title : copy.home.recording}</h3>
          <p>{active.length > 0 ? `Recording on ${active[0].guideNumber}.` : copy.home.recordingBody}</p>
          {storage ? <p>{storageLine(storage)}</p> : null}
        </article>
        <article className="quiet-card">
          <h3>{copy.home.continueTitle}</h3>
          {resume.length === 0 ? (
            <p>{copy.home.continueBody}</p>
          ) : (
            <div className="fav-row">
              {resume.map((recording) => (
                <button key={recording.id} type="button" className="fav-chip" onClick={() => onResume(recording)}>
                  <span>{recording.title}</span>
                  <span className="codec">
                    {formatClockPoint(recording.position ?? 0)}
                    {recording.durationSec ? ` of ${formatClockPoint(recording.durationSec)}` : ""}
                  </span>
                </button>
              ))}
            </div>
          )}
        </article>
      </div>
    </section>
  );
}

export function Library({
  recordings,
  note,
  onPlay,
  onVirtual,
  onDelete,
  onWatched,
}: {
  recordings: Recording[];
  note: string;
  onPlay: (recording: Recording) => void;
  onVirtual: (recording: Recording) => void;
  onDelete: (recording: Recording) => void;
  onWatched: (recording: Recording, watched: boolean) => void;
}) {
  const [armed, setArmed] = useState<number | null>(null);
  const [libraryFilter, setLibraryFilter] = useState<"all" | "unwatched">("all");
  const shown = libraryFilter === "unwatched" ? recordings.filter((rec) => !watched(rec)) : recordings;
  const grouped = groupLibrary(shown);
  return (
    <section className="page">
      <div className="page-head">
        <h2>{copy.library.title}</h2>
      </div>
      {note ? <p className="lede">{note}</p> : null}
      <div className="segmented" role="tablist" aria-label="Library filter">
        <button type="button" role="tab" aria-selected={libraryFilter === "all"} className={libraryFilter === "all" ? "seg on" : "seg"} onClick={() => setLibraryFilter("all")}>
          All
        </button>
        <button type="button" role="tab" aria-selected={libraryFilter === "unwatched"} className={libraryFilter === "unwatched" ? "seg on" : "seg"} onClick={() => setLibraryFilter("unwatched")}>
          Unwatched
        </button>
      </div>
      {shown.length === 0 ? (
        <article className="quiet-card wide">
          <p>{recordings.length === 0 ? copy.library.body : "Everything in the library has been watched."}</p>
        </article>
      ) : (
        <div className="show-grid">
          {grouped.shows.map(([title, items]) => (
            <section key={title}>
              <h3 className="show-title">{title}</h3>
              <ul className="source-list">
                {items.map((rec) => (
                  <LibraryRow key={rec.id} rec={rec} armed={armed} setArmed={setArmed} onPlay={onPlay} onVirtual={onVirtual} onDelete={onDelete} onWatched={onWatched} />
                ))}
              </ul>
            </section>
          ))}
          {grouped.movies.length > 0 ? (
            <section>
              <h3 className="show-title">Movies</h3>
              <ul className="source-list">
                {grouped.movies.map((rec) => (
                  <LibraryRow key={rec.id} rec={rec} armed={armed} setArmed={setArmed} onPlay={onPlay} onVirtual={onVirtual} onDelete={onDelete} onWatched={onWatched} />
                ))}
              </ul>
            </section>
          ) : null}
        </div>
      )}
    </section>
  );
}

function LibraryRow({
  rec,
  armed,
  setArmed,
  onPlay,
  onVirtual,
  onDelete,
  onWatched,
}: {
  rec: Recording;
  armed: number | null;
  setArmed: (id: number | null) => void;
  onPlay: (recording: Recording) => void;
  onVirtual: (recording: Recording) => void;
  onDelete: (recording: Recording) => void;
  onWatched: (recording: Recording, watched: boolean) => void;
}) {
  const seen = watched(rec);
  return (
    <li className="media-card">
      <span className="poster-wrap">
        <img className="poster" alt="" src={`/media/poster/${rec.id}`} onError={(event) => { event.currentTarget.style.visibility = "hidden"; }} />
      </span>
      <div>
        <strong>{rec.subtitle || rec.title}</strong>
        <span className="ch-tags">
          {rec.guideNumber}
          {" · "}
          {rec.status === "recording" ? "Recording" : rec.status}
          {" · "}
          {formatBytes(rec.bytes ?? 0)}
          {rec.durationSec ? ` · ${formatClockPoint(rec.durationSec)}` : ""}
          {(rec.position ?? 0) > 1 ? ` · resume ${formatClockPoint(rec.position ?? 0)}` : ""}
          {seen ? " · Watched" : ""}
          {rec.error ? ` · ${rec.error}` : ""}
        </span>
        <div className="sheet-actions">
      <button type="button" className="btn primary" onClick={() => onPlay(rec)}>Play</button>
      {rec.status !== "recording" ? <button type="button" className="btn" onClick={() => onWatched(rec, !seen)}>{seen ? "Mark unwatched" : "Mark watched"}</button> : null}
      {rec.status !== "recording" ? <button type="button" className="btn" onClick={() => onVirtual(rec)}>Make channel</button> : null}
      <a className="btn" href={`/api/recordings/${rec.id}/file`}>Download</a>
      {rec.status === "recording" ? null : armed === rec.id ? (
        <button type="button" className="btn primary" onClick={() => onDelete(rec)}>Delete this file</button>
      ) : (
        <button type="button" className="btn" onClick={() => setArmed(rec.id)}>Delete</button>
      )}
        </div>
      </div>
    </li>
  );
}

function groupLibrary(recordings: Recording[]) {
  const movies: Recording[] = [];
  const map = new Map<string, Recording[]>();
  for (const rec of recordings) {
    if ((rec.category || "").toLowerCase().includes("movie")) {
      movies.push(rec);
      continue;
    }
    const list = map.get(rec.title) ?? [];
    list.push(rec);
    map.set(rec.title, list);
  }
  return { movies, shows: [...map.entries()] };
}

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

export function Sources({
  devices,
  channels,
  busy,
  error,
  onDiscover,
  onLookup,
  onPatch,
}: {
  devices: Device[];
  channels: Channel[];
  busy: boolean;
  error: string;
  onDiscover: () => void;
  onLookup: (ip: string) => void;
  onPatch: (channel: Channel, patch: { enabled?: boolean; hidden?: boolean; customName?: string; customNumber?: string }) => void;
}) {
  const [ip, setIp] = useState("");
  const [tuners, setTuners] = useState<TunerStatus[]>([]);
  const [encoder, setEncoder] = useState("");
  useEffect(() => {
    let stop = false;
    async function load() {
      try {
        const res = await getTuners();
        if (stop) return;
        setTuners(res.tuners ?? []);
        setEncoder(res.encoder ?? "");
      } catch {
        if (!stop) setTuners([]);
      }
    }
    void load();
    const id = window.setInterval(() => void load(), 5000);
    return () => {
      stop = true;
      window.clearInterval(id);
    };
  }, []);
  return (
    <section className="page">
      <div className="page-head">
        <div>
          <h2>{copy.sources.title}</h2>
          <p className="lede">{copy.sources.lead}</p>
        </div>
        <button type="button" className="btn primary" onClick={onDiscover} disabled={busy}>
          {copy.sources.search}
        </button>
      </div>
      <form
        className="lookup"
        onSubmit={(event) => {
          event.preventDefault();
          onLookup(ip);
        }}
      >
        <label>
          {copy.sources.address}
          <input
            value={ip}
            onChange={(event) => setIp(event.target.value)}
            placeholder="192.168.1.252"
            autoComplete="off"
            spellCheck={false}
          />
        </label>
        <button type="submit" className="btn" disabled={busy || ip.trim() === ""}>
          {copy.sources.lookup}
        </button>
        <p className="hint">{copy.sources.addressHint}</p>
      </form>
      <SourceAdd />
      {error ? <p className="error">{error}</p> : null}
      <h3 className="section-title">Tuners</h3>
      {encoder ? <p className="hint">Picture encoder {encoder}.</p> : null}
      {tuners.length === 0 ? (
        <p className="empty">Tuner status is not available yet.</p>
      ) : (
        <ul className="source-list">
          {tuners.map((tuner) => (
            <li key={tuner.index} className="source-row">
              <span className="ch-num">{tuner.index + 1}</span>
              <span>{tunerLabel(tuner)}</span>
              <span className="codec">
                {tuner.guide || tuner.target
                  ? `Signal ${tuner.strength ?? 0}% · quality ${tuner.quality ?? 0}% · symbols ${tuner.symbol ?? 0}%`
                  : "Idle"}
              </span>
            </li>
          ))}
        </ul>
      )}
      {devices.length === 0 ? <p className="empty">{copy.sources.none}</p> : null}
      <div className="device-grid">
        {devices.map((device) => (
          <article key={device.deviceId} className="device-card">
            <h3>{device.friendlyName || device.modelNumber}</h3>
            <p>
              {device.modelNumber} · {device.tunerCount} {copy.sources.tuners}
            </p>
            <p className="hint">
              {copy.sources.firmware} {device.firmwareVersion}
              {device.upgradeAvailable
                ? `. A newer build, ${device.upgradeAvailable}, is published. This app will not install it.`
                : "."}
            </p>
          </article>
        ))}
      </div>
      <h3 className="section-title">{copy.sources.channels}</h3>
      <ul className="source-list">
        {channels.map((channel) => (
          <li key={channel.id} className={channel.present ? "source-row" : "source-row gone"}>
            <input
              className="num-input"
              aria-label={`${channel.guideName} ${copy.sources.number}`}
              defaultValue={channel.displayNumber}
              key={`${channel.id}-n-${channel.displayNumber}`}
              onBlur={(event) => {
                if (event.target.value.trim() !== channel.displayNumber) {
                  onPatch(channel, { customNumber: event.target.value });
                }
              }}
            />
            <input
              className="name-input"
              aria-label={`${channel.guideNumber} ${copy.sources.name}`}
              defaultValue={channel.displayName}
              key={`${channel.id}-name-${channel.displayName}`}
              onBlur={(event) => {
                if (event.target.value.trim() !== channel.displayName) {
                  onPatch(channel, { customName: event.target.value });
                }
              }}
            />
            <span className="codec">
              {channel.hd ? "HD" : "SD"}
              {channel.videoCodec ? ` ${channel.videoCodec}` : ""}
              {!channel.present ? " · off air" : ""}
            </span>
            <label className="check">
              <input
                type="checkbox"
                checked={channel.enabled}
                onChange={(event) => onPatch(channel, { enabled: event.target.checked })}
              />
              {copy.sources.enabled}
            </label>
            <label className="check">
              <input
                type="checkbox"
                checked={channel.hidden}
                onChange={(event) => onPatch(channel, { hidden: event.target.checked })}
              />
              {copy.sources.hidden}
            </label>
          </li>
        ))}
      </ul>
    </section>
  );
}

function SourceAdd() {
  const [kind, setKind] = useState("m3u");
  const [name, setName] = useState("");
  const [url, setUrl] = useState("");
  const [note, setNote] = useState("");
  return (
    <form
      className="lookup"
      onSubmit={(event) => {
        event.preventDefault();
        void addSource(kind, name, url)
          .then((res) => setNote(kind === "folder" ? `Added ${res.added ?? 0} files to the library.` : "Source added. It shows up with the lineup."))
          .catch((err: unknown) => setNote(err instanceof Error ? err.message : "The source did not add."));
      }}
    >
      <label>
        Add a source
        <select value={kind} onChange={(event) => setKind(event.target.value)}>
          <option value="m3u">Playlist</option>
          <option value="link">Stream link</option>
          <option value="folder">Media folder</option>
        </select>
      </label>
      <label>
        Name
        <input value={name} onChange={(event) => setName(event.target.value)} />
      </label>
      <label>
        Address
        <input value={url} onChange={(event) => setUrl(event.target.value)} placeholder={kind === "folder" ? "D:\\TV" : "https://example/playlist.m3u"} spellCheck={false} />
      </label>
      <button type="submit" className="btn" disabled={url.trim() === ""}>Add</button>
      {note ? <p className="hint">{note}</p> : null}
    </form>
  );
}

function ReserveField({ value, storage, onSave }: { value: string; storage: StorageInfo | null; onSave: (value: string) => void }) {
  const [text, setText] = useState(value);
  useEffect(() => setText(value), [value]);
  return (
    <label className="field">
      {copy.settings.reserve}
      <input
        type="number"
        min={0}
        max={1000000}
        aria-label="Gigabytes to keep free"
        value={text}
        onChange={(event) => setText(event.target.value)}
        onBlur={() => {
          const next = Math.round(Number(text));
          if (!Number.isFinite(next) || next < 0 || next > 1000000) {
            setText(value);
            return;
          }
          setText(String(next));
          if (String(next) !== value) onSave(String(next));
        }}
      />
      <span className="hint">
        {storage ? `${storageLine(storage)} ` : ""}
        {copy.settings.reserveHint}
      </span>
    </label>
  );
}

function storageLine(storage: StorageInfo) {
  const reserve = storage.watermarkGB === 0 ? "The free-space reserve is off." : `New recordings stop under ${storage.watermarkGB} GB.`;
  return `${formatBytes(storage.freeBytes)} free of ${formatBytes(storage.totalBytes)}. ${reserve}`;
}

function RecordingSoon({ channels, now }: { channels: Channel[]; now: Date }) {
  const [items, setItems] = useState<PlannedAiring[]>([]);
  useEffect(() => {
    let cancel = false;
    void getSchedule()
      .then((res) => {
        if (!cancel) setItems(res.items.filter((item) => !item.skipped));
      })
      .catch(() => undefined);
    return () => {
      cancel = true;
    };
  }, []);
  if (items.length === 0) return null;
  return (
    <>
      <h3 className="section-title">Recording soon</h3>
      <div className="fav-row">
        {items.slice(0, 4).map((item) => {
          const channel = channels.find((entry) => entry.id === item.airing.channelId);
          const span = recordWindow(item);
          const minutes = Math.round((span.start.getTime() - now.getTime()) / 60_000);
          return (
            <div key={`${item.passId}-${item.airing.id}`} className="fav-chip">
              <span className="ch-num">{channel?.displayNumber ?? ""}</span>
              <span>{item.airing.title}</span>
              <span className="codec">
                {formatClock(span.start)}–{formatClock(span.end)}
                {minutes > 0 ? ` · in ${minutes} min` : " · now"}
              </span>
            </div>
          );
        })}
      </div>
    </>
  );
}

function UpNext({
  channels,
  airings,
  now,
  onWatch,
}: {
  channels: Channel[];
  airings: Airing[];
  now: Date;
  onWatch: (channel: Channel) => void;
}) {
  const rows = channels
    .map((channel) => {
      const airing = airings
        .filter((item) => item.channelId === channel.id && new Date(item.start).getTime() > now.getTime())
        .sort((a, b) => new Date(a.start).getTime() - new Date(b.start).getTime())[0];
      return airing ? { channel, airing } : null;
    })
    .filter((row): row is { channel: Channel; airing: Airing } => Boolean(row))
    .sort((a, b) => new Date(a.airing.start).getTime() - new Date(b.airing.start).getTime())
    .slice(0, 6);
  if (rows.length === 0) return null;
  return (
    <>
      <h3 className="section-title">Up next</h3>
      <div className="fav-row">
        {rows.map((row) => (
          <button key={row.channel.id} type="button" className="fav-chip" onClick={() => onWatch(row.channel)}>
            <span className="ch-num">{row.channel.displayNumber}</span>
            <span>{row.airing.title}</span>
            <span className="codec">{formatClock(new Date(row.airing.start))}</span>
          </button>
        ))}
      </div>
    </>
  );
}

function OnLater({ channels, airings, now, onWatch }: { channels: Channel[]; airings: Airing[]; now: Date; onWatch: (channel: Channel) => void }) {
  const soon = airings
    .filter((item) => new Date(item.start).getTime() > now.getTime() && new Date(item.start).getTime() < now.getTime() + 12 * 60 * 60_000)
    .slice(0, 8);
  if (soon.length === 0) return null;
  return (
    <>
      <h3 className="section-title">On later</h3>
      <div className="fav-row">
        {soon.map((item) => {
          const channel = channels.find((row) => row.id === item.channelId);
          if (!channel) return null;
          return (
            <button key={item.id} type="button" className="fav-chip" onClick={() => onWatch(channel)}>
              <span className="ch-num">{formatClock(new Date(item.start))}</span>
              <span>{item.title}</span>
              <span className="codec">{channel.displayNumber}{item.subtitle ? ` · ${item.subtitle}` : ""}</span>
            </button>
          );
        })}
      </div>
    </>
  );
}

function recordWindow(item: PlannedAiring) {
  return {
    start: new Date(new Date(item.airing.start).getTime() - (item.padBefore || 0) * 60_000),
    end: new Date(new Date(item.airing.end).getTime() + (item.padAfter || 0) * 60_000),
  };
}

function watched(rec: Recording) {
  if (rec.watched === 1) return true;
  if (rec.watched === 2) return false;
  const position = rec.position ?? 0;
  const duration = rec.durationSec ?? 0;
  if (duration < 10 || position < 1) return false;
  return position >= duration - 15 || position / duration >= 0.9;
}

function formatClockPoint(seconds: number) {
  const total = Math.max(0, Math.floor(seconds));
  const minutes = Math.floor(total / 60);
  const rest = total % 60;
  return `${minutes}:${rest.toString().padStart(2, "0")}`;
}

function formatBytes(bytes: number) {
  if (bytes >= 1e12) return `${(bytes / 1e12).toFixed(1)} TB`;
  if (bytes >= 1e9) return `${(bytes / 1e9).toFixed(1)} GB`;
  if (bytes >= 1e6) return `${(bytes / 1e6).toFixed(1)} MB`;
  if (bytes >= 1e3) return `${(bytes / 1e3).toFixed(0)} KB`;
  return `${bytes} B`;
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

function tunerLabel(tuner: TunerStatus) {
  if (tuner.ours) return `This server${tuner.guide ? ` · ${tuner.guide} ${tuner.name ?? ""}` : ""}`;
  if (tuner.target) return `Another device at ${tuner.target}${tuner.guide ? ` · ${tuner.guide} ${tuner.name ?? ""}` : ""}`;
  if (tuner.guide) return `${tuner.guide} ${tuner.name ?? ""}`;
  return "Free";
}

function currentTitle(airings: Airing[], channelId: number) {
  const now = Date.now();
  const hit = airings.find((airing) => airing.channelId === channelId && new Date(airing.start).getTime() <= now && new Date(airing.end).getTime() > now);
  return hit?.title ?? "";
}

export function SettingsScreen({
  settings,
  storage,
  onChange,
}: {
  settings: Settings;
  storage: StorageInfo | null;
  onChange: (values: Partial<Settings>) => void;
}) {
  const [encoder, setEncoder] = useState("");
  const [tunerLine, setTunerLine] = useState("Checking tuners.");
  const [lastEvent, setLastEvent] = useState("");
  useEffect(() => {
    let stop = false;
    void getTuners()
      .then((res) => {
        if (stop) return;
        setEncoder(res.encoder || "");
        const busy = (res.tuners ?? []).filter((tuner) => tuner.guide || tuner.target).length;
        const total = (res.tuners ?? []).length;
        if (total === 0) setTunerLine("Tuner status is not available yet.");
        else if (busy === 0) setTunerLine(`All ${total} tuners are free.`);
        else setTunerLine(`${busy} of ${total} tuners are in use.`);
      })
      .catch(() => {
        if (!stop) setTunerLine("Tuner status is not available yet.");
      });
    void getEvents()
      .then((res) => {
        if (!stop && res.events[0]) setLastEvent(res.events[0].message);
      })
      .catch(() => undefined);
    return () => {
      stop = true;
    };
  }, []);
  return (
    <section className="page">
      <div className="page-head">
        <h2>{copy.settings.title}</h2>
      </div>
      <h3 className="section-title">Picture</h3>
      <article className="quiet-card wide">
        <h3>This server</h3>
        <p>{encoder ? `Picture encoder ${encoder}.` : "Picture encoder is still being detected."}</p>
        <p>{tunerLine}</p>
        {storage ? <p>{storageLine(storage)}</p> : null}
        {lastEvent ? <p>Latest activity: {lastEvent}</p> : null}
      </article>
      <label className="field">
        {copy.settings.profile}
        <input value="Home" readOnly />
      </label>
      <div className="field">
        <span>Picture</span>
        <div className="segmented" role="group" aria-label="Picture motion">
          {(["broadcast", "smooth", "film"] as const).map((item) => (
            <button
              key={item}
              type="button"
              className={settings.pictureMode === item ? "seg on" : "seg"}
              onClick={() => onChange({ pictureMode: item })}
            >
              {item === "broadcast" ? "Broadcast" : item === "smooth" ? "Smooth" : "Film"}
            </button>
          ))}
        </div>
        <span className="hint">Broadcast rebuilds interlaced channels at 60 frames a second. Smooth adds motion compensation when this server can hold it. Film is for movies.</span>
      </div>
      <h3 className="section-title">DVR</h3>
      <label className="field">
        Play the next episode
        <select value={settings.autoplay || "1"} onChange={(event) => onChange({ autoplay: event.target.value })}>
          <option value="1">On</option>
          <option value="0">Off</option>
        </select>
      </label>
      <h3 className="section-title">Sources</h3>
      <label className="field">
        Offer this server as an HDHomeRun on port 8478
        <select value={settings.hdhrEmulate || "0"} onChange={(event) => onChange({ hdhrEmulate: event.target.value })}>
          <option value="0">Off</option>
          <option value="1">On</option>
        </select>
        <span className="hint">Other apps can add this machine on port 8478. Discovery stays quiet so the real tuner is unchanged. The change applies within a minute.</span>
      </label>
      <h3 className="section-title">Storage</h3>
      <label className="field">
        {copy.settings.layout}
        <select
          value={settings.layout}
          onChange={(event) => onChange({ layout: event.target.value as Settings["layout"] })}
        >
          <option value="auto">Auto</option>
          <option value="desktop">Desktop</option>
          <option value="tv">TV</option>
          <option value="phone">Phone</option>
        </select>
        <span className="hint">{copy.settings.layoutHint}</span>
      </label>
      <label className="field">
        {copy.settings.recordings}
        <input
          value={settings.recordingsPath}
          onChange={(event) => onChange({ recordingsPath: event.target.value })}
          spellCheck={false}
        />
        <span className="hint">{copy.settings.recordingsHint}</span>
      </label>
      <ReserveField value={settings.watermarkGB || "10"} storage={storage} onSave={(watermarkGB) => onChange({ watermarkGB })} />
      <a className="btn" href="/api/backup">
        {copy.settings.backup}
      </a>
      <form
        className="lookup"
        onSubmit={(event) => {
          event.preventDefault();
          const input = event.currentTarget.elements.namedItem("backup") as HTMLInputElement;
          const file = input.files?.[0];
          if (!file) return;
          void fetch("/api/backup", { method: "POST", body: file }).then((res) => {
            if (!res.ok) throw new Error("Restore failed");
            window.location.reload();
          });
        }}
      >
        <label>
          Restore a catalog backup
          <input name="backup" type="file" accept=".db" />
        </label>
        <button type="submit" className="btn">Restore</button>
      </form>
      <p className="hint">{copy.settings.backupHint}</p>
      <article className="quiet-card wide">
        <h3>{copy.settings.passwordTitle}</h3>
        <p>{copy.settings.passwordBody}</p>
      </article>
    </section>
  );
}

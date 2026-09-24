import { useEffect, useState } from "react";
import type { Channel, Device, TunerStatus } from "../../types";
import { addPlaylistFile, addSource, getTuners, lookHarder, startScan, type SourceAdded } from "../../api";
import { copy } from "../../strings";
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
  onPatch: (channel: Channel, patch: { enabled?: boolean; hidden?: boolean; customName?: string; customNumber?: string; guideKey?: string }) => void;
}) {
  const [ip, setIp] = useState("");
  const [looking, setLooking] = useState(false);
  const [scanning, setScanning] = useState("");
  const [looked, setLooked] = useState(false);
  const [hits, setHits] = useState<{ kind: string; name: string; addr: string }[]>([]);
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
        <button type="button" className="btn primary" onClick={onDiscover} disabled={busy || looking}>
          {copy.sources.search}
        </button>
        <button
          type="button"
          className="btn"
          disabled={busy || looking}
          onClick={() => {
            setLooking(true);
            setLooked(false);
            void lookHarder()
              .then((res) => {
                setHits(res.found ?? []);
                setLooked(true);
              })
              .catch(() => {
                setHits([]);
                setLooked(true);
              })
              .finally(() => setLooking(false));
          }}
        >
          {copy.sources.look}
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
      {looking ? <p className="hint">{copy.sources.lookHint}</p> : null}
      {looked && hits.length === 0 ? <p className="empty">{copy.sources.lookEmpty}</p> : null}
      {hits.length > 0 ? (
        <ul className="source-list">
          {hits.map((hit) => (
            <li key={`${hit.kind}-${hit.addr}`} className="source-row">
              <span>{hit.name}</span>
              <span className="codec">{hit.addr}</span>
              <button type="button" className="btn" disabled={busy} onClick={() => onLookup(hit.addr)}>
                {copy.sources.add}
              </button>
            </li>
          ))}
        </ul>
      ) : null}
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
            {device.note ? <p className="hint">{device.note}</p> : null}
            <p className="hint">
              {copy.sources.firmware} {device.firmwareVersion}
              {device.upgradeAvailable
                ? `. A newer build, ${device.upgradeAvailable}, is published. This app will not install it.`
                : "."}
            </p>
            <button
              type="button"
              className="btn"
              disabled={busy || scanning === device.deviceId}
              onClick={() => {
                setScanning(device.deviceId);
                void startScan(device.deviceId).finally(() => setScanning(""));
              }}
            >
              {scanning === device.deviceId ? copy.sources.scanning : copy.sources.scan}
            </button>
          </article>
        ))}
      </div>
      <h3 className="section-title">{copy.sources.channels}</h3>
      <p className="hint">{copy.sources.matchHint}</p>
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
            <input
              className="name-input"
              aria-label={`${channel.guideNumber} ${copy.sources.match}`}
              placeholder={copy.sources.match}
              defaultValue={channel.guideKey ?? ""}
              key={`${channel.id}-guide-${channel.guideKey ?? ""}`}
              onBlur={(event) => {
                if (event.target.value.trim() !== (channel.guideKey ?? "")) {
                  onPatch(channel, { guideKey: event.target.value });
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
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [groups, setGroups] = useState("");
  const [file, setFile] = useState<File | null>(null);
  const [note, setNote] = useState("");
  const [pick, setPick] = useState<SourceAdded | null>(null);
  const [chosen, setChosen] = useState<string[]>([]);
  const options = pick?.groups?.length ? pick.groups : (pick?.channels ?? []).map((ch) => ch.name);
  return (
    <form
      className="lookup"
      onSubmit={(event) => {
        event.preventDefault();
        const fail = (err: unknown) => setNote(err instanceof Error ? err.message : "The source did not add.");
        const finish = (res: SourceAdded) => {
          if (res.pick) {
            setPick(res);
            setNote(res.message || copy.sources.pick);
            return;
          }
          setPick(null);
          setNote(kind === "folder" ? `Added ${res.added ?? 0} files to the library.` : "Source added. It shows up with the lineup.");
        };
        let useGroups = groups;
        let useKeep = "";
        if (pick) {
          if (chosen.length === 0) {
            setNote(copy.sources.pick);
            return;
          }
          if (pick.groups?.length) useGroups = chosen.join(", ");
          else useKeep = chosen.join(", ");
        }
        if (kind === "m3u" && file) {
          void addPlaylistFile(name, useGroups, file, useKeep).then(finish).catch(fail);
          return;
        }
        void addSource(kind, name, url, "", useGroups, useKeep, username, password).then(finish).catch(fail);
      }}
    >
      <label>
        Add a source
        <select value={kind} onChange={(event) => setKind(event.target.value)}>
          <option value="m3u">Playlist</option>
          <option value="xtream">Xtream Codes</option>
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
        <input value={url} onChange={(event) => setUrl(event.target.value)} placeholder={kind === "folder" ? "D:\\TV" : kind === "xtream" ? "http://example:8080" : "https://example/playlist.m3u"} spellCheck={false} />
      </label>
      {kind === "xtream" ? (
        <>
          <label>
            Username
            <input value={username} onChange={(event) => setUsername(event.target.value)} autoComplete="off" />
          </label>
          <label>
            Password
            <input value={password} onChange={(event) => setPassword(event.target.value)} type="password" autoComplete="off" />
          </label>
        </>
      ) : null}
      {kind === "m3u" ? (
        <label>
          {copy.sources.file}
          <input
            type="file"
            accept=".m3u,.m3u8,.gz,application/gzip,application/vnd.apple.mpegurl"
            onChange={(event) => setFile(event.target.files?.[0] ?? null)}
          />
        </label>
      ) : null}
      {kind === "m3u" || kind === "xtream" ? (
        <label>
          {copy.sources.groups}
          <input value={groups} onChange={(event) => setGroups(event.target.value)} placeholder="News, Sports, -Shopping" spellCheck={false} />
        </label>
      ) : null}
      <button type="submit" className="btn" disabled={(url.trim() === "" && file == null) || (kind === "xtream" && (username.trim() === "" || password === ""))}>Add</button>
      {kind === "m3u" || kind === "xtream" ? <p className="hint">{copy.sources.groupsHint}</p> : null}
      {options.length > 0 ? (
        <fieldset>
          <legend>{copy.sources.pick}</legend>
          {options.map((option) => (
            <label className="check" key={option}>
              <input
                type="checkbox"
                checked={chosen.includes(option)}
                onChange={(event) =>
                  setChosen((prev) => (event.target.checked ? [...prev, option] : prev.filter((item) => item !== option)))
                }
              />
              {option}
            </label>
          ))}
        </fieldset>
      ) : null}
      {note ? <p className="hint">{note}</p> : null}
    </form>
  );
}

function tunerLabel(tuner: TunerStatus) {
  if (tuner.ours) return `This server${tuner.guide ? ` · ${tuner.guide} ${tuner.name ?? ""}` : ""}`;
  if (tuner.target) return `Another device at ${tuner.target}${tuner.guide ? ` · ${tuner.guide} ${tuner.name ?? ""}` : ""}`;
  if (tuner.guide) return `${tuner.guide} ${tuner.name ?? ""}`;
  return "Free";
}

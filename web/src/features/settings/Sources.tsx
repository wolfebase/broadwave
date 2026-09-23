import { useEffect, useState } from "react";
import type { Channel, Device, TunerStatus } from "../../types";
import { addSource, getTuners } from "../../api";
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

function tunerLabel(tuner: TunerStatus) {
  if (tuner.ours) return `This server${tuner.guide ? ` · ${tuner.guide} ${tuner.name ?? ""}` : ""}`;
  if (tuner.target) return `Another device at ${tuner.target}${tuner.guide ? ` · ${tuner.guide} ${tuner.name ?? ""}` : ""}`;
  if (tuner.guide) return `${tuner.guide} ${tuner.name ?? ""}`;
  return "Free";
}

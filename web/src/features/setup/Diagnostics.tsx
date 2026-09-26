import { useEffect, useState } from "react";
import { getDeviceHealth } from "../../api";
import type { DeviceHealth } from "../../types";
import { copy } from "../../strings";
import { useDiagnostics } from "./useDiagnostics";
import "./setup.css";

export function DiagnosticsPage() {
  const d = useDiagnostics(undefined, 3000);
  const [health, setHealth] = useState<DeviceHealth[] | null>(null);
  useEffect(() => {
    let dead = false;
    getDeviceHealth()
      .then((body) => {
        if (!dead) setHealth(body.devices);
      })
      .catch(() => {
        if (!dead) setHealth([]);
      });
    return () => {
      dead = true;
    };
  }, []);
  if (!d) return <div className="page-wrap">Checking…</div>;
  return (
    <div className="page-wrap diag">
      <header className="page-header">
        <h1>Diagnostics</h1>
        <p className="page-sub">
          {d.server?.name} · version {d.version || "dev"} · {d.os} · {(d.connectedApps ?? 0) === 1 ? "1 app connected" : `${d.connectedApps ?? 0} apps connected`}
        </p>
      </header>

      <section className="settings-section">
        <h2>Tuners</h2>
        {d.tunerError ? <p className="warn">{d.tunerError}</p> : null}
        <div className="signal-row">
          {(d.tuners ?? []).map((t) => (
            <div key={t.index} className="signal">
              <span>Tuner {t.index + 1}</span>
              <meter min={0} max={100} value={t.strength ?? 0} title="Signal strength" />
              <span className="dim">
                {t.target ? `${t.ours ? "This server" : "Another device"} · ${t.guide ?? ""} ${t.name ?? ""} · signal ${t.strength}% · quality ${t.quality}%` : "Free"}
                {t.viewers ? ` · ${t.viewers} watching` : ""}
              </span>
            </div>
          ))}
        </div>
      </section>

      <section className="settings-section">
        <h2>Tuner health</h2>
        <p className="dim">Model, firmware, and lock. This app does not install firmware.</p>
        {health === null ? <p className="dim">Checking tuners.</p> : null}
        {health !== null && health.length === 0 ? <p className="dim">No tuner answered.</p> : null}
        {(health ?? []).map((device) => (
          <dl key={device.deviceId} className="share-urls">
            <dt>Model</dt>
            <dd>{device.model || "Unknown"}</dd>
            <dt>{copy.sources.firmware}</dt>
            <dd>{device.firmwareVersion || "Unknown"}</dd>
            {(device.tuners ?? []).map((tuner) => (
              <LockLine key={tuner.index} index={tuner.index} locked={tuner.locked} />
            ))}
            {device.error ? (
              <>
                <dt>Status</dt>
                <dd>{device.error}</dd>
              </>
            ) : null}
          </dl>
        ))}
      </section>

      <section className="settings-section">
        <h2>Relay</h2>
        {(d.relay ?? []).length === 0 ? <p className="dim">Nothing is tuned right now.</p> : null}
        <table>
          <tbody>
            {(d.relay ?? []).map((f) => (
              <tr key={f.channelId}>
                <td>
                  <strong>{f.guideNumber}</strong> {f.name}
                </td>
                <td>tuner {f.tuner + 1}</td>
                <td>{f.renditions.map((r) => `${r.key} (${r.viewers})`).join(", ") || "—"}</td>
                <td>{f.recording ? "Recording" : ""}</td>
                <td>{f.exports ? `${f.exports} app streams` : ""}</td>
                <td className="dim">{f.fieldOrder}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </section>

      <section className="settings-section">
        <h2>Encoding and storage</h2>
        <dl className="share-urls">
          <dt>Encoder</dt>
          <dd>
            {d.encoder?.name} {d.encoder?.hardware ? "(hardware)" : "(software)"}
          </dd>
          {d.encoder?.line ? (
            <>
              <dt>Picture</dt>
              <dd>{d.encoder.line}</dd>
            </>
          ) : null}
          <dt>ffmpeg</dt>
          <dd>{d.encoder?.ffmpeg}</dd>
          <dt>Free space</dt>
          <dd>{d.storage ? `${(d.storage.Free / 1e9).toFixed(0)} GB of ${(d.storage.Total / 1e9).toFixed(0)} GB` : "—"}</dd>
          <dt>Guide</dt>
          <dd>
            {d.guide?.channelsWithListings}/{d.guide?.channels} channels listed · {d.guide?.airings} shows
            {d.guide?.listingsUntil ? ` · through ${new Date(d.guide.listingsUntil).toLocaleDateString()}` : ""}
            {d.guide?.nextRefresh ? ` · refreshes ${new Date(d.guide.nextRefresh).toLocaleString([], { month: "short", day: "numeric", hour: "numeric", minute: "2-digit" })}` : ""}
          </dd>
        </dl>
      </section>

      {(d.doctor ?? []).length > 0 ? (
        <section className="settings-section">
          <h2>Fix these</h2>
          <ul className="activity">
            {d.doctor?.map((note) => (
              <li key={note.id}>{note.message}</li>
            ))}
          </ul>
        </section>
      ) : null}

      <section className="settings-section" aria-label="Feed counts">
        <h2>Feeds</h2>
        <p>{feedSummary(d.feeds ?? [])}</p>
        {(d.feeds ?? []).length > 0 ? (
          <ul className="activity">
            {(d.feeds ?? []).map((f) => (
              <li key={f.channelId} className="feed-line">
                <strong>{f.guideNumber}</strong> {f.name} · {f.viewers} watching · {countPhrase(f.ffmpeg, "ffmpeg process", "ffmpeg processes")}
                {f.recording ? " · Recording" : ""}
              </li>
            ))}
          </ul>
        ) : null}
      </section>

      <section className="settings-section">
        <h2>Logs</h2>
        {(d.logs ?? []).length === 0 ? (
          <p className="dim">No log lines yet.</p>
        ) : (
          <pre className="log-view" tabIndex={0} aria-label="Recent logs">
            {d.logs?.join("\n")}
          </pre>
        )}
      </section>

      <section className="settings-section">
        <h2>Recent activity</h2>
        <ul className="activity">
          {(d.recentActivity ?? []).map((e) => (
            <li key={e.id}>
              <time>{new Date(e.at).toLocaleString([], { month: "short", day: "numeric", hour: "numeric", minute: "2-digit" })}</time> {e.message}
            </li>
          ))}
        </ul>
      </section>
    </div>
  );
}

function countPhrase(n: number, one: string, many: string) {
  return `${n} ${n === 1 ? one : many}`;
}

function feedSummary(feeds: { viewers: number; ffmpeg: number }[]) {
  let viewers = 0;
  let ffmpeg = 0;
  for (const feed of feeds) {
    viewers += feed.viewers;
    ffmpeg += feed.ffmpeg;
  }
  return `${countPhrase(feeds.length, "channel", "channels")} · ${viewers} watching · ${countPhrase(ffmpeg, "ffmpeg process", "ffmpeg processes")}`;
}

function LockLine({ index, locked }: { index: number; locked: boolean }) {
  return (
    <>
      <dt>Tuner {index + 1}</dt>
      <dd>{locked ? "Locked" : "Not locked"}</dd>
    </>
  );
}

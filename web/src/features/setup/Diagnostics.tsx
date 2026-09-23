import { useDiagnostics } from "./useDiagnostics";
import "./setup.css";

export function DiagnosticsPage() {
  const d = useDiagnostics(undefined, 3000);
  if (!d) return <div className="page-wrap">Checking…</div>;
  return (
    <div className="page-wrap diag">
      <header className="page-header">
        <h1>Diagnostics</h1>
        <p className="page-sub">
          {d.server?.name} · version {d.version || "dev"} · {d.os} · {d.connectedApps ?? 0} apps connected
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

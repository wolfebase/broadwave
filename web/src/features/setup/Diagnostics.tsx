import { useEffect, useState } from "react";
import { getDeviceHealth } from "../../api";
import type { DeviceHealth } from "../../types";
import { setupMessages } from "../../i18n/messages/setup.ts";
import { bind, formatDateTime } from "../../i18n/runtime.ts";
import { copy } from "../../strings";
import { useDiagnostics, type ChannelStart } from "./useDiagnostics";
import "./setup.css";

const t = bind(setupMessages);

const activityTime: Intl.DateTimeFormatOptions = { month: "short", day: "numeric", hour: "numeric", minute: "2-digit" };
const dayStamp: Intl.DateTimeFormatOptions = { year: "numeric", month: "numeric", day: "numeric" };

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
  if (!d) return <div className="page-wrap">{t("diag.checking")}</div>;
  return (
    <div className="page-wrap diag">
      <header className="page-header">
        <h1>{t("diag.title")}</h1>
        <p className="page-sub">
          {d.server?.name}
          {" · "}
          {d.version ? t("diag.version", { version: d.version }) : t("diag.versionDev")}
          {" · "}
          {d.os}
          {" · "}
          {t("diag.apps", d.connectedApps ?? 0)}
        </p>
      </header>

      <section className="settings-section">
        <h2>{t("diag.tuners")}</h2>
        {d.tunerError ? <p className="warn" role="alert">{d.tunerError}</p> : null}
        <div className="signal-row">
          {(d.tuners ?? []).map((tuner) => (
            <div key={tuner.index} className="signal">
              <span>{t("diag.tuner", { number: tuner.index + 1 })}</span>
              <meter min={0} max={100} value={tuner.strength ?? 0} aria-label={t("diag.signalAria", { number: tuner.index + 1 })} title={t("diag.signalStrength")} />
              <span className="dim">
                {tuner.target ? (
                  <>
                    {tuner.ours ? t("diag.thisServer") : t("diag.otherDevice")}
                    {" · "}
                    {tunedLabel(tuner.guide, tuner.name)}
                    {t("diag.signalQuality", { strength: `${tuner.strength}`, quality: `${tuner.quality}` })}
                  </>
                ) : (
                  t("diag.free")
                )}
                {tuner.viewers ? t("diag.watchingSuffix", { count: tuner.viewers }) : ""}
              </span>
            </div>
          ))}
        </div>
      </section>

      <section className="settings-section">
        <h2>{t("diag.health")}</h2>
        <p className="dim">{t("diag.healthLead")}</p>
        {health === null ? <p className="dim">{t("diag.checkingTuners")}</p> : null}
        {health !== null && health.length === 0 ? <p className="dim">{t("diag.noTuner")}</p> : null}
        {(health ?? []).map((device) => (
          <dl key={device.deviceId} className="share-urls">
            <dt>{t("diag.model")}</dt>
            <dd>{device.model || t("diag.unknown")}</dd>
            <dt>{copy.sources.firmware}</dt>
            <dd>{device.firmwareVersion || t("diag.unknown")}</dd>
            {(device.tuners ?? []).map((tuner) => (
              <LockLine key={tuner.index} index={tuner.index} locked={tuner.locked} />
            ))}
            {device.error ? (
              <>
                <dt>{t("diag.status")}</dt>
                <dd>{device.error}</dd>
              </>
            ) : null}
          </dl>
        ))}
      </section>

      <section className="settings-section">
        <h2>{t("diag.relay")}</h2>
        {(d.relay ?? []).length === 0 ? <p className="dim">{t("diag.nothingTuned")}</p> : null}
        {/* A phone scrolls the table, not the page. A keyboard reaches it to scroll. */}
        <div className="diag-table" role="region" aria-label={t("diag.relay")} tabIndex={0}>
          <table>
            <tbody>
              {(d.relay ?? []).map((f) => (
                <tr key={f.channelId}>
                  <td>
                    <strong>{f.guideNumber}</strong> {f.name}
                  </td>
                  <td>{t("diag.tunerLower", { number: f.tuner + 1 })}</td>
                  <td>{f.renditions.map((r) => `${r.key} (${r.viewers})`).join(", ") || "—"}</td>
                  <td>{f.recording ? t("diag.recording") : ""}</td>
                  <td>{f.exports ? t("diag.appStreams", { count: f.exports }) : ""}</td>
                  <td className="dim">{f.fieldOrder}</td>
                  <td className="dim">{bufferLine(f.buffer)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </section>

      <section className="settings-section" aria-label={t("diag.starts")}>
        <h2>{t("diag.starts")}</h2>
        <p className="dim">{t("diag.startsLead")}</p>
        {(d.starts ?? []).length === 0 ? <p className="dim">{t("diag.noStarts")}</p> : null}
        <ul className="activity">
          {(d.starts ?? []).map((s) => (
            <li key={`${s.at}-${s.channelId}-${s.rendition}`}>
              <time>{shortTime(s.at)}</time> <strong>{s.guideNumber}</strong> {startLine(s)}
            </li>
          ))}
        </ul>
      </section>

      <section className="settings-section">
        <h2>{t("diag.encoding")}</h2>
        <dl className="share-urls">
          <dt>{t("diag.encoder")}</dt>
          <dd>
            {d.encoder?.name} {d.encoder?.hardware ? t("diag.hardware") : t("diag.software")}
          </dd>
          {d.encoder?.line ? (
            <>
              <dt>{t("diag.picture")}</dt>
              <dd>{d.encoder.line}</dd>
            </>
          ) : null}
          <dt>{t("diag.ffmpeg")}</dt>
          <dd>{d.encoder?.ffmpeg}</dd>
          {d.encoder?.ac4 === false ? (
            <>
              <dt>{t("diag.ac4Label")}</dt>
              <dd>{t("diag.ac4")}</dd>
            </>
          ) : null}
          <dt>{t("diag.freeSpaceLabel")}</dt>
          <dd>{d.storage ? t("diag.freeSpace", { free: (d.storage.Free / 1e9).toFixed(0), total: (d.storage.Total / 1e9).toFixed(0) }) : "—"}</dd>
          <dt>{t("diag.guide")}</dt>
          <dd>
            {t("diag.guideCounts", {
              listed: d.guide ? d.guide.channelsWithListings : "",
              channels: d.guide ? d.guide.channels : "",
              shows: d.guide ? d.guide.airings : "",
            })}
            {d.guide?.listingsUntil ? t("diag.guideThrough", { day: formatDateTime(new Date(d.guide.listingsUntil), dayStamp) }) : ""}
            {d.guide?.nextRefresh ? t("diag.guideRefresh", { when: shortTime(d.guide.nextRefresh) }) : ""}
          </dd>
          {d.guide?.lastError ? (
            <>
              <dt>{t("diag.lastPull")}</dt>
              <dd className="warn" role="alert">
                {d.guide.lastErrorAt ? t("diag.failedAt", { when: shortTime(d.guide.lastErrorAt) }) : t("diag.failed")}
                {d.guide.lastError}
              </dd>
            </>
          ) : null}
        </dl>
      </section>

      {(d.doctor ?? []).length > 0 ? (
        <section className="settings-section">
          <h2>{t("diag.fix")}</h2>
          <ul className="activity">
            {d.doctor?.map((note) => (
              <li key={note.id}>{note.message}</li>
            ))}
          </ul>
        </section>
      ) : null}

      <section className="settings-section" aria-label={t("diag.feedsAria")}>
        <h2>{t("diag.feeds")}</h2>
        <p>{feedSummary(d.feeds ?? [])}</p>
        {(d.feeds ?? []).length > 0 ? (
          <ul className="activity">
            {(d.feeds ?? []).map((f) => (
              <li key={f.channelId} className="feed-line">
                <strong>{f.guideNumber}</strong> {f.name}
                {" · "}
                {t("diag.watching", { count: f.viewers })}
                {" · "}
                {t("diag.ffmpegCount", f.ffmpeg)}
                {f.recording ? t("diag.recordingMark") : ""}
              </li>
            ))}
          </ul>
        ) : null}
      </section>

      <section className="settings-section">
        <h2>{t("diag.logs")}</h2>
        {(d.logs ?? []).length === 0 ? (
          <p className="dim">{t("diag.noLogs")}</p>
        ) : (
          <pre className="log-view" tabIndex={0} aria-label={t("diag.logsAria")}>
            {d.logs?.join("\n")}
          </pre>
        )}
      </section>

      <section className="settings-section">
        <h2>{t("diag.activity")}</h2>
        <ul className="activity">
          {(d.recentActivity ?? []).map((e) => (
            <li key={e.id}>
              <time>{shortTime(e.at)}</time> {e.message}
            </li>
          ))}
        </ul>
      </section>
    </div>
  );
}

/** A narrow screen wraps between steps, never between a number and its unit. */
function seconds(n: number) {
  return n.toFixed(1);
}

/** "3.8 s · tuner 0.5 s · keyframe 1.3 s · encoder 1.0 s · first segment 1.0 s" */
export function startLine(s: ChannelStart) {
  const parts = [t("diag.seconds", { value: seconds(s.seconds) })];
  if (s.tune) parts.push(t("diag.tune", { value: seconds(s.tune) }));
  parts.push(t("diag.keyframe", { value: seconds(s.keyframe) }), t("diag.encoderTime", { value: seconds(s.encoder) }), t("diag.segment", { value: seconds(s.segment) }));
  return parts.join(" · ");
}

function feedSummary(feeds: { viewers: number; ffmpeg: number }[]) {
  let viewers = 0;
  let ffmpeg = 0;
  for (const feed of feeds) {
    viewers += feed.viewers;
    ffmpeg += feed.ffmpeg;
  }
  return `${t("diag.feedChannels", feeds.length)} · ${t("diag.watching", { count: viewers })} · ${t("diag.ffmpegCount", ffmpeg)}`;
}

function LockLine({ index, locked }: { index: number; locked: boolean }) {
  return (
    <>
      <dt>{t("diag.tuner", { number: index + 1 })}</dt>
      <dd>{locked ? t("diag.locked") : t("diag.notLocked")}</dd>
    </>
  );
}

function shortTime(iso: string) {
  return formatDateTime(new Date(iso), activityTime);
}

/** A tuner can report its channel name as the number; say it once. */
function tunedLabel(guide?: string, name?: string) {
  const g = (guide ?? "").trim();
  const n = (name ?? "").trim();
  return n && n !== g ? `${g} ${n}`.trim() : g;
}

function bufferLine(b?: { minutes: number; bytes: number; state: "on" | "off" | "full" }) {
  if (!b) return "";
  if (b.state === "off") return t("diag.bufferOff");
  if (b.state === "full") return t("diag.bufferFull");
  return t("diag.buffer", { minutes: Math.floor(b.minutes), size: (b.bytes / 1e9).toFixed(1) });
}

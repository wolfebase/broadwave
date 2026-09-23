import qrcode from "qrcode-generator";
import { useEffect, useMemo, useRef, useState } from "react";
import { refreshGuide } from "../../api";
import { useData } from "../../app/data";
import { navigate } from "../../app/router";
import { ChevronIcon } from "../../ui/icons";
import { useDiagnostics } from "./useDiagnostics";
import "./setup.css";

type Step = "tuner" | "guide" | "recordings" | "apps";
const steps: { id: Step; title: string }[] = [
  { id: "tuner", title: "Tuner" },
  { id: "guide", title: "Guide" },
  { id: "recordings", title: "Recordings" },
  { id: "apps", title: "Apps" },
];

/** First run: find the tuner, fill the guide, check storage and encoding, connect the apps. */
export function Setup() {
  const { devices, channels, rediscover, refresh, saveSettings, settings, storage } = useData();
  const [step, setStep] = useState<Step>("tuner");
  const [address, setAddress] = useState("");
  const [busy, setBusy] = useState(false);
  const [guideNote, setGuideNote] = useState("");
  const diag = useDiagnostics(step);
  const pulledGuide = useRef(false);

  useEffect(() => {
    if (devices.length === 0) void rediscover();
    // Search once when the wizard opens.
  }, []); // eslint-disable-line react-hooks/exhaustive-deps

  // Startup refresh runs before a tuner added by hand exists, so the guide
  // step loads listings itself the first time it opens on an empty catalog.
  useEffect(() => {
    if (step !== "guide" || devices.length === 0 || pulledGuide.current) return;
    if (!diag?.guide) return;
    if (diag.guide.airings > 0) {
      pulledGuide.current = true;
      return;
    }
    pulledGuide.current = true;
    setBusy(true);
    void refreshGuide()
      .then((r) => setGuideNote(`Loaded ${r.airings} listings.`))
      .catch((e: unknown) => setGuideNote(e instanceof Error ? e.message : "Listings did not load."))
      .finally(() => {
        setBusy(false);
        void refresh(["airings"]);
      });
  }, [step, devices.length, diag, refresh]);

  const index = steps.findIndex((s) => s.id === step);
  const next = () => setStep(steps[Math.min(steps.length - 1, index + 1)].id);

  async function finish() {
    await saveSettings({ setupComplete: "1" });
    navigate("/");
  }

  return (
    <div className="setup">
      <header className="setup-head">
        <span className="brand-tally" aria-hidden="true" />
        <h1>Let's set up your TV</h1>
        <ol className="setup-steps">
          {steps.map((s, i) => (
            <li key={s.id} className={i < index ? "done" : i === index ? "on" : ""}>
              <button type="button" onClick={() => setStep(s.id)}>
                {s.title}
              </button>
            </li>
          ))}
        </ol>
      </header>

      {step === "tuner" ? (
        <section className="setup-card glass">
          <h2>Find your HDHomeRun</h2>
          <p className="dim">It needs to be plugged into your network and an antenna.</p>
          {devices.length > 0 ? (
            <ul className="setup-list">
              {devices.map((d) => (
                <li key={d.deviceId}>
                  <strong>{d.friendlyName || d.modelNumber}</strong>
                  <span className="dim">
                    {d.tunerCount} tuners · {channels.length} channels · {d.baseUrl.replace("http://", "")}
                  </span>
                  <span className="ok">Found</span>
                </li>
              ))}
            </ul>
          ) : (
            <p>{busy ? "Searching…" : "No tuner answered yet."}</p>
          )}
          {diag?.tuners?.length ? (
            <div className="signal-row">
              {diag.tuners.map((t) => (
                <div key={t.index} className="signal">
                  <span>Tuner {t.index + 1}</span>
                  <meter min={0} max={100} value={t.strength ?? 0} />
                  <span className="dim">{t.target ? (t.ours ? `Watching ${t.guide ?? ""}` : "In use by another device") : "Free"}</span>
                </div>
              ))}
            </div>
          ) : null}
          <div className="setup-row">
            <button
              type="button"
              className="btn"
              onClick={() => {
                setBusy(true);
                void rediscover().finally(() => setBusy(false));
              }}
            >
              Search again
            </button>
            <input type="text" placeholder="Or type its address, like 192.168.1.50" value={address} onChange={(e) => setAddress(e.target.value)} />
            <button
              type="button"
              className="btn"
              disabled={!address}
              onClick={() => {
                setBusy(true);
                void rediscover(address).finally(() => setBusy(false));
              }}
            >
              Add
            </button>
          </div>
          <Footer onNext={next} canNext={devices.length > 0} />
        </section>
      ) : null}

      {step === "guide" ? (
        <section className="setup-card glass">
          <h2>Fill the guide</h2>
          <p className="dim">Listings come free from SiliconDust through your tuner. They refresh about once a day.</p>
          {diag?.guide ? (
            <div className="stat-row">
              <Stat value={diag.guide.channels} label="channels" />
              <Stat value={diag.guide.channelsWithListings} label="with listings" />
              <Stat value={diag.guide.airings} label="shows listed" />
              <Stat value={diag.guide.listingsUntil ? daysAhead(diag.guide.listingsUntil) : "—"} label="days ahead" />
            </div>
          ) : null}
          {guideNote ? <p className="dim">{guideNote}</p> : null}
          <div className="setup-row">
            <button
              type="button"
              className="btn"
              disabled={busy}
              onClick={() => {
                setBusy(true);
                void refreshGuide()
                  .then((r) => setGuideNote(`Loaded ${r.airings} listings.`))
                  .catch((e: unknown) => setGuideNote(e instanceof Error ? e.message : "Listings did not load."))
                  .finally(() => {
                    setBusy(false);
                    void refresh(["airings"]);
                  });
              }}
            >
              {busy ? "Loading…" : "Reload listings"}
            </button>
          </div>
          <Footer onNext={next} canNext />
        </section>
      ) : null}

      {step === "recordings" ? (
        <section className="setup-card glass">
          <h2>Recordings and playback</h2>
          <div className="stat-row">
            <Stat value={storage ? formatTB(storage.freeBytes) : "—"} label="free for recordings" />
            <Stat value={diag?.encoder ? (diag.encoder.hardware ? "Hardware" : "Software") : "—"} label={diag?.encoder?.name?.replace("h264_", "").toUpperCase() ?? "encoding"} />
          </div>
          {diag?.encoder && !diag.encoder.hardware ? (
            <p className="warn">
              This server is converting video on the CPU. It works, but Intel or AMD graphics (pass <code>/dev/dri</code> to the container) or an NVIDIA card makes it faster and quieter.
            </p>
          ) : null}
          <label className="field">
            Keep this much space free
            <select value={settings.watermarkGB} onChange={(e) => void saveSettings({ watermarkGB: e.target.value })}>
              {["0", "10", "25", "50", "100"].map((v) => (
                <option key={v} value={v}>
                  {v === "0" ? "No reserve" : `${v} GB`}
                </option>
              ))}
            </select>
          </label>
          <p className="dim">Recordings are the original broadcast files, saved in the folder you mapped to /config/work/recordings.</p>
          <Footer onNext={next} canNext />
        </section>
      ) : null}

      {step === "apps" ? <AppsStep onFinish={finish} /> : null}
    </div>
  );
}

function AppsStep({ onFinish }: { onFinish: () => void }) {
  const url = window.location.origin;
  const svg = useMemo(() => {
    const qr = qrcode(0, "M");
    qr.addData(`waveguide://connect?url=${encodeURIComponent(url)}`);
    qr.make();
    return qr.createSvgTag({ cellSize: 6, margin: 2, scalable: true });
  }, [url]);
  return (
    <section className="setup-card glass">
      <h2>Watch on iPhone and Apple TV</h2>
      <p className="dim">The apps find this server on their own. If one doesn't, scan this with your iPhone's camera, or type the address.</p>
      <div className="apps-row">
        <div className="qr" dangerouslySetInnerHTML={{ __html: svg }} aria-label="QR code to connect the app" />
        <div>
          <p className="big-url">{url}</p>
          <p className="dim">Plex, Jellyfin, and Channels can use this server too: turn on sharing in Settings.</p>
        </div>
      </div>
      <div className="setup-foot">
        <button type="button" className="btn primary big" onClick={onFinish}>
          Start watching <ChevronIcon />
        </button>
      </div>
    </section>
  );
}

function Footer({ onNext, canNext }: { onNext: () => void; canNext: boolean }) {
  return (
    <div className="setup-foot">
      <button type="button" className="btn primary" disabled={!canNext} onClick={onNext}>
        Continue <ChevronIcon />
      </button>
    </div>
  );
}

function Stat({ value, label }: { value: number | string; label: string }) {
  return (
    <div className="stat">
      <strong>{value}</strong>
      <span>{label}</span>
    </div>
  );
}

function daysAhead(iso: string) {
  return Math.max(0, Math.round((Date.parse(iso) - Date.now()) / 86_400_000));
}

function formatTB(bytes: number) {
  return bytes >= 1e12 ? `${(bytes / 1e12).toFixed(1)} TB` : `${Math.round(bytes / 1e9)} GB`;
}

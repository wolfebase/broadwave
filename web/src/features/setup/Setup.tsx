import qrcode from "qrcode-generator";
import { useEffect, useMemo, useRef, useState } from "react";
import { addFree, addPlaylistFile, addSource, findFree, lookHarder, refreshGuide, startScan, type FreeFeed } from "../../api";
import { useData } from "../../app/data";
import { navigate } from "../../app/router";
import { ChevronIcon } from "../../ui/icons";
import type { Channel, Device } from "../../types";
import { useDiagnostics } from "./useDiagnostics";
import "./setup.css";

type Step = "sources" | "channels" | "guide" | "recordings" | "apps";
const steps: { id: Step; title: string }[] = [
  { id: "sources", title: "Sources" },
  { id: "channels", title: "Channels" },
  { id: "guide", title: "Guide" },
  { id: "recordings", title: "Recordings" },
  { id: "apps", title: "Apps" },
];

/** First run: find a tuner, pick channels, check the guide, then the apps. */
export function Setup() {
  const { devices, channels, allChannels, airings, rediscover, refresh, saveSettings, settings, storage, editChannel } = useData();
  const [step, setStep] = useState<Step>("sources");
  const [address, setAddress] = useState("");
  const [busy, setBusy] = useState(false);
  const [note, setNote] = useState("");
  const [hits, setHits] = useState<{ kind: string; name: string; addr: string }[]>([]);
  const [feeds, setFeeds] = useState<FreeFeed[]>([]);
  const [freeGuide, setFreeGuide] = useState("");
  const [playlistURL, setPlaylistURL] = useState("");
  const [xtreamUser, setXtreamUser] = useState("");
  const [xtreamPass, setXtreamPass] = useState("");
  const diag = useDiagnostics(step === "recordings" || step === "guide" ? step : "guide");
  const pulledGuide = useRef(false);
  const scanned = useRef(false);
  const starred = useRef(false);

  useEffect(() => {
    if (devices.length === 0) void rediscover();
  }, []); // eslint-disable-line react-hooks/exhaustive-deps

  useEffect(() => {
    if (step !== "channels" || scanned.current) return;
    const tuner = devices.find((d) => d.tunerCount > 0);
    if (!tuner || channels.length > 0) return;
    scanned.current = true;
    queueMicrotask(() => setBusy(true));
    void startScan(tuner.deviceId)
      .then(() => refresh(["channels", "devices"]))
      .catch((e: unknown) => setNote(e instanceof Error ? e.message : "The scan did not start."))
      .finally(() => setBusy(false));
  }, [step, devices, channels.length, refresh]);

  useEffect(() => {
    if (step !== "channels" || starred.current || channels.length === 0) return;
    const picks = channels.filter((ch) => bigFour(ch) && !ch.favorite && !ch.hidden);
    if (picks.length === 0) {
      starred.current = true;
      return;
    }
    starred.current = true;
    void Promise.all(picks.map((ch) => editChannel(ch, { favorite: true }))).then(() => refresh(["channels"]));
  }, [step, channels, editChannel, refresh]);

  useEffect(() => {
    if (step !== "guide" || devices.length === 0 || pulledGuide.current) return;
    if (!diag?.guide) return;
    if (diag.guide.airings > 0) {
      pulledGuide.current = true;
      return;
    }
    pulledGuide.current = true;
    queueMicrotask(() => setBusy(true));
    void refreshGuide()
      .then((r) => setNote(`Loaded ${r.airings} listings.`))
      .catch((e: unknown) => setNote(e instanceof Error ? e.message : "Listings did not load."))
      .finally(() => {
        setBusy(false);
        void refresh(["airings"]);
      });
  }, [step, devices.length, diag, refresh]);

  const index = steps.findIndex((s) => s.id === step);
  const next = () => setStep(steps[Math.min(steps.length - 1, index + 1)].id);
  const listed = allChannels.filter((ch) => !ch.hidden);

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

      {step === "sources" ? (
        <section className="setup-card glass">
          <h2>Looking for your tuner…</h2>
          <p className="dim">An HDHomeRun on this network is added for you. Everything else waits for a tap.</p>
          {devices.length > 0 ? (
            <ul className="setup-list">
              {devices.map((d) => (
                <li key={d.deviceId}>
                  <strong>{d.friendlyName || d.modelNumber}</strong>
                  <span className="dim">
                    {d.tunerCount > 0 ? `${d.tunerCount} tuners` : "Streamed"} · {channels.filter((c) => c.deviceId === d.deviceId).length} channels
                  </span>
                  <span className="ok">Found</span>
                </li>
              ))}
            </ul>
          ) : (
            <p>{busy ? "Searching…" : "No tuner answered yet."}</p>
          )}
          <div className="setup-row">
            <button
              type="button"
              className="btn"
              disabled={busy}
              onClick={() => {
                setBusy(true);
                void lookHarder()
                  .then((res) => setHits(res.found ?? []))
                  .catch(() => setHits([]))
                  .finally(() => setBusy(false));
              }}
            >
              Look harder
            </button>
            <button
              type="button"
              className="btn"
              disabled={busy}
              onClick={() => {
                setBusy(true);
                setFreeGuide("");
                void findFree()
                  .then((res) => {
                    setFeeds(res.found ?? []);
                    setFreeGuide(res.found?.length ? "" : res.guide);
                  })
                  .catch(() => setNote("No free-channel server answered."))
                  .finally(() => setBusy(false));
              }}
            >
              Add free channels
            </button>
          </div>
          {hits.length > 0 ? (
            <ul className="setup-list">
              {hits.map((hit) => (
                <li key={`${hit.kind}-${hit.addr}`}>
                  <strong>{hit.name}</strong>
                  <span className="dim">{hit.addr}</span>
                  <button type="button" className="btn" onClick={() => void addHit(hit, rediscover, setNote, refresh)}>
                    Add
                  </button>
                </li>
              ))}
            </ul>
          ) : null}
          {feeds.map((feed) => (
            <div key={feed.playlist} className="setup-row">
              <span>{feed.name}</span>
              <button
                type="button"
                className="btn"
                onClick={() => {
                  setBusy(true);
                  void addFree(feed)
                    .then((res) => {
                      setNote(res.message || "Source added. It shows up with the lineup.");
                      return refresh(["devices", "channels"]);
                    })
                    .catch((e: unknown) => setNote(e instanceof Error ? e.message : "That feed did not add."))
                    .finally(() => setBusy(false));
                }}
              >
                Add
              </button>
            </div>
          ))}
          {freeGuide ? <pre className="hint">{freeGuide}</pre> : null}
          <form
            className="setup-form"
            onSubmit={(event) => {
              event.preventDefault();
              setBusy(true);
              const kind = xtreamUser ? "xtream" : "m3u";
              void addSource(kind, "", playlistURL, "", "", "", xtreamUser, xtreamPass)
                .then(() => refresh(["devices", "channels"]))
                .then(() => setNote("Playlist added."))
                .catch((e: unknown) => setNote(e instanceof Error ? e.message : "That playlist did not add."))
                .finally(() => setBusy(false));
            }}
          >
            <label>
              Playlist or Xtream server
              <input value={playlistURL} onChange={(e) => setPlaylistURL(e.target.value)} placeholder="https://example/playlist.m3u" spellCheck={false} />
            </label>
            <label>
              Username
              <input value={xtreamUser} onChange={(e) => setXtreamUser(e.target.value)} autoComplete="off" />
            </label>
            <label>
              Password
              <input type="password" value={xtreamPass} onChange={(e) => setXtreamPass(e.target.value)} autoComplete="off" />
            </label>
            <label>
              Playlist file
              <input
                type="file"
                accept=".m3u,.m3u8"
                onChange={(e) => {
                  const file = e.target.files?.[0];
                  if (!file) return;
                  setBusy(true);
                  void addPlaylistFile("", "", file)
                    .then(() => refresh(["devices", "channels"]))
                    .then(() => setNote("Playlist added."))
                    .catch((err: unknown) => setNote(err instanceof Error ? err.message : "That file did not add."))
                    .finally(() => setBusy(false));
                }}
              />
            </label>
            <button type="submit" className="btn" disabled={busy || playlistURL.trim() === ""}>
              Add playlist
            </button>
          </form>
          <div className="setup-row">
            <input type="text" placeholder="Tuner address, like 192.168.1.50" value={address} onChange={(e) => setAddress(e.target.value)} />
            <button
              type="button"
              className="btn"
              disabled={!address || busy}
              onClick={() => {
                setBusy(true);
                void rediscover(address)
                  .catch((e: unknown) => setNote(e instanceof Error ? e.message : "Nothing answered at that address."))
                  .finally(() => setBusy(false));
              }}
            >
              Add by address
            </button>
          </div>
          {note ? <p className="dim">{note}</p> : null}
          <Footer onNext={next} canNext={devices.length > 0 || channels.length > 0} />
        </section>
      ) : null}

      {step === "channels" ? (
        <section className="setup-card glass">
          <h2>Choose your channels</h2>
          <p className="dim">{busy ? "Scanning for channels." : "ABC, CBS, FOX, and NBC are already favorites when we can tell."}</p>
          <ul className="setup-channels">
            {listed.map((ch) => (
              <li key={ch.id}>
                {ch.artUrl ? <img src={ch.artUrl} alt="" /> : <span className="ch-fallback">{ch.displayNumber || ch.guideNumber}</span>}
                <span>{ch.displayName || ch.guideName}</span>
                <button type="button" className="btn" aria-pressed={ch.favorite} onClick={() => void editChannel(ch, { favorite: !ch.favorite })}>
                  {ch.favorite ? "Favorite" : "Add favorite"}
                </button>
              </li>
            ))}
          </ul>
          {listed.length === 0 ? <p>No channels yet.</p> : null}
          <div className="setup-row">
            <button type="button" className="btn" onClick={() => void hideExtras(allChannels, editChannel, refresh, setNote)}>
              Hide duplicates and shopping
            </button>
          </div>
          {note ? <p className="dim">{note}</p> : null}
          <Footer onNext={next} canNext={listed.length > 0} />
        </section>
      ) : null}

      {step === "guide" ? (
        <section className="setup-card glass">
          <h2>Guide coverage</h2>
          <ul className="setup-list">
            {coverage(devices, allChannels, airings).map((row) => (
              <li key={row.id}>
                <strong>{row.name}</strong>
                <span className="dim">
                  {row.channels} channels · {row.withListings} with listings
                </span>
              </li>
            ))}
          </ul>
          {note ? <p className="dim">{note}</p> : null}
          <Footer onNext={next} canNext />
        </section>
      ) : null}

      {step === "recordings" ? (
        <section className="setup-card glass">
          <h2>Where recordings go</h2>
          <div className="stat-row">
            <Stat value={storage ? formatTB(storage.freeBytes) : "—"} label="free" />
            <Stat value={diag?.encoder ? (diag.encoder.hardware ? "Hardware" : "Software") : "—"} label={diag?.encoder?.name?.replace("h264_", "").toUpperCase() ?? "encoding"} />
          </div>
          <p className="dim">{settings.recordingsPath || "Recordings save in the folder mapped for this server."}</p>
          {storage && storage.freeBytes < 20 * 1e9 ? <p className="warn">Less than 20 GB is free. Free some space before a long recording.</p> : null}
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
      <p className="dim">Scan this with your iPhone. On Apple TV, open Waveguide. It finds this server on its own.</p>
      <div className="apps-row">
        <div className="qr" dangerouslySetInnerHTML={{ __html: svg }} aria-label="QR code to connect the app" />
        <div>
          <p className="big-url">{url}</p>
          <p className="dim">Open Waveguide on your Apple TV.</p>
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

function formatTB(bytes: number) {
  return bytes >= 1e12 ? `${(bytes / 1e12).toFixed(1)} TB` : `${Math.round(bytes / 1e9)} GB`;
}

function bigFour(ch: Channel) {
  const name = `${ch.guideName} ${ch.displayName}`.toUpperCase();
  return /\b(ABC|CBS|FOX|NBC)\b/.test(name);
}

function shopping(ch: Channel) {
  return /shop|qvc|hsn|jewelry/i.test(`${ch.guideName} ${ch.displayName}`);
}

async function hideExtras(
  channels: Channel[],
  edit: (channel: Channel, patch: { hidden?: boolean }) => Promise<void>,
  refresh: (what?: ("channels")[]) => Promise<void>,
  setNote: (note: string) => void,
) {
  const seen = new Set<string>();
  const hide: Channel[] = [];
  for (const ch of channels) {
    if (ch.hidden) continue;
    if (shopping(ch)) {
      hide.push(ch);
      continue;
    }
    const key = `${ch.guideNumber}|${ch.guideName}`.toLowerCase();
    if (seen.has(key)) hide.push(ch);
    else seen.add(key);
  }
  await Promise.all(hide.map((ch) => edit(ch, { hidden: true })));
  await refresh(["channels"]);
  setNote(hide.length ? `Hid ${hide.length} channels.` : "Nothing to hide.");
}

function coverage(devices: Device[], channels: Channel[], airings: { channelId: number }[]) {
  const listed = new Set(airings.map((a) => a.channelId));
  return devices.map((d) => {
    const mine = channels.filter((c) => c.deviceId === d.deviceId && !c.hidden);
    return {
      id: d.deviceId,
      name: d.friendlyName || d.modelNumber || "Source",
      channels: mine.length,
      withListings: mine.filter((c) => listed.has(c.id)).length,
    };
  });
}

async function addHit(
  hit: { kind: string; addr: string },
  rediscover: (ip?: string) => Promise<void>,
  setNote: (note: string) => void,
  refresh: (what?: ("devices" | "channels")[]) => Promise<void>,
) {
  if (hit.kind === "fastchannels" || hit.kind === "pluto" || hit.kind === "samsung") {
    await addFree({ kind: hit.kind, addr: hit.addr.includes("://") ? hit.addr : `http://${hit.addr}` });
    await refresh(["devices", "channels"]);
    setNote("Source added.");
    return;
  }
  await rediscover(hit.addr);
}

import qrcode from "qrcode-generator";
import { useEffect, useMemo, useRef, useState } from "react";
import { addFree, addPlaylistFile, addSource, findFree, lookHarder, setupFinish, startSetupFinish, type FreeFeed, type SetupFinish } from "../../api";
import { useData } from "../../app/data";
import { navigate } from "../../app/router";
import { ChevronIcon } from "../../ui/icons";
import { HomeList } from "./HomeList";
import "./setup.css";

type Step = "sources" | "finish";
const steps: { id: Step; title: string }[] = [
  { id: "sources", title: "Sources" },
  { id: "finish", title: "Ready" },
];

/** First run: find a tuner, then the server finishes setup on its own. */
export function Setup() {
  const { devices, channels, rediscover, refresh, saveSettings } = useData();
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
  const [progress, setProgress] = useState<SetupFinish | null>(null);
  const hold = useRef(false);

  useEffect(() => {
    if (window.location.pathname !== "/setup") navigate("/setup", true);
  }, []);

  useEffect(() => {
    if (devices.length === 0) void rediscover();
  }, []); // eslint-disable-line react-hooks/exhaustive-deps

  useEffect(() => {
    if (step !== "sources" || hold.current) return;
    if (devices.length === 0 && channels.length === 0) return;
    const timer = window.setTimeout(() => setStep("finish"), 2000);
    return () => window.clearTimeout(timer);
  }, [step, devices.length, channels.length]);

  useEffect(() => {
    if (step !== "finish") return;
    let stop = false;
    void (async () => {
      try {
        let status = await setupFinish();
        if (!status.running && !status.ready) status = await startSetupFinish();
        if (!stop) setProgress(status);
        while (!stop && status.running) {
          await new Promise((resolve) => setTimeout(resolve, 500));
          status = await setupFinish();
          if (!stop) setProgress(status);
        }
        if (!stop && !status.ready) setNote("Setup did not finish.");
      } catch (e: unknown) {
        if (!stop) setNote(e instanceof Error ? e.message : "Setup did not finish.");
      }
    })();
    return () => {
      stop = true;
    };
  }, [step]);

  const index = steps.findIndex((s) => s.id === step);

  async function watch() {
    await saveSettings({ setupComplete: "1" });
    await refresh(["channels", "devices", "airings"]);
    const id = progress?.channelId;
    navigate(id ? `/watch?channel=${id}` : "/");
  }

  return (
    <div className="setup" data-setup={step}>
      <header className="setup-head">
        <span className="brand-tally" aria-hidden="true" />
        <h1>Let's set up your TV</h1>
        <ol className="setup-steps">
          {steps.map((s, i) => (
            <li key={s.id} className={i < index ? "done" : i === index ? "on" : ""}>
              <button
                type="button"
                onClick={() => {
                  if (s.id === "sources" && step === "finish") hold.current = true;
                  setStep(s.id);
                }}
              >
                {s.title}
              </button>
            </li>
          ))}
        </ol>
      </header>

      {step === "sources" ? (
        <section className="setup-card glass">
          <h2>{devices.length > 0 ? "Your tuner" : "Looking for your tuner…"}</h2>
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
          <HomeList hideAdded />
          <Footer onNext={() => setStep("finish")} canNext={devices.length > 0 || channels.length > 0} />
        </section>
      ) : null}

      {step === "finish" ? <FinishStep progress={progress} note={note} onWatch={() => void watch()} /> : null}
    </div>
  );
}

function FinishStep({ progress, note, onWatch }: { progress: SetupFinish | null; note: string; onWatch: () => void }) {
  const ready = progress?.ready ?? "";
  const url = window.location.origin;
  const svg = useMemo(() => {
    const qr = qrcode(0, "M");
    qr.addData(`broadwave://connect?url=${encodeURIComponent(url)}`);
    qr.make();
    return qr.createSvgTag({ cellSize: 6, margin: 2, scalable: true });
  }, [url]);
  return (
    <section className="setup-card glass setup-finish" aria-busy={progress?.running ? true : undefined} data-ready={ready ? "1" : "0"}>
      <h2>{ready || "Setting up your TV"}</h2>
      <ul className="setup-list">
        {(progress?.steps ?? []).map((item) => (
          <li key={item.id}>
            <strong>{item.title}</strong>
            <span className={item.state === "done" ? "ok" : "dim"}>{stateWord(item.state)}</span>
            {item.detail ? <span className="dim">{item.detail}</span> : null}
          </li>
        ))}
      </ul>
      {note ? <p className="dim">{note}</p> : null}
      {ready ? (
        <>
          <div className="setup-foot">
            <button type="button" className="btn primary big" onClick={onWatch}>
              Watch <ChevronIcon />
            </button>
          </div>
          <p className="dim">Scan this with your iPhone. On Apple TV, open Broadwave. It finds this server on its own.</p>
          <div className="apps-row">
            <div className="qr" dangerouslySetInnerHTML={{ __html: svg }} aria-label="QR code to connect the app" />
            <p className="big-url">{url}</p>
          </div>
        </>
      ) : null}
    </section>
  );
}

function stateWord(state: string) {
  if (state === "running") return "Working";
  if (state === "done") return "Done";
  if (state === "skipped") return "Skipped";
  return "";
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

import qrcode from "qrcode-generator";
import { useEffect, useMemo, useRef, useState, type ComponentProps } from "react";
import { addFree, addPlaylistFile, addSource, findFree, lookHarder, setupFinish, startSetupFinish, type FreeFeed, type SetupFinish } from "../../api";
import { useData } from "../../app/data";
import { navigate } from "../../app/router";
import { setupMessages } from "../../i18n/messages/setup.ts";
import { bind } from "../../i18n/runtime.ts";
import { actionName } from "../../lib/actionName";
import { copy } from "../../strings";
import { focusRing } from "../../app/remote";
import { ChevronIcon } from "../../ui/icons";
import { noteAfterFetch } from "./fetchNote";
import { guideAfterFree, hitsAfterLook } from "./lookHits";
import { HomeList } from "./HomeList";
import { GetListings } from "../settings/GuideSources";
import { guideSourcesMessages } from "../../i18n/messages/guideSources.ts";
import { sameZone } from "./zone";
import { foundHeading, setupSummary } from "./summary";
import "../manage.css";
import "./setup.css";

const t = bind(setupMessages);
const g = bind(guideSourcesMessages);

// How long setup looks before it says nothing answered. Discovery answers in a
// few seconds when a tuner is there.
const SEARCH_WINDOW_MS = 15_000;

type Step = "sources" | "finish";
const steps: { id: Step; readonly title: string }[] = [
  {
    id: "sources",
    get title() {
      return copy.sources.title;
    },
  },
  {
    id: "finish",
    get title() {
      return t("setup.ready");
    },
  },
];

/** First run: find a tuner, then the server finishes setup on its own. */
export function Setup() {
  const { devices, channels, settings, rediscover, refresh, saveSettings } = useData();
  const [step, setStep] = useState<Step>("sources");
  const [address, setAddress] = useState("");
  const [busy, setBusy] = useState(false);
  const [note, setNote] = useState("");
  const [hits, setHits] = useState<{ kind: string; name: string; addr: string }[]>([]);
  const [feeds, setFeeds] = useState<FreeFeed[]>([]);
  const [freeGuide, setFreeGuide] = useState("");
  const [findingFree, setFindingFree] = useState(false);
  const [playlistURL, setPlaylistURL] = useState("");
  const [xtreamUser, setXtreamUser] = useState("");
  const [xtreamPass, setXtreamPass] = useState("");
  const [progress, setProgress] = useState<SetupFinish | null>(null);
  const [held, setHeld] = useState(false);
  const [gaveUp, setGaveUp] = useState(false);
  const hold = useRef(false);
  const lookN = useRef(0);
  const freeN = useRef(0);
  const continueRef = useRef<HTMLButtonElement>(null);
  const lookRef = useRef<HTMLButtonElement>(null);
  const addressRef = useRef<HTMLInputElement>(null);
  const watchRef = useRef<HTMLButtonElement>(null);
  const autoFocus = useRef<HTMLElement | null>(null);
  const found = devices.length > 0 || channels.length > 0;
  const ready = Boolean(progress?.ready);

  const stay = () => {
    hold.current = true;
    setHeld(true);
  };

  useEffect(() => {
    if (window.location.pathname !== "/setup") navigate("/setup", true);
  }, []);

  useEffect(() => {
    if (devices.length === 0) void rediscover();
  }, []); // eslint-disable-line react-hooks/exhaustive-deps

  useEffect(() => {
    if (found) return;
    const timer = window.setTimeout(() => setGaveUp(true), SEARCH_WINDOW_MS);
    return () => window.clearTimeout(timer);
  }, [found]);
  const noneFound = gaveUp && !found;

  useEffect(() => {
    if (step !== "sources" || hold.current) return;
    if (!found) return;
    const timer = window.setTimeout(() => {
      if (!hold.current) setStep("finish");
    }, 4000);
    return () => window.clearTimeout(timer);
  }, [step, found]);

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
        if (!stop && !status.ready) setNote(t("setup.didNotFinish"));
      } catch (e: unknown) {
        if (!stop) setNote(e instanceof Error ? e.message : t("setup.didNotFinish"));
      }
    })();
    return () => {
      stop = true;
    };
  }, [step]);

  // The summary counts what the server just finished, not what loaded before it.
  useEffect(() => {
    if (ready) void refresh(["channels", "devices"]);
  }, [ready]); // eslint-disable-line react-hooks/exhaustive-deps

  // A remote starts on the next thing to press, unless it is already somewhere on the page.
  useEffect(() => {
    if (document.documentElement.dataset.layout !== "tv") return;
    const frame = window.requestAnimationFrame(() => {
      const active = document.activeElement;
      const placed = active instanceof HTMLElement && active !== document.body && active.closest(".setup");
      // Focus this effect put on Look harder moves on to Continue once a tuner turns up.
      if (placed && active !== autoFocus.current) return;
      const target = step === "finish" ? watchRef.current : found ? continueRef.current : noneFound ? addressRef.current : lookRef.current;
      if (!target) return;
      autoFocus.current = target;
      focusRing(target);
    });
    return () => window.cancelAnimationFrame(frame);
  }, [step, ready, found, noneFound]);

  const index = steps.findIndex((s) => s.id === step);

  async function watch() {
    await saveSettings({ setupComplete: "1" });
    await refresh(["channels", "devices", "airings"]);
    const id = progress?.channelId;
    navigate(id ? `/watch?channel=${id}` : "/");
  }

  const lead = noneFound ? t("setup.leadNoneFound") : !found ? t("setup.leadSearching") : held ? t("setup.leadHeld") : t("setup.leadFound");
  const heading = devices.length > 0 ? foundHeading(devices) : noneFound ? t("setup.noneFound") : t("setup.looking");

  return (
    <div className="setup manage" data-setup={step}>
      <div className="setup-inner">
        <header className="setup-head">
          <span className="brand-tally" aria-hidden="true" />
          <h1>{t("setup.title")}</h1>
          <ol className="setup-steps">
            {steps.map((s, i) => (
              <li key={s.id} className={i < index ? "done" : i === index ? "on" : ""}>
                <button
                  type="button"
                  aria-current={i === index ? "step" : undefined}
                  onClick={() => {
                    if (s.id === "sources" && step === "finish") stay();
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
          <section className="setup-card" aria-labelledby="setup-sources-title" onPointerDownCapture={stay} onKeyDownCapture={stay}>
            <div className="setup-card-head">
              <h2 id="setup-sources-title">{heading}</h2>
              <p className="dim">{lead}</p>
            </div>
            {devices.length > 0 ? (
              <ul className="setup-found">
                {devices.map((d) => (
                  <li key={d.deviceId}>
                    <span className="setup-found-mark" aria-hidden="true">
                      <Check />
                    </span>
                    <span className="setup-found-text">
                      <strong>{d.friendlyName || d.modelNumber}</strong>
                      <span className="dim">
                        {d.tunerCount > 0 ? t("setup.tuners", { count: d.tunerCount }) : t("setup.streamed")}
                        {" · "}
                        {t("setup.channels", { count: channels.filter((c) => c.deviceId === d.deviceId).length })}
                      </span>
                    </span>
                    <span className="ok">{t("setup.found")}</span>
                  </li>
                ))}
              </ul>
            ) : noneFound && !busy ? null : (
              <p className="setup-searching" role="status">
                <span className="setup-spinner" aria-hidden="true" />
                {t("setup.searching")}
              </p>
            )}

            <div className={noneFound ? "setup-block setup-block-first" : "setup-block"}>
              <h3>{noneFound ? t("setup.addByAddress") : t("setup.notSeeing")}</h3>
              <form
                className="setup-address"
                onSubmit={(event) => {
                  event.preventDefault();
                  if (!address.trim() || busy) return;
                  setBusy(true);
                  void rediscover(address.trim())
                    .catch((e: unknown) => setNote(e instanceof Error ? e.message : t("setup.nothingAtAddress")))
                    .finally(() => setBusy(false));
                }}
              >
                <div className="field">
                  <label htmlFor="setup-address">{copy.sources.address}</label>
                  <div className="setup-inline">
                    <input
                      ref={addressRef}
                      id="setup-address"
                      value={address}
                      onChange={(e) => setAddress(e.target.value)}
                      autoComplete="off"
                      spellCheck={false}
                      placeholder={copy.sources.addressPlaceholder}
                      aria-describedby="setup-address-hint"
                    />
                    <button type="submit" className="btn" disabled={!address.trim() || busy}>
                      {copy.sources.lookup}
                    </button>
                  </div>
                  <span className="hint" id="setup-address-hint">
                    {copy.sources.addressHint}
                  </span>
                </div>
              </form>
              {noneFound ? (
                <div className="setup-causes">
                  <h4>{t("setup.causesTitle")}</h4>
                  <ul>
                    <li>{t("setup.causeNetwork")}</li>
                    <li>{t("setup.causeDocker")}</li>
                    <li>{t("setup.causePower")}</li>
                  </ul>
                </div>
              ) : null}
              <div className="setup-row">
                <button
                  ref={lookRef}
                  type="button"
                  className="btn"
                  disabled={busy}
                  onClick={() => {
                    const mine = ++lookN.current;
                    setBusy(true);
                    void lookHarder()
                      .then((res) => {
                        setHits((shown) => hitsAfterLook(mine, lookN.current, shown, true, res.found) ?? shown);
                        if (mine === lookN.current && (res.found ?? []).length === 0) setGaveUp(true);
                      })
                      .catch(() => {
                        setHits((shown) => hitsAfterLook(mine, lookN.current, shown, false, undefined) ?? shown);
                        if (mine === lookN.current) setGaveUp(true);
                      })
                      .finally(() => {
                        if (mine === lookN.current) setBusy(false);
                      });
                  }}
                >
                  {copy.sources.look}
                </button>
                <button
                  type="button"
                  className="btn"
                  disabled={busy}
                  onClick={() => {
                    const mine = ++freeN.current;
                    setBusy(true);
                    setFindingFree(true);
                    setFreeGuide("");
                    void findFree()
                      .then((res) => {
                        if (mine !== freeN.current) return;
                        setFeeds((shown) => hitsAfterLook(mine, freeN.current, shown, true, res.found) ?? shown);
                        const guide = guideAfterFree(true, res.found, res.guide);
                        if (guide != null) setFreeGuide(guide);
                        setNote((n) => noteAfterFetch(n, true, copy.sources.freeEmpty));
                      })
                      .catch(() => {
                        setFeeds((shown) => hitsAfterLook(mine, freeN.current, shown, false, undefined) ?? shown);
                        if (mine === freeN.current) setNote((n) => noteAfterFetch(n, false, copy.sources.freeEmpty));
                      })
                      .finally(() => {
                        if (mine !== freeN.current) return;
                        setBusy(false);
                        setFindingFree(false);
                      });
                  }}
                >
                  {copy.sources.free}
                </button>
              </div>
              {/* The guide below says it all once FastChannels did not answer. */}
              <p className="hint" role="status">
                {findingFree ? copy.sources.freeLooking : freeGuide ? "" : copy.sources.freeHint}
              </p>
              {hits.length > 0 || feeds.length > 0 ? (
                <ul className="setup-list">
                  {hits.map((hit) => (
                    <li key={`${hit.kind}-${hit.addr}`}>
                      <strong>{hit.name}</strong>
                      <button type="button" className="btn small" aria-label={actionName(copy.sources.add, hit.name)} onClick={() => void addHit(hit, rediscover, setNote, refresh)}>
                        {copy.sources.add}
                      </button>
                      <span className="dim">{hit.addr}</span>
                    </li>
                  ))}
                  {feeds.map((feed) => (
                    <li key={feed.playlist}>
                      <strong>{feed.name}</strong>
                      <button
                        type="button"
                        className="btn small"
                        aria-label={actionName(copy.sources.add, feed.name)}
                        onClick={() => {
                          setBusy(true);
                          void addFree(feed)
                            .then((res) => {
                              setNote(res.message || t("setup.addedLineup"));
                              return refresh(["devices", "channels"]);
                            })
                            .catch((e: unknown) => setNote(e instanceof Error ? e.message : t("setup.feedFailed")))
                            .finally(() => setBusy(false));
                        }}
                      >
                        {copy.sources.add}
                      </button>
                    </li>
                  ))}
                </ul>
              ) : null}
              {freeGuide ? <pre className="hint setup-pre">{freeGuide}</pre> : null}
            </div>

            <details className="setup-other">
              {/* The remote walks focusable things only, and a summary is not one by default. */}
              <summary tabIndex={0}>
                <span className="setup-other-title">{t("setup.otherSources")}</span>
                <span className="hint">{t("setup.otherSourcesHint")}</span>
              </summary>
              <form
                className="setup-form"
                onSubmit={(event) => {
                  event.preventDefault();
                  setBusy(true);
                  const kind = xtreamUser ? "xtream" : "m3u";
                  void addSource(kind, "", playlistURL, "", "", "", xtreamUser, xtreamPass)
                    .then(() => refresh(["devices", "channels"]))
                    .then(() => setNote(t("setup.playlistAdded")))
                    .catch((e: unknown) => setNote(e instanceof Error ? e.message : t("setup.playlistFailed")))
                    .finally(() => setBusy(false));
                }}
              >
                <label className="field">
                  {t("setup.playlistOrXtream")}
                  <input value={playlistURL} onChange={(e) => setPlaylistURL(e.target.value)} placeholder={t("setup.playlistPlaceholder")} spellCheck={false} />
                </label>
                <div className="setup-pair">
                  <label className="field">
                    {t("setup.username")}
                    <input value={xtreamUser} onChange={(e) => setXtreamUser(e.target.value)} autoComplete="off" />
                  </label>
                  <label className="field">
                    {t("setup.password")}
                    <input type="password" value={xtreamPass} onChange={(e) => setXtreamPass(e.target.value)} autoComplete="off" />
                  </label>
                </div>
                <button type="submit" className="btn" disabled={busy || playlistURL.trim() === ""}>
                  {t("setup.addPlaylist")}
                </button>
                <label className="field setup-file">
                  {copy.sources.file}
                  <input
                    type="file"
                    accept=".m3u,.m3u8"
                    onChange={(e) => {
                      const file = e.target.files?.[0];
                      if (!file) return;
                      setBusy(true);
                      void addPlaylistFile("", "", file)
                        .then(() => refresh(["devices", "channels"]))
                        .then(() => setNote(t("setup.playlistAdded")))
                        .catch((err: unknown) => setNote(err instanceof Error ? err.message : t("setup.fileFailed")))
                        .finally(() => setBusy(false));
                    }}
                  />
                </label>
              </form>
            </details>
            {note ? <p className="setup-note" role={/did not|not finish|nothing|no free/i.test(note) ? "alert" : "status"}>{note}</p> : null}
            <div className="setup-block">
              <HomeList hideAdded />
            </div>
          </section>
        ) : null}

        {step === "finish" ? (
          <FinishStep
            progress={progress}
            note={note}
            summary={setupSummary(devices, channels.length)}
            saved={{ user: settings.sdUser, passwordSaved: settings.sdPasswordSet === "1", lineup: settings.sdLineup, url: settings.guideUrl }}
            onListings={() => {
              void setupFinish().then(setProgress, () => undefined);
              void refresh(["channels", "airings", "settings"]);
            }}
          />
        ) : null}
      </div>
      <div className="setup-foot">
        <div className="setup-foot-inner">
          {step === "sources" ? (
            <button ref={continueRef} type="button" className="btn primary big" disabled={!found} onClick={() => setStep("finish")}>
              {t("setup.continue")} <ChevronIcon />
            </button>
          ) : ready ? (
            <button ref={watchRef} type="button" className="btn primary big" onClick={() => void watch()}>
              {t("setup.watch")} <ChevronIcon />
            </button>
          ) : null}
        </div>
      </div>
    </div>
  );
}

function Check() {
  return (
    <svg viewBox="0 0 24 24" width="1em" height="1em" fill="none" stroke="currentColor" strokeWidth="2.6" strokeLinecap="round" strokeLinejoin="round">
      <path d="M5 12.5l4.5 4.5L19 7.5" />
    </svg>
  );
}

function FinishStep({
  progress,
  note,
  summary,
  saved,
  onListings,
}: {
  progress: SetupFinish | null;
  note: string;
  summary: string;
  saved: ComponentProps<typeof GetListings>["saved"];
  onListings: () => void;
}) {
  const ready = progress?.ready ?? "";
  const guideStep = progress?.steps.find((s) => s.id === "guide");
  const needsGuide = Boolean(ready && progress?.needsGuide && guideStep?.state !== "done");
  const url = window.location.origin;
  const loopback = /^(localhost|127\.|\[::1\]$)/.test(window.location.hostname);
  const svg = useMemo(() => {
    const qr = qrcode(0, "M");
    qr.addData(`broadwave://connect?url=${encodeURIComponent(url)}`);
    qr.make();
    return qr.createSvgTag({ cellSize: 6, margin: 2, scalable: true });
  }, [url]);
  return (
    <section className="setup-card setup-finish" aria-labelledby="setup-ready-title" aria-busy={progress?.running ? true : undefined} data-ready={ready ? "1" : "0"}>
      <div className="setup-summary">
        <span className={ready ? "setup-summary-mark done" : "setup-summary-mark"} aria-hidden="true">
          {ready ? <Check /> : <span className="setup-spinner" />}
        </span>
        <div>
          <h2 id="setup-ready-title">{ready ? summary : t("setup.settingUp")}</h2>
          <p className="dim">{ready ? t("setup.readyToWatch") : t("setup.settingUpLead")}</p>
        </div>
      </div>
      {(progress?.steps ?? []).length > 0 ? (
        <ul className="setup-checks" aria-label={t("setup.stepsAria")}>
          {(progress?.steps ?? []).map((item) => (
            <li key={item.id} data-state={item.state} data-check={item.id}>
              <span className="setup-check-mark" aria-hidden="true">
                {item.state === "done" ? <Check /> : item.state === "check" ? "!" : item.state === "running" ? <span className="setup-spinner" /> : "–"}
              </span>
              <span className="setup-check-text">
                <strong>{item.title}</strong>
                {item.detail ? <span className="dim">{item.detail}</span> : null}
              </span>
              <span className="setup-check-word">{stateWord(item.state)}</span>
            </li>
          ))}
        </ul>
      ) : null}
      {note ? <p className="setup-note" role={/did not|not finish|nothing|no free/i.test(note) ? "alert" : "status"}>{note}</p> : null}
      {ready && progress?.timeZone ? <ZoneNote zone={progress.timeZone} offset={progress.utcOffset ?? 0} /> : null}
      {needsGuide ? (
        <div className="setup-listings">
          <h3>{g("gs.title")}</h3>
          <GetListings saved={saved} onLoaded={onListings} lede />
        </div>
      ) : null}
      {ready ? (
        <div className="setup-connect">
          <h3>{t("setup.connect")}</h3>
          {loopback ? (
            <p className="dim">{t("setup.loopback")}</p>
          ) : (
            <div className="apps-row">
              <div className="qr" dangerouslySetInnerHTML={{ __html: svg }} role="img" aria-label={t("setup.qr")} />
              <div className="setup-connect-text">
                <p className="dim">{t("setup.scan")}</p>
                <p className="big-url">{url}</p>
              </div>
            </div>
          )}
        </div>
      ) : null}
    </section>
  );
}

/** The server's zone, and how to fix it when it is not the zone this browser is in. */
function ZoneNote({ zone, offset }: { zone: string; offset: number }) {
  const browser = Intl.DateTimeFormat().resolvedOptions().timeZone;
  const same = sameZone(zone, offset, browser, new Date());
  return (
    <p className={same ? "setup-zone dim" : "setup-zone setup-zone-off"} role={same ? undefined : "alert"}>
      {same ? g("gs.zoneSame", { zone }) : g("gs.zoneDiffers", { server: zone, browser })}
    </p>
  );
}

function stateWord(state: string) {
  if (state === "running") return t("setup.working");
  if (state === "done") return t("setup.done");
  if (state === "check") return t("setup.needsLook");
  if (state === "skipped") return t("setup.skipped");
  return "";
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
    setNote(t("setup.sourceAdded"));
    return;
  }
  await rediscover(hit.addr);
}

import { lazy, Suspense, useEffect, useRef, useState } from "react";
import { Guide } from "../features/guide/Guide";
import { Home } from "../features/home/Home";
import { gateApp } from "../lib/compat";
import { events } from "../lib/events";
import { shellMessages } from "../i18n/messages/shell";
import { bind } from "../i18n/runtime";
import { DataProvider, useData } from "./data";
import { useLayout } from "./layout";
import { Nav, routeTitle } from "./Nav";
import { NotFound } from "./NotFound";
import { PlayerProvider, usePlayer } from "./player";
import { focusRing, installTvRemote } from "./remote";
import { navigate, useRoute } from "./router";
import "./app.css";

const shell = bind(shellMessages);

const Sports = lazy(() => import("../features/sports/Sports").then((m) => ({ default: m.Sports })));
const RecordingsPage = lazy(() => import("../features/pages").then((m) => ({ default: m.RecordingsPage })));
const SchedulePage = lazy(() => import("../features/pages").then((m) => ({ default: m.SchedulePage })));
const SettingsPage = lazy(() => import("../features/settings/SettingsPage").then((m) => ({ default: m.SettingsPage })));
const AboutPage = lazy(() => import("../features/settings/About").then((m) => ({ default: m.AboutPage })));
const GameAlerts = lazy(() => import("../features/sports/GameAlerts").then((m) => ({ default: m.GameAlerts })));
const ScreenMoves = lazy(() => import("../features/player/ScreenMoves").then((m) => ({ default: m.ScreenMoves })));
const PlayPage = lazy(() => import("../features/pages").then((m) => ({ default: m.PlayPage })));
const Setup = lazy(() => import("../features/setup/Setup").then((m) => ({ default: m.Setup })));
const DiagnosticsPage = lazy(() => import("../features/setup/Diagnostics").then((m) => ({ default: m.DiagnosticsPage })));
const VirtualPage = lazy(() => import("../features/pages").then((m) => ({ default: m.VirtualPage })));
const Multiview = lazy(() => import("../features/multiview/Multiview").then((m) => ({ default: m.Multiview })));
const SearchPage = lazy(() => import("../features/search/Search").then((m) => ({ default: m.SearchPage })));
const PairScreen = lazy(() => import("../features/settings/PairScreen").then((m) => ({ default: m.PairScreen })));

/** The recording player arrives after the page that opened it is gone, so nothing else is focused. */
function PlayClaim() {
  useEffect(() => {
    const frame = window.requestAnimationFrame(() => {
      if (document.documentElement.dataset.layout !== "tv") return;
      // Leaving the player leaves focus on the docked mini player, which no key reaches and no ring shows.
      const active = document.activeElement;
      const lost = !(active instanceof HTMLElement) || active === document.body || active === document.documentElement || active.matches(".stage.mini") || Boolean(active.closest("[inert]"));
      if (!lost) return;
      const stage = document.querySelector<HTMLElement>(".stage:not(.mini)");
      // Play, not Back: the first OK on a remote must not leave the recording.
      const pick = (selector: string) => {
        const el = stage?.querySelector<HTMLElement>(selector);
        return el && !el.closest("[inert]") ? el : null;
      };
      focusRing(pick(".transport-play") ?? pick("button:not([disabled])") ?? stage);
    });
    return () => window.cancelAnimationFrame(frame);
  }, []);
  return null;
}

const knownPaths = new Set(["/", "/index.html", "/guide", "/search", "/sports", "/recordings", "/library", "/schedule", "/settings", "/sources", "/about", "/play", "/watch", "/multiview", "/setup", "/diagnostics", "/pair"]);

function notesHref(url: string | undefined): string {
  if (!url) return "";
  try {
    const parsed = new URL(url);
    if (parsed.protocol !== "https:" || parsed.hostname !== "github.com" || parsed.username || parsed.password) return "";
    if (parsed.search || parsed.hash || !parsed.pathname.startsWith("/wolfebase/broadwave/")) return "";
    return parsed.toString();
  } catch {
    return "";
  }
}

export function App() {
  return (
    <DataProvider>
      <PlayerProvider>
        <Shell />
      </PlayerProvider>
    </DataProvider>
  );
}

function Shell() {
  const { path, params } = useRoute();
  const layout = useLayout();
  const { ready, booting, error, recordings, settings, notices, dismissNotice, update, server, freshAt, needsPair, notePaired } = useData();
  useEffect(() => {
    if (layout !== "tv") return;
    return installTvRemote();
  }, [layout]);
  const [hiddenUpdate, setHiddenUpdate] = useState("");
  const notice = update && update.message && hiddenUpdate !== update.message ? update : undefined;
  const player = usePlayer();
  const [online, setOnline] = useState(true);
  const recordingCount = recordings.filter((r) => r.status === "recording").length;
  const fullPlayer = !needsPair && path === "/watch" && player.mode === "full" && !!player.channel;
  const firstRun = ready && settings.needsSetup === "1";
  const immersive = needsPair || path === "/pair" || path === "/play" || path === "/multiview" || (path === "/watch" && params.has("virtual")) || path === "/setup" || (firstRun && path !== "/diagnostics");

  useEffect(() => {
    if (layout !== "tv" || booting) return;
    let frame = 0;
    let tries = 0;
    const place = () => {
      // Leaving the player leaves focus on the docked mini player, which no key reaches and no ring shows.
      const active = document.activeElement;
      const lost = !(active instanceof HTMLElement) || active === document.body || active === document.documentElement || active.matches(".stage.mini") || Boolean(active.closest("[inert]"));
      if (!lost) return;
      const pick = (selector: string) => {
        const el = document.querySelector<HTMLElement>(selector);
        return el && !el.closest("[inert]") ? el : null;
      };
      const target =
        pick("[role='dialog'] .btn.primary") ||
        pick(".mv-guide button:not([disabled])") ||
        pick(".mv") ||
        pick(".stage:not(.mini) [data-channels]") ||
        pick(".stage:not(.mini) .transport-play") ||
        pick(".stage:not(.mini) button:not([disabled])") ||
        pick(".stage:not(.mini)") ||
        // The page's content first: the bar is one press up.
        pick("main .guide-canvas") ||
        pick("main .hero-actions .btn") ||
        pick("main button:not([disabled]):not(.skip-link), main a[href]:not(.skip-link), main input, main [tabindex='0']");
      // A page still loading has no content yet. Wait a moment for it before settling for the bar.
      if (!target && tries++ < 12) {
        frame = window.setTimeout(place, 100);
        return;
      }
      focusRing(target ?? pick(".tabs [aria-current='page']"));
    };
    frame = window.setTimeout(place, 16);
    return () => window.clearTimeout(frame);
  }, [path, layout, booting, fullPlayer]);

  useEffect(() => {
    // A blip that reconnects within a few seconds is not worth a word.
    let timer = 0;
    const off = events().on("connection", (v) => {
      if (v) {
        window.clearTimeout(timer);
        timer = 0;
        setOnline(true);
      } else if (!timer) {
        timer = window.setTimeout(() => setOnline(false), 3_000);
      }
    });
    return () => {
      window.clearTimeout(timer);
      off();
    };
  }, []);

  const known = knownPaths.has(path);
  useEffect(() => {
    // The player names its channel; every other page names itself.
    if (path === "/watch" && player.channel) return;
    document.title = routeTitle(path, known);
  }, [path, known, player.channel]);

  const incompatible = gateApp(server);
  if (incompatible) {
    return (
      <div className="shell">
        <main className="content">
          <div className="version-block" role="alert">
            <span className="brand-tally" aria-hidden="true" />
            <p>{incompatible}</p>
          </div>
        </main>
      </div>
    );
  }

  let page: React.ReactNode;
  if (needsPair || path === "/pair") page = <PairScreen onPaired={() => { notePaired(); navigate("/"); }} />;
  else if (path === "/diagnostics") page = <DiagnosticsPage />;
  else if (path === "/setup" || firstRun) page = <Setup />;
  else if (path === "/guide") page = <Guide />;
  else if (path === "/search") page = <SearchPage />;
  else if (path === "/sports") page = <Sports />;
  else if (path === "/recordings" || path === "/library") page = <RecordingsPage />;
  else if (path === "/schedule") page = <SchedulePage />;
  else if (path === "/settings" || path.startsWith("/settings/") || path === "/sources") page = <SettingsPage />;
  else if (path === "/about") page = <AboutPage />;
  else if (path === "/play") page = <PlayPage />;
  else if (path === "/watch" && params.has("virtual")) page = <VirtualPage />;
  else if (path === "/watch" || path === "/multiview") page = path === "/multiview" ? <Multiview /> : null;
  else if (known) page = <Home />;
  else page = <NotFound tv={layout === "tv"} />;

  return (
    <div className={`shell layout-${layout}${fullPlayer || immersive ? " immersive" : ""}${player.channel && !fullPlayer && path !== "/multiview" ? " has-mini" : ""}`}>
      {!immersive ? (
        <a className="skip-link" href="#main" onClick={(event) => {
          event.preventDefault();
          const main = document.getElementById("main");
          main?.focus();
          main?.scrollIntoView({ block: "start" });
        }}>
          {shell("nav.skip")}
        </a>
      ) : null}
      {!immersive ? (
        <Nav path={path} layout={layout} inert={fullPlayer} recordingCount={recordingCount} online={online} freshAt={freshAt} />
      ) : null}
      <main id="main" tabIndex={-1} className="content" aria-busy={booting} data-ready={ready ? "1" : "0"} inert={fullPlayer ? true : undefined}>
        {booting ? <div className="boot"><span className="brand-tally" /> {shell("shell.booting")}</div> : null}
        {!booting && error ? <p className="banner-error" role="alert">{error}</p> : null}
        {!booting && notice?.message ? (
          <div className="banner-update" role="status">
            <p>{notice.message}</p>
            {notesHref(notice.notesUrl) ? (
              <a className="btn small" href={notesHref(notice.notesUrl)} target="_blank" rel="noreferrer">{shell("shell.releaseNotes")}</a>
            ) : null}
            <button type="button" className="btn small ghost" onClick={() => setHiddenUpdate(notice.message)}>{shell("shell.notNow")}</button>
          </div>
        ) : null}
        {!booting && notices[0] && !immersive && !fullPlayer ? (
          <HomeNotice key={`${notices.length}:${notices[0]}`} message={notices[0]} onDismiss={dismissNotice} />
        ) : null}
        {!booting ? (
          <Suspense fallback={null}>
            <GameAlerts held={immersive || fullPlayer || Boolean(notices[0])} />
            <ScreenMoves />
          </Suspense>
        ) : null}
        {!booting ? (
          <Suspense fallback={null}>
            {path === "/play" ? <PlayClaim /> : null}
            {page}
          </Suspense>
        ) : null}
      </main>
    </div>
  );
}

/** A device that joined the house. It waits off the player and multiview, and goes by itself. */
function HomeNotice({ message, onDismiss }: { message: string; onDismiss: () => void }) {
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => {
    // 12 s once it can be seen: a tab in the background has not seen it yet.
    let timer = 0;
    const arm = () => {
      window.clearTimeout(timer);
      if (document.visibilityState === "visible") timer = window.setTimeout(onDismiss, 12_000);
    };
    // It floats over the page. A control under it that takes focus wins.
    const onFocus = (event: FocusEvent) => {
      const box = ref.current?.getBoundingClientRect();
      const target = event.target;
      if (!box || !(target instanceof HTMLElement) || ref.current?.contains(target)) return;
      const r = target.getBoundingClientRect();
      if (r.left < box.right && box.left < r.right && r.top < box.bottom && box.top < r.bottom) onDismiss();
    };
    arm();
    document.addEventListener("visibilitychange", arm);
    document.addEventListener("focusin", onFocus);
    return () => {
      window.clearTimeout(timer);
      document.removeEventListener("visibilitychange", arm);
      document.removeEventListener("focusin", onFocus);
    };
  }, [message, onDismiss]);
  return (
    <div ref={ref} className="banner-home" role="status">
      <p>{message}</p>
      <button type="button" className="btn small" onClick={() => navigate("/settings")}>{shell("shell.yourHome")}</button>
      <button type="button" className="btn small ghost" onClick={onDismiss}>{shell("shell.notNow")}</button>
    </div>
  );
}

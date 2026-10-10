import { lazy, Suspense, useEffect, useRef, useState } from "react";
import { Guide } from "../features/guide/Guide";
import { Home } from "../features/home/Home";
import { gateApp } from "../lib/compat";
import { events } from "../lib/events";
import { formatClock } from "../time";
import { GuideIcon, HomeIcon, RecordingsIcon, ScheduleIcon, SearchIcon, SettingsIcon, SportsIcon } from "../ui/icons";
import { DataProvider, useData } from "./data";
import { useLayout } from "./layout";
import { PlayerProvider, usePlayer } from "./player";
import { focusRing, installTvRemote } from "./remote";
import { navigate, useRoute } from "./router";
import "./app.css";

const Sports = lazy(() => import("../features/sports/Sports").then((m) => ({ default: m.Sports })));
const RecordingsPage = lazy(() => import("../features/pages").then((m) => ({ default: m.RecordingsPage })));
const SchedulePage = lazy(() => import("../features/pages").then((m) => ({ default: m.SchedulePage })));
const SettingsPage = lazy(() => import("../features/pages").then((m) => ({ default: m.SettingsPage })));
const AboutPage = lazy(() => import("../features/settings/About").then((m) => ({ default: m.AboutPage })));
const GameAlerts = lazy(() => import("../features/sports/GameAlerts").then((m) => ({ default: m.GameAlerts })));
const ScreenMoves = lazy(() => import("../features/player/ScreenMoves").then((m) => ({ default: m.ScreenMoves })));
const PlayPage = lazy(() => import("../features/pages").then((m) => ({ default: m.PlayPage })));
const Setup = lazy(() => import("../features/setup/Setup").then((m) => ({ default: m.Setup })));
const DiagnosticsPage = lazy(() => import("../features/setup/Diagnostics").then((m) => ({ default: m.DiagnosticsPage })));
const VirtualPage = lazy(() => import("../features/pages").then((m) => ({ default: m.VirtualPage })));
const Multiview = lazy(() => import("../features/multiview/Multiview").then((m) => ({ default: m.Multiview })));
const SearchPage = lazy(() => import("../features/search/Search").then((m) => ({ default: m.SearchPage })));

/** The recording player arrives after the page that opened it is gone, so nothing else is focused. */
function PlayClaim() {
  useEffect(() => {
    const frame = window.requestAnimationFrame(() => {
      if (document.documentElement.dataset.layout !== "tv") return;
      // Leaving the player leaves focus on the docked mini player, which no key reaches and no ring shows.
      const active = document.activeElement;
      const lost = !(active instanceof HTMLElement) || active === document.body || active === document.documentElement || active.matches(".stage.mini") || Boolean(active.closest("[inert]"));
      if (!lost) return;
      focusRing(document.querySelector<HTMLElement>(".stage:not(.mini)"));
    });
    return () => window.cancelAnimationFrame(frame);
  }, []);
  return null;
}

const tabs = [
  { path: "/", label: "Home", Icon: HomeIcon },
  { path: "/guide", label: "Guide", Icon: GuideIcon },
  { path: "/search", label: "Search", Icon: SearchIcon },
  { path: "/sports", label: "Sports", Icon: SportsIcon },
  { path: "/recordings", label: "Recordings", Icon: RecordingsIcon },
  { path: "/schedule", label: "Schedule", Icon: ScheduleIcon },
];

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
  const { ready, booting, error, recordings, settings, notices, dismissNotice, update, server, freshAt } = useData();
  useEffect(() => {
    if (layout !== "tv") return;
    return installTvRemote();
  }, [layout]);
  const [hiddenUpdate, setHiddenUpdate] = useState("");
  const notice = update && update.message && hiddenUpdate !== update.message ? update : undefined;
  const player = usePlayer();
  const [online, setOnline] = useState(true);
  const recordingCount = recordings.filter((r) => r.status === "recording").length;
  const fullPlayer = path === "/watch" && player.mode === "full" && !!player.channel;
  const firstRun = ready && settings.needsSetup === "1";
  const immersive = path === "/play" || path === "/multiview" || (path === "/watch" && params.has("virtual")) || path === "/setup" || (firstRun && path !== "/diagnostics");

  useEffect(() => {
    if (layout !== "tv" || booting) return;
    const frame = window.requestAnimationFrame(() => {
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
        pick(".stage:not(.mini) [aria-label='Channels']") ||
        pick(".stage:not(.mini)") ||
        pick(".has-mini .guide-canvas") ||
        pick(".tabs [role='tab'][aria-selected='true']") ||
        pick("main button:not([disabled]), main a[href], main [tabindex='0']");
      focusRing(target);
    });
    return () => window.cancelAnimationFrame(frame);
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

  const active = (p: string) => (p === "/" ? path === "/" : path.startsWith(p));
  let page: React.ReactNode;
  if (path === "/diagnostics") page = <DiagnosticsPage />;
  else if (path === "/setup" || firstRun) page = <Setup />;
  else if (path === "/guide") page = <Guide />;
  else if (path === "/search") page = <SearchPage />;
  else if (path === "/sports") page = <Sports />;
  else if (path === "/recordings" || path === "/library") page = <RecordingsPage />;
  else if (path === "/schedule") page = <SchedulePage />;
  else if (path === "/settings" || path === "/sources") page = <SettingsPage />;
  else if (path === "/about") page = <AboutPage />;
  else if (path === "/play") page = <PlayPage />;
  else if (path === "/watch" && params.has("virtual")) page = <VirtualPage />;
  else if (path === "/watch" || path === "/multiview") page = path === "/multiview" ? <Multiview /> : null;
  else page = <Home />;

  return (
    <div className={`shell layout-${layout}${fullPlayer || immersive ? " immersive" : ""}${player.channel && !fullPlayer && path !== "/multiview" ? " has-mini" : ""}`}>
      {!immersive ? (
        <nav className="topbar glass" aria-label="Primary" inert={fullPlayer ? true : undefined}>
          <button type="button" className="brand" onClick={() => navigate("/")} aria-label="Broadwave home">
            <span className="brand-tally" aria-hidden="true" />
            <span className="brand-word">Broadwave</span>
          </button>
          <div className="tabs" role="tablist">
            {tabs.map(({ path: p, label, Icon }) => (
              <button key={p} type="button" role="tab" aria-selected={active(p)} className={active(p) ? "tab on" : "tab"} onClick={() => navigate(p)}>
                <Icon className="tab-icon" />
                <span className="tab-label">{label}</span>
                {p === "/recordings" && recordingCount > 0 ? <span className="tab-rec" aria-label={`${recordingCount} recording`} /> : null}
              </button>
            ))}
          </div>
          <div className="topbar-right">
            {!online ? (
              <span className="offline" role="status">
                {freshAt ? `Can't reach the server. Showing what was saved at ${formatClock(new Date(freshAt))}.` : "Can't reach the server. Trying again."}
              </span>
            ) : null}
            <button type="button" className={active("/settings") ? "glass-icon on" : "glass-icon"} onClick={() => navigate("/settings")} aria-label="Settings">
              <SettingsIcon />
            </button>
          </div>
        </nav>
      ) : null}
      <main className="content" aria-busy={booting} data-ready={ready ? "1" : "0"} inert={fullPlayer ? true : undefined}>
        {booting ? <div className="boot"><span className="brand-tally" /> Finding your tuner…</div> : null}
        {!booting && error ? <p className="banner-error" role="alert">{error}</p> : null}
        {!booting && notice?.message ? (
          <div className="banner-update" role="status">
            <p>{notice.message}</p>
            {notesHref(notice.notesUrl) ? (
              <a className="btn small" href={notesHref(notice.notesUrl)} target="_blank" rel="noreferrer">Release notes</a>
            ) : null}
            <button type="button" className="btn small ghost" onClick={() => setHiddenUpdate(notice.message)}>Not now</button>
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
      <button type="button" className="btn small" onClick={() => navigate("/settings")}>Your home</button>
      <button type="button" className="btn small ghost" onClick={onDismiss}>Not now</button>
    </div>
  );
}

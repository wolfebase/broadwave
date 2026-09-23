import { lazy, Suspense, useEffect, useState } from "react";
import { Guide } from "../features/guide/Guide";
import { Home } from "../features/home/Home";
import { events } from "../lib/events";
import { GuideIcon, HomeIcon, RecordingsIcon, ScheduleIcon, SettingsIcon, SportsIcon } from "../ui/icons";
import { DataProvider, useData } from "./data";
import { useLayout } from "./layout";
import { PlayerProvider, usePlayer } from "./player";
import { navigate, useRoute } from "./router";
import "./app.css";

const Sports = lazy(() => import("../features/sports/Sports").then((m) => ({ default: m.Sports })));
const RecordingsPage = lazy(() => import("../features/pages").then((m) => ({ default: m.RecordingsPage })));
const SchedulePage = lazy(() => import("../features/pages").then((m) => ({ default: m.SchedulePage })));
const SettingsPage = lazy(() => import("../features/pages").then((m) => ({ default: m.SettingsPage })));
const PlayPage = lazy(() => import("../features/pages").then((m) => ({ default: m.PlayPage })));
const Setup = lazy(() => import("../features/setup/Setup").then((m) => ({ default: m.Setup })));
const DiagnosticsPage = lazy(() => import("../features/setup/Diagnostics").then((m) => ({ default: m.DiagnosticsPage })));
const VirtualPage = lazy(() => import("../features/pages").then((m) => ({ default: m.VirtualPage })));

const tabs = [
  { path: "/", label: "Home", Icon: HomeIcon },
  { path: "/guide", label: "Guide", Icon: GuideIcon },
  { path: "/sports", label: "Sports", Icon: SportsIcon },
  { path: "/recordings", label: "Recordings", Icon: RecordingsIcon },
  { path: "/schedule", label: "Schedule", Icon: ScheduleIcon },
];

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
  const { ready, error, recordings, settings } = useData();
  const player = usePlayer();
  const [online, setOnline] = useState(true);
  const recordingCount = recordings.filter((r) => r.status === "recording").length;
  const fullPlayer = path === "/watch" && player.mode === "full" && !!player.channel;
  const firstRun = ready && settings.needsSetup === "1";
  const immersive = path === "/play" || (path === "/watch" && params.has("virtual")) || path === "/setup" || firstRun;

  useEffect(() => {
    const off = events().on("connection", (v) => setOnline(Boolean(v)));
    return () => {
      off();
    };
  }, []);

  const active = (p: string) => (p === "/" ? path === "/" : path.startsWith(p));
  let page: React.ReactNode;
  if (path === "/setup" || firstRun) page = <Setup />;
  else if (path === "/diagnostics") page = <DiagnosticsPage />;
  else if (path === "/guide") page = <Guide />;
  else if (path === "/sports") page = <Sports />;
  else if (path === "/recordings" || path === "/library") page = <RecordingsPage />;
  else if (path === "/schedule") page = <SchedulePage />;
  else if (path === "/settings" || path === "/sources") page = <SettingsPage />;
  else if (path === "/play") page = <PlayPage />;
  else if (path === "/watch" && params.has("virtual")) page = <VirtualPage />;
  else if (path === "/watch") page = null;
  else page = <Home />;

  return (
    <div className={`shell layout-${layout}${fullPlayer || immersive ? " immersive" : ""}${player.channel && !fullPlayer ? " has-mini" : ""}`}>
      {!immersive ? (
        <nav className="topbar glass" aria-label="Primary">
          <button type="button" className="brand" onClick={() => navigate("/")} aria-label="Waveguide home">
            <span className="brand-tally" aria-hidden="true" />
            <span className="brand-word">Waveguide</span>
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
            {!online ? <span className="offline">Reconnecting…</span> : null}
            <button type="button" className={active("/settings") ? "glass-icon on" : "glass-icon"} onClick={() => navigate("/settings")} aria-label="Settings">
              <SettingsIcon />
            </button>
          </div>
        </nav>
      ) : null}
      <main className="content" aria-busy={!ready}>
        {!ready ? <div className="boot"><span className="brand-tally" /> Finding your tuner…</div> : null}
        {ready && error ? <p className="banner-error" role="alert">{error}</p> : null}
        {ready ? <Suspense fallback={null}>{page}</Suspense> : null}
      </main>
    </div>
  );
}

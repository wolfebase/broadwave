import { useEffect, useRef, useState, type MouseEvent } from "react";
import { shellMessages } from "../i18n/messages/shell";
import { bind } from "../i18n/runtime";
import { recordingCountLabel } from "../lib/guide";
import { formatClock } from "../time";
import { GuideIcon, HomeIcon, MoreIcon, RecordingsIcon, ScheduleIcon, SearchIcon, SettingsIcon, SportsIcon } from "../ui/icons";
import type { Layout } from "./layout";
import { focusRing } from "./remote";
import { navigate } from "./router";

const shell = bind(shellMessages);

export const navTabs = [
  { path: "/", label: "nav.home", Icon: HomeIcon },
  { path: "/guide", label: "nav.guide", Icon: GuideIcon },
  { path: "/search", label: "nav.search", Icon: SearchIcon },
  { path: "/sports", label: "nav.sports", Icon: SportsIcon },
  { path: "/recordings", label: "nav.recordings", Icon: RecordingsIcon },
  { path: "/schedule", label: "nav.schedule", Icon: ScheduleIcon },
] as const;

/** The phone bar keeps five tabs. The rest sit behind More. */
const phoneMore = new Set(["/schedule"]);

/** A plain click stays in the app. A modified or middle click opens the link the browser's way. */
export function followLink(event: MouseEvent<HTMLAnchorElement>, to: string) {
  if (event.defaultPrevented || event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return;
  event.preventDefault();
  navigate(to);
}

export function isActive(tab: string, path: string): boolean {
  if (tab === "/") return path === "/";
  if (tab === "/recordings") return path.startsWith("/recordings") || path.startsWith("/library");
  if (tab === "/settings") return path.startsWith("/settings") || path.startsWith("/sources") || path.startsWith("/about");
  return path.startsWith(tab);
}

export function Nav({ path, layout, inert, recordingCount, online, freshAt }: { path: string; layout: Layout; inert: boolean; recordingCount: number; online: boolean; freshAt: number }) {
  const phone = layout === "phone";
  // More closes on any route or layout change: it remembers where it was opened.
  const [moreAt, setMoreAt] = useState("");
  const more = moreAt === `${path}:${layout}`;
  const setMore = (next: boolean | ((was: boolean) => boolean)) => {
    const open = typeof next === "function" ? next(more) : next;
    setMoreAt(open ? `${path}:${layout}` : "");
  };
  const moreRef = useRef<HTMLDivElement>(null);
  const moreButton = useRef<HTMLButtonElement>(null);
  // A tap opens More without a ring. A key (a click with no pointer) gets one.
  const byKey = useRef(false);
  const tabs = phone ? navTabs.filter((tab) => !phoneMore.has(tab.path)) : navTabs;
  const moreItems = [
    ...navTabs.filter((tab) => phoneMore.has(tab.path)),
    { path: "/settings", label: "nav.settings", Icon: SettingsIcon },
  ] as const;
  const moreActive = phone && moreItems.some((item) => isActive(item.path, path));

  useEffect(() => {
    if (!more) return;
    const first = moreRef.current?.querySelector<HTMLElement>("a");
    if (byKey.current) focusRing(first);
    else first?.focus({ preventScroll: true });
    const onKey = (event: KeyboardEvent) => {
      if (event.key !== "Escape") return;
      event.preventDefault();
      event.stopPropagation();
      setMoreAt("");
      focusRing(moreButton.current);
    };
    const onDown = (event: PointerEvent) => {
      const target = event.target as Node;
      if (moreRef.current?.contains(target) || moreButton.current?.contains(target)) return;
      setMoreAt("");
    };
    window.addEventListener("keydown", onKey, true);
    window.addEventListener("pointerdown", onDown);
    return () => {
      window.removeEventListener("keydown", onKey, true);
      window.removeEventListener("pointerdown", onDown);
    };
  }, [more]);

  return (
    <nav className="topbar glass" aria-label={shell("nav.primary")} inert={inert ? true : undefined}>
      <a className="brand" href="/" onClick={(event) => followLink(event, "/")} aria-label={shell("nav.homeAria")}>
        <span className="brand-tally" aria-hidden="true" />
        <span className="brand-word">{shell("shell.brand")}</span>
      </a>
      <ul className="tabs">
        {tabs.map(({ path: p, label, Icon }) => {
          const on = isActive(p, path);
          return (
            <li key={p}>
              {/* The tab's name stays its name; how many are recording is its description. */}
              <a
                href={p}
                className={on ? "tab on" : "tab"}
                aria-current={on ? "page" : undefined}
                aria-label={shell(label)}
                aria-describedby={p === "/recordings" && recordingCount > 0 ? "nav-rec-count" : undefined}
                onClick={(event) => followLink(event, p)}
                data-path={p}
              >
                <Icon className="tab-icon" />
                <span className="tab-label" aria-hidden="true">{shell(label)}</span>
                {p === "/recordings" && recordingCount > 0 ? (
                  <span className="tab-rec">
                    <span id="nav-rec-count" className="sr-only">{recordingCountLabel(recordingCount)}</span>
                  </span>
                ) : null}
              </a>
            </li>
          );
        })}
        {phone ? (
          <li className="tab-more-item">
            <button ref={moreButton} type="button" className={moreActive || more ? "tab on" : "tab"} aria-expanded={more} aria-haspopup="true" aria-controls="nav-more" onClick={(event) => {
                byKey.current = event.detail === 0;
                setMore((v) => !v);
              }}>
              <MoreIcon className="tab-icon" />
              <span className="tab-label">{shell("nav.more")}</span>
            </button>
          </li>
        ) : null}
      </ul>
      {phone && more ? (
        <div ref={moreRef} id="nav-more" className="nav-more glass">
          <ul>
            {moreItems.map(({ path: p, label, Icon }) => (
              <li key={p}>
                <a href={p} className={isActive(p, path) ? "nav-more-link on" : "nav-more-link"} aria-current={isActive(p, path) ? "page" : undefined} onClick={(event) => followLink(event, p)}>
                  <Icon />
                  {shell(label)}
                </a>
              </li>
            ))}
          </ul>
        </div>
      ) : null}
      <div className="topbar-right">
        {!online ? (
          <span className="offline" role="status">
            <span className="offline-full">{freshAt ? shell("shell.offlineSaved", { time: formatClock(new Date(freshAt)) }) : shell("shell.offline")}</span>
            <span className="offline-short" aria-hidden="true">
              {freshAt ? shell("shell.offlineShortSaved", { time: formatClock(new Date(freshAt)) }) : shell("shell.offlineShort")}
            </span>
          </span>
        ) : null}
        {phone ? null : (
          <a href="/settings" className={isActive("/settings", path) ? "glass-icon on" : "glass-icon"} aria-current={isActive("/settings", path) ? "page" : undefined} onClick={(event) => followLink(event, "/settings")} aria-label={shell("nav.settings")} title={shell("nav.settings")}>
            <SettingsIcon />
          </a>
        )}
      </div>
    </nav>
  );
}

/** The document title for a page. The player names its own channel. */
export function routeTitle(path: string, known: boolean): string {
  const brand = shell("shell.brand");
  if (!known) return `${shell("shell.notFoundTitle")} · ${brand}`;
  const all = [...navTabs, { path: "/settings", label: "nav.settings" } as const];
  const hit = path === "/" ? undefined : all.find((tab) => tab.path !== "/" && isActive(tab.path, path));
  return hit ? `${shell(hit.label)} · ${brand}` : brand;
}

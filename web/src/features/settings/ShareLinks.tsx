import { useEffect, useRef, useState } from "react";
import { settingsUiMessages } from "../../i18n/messages/settingsUi.ts";
import { settingsHubMessages } from "../../i18n/messages/settingsHub.ts";
import { bind } from "../../i18n/runtime.ts";
import { shareRows } from "./tunerShare";
import { Choice } from "./rows";

const t = bind(settingsUiMessages);
const h = bind(settingsHubMessages);

const steps = {
  Plex: "share.steps.Plex",
  Jellyfin: "share.steps.Jellyfin",
  Emby: "share.steps.Emby",
  Channels: "share.steps.Channels",
} as const;

type App = keyof typeof steps;

/** One app at a time: how to connect it, and the addresses it needs. */
export function ShareLinks({ pageURL, sharing, hdhr = true }: { pageURL: string; sharing: boolean; hdhr?: boolean }) {
  const rows = shareRows(pageURL, sharing, hdhr);
  const apps = [...new Set(rows.map((row) => row.app))] as App[];
  const [app, setApp] = useState<App>(apps[0] ?? "Plex");
  const [copied, setCopied] = useState("");
  const timer = useRef(0);
  useEffect(() => () => window.clearTimeout(timer.current), []);
  const mine = rows.filter((row) => row.app === app);
  // Channels reads a playlist first. The others start from a tuner.
  const ordered = app === "Channels" ? [...mine.filter((row) => row.label !== t("share.hdhr")), ...mine.filter((row) => row.label === t("share.hdhr"))] : mine;
  const nameFor = (label: string) => (label === t("share.hdhr") ? h("share.tuner") : label === t("share.m3u") ? h("share.playlist") : h("share.guide"));
  return (
    <div className="share-picker">
      <div className="share-apps-pick">
        <span className="set-row-label" id="share-app-label">
          {h("share.appPick")}
        </span>
        <Choice label={h("share.appPick")} value={app} options={apps.map((item) => ({ value: item, label: item }))} onChange={setApp} />
      </div>
      <section className="share-connect" aria-label={h("share.connect", { app })}>
        <p className="share-steps">{h(steps[app])}</p>
        <ul className="share-rows">
          {ordered.map((row) => {
            const key = `${app} ${row.label}`;
            const off = Boolean(row.hint);
            return (
              <li key={row.label} className={off ? "share-row off" : "share-row"}>
                <div className="share-row-text">
                  <span className="share-row-name">{nameFor(row.label)}</span>
                  <code>{row.value}</code>
                  {off ? <span className="hint">{h("share.offHint")}</span> : null}
                </div>
                <button
                  type="button"
                  className="btn small"
                  aria-label={`${t("share.copy")} ${nameFor(row.label)}`}
                  onClick={() => {
                    const write = navigator.clipboard?.writeText(row.value);
                    if (!write) return;
                    void write.then(() => {
                      setCopied(key);
                      window.clearTimeout(timer.current);
                      timer.current = window.setTimeout(() => setCopied((current) => (current === key ? "" : current)), 2000);
                    }, () => undefined);
                  }}
                >
                  {copied === key ? t("share.copied") : t("share.copy")}
                </button>
              </li>
            );
          })}
        </ul>
      </section>
      <p className="sr-only" aria-live="polite">
        {copied ? t("share.copiedLive") : ""}
      </p>
    </div>
  );
}

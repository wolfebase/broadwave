import { useCallback, useEffect, useRef, useState, type FormEvent, type SVGProps } from "react";
import type { ApiFailure } from "../../api";
import { settingsUiMessages } from "../../i18n/messages/settingsUi.ts";
import { bind } from "../../i18n/runtime.ts";
import { isForbidden, isUnauthorized } from "../../lib/deviceToken";
import { events } from "../../lib/events";
import type { RemoteStatus } from "../../types";
import { cancelCloudflareLogin, getCloudflare, setupCloudflare, startCloudflareLogin, turnOffCloudflare } from "./cloudflareApi";
import { addressOf, countdown, hostFor, knownProblem, linkState, noticeOf, secondsLeft, setupSteps, stageOf, stepMarks, type CfStatus } from "./cloudflareFlow";
import "./remote.css";

const t = bind(settingsUiMessages);

const tailscaleURL = "https://github.com/wolfebase/broadwave/blob/main/docs/guide/remote.md#tailscale";
const addSiteURL = "https://dash.cloudflare.com/?to=/:account/add-site";
// The sign-in lasts about nine minutes; the bar empties over that.
const loginSeconds = 540;

type P = SVGProps<SVGSVGElement>;

function Icon({ children, ...rest }: P) {
  return (
    <svg viewBox="0 0 24 24" width="1em" height="1em" fill="none" stroke="currentColor" strokeWidth={1.8} strokeLinecap="round" strokeLinejoin="round" aria-hidden="true" {...rest}>
      {children}
    </svg>
  );
}

const CloudIcon = () => (
  <Icon>
    <path d="M7 18.5h10.5a4 4 0 0 0 .6-7.96A6 6 0 0 0 6.5 9.6 4.5 4.5 0 0 0 7 18.5z" />
  </Icon>
);
const OutIcon = () => (
  <Icon>
    <path d="M14 5h5v5M19 5l-8 8M17 14v4a1 1 0 0 1-1 1H6a1 1 0 0 1-1-1V8a1 1 0 0 1 1-1h4" />
  </Icon>
);
const CheckIcon = () => (
  <Icon>
    <path d="m5 12.5 4.5 4.5L19 7.5" />
  </Icon>
);
const CopyIcon = () => (
  <Icon>
    <rect x="9" y="9" width="11" height="11" rx="2" />
    <path d="M5 15V6a1 1 0 0 1 1-1h9" />
  </Icon>
);
const HouseIcon = () => (
  <Icon>
    <path d="M4 11.5 12 5l8 6.5V19a1 1 0 0 1-1 1h-4.5v-5h-5v5H5a1 1 0 0 1-1-1z" />
  </Icon>
);

function message(err: unknown): string {
  return err instanceof Error && err.message ? err.message : t("devices.failed");
}

function problemText(code: string, fallback: string): string {
  const known = knownProblem(code);
  return known ? t(`remote.cf.err.${known}`) : fallback;
}

/** The card for Sign in with Cloudflare: sign in, name the address, watch it connect. */
export function CloudflareCard({ remote, onRemote }: { remote: RemoteStatus | null; onRemote: () => void }) {
  const [status, setStatus] = useState<CfStatus | null>(null);
  const [notHome, setNotHome] = useState(false);
  const [hidden, setHidden] = useState(false);
  const [label, setLabel] = useState("tv");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [blocked, setBlocked] = useState(false);
  const [confirmOff, setConfirmOff] = useState(false);
  const [copied, setCopied] = useState(false);
  const [now, setNow] = useState(() => Date.now());
  const card = useRef<HTMLElement | null>(null);
  const setCard = useCallback((el: HTMLElement | null) => {
    card.current = el;
  }, []);

  const load = useCallback(() => {
    return getCloudflare()
      .then((res) => {
        // An older server has no sign-in.
        if (!res?.state) {
          setHidden(true);
          return;
        }
        setStatus(res as CfStatus);
        setNotHome(false);
      })
      .catch((err: unknown) => {
        if ((err as ApiFailure).code === "not_home") setNotHome(true);
        else if (isForbidden(err) || isUnauthorized(err) || (err as ApiFailure).status === 404) setHidden(true);
        else setError(message(err));
      });
  }, []);

  useEffect(() => {
    void load();
    const bus = events();
    const off = bus.on("remote.changed", () => void load());
    const again = bus.on("reconnected", () => void load());
    // Coming back from the Cloudflare tab.
    const focus = () => void load();
    window.addEventListener("focus", focus);
    return () => {
      off();
      again();
      window.removeEventListener("focus", focus);
    };
  }, [load]);

  const stage = stageOf(status, notHome);
  const ticking = stage === "waiting" || stage === "removing";
  useEffect(() => {
    if (!ticking) return;
    const timer = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(timer);
  }, [ticking]);

  // The address in /remote follows the tunnel; read it again when the stage moves.
  const lastStage = useRef(stage);
  useEffect(() => {
    if (lastStage.current === stage) return;
    const first = lastStage.current === "loading";
    lastStage.current = stage;
    onRemote();
    setConfirmOff(false);
    // The first answer only draws the card; opening Settings must not jump here.
    if (first) return;
    // Keep focus in the card when the control that had it went away.
    const active = document.activeElement;
    if (!active || active === document.body || card.current?.contains(active)) {
      card.current?.querySelector<HTMLElement>("[data-autofocus]")?.focus();
    }
  }, [stage, onRemote]);

  async function run(work: () => Promise<CfStatus | void>) {
    setBusy(true);
    setError("");
    try {
      const res = await work();
      if (res) setStatus(res as CfStatus);
    } catch (err: unknown) {
      if ((err as ApiFailure).code === "not_home") setNotHome(true);
      else setError(message(err));
      void load();
    } finally {
      setBusy(false);
    }
  }

  function signIn(purpose: "setup" | "remove") {
    // Opened now, inside the click, so the browser allows it; the address comes after.
    const tab = window.open("", "_blank");
    setBlocked(!tab);
    void run(async () => {
      try {
        const res = (await startCloudflareLogin(purpose)) as CfStatus;
        if (tab && res.loginUrl) {
          tab.opener = null;
          tab.location.href = res.loginUrl;
        }
        return res;
      } catch (err) {
        tab?.close();
        throw err;
      }
    });
  }

  function turnOn(event: FormEvent, overwrite = false) {
    event.preventDefault();
    void run(() => setupCloudflare({ label: label.trim(), overwrite }) as Promise<CfStatus>);
  }

  if (hidden) return null;

  const zone = status?.zone?.name ?? "";
  const host = hostFor(label, zone);
  const notice = noticeOf(status);
  const requestError = error ? (
    <p className="error" role="alert">
      {error}
    </p>
  ) : null;

  if (stage === "loading") return requestError;

  if (stage === "notHome") {
    return (
      <div className="cf-card" ref={setCard}>
        <div className="cf-head">
          <span className="cf-mark">
            <HouseIcon />
          </span>
          <div className="cf-text">
            <h3 className="cf-title">{t("remote.cf.notHome")}</h3>
            <p className="cf-body">{t("remote.cf.notHomeHint")}</p>
          </div>
        </div>
      </div>
    );
  }

  if (stage === "waiting" || stage === "removing") {
    const left = secondsLeft(status?.expiresAt, now);
    return (
      <div className="cf-card" ref={setCard} aria-busy="true">
        <div className="cf-head">
          <span className="cf-mark cf-pulse">
            <CloudIcon />
          </span>
          <div className="cf-text">
            <h3 className="cf-title" role="status">
              {t("remote.cf.waiting")}
            </h3>
            <p className="cf-body">{stage === "removing" ? t("remote.cf.waitingRemove") : t("remote.cf.waitingHint")}</p>
          </div>
        </div>
        <div className="cf-timer">
          <span className="cf-bar" aria-hidden="true">
            <span style={{ transform: `scaleX(${Math.min(1, left / loginSeconds)})` }} />
          </span>
          <span className="cf-count">{t("remote.cf.timeLeft", { time: countdown(left) })}</span>
        </div>
        {blocked ? <p className="hint">{t("remote.cf.popupBlocked")}</p> : null}
        <div className="cf-actions">
          {status?.loginUrl ? (
            <a className="btn" href={status.loginUrl} target="_blank" rel="noreferrer" data-autofocus>
              {t("remote.cf.openAgain")} <OutIcon />
            </a>
          ) : null}
          <button type="button" className="btn ghost" disabled={busy} data-autofocus={status?.loginUrl ? undefined : true} onClick={() => void run(cancelCloudflareLogin as () => Promise<CfStatus>)}>
            {t("remote.cf.cancel")}
          </button>
        </div>
        {requestError}
      </div>
    );
  }

  if (stage === "signedIn") {
    // A taken name stops mattering once the owner types another one.
    const takenError = status?.error?.code === "name_taken";
    const taken = takenError && (!status?.hostname || status.hostname === host);
    const other = status?.error && !takenError ? status.error : null;
    // Without the domain an unfinished name (no dots yet) is not wrong, only short.
    const bad = label.trim() !== "" && !host && (zone !== "" || !/^[a-z0-9.-]*$/i.test(label.trim()) || label.includes(".."));
    return (
      <form className="cf-card" ref={setCard} onSubmit={(event) => turnOn(event)} aria-labelledby="cf-signed-title">
        <div className="cf-head">
          <span className="cf-mark cf-ok">
            <CheckIcon />
          </span>
          <div className="cf-text">
            <h3 className="cf-title" id="cf-signed-title" role="status">
              {zone ? t("remote.cf.signedInZone", { zone }) : t("remote.cf.signedIn")}
            </h3>
            <p className="cf-body">{zone ? t("remote.cf.chooseName") : t("remote.cf.wholeName")}</p>
          </div>
        </div>
        <label className="field cf-field">
          <span>{t("remote.cf.nameLabel")}</span>
          <span className={zone ? "cf-name" : "cf-name whole"}>
            <input
              value={label}
              data-autofocus
              autoComplete="off"
              autoCapitalize="off"
              spellCheck={false}
              inputMode="url"
              maxLength={zone ? 63 : 253}
              placeholder={zone ? "tv" : "tv.example.com"}
              aria-label={zone ? t("remote.cf.nameAria", { zone }) : t("remote.cf.nameLabel")}
              aria-invalid={bad}
              aria-describedby="cf-preview"
              style={zone ? { width: `${Math.max(2, label.length) + 0.3}ch` } : undefined}
              onChange={(event) => setLabel(event.target.value)}
            />
            {zone ? (
              <span className="cf-suffix" aria-hidden="true">
                .{zone}
              </span>
            ) : null}
          </span>
        </label>
        <p className={bad ? "hint cf-bad" : "hint"} id="cf-preview" aria-live="polite">
          {host ? t("remote.cf.preview", { url: addressOf(host) }) : bad ? t("remote.cf.badName") : zone ? " " : t("remote.cf.wholeNameHint")}
        </p>
        {taken ? (
          <div className="cf-alert cf-choice" role="alert">
            <p>
              <strong>{t("remote.cf.taken", { host: status?.hostname ?? host })}</strong> {t("remote.cf.takenHint")}
            </p>
          </div>
        ) : null}
        {other ? (
          <div className="cf-alert" role="alert">
            <p>{problemText(other.code, other.message)}</p>
          </div>
        ) : null}
        <div className="cf-actions">
          <button type="submit" className="btn primary big" disabled={busy || !host}>
            {t("remote.cf.turnOn")}
          </button>
          {taken ? (
            <button type="button" className="btn" disabled={busy || !host} onClick={(event) => turnOn(event, true)}>
              {t("remote.cf.replace")}
            </button>
          ) : null}
          <button type="button" className="btn ghost" disabled={busy} onClick={() => void run(cancelCloudflareLogin as () => Promise<CfStatus>)}>
            {t("remote.cf.cancel")}
          </button>
        </div>
        {requestError}
      </form>
    );
  }

  if (stage === "working") {
    const marks = stepMarks(status?.step);
    const name = status?.hostname ?? host;
    return (
      <div className="cf-card" ref={setCard} aria-busy="true">
        <div className="cf-head">
          <span className="cf-mark cf-pulse">
            <CloudIcon />
          </span>
          <div className="cf-text">
            <h3 className="cf-title">{t("remote.cf.settingUp", { host: name })}</h3>
          </div>
        </div>
        <ol className="cf-steps" aria-live="polite">
          {setupSteps.map((step) => (
            <li key={step} data-mark={marks[step]}>
              <span className="cf-step-icon" aria-hidden="true">
                {marks[step] === "done" ? <CheckIcon /> : null}
              </span>
              <span>{step === "dns" ? t("remote.cf.step.dns", { host: name }) : t(`remote.cf.step.${step}`)}</span>
              <span className="sr-only">{t(`remote.cf.step.${marks[step]}`)}</span>
            </li>
          ))}
        </ol>
        {requestError}
      </div>
    );
  }

  if (stage === "connected") {
    const url = addressOf(status?.hostname ?? "");
    const link = linkState(status?.tunnel);
    const down = link === "down" ? status?.tunnel.message || status?.error?.message : "";
    return (
      <div className="cf-card cf-on" ref={setCard}>
        <p className="cf-live" data-state={link} role="status">
          <span className="cf-dot" aria-hidden="true" />
          {t(`remote.cf.${link}`)}
        </p>
        <p className="cf-url">{url}</p>
        <p className={down ? "error" : "hint"}>{down || t("remote.cf.onlyInvited")}</p>
        <div className="cf-actions">
          <button
            type="button"
            className="btn"
            data-autofocus
            onClick={() => {
              void navigator.clipboard
                ?.writeText(url)
                .then(() => setCopied(true))
                .catch(() => undefined);
            }}
          >
            <CopyIcon /> {copied ? t("remote.copied") : t("remote.cf.copyAddress")}
          </button>
          <a className="btn ghost" href={url} target="_blank" rel="noreferrer">
            {t("remote.cf.openAddress")} <OutIcon />
          </a>
          <button type="button" className="btn ghost" aria-expanded={confirmOff} aria-controls="cf-off" onClick={() => setConfirmOff((open) => !open)}>
            {t("remote.cf.turnOff")}
          </button>
        </div>
        {confirmOff ? (
          <div className="cf-confirm" id="cf-off" role="group" aria-labelledby="cf-off-title">
            <h4 id="cf-off-title">{t("remote.cf.offTitle")}</h4>
            <p className="hint">{t("remote.cf.offHint")}</p>
            <div className="cf-actions">
              <button type="button" className="btn" disabled={busy} onClick={() => void run(turnOffCloudflare as () => Promise<CfStatus>)}>
                {t("remote.cf.turnOff")}
              </button>
              <button type="button" className="btn" disabled={busy} aria-describedby="cf-off-remove" onClick={() => signIn("remove")}>
                {t("remote.cf.offRemove")}
              </button>
              <button type="button" className="btn ghost" onClick={() => setConfirmOff(false)}>
                {t("remote.cf.keep")}
              </button>
            </div>
            <p className="hint" id="cf-off-remove">
              {t("remote.cf.offRemoveHint")}
            </p>
          </div>
        ) : null}
        {copied ? (
          <span className="sr-only" role="status">
            {t("remote.copied")}
          </span>
        ) : null}
        {requestError}
      </div>
    );
  }

  // Idle. A token or address from Advanced is its own way in; show that instead.
  if (remote && (remote.tokenSet || remote.configured)) {
    const link = linkState(remote.tunnel);
    return (
      <div className="cf-card cf-on" ref={setCard}>
        {remote.tokenSet ? (
          <p className="cf-live" data-state={link} role="status">
            <span className="cf-dot" aria-hidden="true" />
            {t(`remote.cf.${link}`)}
          </p>
        ) : null}
        {remote.remoteUrl ? <p className="cf-url">{remote.remoteUrl}</p> : null}
        <p className="hint">{remote.tokenSet ? t("remote.cf.pasted") : t("remote.cf.ownTunnel")}</p>
        {remote.tokenSet && link === "down" && remote.tunnel.message ? <p className="error">{remote.tunnel.message}</p> : null}
      </div>
    );
  }

  return (
    <div className="cf-card cf-hero" ref={setCard}>
      <div className="cf-head">
        <span className="cf-mark">
          <CloudIcon />
        </span>
        <div className="cf-text">
          <h3 className="cf-title">{t("remote.cf.heroTitle")}</h3>
          <p className="cf-body">{t("remote.cf.heroBody")}</p>
        </div>
      </div>
      {notice?.kind === "problem" ? (
        <div className="cf-alert" role="alert">
          <p>{problemText(notice.problem.code, notice.problem.message)}</p>
          {notice.problem.code === "no_zone" ? (
            <p className="cf-links">
              <a href={addSiteURL} target="_blank" rel="noreferrer">
                {t("remote.cf.addSite")}
              </a>
              <a href={tailscaleURL} target="_blank" rel="noreferrer">
                {t("remote.cf.tailscale")}
              </a>
            </p>
          ) : null}
        </div>
      ) : null}
      {notice?.kind === "dnsLeft" ? (
        <p className="cf-note" role="status">
          {t("remote.cf.dnsLeft", { host: notice.host })}
        </p>
      ) : null}
      <div className="cf-actions">
        <button type="button" className="btn primary big cf-go" data-autofocus disabled={busy} aria-describedby="cf-opens" onClick={() => signIn("setup")}>
          {notice?.kind === "problem" ? t("remote.cf.tryAgain") : t("remote.cf.signIn")} <OutIcon />
        </button>
        {notice ? (
          <button type="button" className="btn ghost" disabled={busy} onClick={() => void run(cancelCloudflareLogin as () => Promise<CfStatus>)}>
            {t("remote.cf.dismiss")}
          </button>
        ) : null}
      </div>
      <p className="hint" id="cf-opens">
        <span className="sr-only">{t("remote.cf.opensTab")} </span>
        {t("remote.cf.needs")}
        {notice?.kind === "problem" && notice.problem.code === "no_zone" ? null : (
          <>
            {" "}
            <a href={tailscaleURL} target="_blank" rel="noreferrer">
              {t("remote.cf.tailscale")}
            </a>
          </>
        )}
      </p>
      {requestError}
    </div>
  );
}

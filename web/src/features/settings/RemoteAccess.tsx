import { useCallback, useEffect, useState, type FormEvent } from "react";
import { settingsUiMessages } from "../../i18n/messages/settingsUi.ts";
import { bind } from "../../i18n/runtime.ts";
import { isForbidden, isUnauthorized } from "../../lib/deviceToken";
import type { InviteGrant, RemoteStatus, Settings } from "../../types";
import { events } from "../../lib/events";
import { CloudflareCard } from "./CloudflareCard";
import { createInvite, deleteInvite, getRemote } from "./pairApi";
import { scopeLabel, scopesForAccess } from "./pairCode";

const t = bind(settingsUiMessages);

const guideURL = "https://github.com/wolfebase/broadwave/blob/main/docs/guide/remote.md#advanced-a-token-or-your-own-cloudflared";

type Access = "watch" | "record" | "admin";

function message(err: unknown): string {
  return err instanceof Error && err.message ? err.message : t("devices.failed");
}

function stateLine(status: RemoteStatus): string {
  const tunnel = status.tunnel;
  if (tunnel.state === "connected") return t("remote.state.connected");
  if (tunnel.state === "starting") return t("remote.state.starting");
  if (tunnel.state === "missing") return t("remote.state.missing");
  if (tunnel.state === "error") return tunnel.message ? `${t("remote.state.error")} ${tunnel.message}` : t("remote.state.error");
  return status.remoteUrl ? "" : t("remote.state.off");
}

function CopyRow({ label, value }: { label: string; value: string }) {
  const [copied, setCopied] = useState(false);
  return (
    <label className="field">
      {label}
      <span className="copy-row">
        <input readOnly value={value} spellCheck={false} onFocus={(event) => event.target.select()} />
        <button
          type="button"
          className="btn"
          onClick={() => {
            void navigator.clipboard
              ?.writeText(value)
              .then(() => setCopied(true))
              .catch(() => undefined);
          }}
        >
          {t("remote.copy")}
        </button>
      </span>
      {copied ? (
        <span className="hint" role="status">
          {t("remote.copied")}
        </span>
      ) : null}
    </label>
  );
}

export function RemoteAccess({ onChange }: { onChange: (values: Partial<Settings>) => Promise<void> | void }) {
  const [status, setStatus] = useState<RemoteStatus | null>(null);
  const [hidden, setHidden] = useState(false);
  const [address, setAddress] = useState("");
  const [token, setToken] = useState("");
  const [who, setWho] = useState("");
  const [access, setAccess] = useState<Access>("watch");
  const [made, setMade] = useState<InviteGrant | null>(null);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  const apply = useCallback((res: RemoteStatus) => {
    // An older server answers without this shape.
    if (!res?.tunnel) {
      setHidden(true);
      return;
    }
    setStatus({ ...res, invites: res.invites ?? [] });
    setAddress(res.configured);
    setHidden(false);
  }, []);

  const fail = useCallback((err: unknown) => {
    // Only an admin sees this section.
    if (isForbidden(err) || isUnauthorized(err)) setHidden(true);
    else setError(message(err));
  }, []);

  const load = useCallback(() => getRemote().then(apply).catch(fail), [apply, fail]);

  useEffect(() => {
    let stop = false;
    void getRemote()
      .then((res) => {
        if (!stop) apply(res);
      })
      .catch((err: unknown) => {
        if (!stop) fail(err);
      });
    return () => {
      stop = true;
    };
  }, [apply, fail]);

  // The server says when the tunnel or the address changes.
  useEffect(() => {
    const off = events().on("remote.changed", () => void load());
    return () => {
      off();
    };
  }, [load]);
  const reload = useCallback(() => void load(), [load]);

  async function save(values: Partial<Settings>) {
    setBusy(true);
    setError("");
    try {
      await onChange(values);
      setToken("");
      await load();
    } catch (err: unknown) {
      setError(message(err));
    } finally {
      setBusy(false);
    }
  }

  async function invite(event: FormEvent) {
    event.preventDefault();
    const name = who.trim();
    if (!name) {
      setError(t("remote.needName"));
      return;
    }
    const scopes = scopesForAccess(access);
    if (!scopes) return;
    setBusy(true);
    setError("");
    try {
      const res = await createInvite({ name, scopes });
      setMade(res);
      setWho("");
      await load();
    } catch (err: unknown) {
      setError(message(err));
    } finally {
      setBusy(false);
    }
  }

  async function remove(id: string) {
    setBusy(true);
    setError("");
    try {
      await deleteInvite(id);
      if (made?.invite.id === id) setMade(null);
      await load();
    } catch (err: unknown) {
      setError(message(err));
    } finally {
      setBusy(false);
    }
  }

  if (hidden) return null;
  const line = status ? stateLine(status) : "";

  return (
    <section aria-labelledby="remote-title" className="remote">
      <h2 id="remote-title">{t("remote.title")}</h2>
      <p className="dim">{t("remote.lead")}</p>
      {status?.here ? <p className="hint">{t("remote.here")}</p> : null}
      <CloudflareCard remote={status} onRemote={reload} />
      <h3 className="section-title">{t("remote.invite")}</h3>
      {status && !status.remoteUrl ? <p className="hint">{t("remote.inviteNeedsAddress")}</p> : null}
      <form onSubmit={(event) => void invite(event)}>
        <label className="field">
          {t("remote.inviteName")}
          <input value={who} maxLength={63} autoComplete="off" onChange={(event) => setWho(event.target.value)} />
        </label>
        <label className="field">
          {t("devices.codeAllows")}
          <select value={access} onChange={(event) => setAccess(event.target.value as Access)}>
            <option value="watch">{t("devices.watch")}</option>
            <option value="record">{t("devices.record")}</option>
            <option value="admin">{t("devices.admin")}</option>
          </select>
        </label>
        <button type="submit" className="btn primary" disabled={busy}>
          {t("remote.inviteMake")}
        </button>
      </form>
      {made ? (
        <div className="invite-card" role="status">
          <p className="hint">{t("remote.inviteCode")}</p>
          <p className="pair-code">{made.code}</p>
          <p className="hint">{t("remote.inviteHint")}</p>
          {made.link ? <CopyRow label={t("remote.inviteLink")} value={made.link} /> : null}
          {made.appLink ? <CopyRow label={t("remote.inviteApp")} value={made.appLink} /> : null}
        </div>
      ) : null}
      {status && status.invites.length > 0 ? (
        <>
          <h3 className="section-title">{t("remote.waiting")}</h3>
          <ul className="source-list">
            {status.invites.map((item) => (
              <li key={item.id} className="source-row">
                <span>{item.name}</span>
                <span>{scopeLabel(item.scopes ?? [])}</span>
                <span className="hint">{t("remote.expires", { date: new Date(item.expiresAt).toLocaleDateString() })}</span>
                <button
                  type="button"
                  className="btn"
                  aria-label={t("remote.removeNamed", { name: item.name })}
                  disabled={busy}
                  onClick={() => void remove(item.id)}
                >
                  {t("remote.removeInvite")}
                </button>
              </li>
            ))}
          </ul>
        </>
      ) : null}
      <details className="remote-advanced">
        <summary>{t("remote.cf.advanced")}</summary>
        <p className="hint">{t("remote.cf.advancedHint")}</p>
        {line && status?.tokenSet ? (
          <p className={status?.tunnel.state === "error" || status?.tunnel.state === "missing" ? "error" : "hint"} role="status">
            {line}
          </p>
        ) : null}
        <form
          onSubmit={(event) => {
            event.preventDefault();
            if (token.trim()) void save({ tunnelToken: token.trim() });
          }}
        >
          <label className="field">
            {t("remote.token")}
            <input
              type="password"
              value={token}
              autoComplete="off"
              spellCheck={false}
              placeholder={status?.tokenSet ? t("remote.tokenSet") : "cloudflared service install eyJ…"}
              onChange={(event) => setToken(event.target.value)}
            />
            <span className="hint">
              {t("remote.tokenHint")}{" "}
              <a href={guideURL} target="_blank" rel="noreferrer">
                {t("remote.guide")}
              </a>
            </span>
          </label>
          <span className="remote-buttons">
            <button type="submit" className="btn" disabled={busy || !token.trim()}>
              {t("remote.save")}
            </button>
            {status?.tokenSet ? (
              <button type="button" className="btn" disabled={busy} onClick={() => void save({ tunnelToken: "" })}>
                {t("remote.remove")}
              </button>
            ) : null}
          </span>
        </form>
        <form
          onSubmit={(event) => {
            event.preventDefault();
            void save({ remoteUrl: address.trim() });
          }}
        >
          <label className="field">
            {t("remote.address")}
            <input
              value={address}
              inputMode="url"
              autoComplete="off"
              spellCheck={false}
              placeholder={status?.remoteUrl || "tv.example.com"}
              onChange={(event) => setAddress(event.target.value)}
            />
            <span className="hint">{t("remote.addressHint")}</span>
          </label>
          <span className="remote-buttons">
            <button type="submit" className="btn" disabled={busy || address.trim() === (status?.configured ?? "")}>
              {t("remote.save")}
            </button>
          </span>
        </form>
      </details>
      {error ? (
        <p className="error" role="alert">
          {error}
        </p>
      ) : null}
    </section>
  );
}

import { useCallback, useEffect, useId, useState, type FormEvent } from "react";
import { settingsUiMessages } from "../../i18n/messages/settingsUi.ts";
import { bind } from "../../i18n/runtime.ts";
import { isForbidden, isUnauthorized } from "../../lib/deviceToken";
import type { InviteGrant, RemoteStatus, Settings } from "../../types";
import { events } from "../../lib/events";
import { CloudflareCard } from "./CloudflareCard";
import { createInvite, deleteInvite, getRemote } from "./pairApi";
import { scopeLabel, scopesForAccess } from "./pairCode";
import { ChoiceRow, Group, Row } from "./rows";

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
  const id = useId();
  return (
    <div className="invite-copy">
      <label className="set-row-hint" htmlFor={id}>
        {label}
      </label>
      <span className="set-inline">
        <input id={id} className="set-input" readOnly value={value} spellCheck={false} onFocus={(event) => event.target.select()} />
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
          {copied ? t("remote.copied") : t("remote.copy")}
        </button>
      </span>
    </div>
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
  const whoId = useId();
  const tokenId = useId();
  const addressId = useId();

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
  // An invite names the address to open, so it waits for one.
  const canInvite = Boolean(status?.remoteUrl);

  return (
    <section aria-labelledby="remote-title" className="remote">
      <header className="set-group-head remote-head">
        <h3 id="remote-title">{t("remote.title")}</h3>
      </header>
      <p className="set-group-lede">{t("remote.lead")}</p>
      {status?.here ? <p className="set-group-lede">{t("remote.here")}</p> : null}
      <CloudflareCard remote={status} onRemote={reload} />
      <Group title={t("remote.invite")} lede={canInvite ? t("remote.inviteLede") : null}>
        {canInvite ? (
          <form className="remote-invite" onSubmit={(event) => void invite(event)}>
            <Row label={t("remote.inviteName")} labelFor={whoId} stack>
              <input id={whoId} className="set-input" value={who} maxLength={63} autoComplete="off" placeholder={t("remote.invitePlaceholder")} onChange={(event) => setWho(event.target.value)} />
            </Row>
            <ChoiceRow
              label={t("devices.codeAllows")}
              value={access}
              options={[
                { value: "watch", label: t("devices.watch") },
                { value: "record", label: t("devices.record") },
                { value: "admin", label: t("devices.admin") },
              ]}
              onChange={setAccess}
            />
            <div className="set-row">
              <button type="submit" className="btn primary" disabled={busy || who.trim() === ""}>
                {t("remote.inviteMake")}
              </button>
            </div>
          </form>
        ) : (
          <p className="set-note">{t("remote.inviteNeedsAddress")}</p>
        )}
        {made ? (
          <div className="set-row stack invite-made" role="status">
            <div className="set-row-text">
              <span className="set-row-label">{t("remote.inviteCode")}</span>
              <span className="set-row-hint">{t("remote.inviteHint")}</span>
            </div>
            <p className="pair-code set-code">{made.code}</p>
            {made.link ? <CopyRow label={t("remote.inviteLink")} value={made.link} /> : null}
            {made.appLink ? <CopyRow label={t("remote.inviteApp")} value={made.appLink} /> : null}
          </div>
        ) : null}
      </Group>
      {status && status.invites.length > 0 ? (
        <Group title={t("remote.waiting")}>
          <ul className="set-list">
            {status.invites.map((item) => (
              <li key={item.id} className="set-row">
                <div className="set-row-text">
                  <span className="set-row-label">{item.name}</span>
                  <span className="set-row-hint">
                    {scopeLabel(item.scopes ?? [])} · {t("remote.expires", { date: new Date(item.expiresAt).toLocaleDateString() })}
                  </span>
                </div>
                <button type="button" className="btn small ghost" aria-label={t("remote.removeNamed", { name: item.name })} disabled={busy} onClick={() => void remove(item.id)}>
                  {t("remote.removeInvite")}
                </button>
              </li>
            ))}
          </ul>
        </Group>
      ) : null}
      <details className="set-disclosure">
        <summary>
          <span className="set-row-label">{t("remote.cf.advanced")}</span>
          <span className="set-row-hint">{t("remote.cf.advancedHint")}</span>
        </summary>
        <div className="set-card">
          {line && status?.tokenSet ? (
            <p className={status?.tunnel.state === "error" || status?.tunnel.state === "missing" ? "set-note error" : "set-note"} role="status">
              {line}
            </p>
          ) : null}
          <form
            className="set-row stack"
            onSubmit={(event) => {
              event.preventDefault();
              if (token.trim()) void save({ tunnelToken: token.trim() });
            }}
          >
            <div className="set-row-text">
              <label className="set-row-label" htmlFor={tokenId}>
                {t("remote.token")}
              </label>
              <span className="set-row-hint">
                {t("remote.tokenHint")}{" "}
                <a href={guideURL} target="_blank" rel="noreferrer">
                  {t("remote.guide")}
                </a>
              </span>
            </div>
            <div className="set-inline">
              <input
                id={tokenId}
                className="set-input"
                type="password"
                value={token}
                autoComplete="off"
                spellCheck={false}
                placeholder={status?.tokenSet ? t("remote.tokenSet") : "cloudflared service install eyJ…"}
                onChange={(event) => setToken(event.target.value)}
              />
              <button type="submit" className="btn" disabled={busy || !token.trim()}>
                {t("remote.save")}
              </button>
            </div>
            {status?.tokenSet ? (
              <button type="button" className="btn small ghost remote-forget" disabled={busy} onClick={() => void save({ tunnelToken: "" })}>
                {t("remote.remove")}
              </button>
            ) : null}
          </form>
          <form
            className="set-row stack"
            onSubmit={(event) => {
              event.preventDefault();
              void save({ remoteUrl: address.trim() });
            }}
          >
            <div className="set-row-text">
              <label className="set-row-label" htmlFor={addressId}>
                {t("remote.address")}
              </label>
              <span className="set-row-hint">{t("remote.addressHint")}</span>
            </div>
            <div className="set-inline">
              <input
                id={addressId}
                className="set-input"
                value={address}
                inputMode="url"
                autoComplete="off"
                spellCheck={false}
                placeholder={status?.remoteUrl || "tv.example.com"}
                onChange={(event) => setAddress(event.target.value)}
              />
              <button type="submit" className="btn" disabled={busy || address.trim() === (status?.configured ?? "")}>
                {t("remote.save")}
              </button>
            </div>
          </form>
        </div>
      </details>
      {error ? (
        <p className="set-note error" role="alert">
          {error}
        </p>
      ) : null}
    </section>
  );
}

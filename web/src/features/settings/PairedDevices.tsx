import { useEffect, useState, type FormEvent } from "react";
import { getClientMe } from "../../api";
import { settingsUiMessages } from "../../i18n/messages/settingsUi.ts";
import { bind } from "../../i18n/runtime.ts";
import { approvePair, createPairCode, listClients, revokeClient } from "./pairApi";
import { navigate } from "../../app/router";
import { isForbidden, isUnauthorized } from "../../lib/deviceToken";
import { canEditSignIn, formatPairCode, kindLabel, normalizePairCode, scopeLabel, scopesForAccess } from "./pairCode";
import { lastSeenPhrase } from "../../time";
import type { ClientDevice } from "../../types";

const t = bind(settingsUiMessages);

type Access = "watch" | "record" | "admin";

function message(err: unknown): string {
  return err instanceof Error && err.message ? err.message : t("devices.failed");
}

export function PairedDevices({
  deviceAuth,
  onChange,
}: {
  deviceAuth: string;
  onChange: (values: { deviceAuth: string }) => Promise<void> | void;
}) {
  const [devices, setDevices] = useState<ClientDevice[] | null>(null);
  const [canEdit, setCanEdit] = useState(true);
  const [access, setAccess] = useState<Access>("watch");
  const [issued, setIssued] = useState("");
  const [code, setCode] = useState("");
  const [note, setNote] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const on = deviceAuth === "1";

  useEffect(() => {
    let stop = false;
    void listClients()
      .then((res) => {
        if (!stop) setDevices(res.devices ?? []);
      })
      .catch((err: unknown) => {
        if (stop) return;
        // A watch seat is not allowed to list devices. That is not a failure of this page.
        if (isForbidden(err) || isUnauthorized(err)) return;
        setError(message(err));
      });
    return () => {
      stop = true;
    };
  }, [deviceAuth]);

  useEffect(() => {
    let stop = false;
    void getClientMe()
      .then((seat) => {
        if (!stop) setCanEdit(canEditSignIn(seat));
      })
      .catch(() => {
        if (!stop) setCanEdit(false);
      });
    return () => {
      stop = true;
    };
  }, [deviceAuth]);

  async function changeAuth(value: string) {
    setError("");
    try {
      await onChange({ deviceAuth: value });
    } catch (err: unknown) {
      setError(message(err));
    }
  }

  async function issue() {
    setBusy(true);
    setError("");
    setNote("");
    try {
      const scopes = scopesForAccess(access);
      if (!scopes) return;
      const res = await createPairCode({ scopes, kind: "other" });
      setIssued(res.code);
    } catch (err: unknown) {
      setError(message(err));
    } finally {
      setBusy(false);
    }
  }

  async function revoke(device: ClientDevice) {
    if (!window.confirm(t("devices.revokeConfirm", { name: device.name }))) return;
    setBusy(true);
    setError("");
    setNote("");
    try {
      const res = await revokeClient(device.id);
      setDevices(res.devices ?? []);
    } catch (err: unknown) {
      setError(message(err));
    } finally {
      setBusy(false);
    }
  }

  async function approve(event: FormEvent) {
    event.preventDefault();
    const normalized = normalizePairCode(code);
    if (!normalized) {
      setNote("");
      setError(t("devices.needCode"));
      return;
    }
    setBusy(true);
    setError("");
    setNote("");
    try {
      const scopes = scopesForAccess(access);
      if (!scopes) return;
      const res = await approvePair({ code: normalized, scopes });
      setNote(t("devices.canSignIn", { name: res.device.name }));
      setCode("");
      setDevices((rows) => [res.device, ...(rows ?? []).filter((item) => item.id !== res.device.id)]);
    } catch (err: unknown) {
      setError(message(err));
    } finally {
      setBusy(false);
    }
  }

  return (
    <section>
      <h3 className="section-title">{t("devices.title")}</h3>
      <p className="hint">{t("devices.lead")}</p>
      {canEdit ? (
        <label className="field">
          {t("devices.require")}
          <select value={on ? "1" : "0"} onChange={(event) => void changeAuth(event.target.value)}>
            <option value="0">{t("settings.off")}</option>
            <option value="1">{t("settings.on")}</option>
          </select>
          <span className="hint">{on ? t("devices.closedHint") : t("devices.openHint")}</span>
        </label>
      ) : (
        <p className="hint">{on ? t("devices.closedHint") : t("devices.adminOnly")}</p>
      )}
      {error ? (
        <p className="error" role="alert">
          {error}
        </p>
      ) : null}
      {note ? (
        <p className="hint" role="status">
          {note}
        </p>
      ) : null}
      {canEdit && devices && devices.length === 0 ? <p className="empty">{t("devices.none")}</p> : null}
      {canEdit && devices && devices.length > 0 ? (
        <ul className="source-list">
          {devices.map((device) => {
            const seen = lastSeenPhrase(device.lastSeenAt);
            return (
              <li key={device.id} className="source-row">
                <span>{device.name}</span>
                <span>{kindLabel(device.kind)}</span>
                <span>{scopeLabel(device.scopes ?? [])}</span>
                <span className="hint">{seen || t("devices.notSeen")}</span>
                <button type="button" className="btn" aria-label={t("devices.revokeNamed", { name: device.name })} disabled={busy} onClick={() => void revoke(device)}>
                  {t("devices.revoke")}
                </button>
              </li>
            );
          })}
        </ul>
      ) : null}
      {canEdit ? (
        <>
          <label className="field">
            {t("devices.codeAllows")}
            <select value={access} onChange={(event) => setAccess(event.target.value as Access)}>
              <option value="watch">{t("devices.watch")}</option>
              <option value="record">{t("devices.record")}</option>
              <option value="admin">{t("devices.admin")}</option>
            </select>
          </label>
          <button type="button" className="btn" disabled={busy} onClick={() => void issue()}>
            {t("devices.pair")}
          </button>
          <button type="button" className="btn" onClick={() => navigate("/pair")}>
            {t("devices.pairBrowser")}
          </button>
          {issued ? (
            <>
              <p className="pair-code">{formatPairCode(issued)}</p>
              <p className="hint">{t("devices.codeHint")}</p>
            </>
          ) : null}
          <form onSubmit={(event) => void approve(event)}>
            <label className="field">
              {t("devices.approveCode")}
              <input
                value={code}
                inputMode="numeric"
                autoComplete="one-time-code"
                spellCheck={false}
                onChange={(event) => setCode(event.target.value)}
              />
            </label>
            <button type="submit" className="btn" disabled={busy}>
              {t("devices.approve")}
            </button>
          </form>
        </>
      ) : null}
    </section>
  );
}

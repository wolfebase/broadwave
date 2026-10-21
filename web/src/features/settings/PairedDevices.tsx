import { useEffect, useState, type FormEvent } from "react";
import { getClientMe } from "../../api";
import { settingsUiMessages } from "../../i18n/messages/settingsUi.ts";
import { bind } from "../../i18n/runtime.ts";
import { approvePair, createPairCode, listClients, revokeClient } from "./pairApi";
import { navigate } from "../../app/router";
import { isForbidden, isUnauthorized } from "../../lib/deviceToken";
import { canEditSignIn, formatPairCode, revokeWarnings, kindLabel, normalizePairCode, scopeLabel, scopesForAccess } from "./pairCode";
import { lastSeenPhrase } from "../../time";
import type { ClientDevice } from "../../types";
import { ChoiceRow, Group, SwitchRow } from "./rows";
import { Confirm } from "./Confirm";
import { settingsHubMessages } from "../../i18n/messages/settingsHub.ts";

const t = bind(settingsUiMessages);
const h = bind(settingsHubMessages);

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
  // The device this browser is signed in as, when it is.
  const [self, setSelf] = useState<{ id: string; name: string } | null>(null);
  const [access, setAccess] = useState<Access>("watch");
  const [issued, setIssued] = useState("");
  const [code, setCode] = useState("");
  const [note, setNote] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [asking, setAsking] = useState<ClientDevice | null>(null);
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
        if (stop) return;
        setCanEdit(canEditSignIn(seat));
        setSelf(seat?.device ? { id: seat.device.id, name: seat.device.name } : null);
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
    setAsking(null);
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
    <>
      <Group title={t("devices.title")} lede={t("devices.lead")}>
        {canEdit ? (
          <SwitchRow label={t("devices.require")} hint={on ? t("devices.closedHint") : t("devices.openHint")} checked={on} onChange={(next) => void changeAuth(next ? "1" : "0")} />
        ) : (
          <p className="set-note">{on ? t("devices.closedHint") : t("devices.adminOnly")}</p>
        )}
        {error ? (
          <p className="set-note error" role="alert">
            {error}
          </p>
        ) : null}
        {note ? (
          <p className="set-note" role="status">
            {note}
          </p>
        ) : null}
        {canEdit && devices && devices.length === 0 ? <p className="set-note">{t("devices.none")}</p> : null}
        {canEdit && devices && devices.length > 0 ? (
          <ul className="set-list">
            {devices.map((device) => {
              const seen = lastSeenPhrase(device.lastSeenAt);
              return (
                <li key={device.id} className="set-row">
                  <div className="set-row-text">
                    <span className="set-row-label">{device.name}</span>
                    <span className="set-row-hint">{[kindLabel(device.kind), scopeLabel(device.scopes ?? []), seen || t("devices.notSeen")].filter(Boolean).join(" · ")}</span>
                  </div>
                  <button type="button" className="btn small ghost" aria-label={t("devices.revokeNamed", { name: device.name })} disabled={busy} onClick={() => setAsking(device)}>
                    {t("devices.revoke")}
                  </button>
                </li>
              );
            })}
          </ul>
        ) : null}
      </Group>
      {canEdit ? (
        <Group title={t("devices.pair")}>
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
            <div className="set-row-text">
              <span className="set-row-label">{t("devices.showCode")}</span>
              <span className="set-row-hint">{issued ? t("devices.codeHint") : t("devices.showCodeHint")}</span>
            </div>
            <button type="button" className="btn small" disabled={busy} onClick={() => void issue()}>
              {issued ? t("devices.newCode") : t("devices.showCode")}
            </button>
          </div>
          {issued ? <p className="pair-code set-code">{formatPairCode(issued)}</p> : null}
          <form className="set-row stack" onSubmit={(event) => void approve(event)}>
            <div className="set-row-text">
              <label className="set-row-label" htmlFor="approve-code">
                {t("devices.approveCode")}
              </label>
              <span className="set-row-hint">{t("devices.approveHint")}</span>
            </div>
            <div className="set-inline">
              <input id="approve-code" className="set-input code" value={code} inputMode="numeric" maxLength={7} placeholder="000000" autoComplete="one-time-code" spellCheck={false} onChange={(event) => setCode(event.target.value)} />
              <button type="submit" className="btn" disabled={busy || normalizePairCode(code) == null}>
                {t("devices.approve")}
              </button>
            </div>
          </form>
          {self ? (
            <div className="set-row">
              <div className="set-row-text">
                <span className="set-row-label">{t("devices.thisBrowser")}</span>
                <span className="set-row-hint">{t("devices.signedInAs", { name: self.name })}</span>
              </div>
            </div>
          ) : (
            <div className="set-row">
              <div className="set-row-text">
                <span className="set-row-label">{t("devices.thisBrowser")}</span>
                <span className="set-row-hint">{t("devices.thisBrowserHint")}</span>
              </div>
              <button type="button" className="btn small ghost" onClick={() => navigate("/pair")}>
                {t("devices.pairBrowser")}
              </button>
            </div>
          )}
        </Group>
      ) : null}
      {asking ? (
        <Confirm
          title={t("devices.revokeNamed", { name: asking.name })}
          body={
            <>
              <p>{t("devices.revokeConfirm", { name: asking.name })}</p>
              {revokeWarnings(asking, devices ?? [], self?.id ?? "", on).map((line) => (
                <p key={line}>
                  <strong>{line}</strong>
                </p>
              ))}
            </>
          }
          confirm={t("devices.revoke")}
          cancel={h("backups.cancel")}
          busy={busy}
          onConfirm={() => void revoke(asking)}
          onCancel={() => setAsking(null)}
        />
      ) : null}
    </>
  );
}

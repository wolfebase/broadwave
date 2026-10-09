import { useEffect, useState, type FormEvent } from "react";
import { approvePair, createPairCode, getClientMe, listClients, revokeClient, unreadBody } from "../../api";
import { navigate } from "../../app/router";
import { canEditSignIn, formatPairCode, isForbidden, isUnauthorized, kindLabel, normalizePairCode, scopeLabel, scopesForAccess } from "../../lib/deviceToken";
import { lastSeenPhrase } from "../../time";
import type { ClientDevice } from "../../types";

type Access = "watch" | "record" | "admin";

const openHint = "Off leaves this home network open. On asks a new phone, TV, or browser for a code. This browser is signed in when you turn it on.";
const closedHint = "A new device needs a code. This browser stays signed in. Health checks, playlists, and the HDHomeRun port stay open. The guide needs a signed-in device.";

function message(err: unknown): string {
  return err instanceof Error && err.message ? err.message : unreadBody;
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
    if (!window.confirm(`Revoke ${device.name}? It will need a new code to sign in.`)) return;
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
      setError("Enter the 6-digit code.");
      return;
    }
    setBusy(true);
    setError("");
    setNote("");
    try {
      const scopes = scopesForAccess(access);
      if (!scopes) return;
      const res = await approvePair({ code: normalized, scopes });
      setNote(`${res.device.name} can sign in.`);
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
      <h3 className="section-title">Devices</h3>
      <p className="hint">Phones, TVs, and browsers that sign in to this server.</p>
      {canEdit ? (
        <label className="field">
          Require a device to sign in
          <select value={on ? "1" : "0"} onChange={(event) => void changeAuth(event.target.value)}>
            <option value="0">Off</option>
            <option value="1">On</option>
          </select>
          <span className="hint">{on ? closedHint : openHint}</span>
        </label>
      ) : (
        <p className="hint">{on ? closedHint : "An admin turns device sign-in on or off."}</p>
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
      {canEdit && devices && devices.length === 0 ? <p className="empty">No devices paired yet.</p> : null}
      {canEdit && devices && devices.length > 0 ? (
        <ul className="source-list">
          {devices.map((device) => {
            const seen = lastSeenPhrase(device.lastSeenAt);
            return (
              <li key={device.id} className="source-row">
                <span>{device.name}</span>
                <span>{kindLabel(device.kind)}</span>
                <span>{scopeLabel(device.scopes ?? [])}</span>
                <span className="hint">{seen || "Hasn't signed in yet."}</span>
                <button type="button" className="btn" aria-label={`Revoke ${device.name}`} disabled={busy} onClick={() => void revoke(device)}>
                  Revoke
                </button>
              </li>
            );
          })}
        </ul>
      ) : null}
      {canEdit ? (
        <>
          <label className="field">
            This code allows
            <select value={access} onChange={(event) => setAccess(event.target.value as Access)}>
              <option value="watch">Watch</option>
              <option value="record">Record</option>
              <option value="admin">Admin</option>
            </select>
          </label>
          <button type="button" className="btn" disabled={busy} onClick={() => void issue()}>
            Pair a device
          </button>
          <button type="button" className="btn" onClick={() => navigate("/pair")}>
            Pair this browser
          </button>
          {issued ? (
            <>
              <p className="pair-code">{formatPairCode(issued)}</p>
              <p className="hint">Enter this code on the other device. It lasts 10 minutes.</p>
            </>
          ) : null}
          <form onSubmit={(event) => void approve(event)}>
            <label className="field">
              Approve a code
              <input
                value={code}
                inputMode="numeric"
                autoComplete="one-time-code"
                spellCheck={false}
                onChange={(event) => setCode(event.target.value)}
              />
            </label>
            <button type="submit" className="btn" disabled={busy}>
              Approve
            </button>
          </form>
        </>
      ) : null}
    </section>
  );
}

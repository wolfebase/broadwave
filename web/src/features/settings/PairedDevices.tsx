import { useEffect, useState, type FormEvent } from "react";
import { approvePair, createPairCode, listClients, revokeClient, unreadBody } from "../../api";
import { navigate } from "../../app/router";
import { formatPairCode, kindLabel, normalizePairCode, scopeLabel } from "../../lib/deviceToken";
import { lastSeenPhrase } from "../../time";
import type { ClientDevice } from "../../types";

type Access = "watch" | "record" | "admin";

const accessScopes: Record<Access, string[]> = {
  watch: ["watch"],
  record: ["watch", "record"],
  admin: ["watch", "record", "admin"],
};

const openHint = "Off is the home network as it is today. On asks each phone, TV, and browser for a pairing code.";
const closedHint = "A device needs a pairing code. Health checks, the guide page, and apps such as Plex keep working.";

function message(err: unknown): string {
  return err instanceof Error && err.message ? err.message : unreadBody;
}

export function PairedDevices({
  deviceAuth,
  onChange,
}: {
  deviceAuth: string;
  onChange: (values: { deviceAuth: string }) => void;
}) {
  const [devices, setDevices] = useState<ClientDevice[] | null>(null);
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
        if (!stop) setError(message(err));
      });
    return () => {
      stop = true;
    };
  }, [deviceAuth]);

  async function issue() {
    setBusy(true);
    setError("");
    setNote("");
    try {
      const res = await createPairCode({ scopes: accessScopes[access], kind: "other" });
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
      const res = await approvePair({ code: normalized });
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
      <label className="field">
        Require a device to sign in
        <select value={on ? "1" : "0"} onChange={(event) => onChange({ deviceAuth: event.target.value })}>
          <option value="0">Off</option>
          <option value="1">On</option>
        </select>
        <span className="hint">{on ? closedHint : openHint}</span>
      </label>
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
      {devices && devices.length === 0 ? <p className="empty">No devices paired yet.</p> : null}
      {devices && devices.length > 0 ? (
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
    </section>
  );
}

import { useEffect, useRef, useState, type FormEvent } from "react";
import { claimPair, pollPair, startPair, unreadBody } from "../../api";
import { formatPairCode, normalizePairCode, pairPollStep, writeToken } from "../../lib/deviceToken";

type Kind = "web" | "phone" | "tv";

type PairSession = { id: string; secret: string };

function message(err: unknown): string {
  return err instanceof Error && err.message ? err.message : unreadBody;
}

export function PairScreen({ onPaired }: { onPaired: () => void }) {
  const [name, setName] = useState("");
  const [kind, setKind] = useState<Kind>("web");
  const [code, setCode] = useState("");
  const [issued, setIssued] = useState("");
  const [session, setSession] = useState<PairSession | null>(null);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const onPairedRef = useRef(onPaired);

  useEffect(() => {
    onPairedRef.current = onPaired;
  }, [onPaired]);

  useEffect(() => {
    if (!session) return;
    let stop = false;
    let paired = false;
    let missed = false;
    let timer = 0;
    const finish = (notice: string) => {
      stop = true;
      window.clearInterval(timer);
      setSession(null);
      setError(notice);
    };
    const tick = () => {
      if (stop) return;
      void pollPair(session.id, session.secret)
        .then((res) => {
          const step = pairPollStep(res.state, res.token, missed);
          missed = step.missed;
          // A token wins even if an earlier empty poll already gave up.
          // Two polls can be in flight, and the empty one can resolve first.
          if (step.action === "ready" && res.token) {
            if (paired) return;
            paired = true;
            stop = true;
            window.clearInterval(timer);
            writeToken(res.token);
            setSession(null);
            setError("");
            onPairedRef.current();
            return;
          }
          if (stop) return;
          if (step.action === "miss") {
            finish("The code was approved, but this screen missed the token. Show a new code.");
            return;
          }
          if (step.action === "expired" || step.action === "denied") {
            finish(step.action === "expired" ? "This code expired. Show a new code." : "This code was denied.");
            return;
          }
          setError("");
        })
        .catch((err: unknown) => {
          if (!stop) setError(message(err));
        });
    };
    timer = window.setInterval(tick, 2000);
    tick();
    return () => {
      stop = true;
      window.clearInterval(timer);
    };
  }, [session]);

  async function claim(event: FormEvent) {
    event.preventDefault();
    const trimmed = name.trim();
    const normalized = normalizePairCode(code);
    if (!trimmed) {
      setError("Enter a name.");
      return;
    }
    if (!normalized) {
      setError("Enter the 6-digit code.");
      return;
    }
    setBusy(true);
    setError("");
    setSession(null);
    try {
      const res = await claimPair({ code: normalized, name: trimmed, kind });
      if (!res.token) {
        setError("This device did not get a token. Try the code again.");
        return;
      }
      writeToken(res.token);
      onPaired();
    } catch (err: unknown) {
      setError(message(err));
    } finally {
      setBusy(false);
    }
  }

  async function showCode() {
    setBusy(true);
    setError("");
    setIssued("");
    setSession(null);
    try {
      const res = await startPair({
        name: name.trim() || "This browser",
        kind,
        scopes: ["watch"],
      });
      setIssued(res.code);
      if (!res.pollSecret) {
        setError("This screen cannot wait on that code. Show a new code.");
        return;
      }
      setSession({ id: res.id, secret: res.pollSecret });
    } catch (err: unknown) {
      setError(message(err));
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="page-wrap">
      <header className="page-header">
        <h1>Pair this device</h1>
        <p className="lede">Enter the code from Settings on a device that is already signed in.</p>
      </header>
      <form onSubmit={(event) => void claim(event)}>
        <label className="field">
          Name
          <input required value={name} autoComplete="off" spellCheck={false} onChange={(event) => setName(event.target.value)} />
        </label>
        <label className="field">
          Device
          <select value={kind} onChange={(event) => setKind(event.target.value as Kind)}>
            <option value="web">Browser</option>
            <option value="phone">Phone</option>
            <option value="tv">TV</option>
          </select>
        </label>
        <label className="field">
          Code
          <input
            required
            value={code}
            inputMode="numeric"
            autoComplete="one-time-code"
            spellCheck={false}
            onChange={(event) => setCode(event.target.value)}
          />
        </label>
        <button type="submit" className="btn primary" disabled={busy}>
          Pair
        </button>
      </form>
      <button type="button" className="btn" disabled={busy} onClick={() => void showCode()}>
        Show a code instead
      </button>
      {issued ? (
        <>
          <p className="pair-code">{formatPairCode(issued)}</p>
          <p className="hint">Enter this code in Settings. It lasts 10 minutes.</p>
        </>
      ) : null}
      {error ? (
        <p className="error" role="alert">
          {error}
        </p>
      ) : null}
    </div>
  );
}

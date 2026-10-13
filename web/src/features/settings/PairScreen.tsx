import { useEffect, useRef, useState, type FormEvent } from "react";
import { settingsUiMessages } from "../../i18n/messages/settingsUi.ts";
import { bind } from "../../i18n/runtime.ts";
import { writeToken } from "../../lib/deviceToken";
import { claimPair, pollPair, startPair } from "./pairApi";
import { codeAfterPoll, formatPairCode, normalizeClaimCode, normalizeInviteCode, pairPollStep } from "./pairCode";

const t = bind(settingsUiMessages);

type Kind = "web" | "phone" | "tv";

type PairSession = { id: string; secret: string };

// A link from an invite opens /pair?invite=CODE.
function invitedCode(): string {
  try {
    const raw = new URLSearchParams(window.location.search).get("invite") ?? "";
    return normalizeInviteCode(raw) ?? "";
  } catch {
    return "";
  }
}

function message(err: unknown): string {
  return err instanceof Error && err.message ? err.message : t("devices.failed");
}

export function PairScreen({ onPaired }: { onPaired: () => void }) {
  const [name, setName] = useState("");
  const [kind, setKind] = useState<Kind>("web");
  const [invite] = useState(invitedCode);
  const [code, setCode] = useState(invite);
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
    const finish = (notice: string, action: "miss" | "expired" | "denied") => {
      stop = true;
      window.clearInterval(timer);
      setSession(null);
      setIssued((current) => codeAfterPoll(action, current));
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
            finish(t("pair.missed"), "miss");
            return;
          }
          if (step.action === "expired" || step.action === "denied") {
            finish(step.action === "expired" ? t("pair.expired") : t("pair.denied"), step.action);
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
    const normalized = normalizeClaimCode(code);
    if (!trimmed) {
      setError(t("pair.needName"));
      return;
    }
    if (!normalized) {
      setError(t("pair.needCode"));
      return;
    }
    setBusy(true);
    setError("");
    setSession(null);
    try {
      const res = await claimPair({ code: normalized, name: trimmed, kind });
      if (!res.token) {
        setError(t("pair.noToken"));
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
        name: name.trim() || t("pair.thisBrowser"),
        kind,
        scopes: ["watch"],
      });
      setIssued(res.code);
      if (!res.pollSecret) {
        setError(t("pair.cannotWait"));
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
        <h1>{invite ? t("pair.inviteTitle") : t("pair.title")}</h1>
        <p className="lede">{invite ? t("pair.inviteLede") : t("pair.lede")}</p>
      </header>
      <form onSubmit={(event) => void claim(event)}>
        <label className="field">
          {t("pair.name")}
          <input required value={name} autoComplete="off" spellCheck={false} onChange={(event) => setName(event.target.value)} />
        </label>
        <label className="field">
          {t("pair.device")}
          <select value={kind} onChange={(event) => setKind(event.target.value as Kind)}>
            <option value="web">{t("devices.browser")}</option>
            <option value="phone">{t("devices.phone")}</option>
            <option value="tv">{t("devices.tv")}</option>
          </select>
        </label>
        <label className="field">
          {t("pair.code")}
          <input
            required
            value={code}
            autoComplete="one-time-code"
            autoCapitalize="characters"
            spellCheck={false}
            onChange={(event) => setCode(event.target.value)}
          />
        </label>
        <button type="submit" className="btn primary" disabled={busy}>
          {t("pair.submit")}
        </button>
      </form>
      {invite ? null : (
        <button type="button" className="btn" disabled={busy} onClick={() => void showCode()}>
          {t("pair.showCode")}
        </button>
      )}
      {issued ? (
        <>
          <p className="pair-code">{formatPairCode(issued)}</p>
          <p className="hint">{t("pair.codeHint")}</p>
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

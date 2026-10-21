import { useEffect, useRef, useState } from "react";
import { focusRing } from "../../app/remote";
import { pairMessages } from "../../i18n/messages/pair.ts";
import { settingsUiMessages } from "../../i18n/messages/settingsUi.ts";
import { bind } from "../../i18n/runtime.ts";
import { browserName } from "../../lib/events";
import { writeToken } from "../../lib/deviceToken";
import { claimPair, pollPair, startPair } from "./pairApi";
import { codeAfterPoll, codeLooksLikeInvite, detectKind, formatPairCode, normalizeClaimCode, normalizeInviteCode, normalizePairCode, pairPollStep, type PairKind } from "./pairCode";
import "../manage.css";
import "./pair.css";

const t = bind(settingsUiMessages);
const p = bind(pairMessages);
const lockedOutURL = "https://wolfebase.github.io/broadwave/docs/remote.html#locked-out";

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

function guessKind(): PairKind {
  return detectKind(navigator.userAgent, navigator.maxTouchPoints ?? 0, document.documentElement.dataset.layout ?? "");
}

const kinds: { id: PairKind; label: () => string }[] = [
  { id: "phone", label: () => p("pair.kindPhone") },
  { id: "web", label: () => p("pair.kindComputer") },
  { id: "tv", label: () => p("pair.kindTv") },
];

export function PairScreen({ onPaired }: { onPaired: () => void }) {
  const [name, setName] = useState(() => browserName());
  const [kind, setKind] = useState<PairKind>(guessKind);
  const [invite] = useState(invitedCode);
  const [code, setCode] = useState(invite);
  const [letters, setLetters] = useState(Boolean(invite));
  const [issued, setIssued] = useState("");
  const [session, setSession] = useState<PairSession | null>(null);
  const [error, setError] = useState("");
  const [showError, setShowError] = useState("");
  const [busy, setBusy] = useState(false);
  const onPairedRef = useRef(onPaired);
  const showRef = useRef<HTMLButtonElement>(null);
  const submitRef = useRef<HTMLButtonElement>(null);
  const codeRef = useRef<HTMLInputElement>(null);
  const invited = letters || codeLooksLikeInvite(code);

  useEffect(() => {
    onPairedRef.current = onPaired;
  }, [onPaired]);

  useEffect(() => {
    // A remote cannot type a code easily, so a TV starts on showing one.
    if (document.documentElement.dataset.layout !== "tv") return;
    const frame = window.requestAnimationFrame(() => focusRing(showRef.current ?? submitRef.current));
    return () => window.cancelAnimationFrame(frame);
  }, []);

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
      setShowError(notice);
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
            setShowError("");
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
          setShowError("");
        })
        .catch((err: unknown) => {
          if (!stop) setShowError(message(err));
        });
    };
    timer = window.setInterval(tick, 2000);
    tick();
    return () => {
      stop = true;
      window.clearInterval(timer);
    };
  }, [session]);

  async function claim(raw: string) {
    const trimmed = name.trim();
    const normalized = normalizeClaimCode(raw);
    if (!trimmed) {
      setError(t("pair.needName"));
      return;
    }
    if (!normalized) {
      setError(t("pair.needCode"));
      codeRef.current?.focus();
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
    setShowError("");
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
        setShowError(t("pair.cannotWait"));
        return;
      }
      setSession({ id: res.id, secret: res.pollSecret });
    } catch (err: unknown) {
      setShowError(message(err));
    } finally {
      setBusy(false);
    }
  }

  const help = invited ? p("pair.inviteHelp") : p("pair.codeHelp");

  return (
    <div className="pair-page manage">
      <section className="pair-card" aria-labelledby="pair-title">
        <header className="pair-head">
          <span className="brand-tally" aria-hidden="true" />
          <h1 id="pair-title">{invite ? t("pair.inviteTitle") : t("pair.title")}</h1>
          <p className="lede">{invite ? t("pair.inviteLede") : t("pair.lede")}</p>
        </header>
        <form
          className="pair-form"
          noValidate
          onSubmit={(event) => {
            event.preventDefault();
            if (!busy) void claim(code);
          }}
        >
          <div className="pair-group">
            <label className="field pair-code-field" htmlFor="pair-code">
              {invited ? p("pair.inviteLabel") : p("pair.codeLabel")}
            </label>
            <input
              ref={codeRef}
              id="pair-code"
              className={invited ? "pair-code-input letters" : "pair-code-input"}
              value={code}
              inputMode={invited ? "text" : "numeric"}
              pattern={invited ? undefined : "[0-9 ]*"}
              autoComplete="one-time-code"
              autoCapitalize="characters"
              autoCorrect="off"
              spellCheck={false}
              maxLength={invited ? 20 : 7}
              placeholder={invited ? "ABCD-EFGH-JKLM" : "000 000"}
              aria-describedby={error ? "pair-code-help pair-error" : "pair-code-help"}
              aria-invalid={error ? true : undefined}
              onChange={(event) => {
                const next = event.target.value;
                setCode(next);
                if (error) setError("");
                // Six digits is the whole code. Pair as soon as it is in.
                if (!invited && !busy && name.trim() && normalizePairCode(next) && !normalizePairCode(code)) void claim(next);
              }}
            />
            <span className="hint" id="pair-code-help">
              {help}
            </span>
            {error ? (
              <p className="error pair-error" id="pair-error" role="alert">
                {error}
              </p>
            ) : null}
            {invite ? null : (
              <button
                type="button"
                className="text-btn pair-switch"
                onClick={() => {
                  setLetters((on) => !on);
                  setCode("");
                  setError("");
                  window.requestAnimationFrame(() => codeRef.current?.focus());
                }}
              >
                {invited ? p("pair.useDigits") : p("pair.useInvite")}
              </button>
            )}
          </div>
          <fieldset className="pair-group">
            <legend>{p("pair.thisDevice")}</legend>
            <label className="field">
              {p("pair.nameLabel")}
              <input value={name} maxLength={40} autoComplete="off" spellCheck={false} onChange={(event) => setName(event.target.value)} />
              <span className="hint">{p("pair.nameHelp")}</span>
            </label>
            <div className="field">
              <span id="pair-kind-label">{p("pair.kindLabel")}</span>
              <div className="segmented pair-kinds" role="group" aria-labelledby="pair-kind-label">
                {kinds.map((item) => (
                  <button key={item.id} type="button" className={kind === item.id ? "seg on" : "seg"} aria-pressed={kind === item.id} onClick={() => setKind(item.id)}>
                    {item.label()}
                  </button>
                ))}
              </div>
            </div>
          </fieldset>
          <button ref={submitRef} type="submit" className="btn primary big pair-submit" disabled={busy}>
            {busy && !issued ? p("pair.pairing") : t("pair.submit")}
          </button>
        </form>
        {invite ? null : (
          <div className="pair-alt">
            <h2>{p("pair.or")}</h2>
            <p className="hint">{p("pair.orLede")}</p>
            {issued ? (
              <div className="pair-issued" role="status" aria-live="polite">
                <span className="kicker">{p("pair.yourCode")}</span>
                <p className="pair-code">{formatPairCode(issued)}</p>
                <p className="hint">{t("pair.codeHint")}</p>
                {session ? (
                  <p className="pair-waiting">
                    <span className="pair-spinner" aria-hidden="true" />
                    {p("pair.waiting")}
                  </p>
                ) : null}
              </div>
            ) : null}
            {showError ? (
              <p className="error" role="alert">
                {showError}
              </p>
            ) : null}
            <button ref={showRef} type="button" className="btn" disabled={busy} onClick={() => void showCode()}>
              {issued || showError ? p("pair.newCode") : t("pair.showCode")}
            </button>
          </div>
        )}
        {invite ? null : (
          <div className="pair-alt pair-locked">
            <h2>{p("pair.lockedTitle")}</h2>
            <p className="hint">{p("pair.lockedLede")}</p>
            <a href={lockedOutURL} target="_blank" rel="noreferrer">
              {p("pair.lockedLink")}
            </a>
          </div>
        )}
      </section>
    </div>
  );
}

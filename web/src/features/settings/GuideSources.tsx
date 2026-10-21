import { useEffect, useId, useState } from "react";
import { chooseSDLineup, findSDLineups, getGuideSources, loadXMLTV, type GuideSourcePullResult, type GuideSourceState, type SDLineupList } from "../../api";
import { guideSourcesMessages } from "../../i18n/messages/guideSources.ts";
import { bind, formatDateTime } from "../../i18n/runtime.ts";
import { actionName } from "../../lib/actionName";
import "./guideSources.css";

const t = bind(guideSourcesMessages);

type Saved = { user?: string; passwordSaved?: boolean; lineup?: string; url?: string };

function failure(e: unknown): string {
  return e instanceof Error ? e.message : String(e);
}

/**
 * Schedules Direct by postal code, or an XMLTV link. Each loads its listings
 * as soon as it is saved and says plainly what happened.
 */
export function GetListings({ saved, onLoaded, lede = false }: { saved: Saved; onLoaded?: (result: GuideSourcePullResult) => void; lede?: boolean }) {
  const ids = useId();
  const [user, setUser] = useState(saved.user ?? "");
  const [password, setPassword] = useState("");
  const [postal, setPostal] = useState("");
  const [found, setFound] = useState<SDLineupList | null>(null);
  const [pick, setPick] = useState(saved.lineup ?? "");
  const [url, setUrl] = useState("");
  const [busy, setBusy] = useState<"" | "find" | "sd" | "xmltv">("");
  const [note, setNote] = useState<{ text: string; bad: boolean } | null>(null);

  const loaded = (result: GuideSourcePullResult) => {
    setNote({ text: t("gs.loaded", result.channels), bad: false });
    onLoaded?.(result);
  };

  const find = () => {
    setBusy("find");
    setNote(null);
    setFound(null);
    void findSDLineups(user.trim(), password, postal)
      .then((list) => {
        setFound(list);
        const mine = list.lineups.find((l) => l.id === saved.lineup) ?? list.lineups.find((l) => list.account.lineups.includes(l.id)) ?? list.lineups[0];
        setPick(mine?.id ?? "");
        if (list.lineups.length === 0) setNote({ text: t("gs.none"), bad: true });
      })
      .catch((e: unknown) => setNote({ text: failure(e), bad: true }))
      .finally(() => setBusy(""));
  };

  const takeLineup = () => {
    setBusy("sd");
    setNote({ text: t("gs.loading"), bad: false });
    void chooseSDLineup(user.trim(), password, pick)
      .then(loaded)
      .catch((e: unknown) => setNote({ text: failure(e), bad: true }))
      .finally(() => setBusy(""));
  };

  const takeLink = () => {
    setBusy("xmltv");
    setNote({ text: t("gs.loading"), bad: false });
    void loadXMLTV(url.trim())
      .then(loaded)
      .catch((e: unknown) => setNote({ text: failure(e), bad: true }))
      .finally(() => setBusy(""));
  };

  const account = found?.account;
  return (
    <div className="gs" aria-busy={busy ? true : undefined}>
      {lede ? <p className="gs-lede">{t("gs.lede")}</p> : null}
      <section className="gs-card" aria-labelledby={`${ids}-sd`}>
        <h4 id={`${ids}-sd`}>{t("gs.sdTitle")}</h4>
        <p className="gs-hint">{t("gs.sdHint")}</p>
        <form
          className="gs-form"
          onSubmit={(event) => {
            event.preventDefault();
            if (!busy && postal.trim()) find();
          }}
        >
          <label className="gs-field">
            <span>{t("gs.user")}</span>
            <input value={user} onChange={(e) => setUser(e.target.value)} autoComplete="username" spellCheck={false} />
          </label>
          <label className="gs-field">
            <span>{t("gs.password")}</span>
            <input
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              autoComplete="current-password"
              placeholder={saved.passwordSaved && user.trim() === (saved.user ?? "") ? t("gs.passwordSaved") : ""}
            />
          </label>
          <label className="gs-field gs-postal">
            <span>{t("gs.postal")}</span>
            <input value={postal} onChange={(e) => setPostal(e.target.value)} autoComplete="postal-code" placeholder={t("gs.postalPlaceholder")} spellCheck={false} />
          </label>
          <button type="submit" className="btn" disabled={busy !== "" || !postal.trim() || !user.trim()}>
            {busy === "find" ? t("gs.finding") : t("gs.find")}
          </button>
        </form>
        {found && found.lineups.length > 0 ? (
          <fieldset className="gs-lineups">
            <legend>{t("gs.pick")}</legend>
            {found.lineups.map((l) => (
              <label key={l.id} className="gs-lineup" data-on={pick === l.id ? "1" : undefined}>
                <input type="radio" name={`${ids}-lineup`} value={l.id} checked={pick === l.id} onChange={() => setPick(l.id)} />
                <span className="gs-lineup-text">
                  <strong>{l.name || l.id}</strong>
                  <span className="gs-hint">
                    {[l.transport, l.location && l.location !== found.postalCode ? l.location : "", l.id].filter(Boolean).join(" · ")}
                  </span>
                </span>
                {l.id === saved.lineup ? <span className="gs-badge">{t("gs.current")}</span> : account?.lineups.includes(l.id) ? <span className="gs-badge">{t("gs.onAccount")}</span> : null}
              </label>
            ))}
            {account && account.maxLineups > 0 ? <p className="gs-hint">{t("gs.room", account.lineups.length, { max: account.maxLineups })}</p> : null}
            <div>
              <button type="button" className="btn primary" disabled={busy !== "" || !pick} onClick={takeLineup}>
                {t("gs.use")}
              </button>
            </div>
          </fieldset>
        ) : null}
      </section>
      <section className="gs-card" aria-labelledby={`${ids}-xmltv`}>
        <h4 id={`${ids}-xmltv`}>{t("gs.xmltvTitle")}</h4>
        <p className="gs-hint">{t("gs.xmltvHint")}</p>
        <form
          className="gs-inline"
          onSubmit={(event) => {
            event.preventDefault();
            if (!busy && url.trim()) takeLink();
          }}
        >
          <input aria-label={t("gs.xmltvTitle")} value={url} onChange={(e) => setUrl(e.target.value)} placeholder={saved.url || t("gs.xmltvPlaceholder")} spellCheck={false} inputMode="url" />
          <button type="submit" className="btn" disabled={busy !== "" || !url.trim()}>
            {t("gs.load")}
          </button>
        </form>
      </section>
      {note ? (
        <p className={note.bad ? "gs-note bad" : "gs-note"} role={note.bad ? "alert" : "status"}>
          {note.text}
        </p>
      ) : null}
    </div>
  );
}

/** Each listing source: whether it is set up, its last pull, and its last failure. */
export function GuideSourceList({ version = 0, onTurnOff }: { version?: number; onTurnOff?: (source: string) => void }) {
  const [sources, setSources] = useState<GuideSourceState[] | null>(null);
  useEffect(() => {
    let live = true;
    void getGuideSources()
      .then((r) => live && setSources(r.sources))
      .catch(() => live && setSources(null));
    return () => {
      live = false;
    };
  }, [version]);
  if (!sources) return null;
  return (
    <ul className="gs-sources" aria-label={t("gs.status")}>
      {sources.map((s) => (
        <li key={s.source} data-state={!s.on ? "off" : s.error ? "bad" : s.lastOk ? "ok" : "wait"}>
          <span className="gs-source-text">
            <strong>{sourceName(s.source)}</strong>
            <span className="gs-hint">{sourceLine(s)}</span>
          </span>
          {onTurnOff && s.on && s.source !== "silicondust" ? (
            <button type="button" className="btn small ghost" aria-label={actionName(t("gs.turnOff"), sourceName(s.source))} onClick={() => onTurnOff(s.source)}>
              {t("gs.turnOff")}
            </button>
          ) : null}
        </li>
      ))}
    </ul>
  );
}

function sourceName(source: string): string {
  if (source === "silicondust") return t("gs.source.silicondust");
  if (source === "schedules-direct") return t("gs.source.schedules-direct");
  if (source === "xmltv") return t("gs.source.xmltv");
  return source;
}

function when(iso: string): string {
  return formatDateTime(new Date(iso), { weekday: "short", hour: "numeric", minute: "2-digit" });
}

export function sourceLine(s: GuideSourceState): string {
  if (!s.on) return t("gs.off");
  const parts: string[] = [];
  if (s.lastOk) parts.push(t("gs.ok", s.listings ?? 0, { when: when(s.lastOk) }));
  if (s.error) parts.push(t("gs.failed", { why: s.error }));
  if (!s.lastOk && !s.error) parts.push(t("gs.waiting"));
  if (s.next && (s.error || !s.lastOk)) parts.push(t("gs.next", { when: when(s.next) }));
  return parts.join(" ");
}

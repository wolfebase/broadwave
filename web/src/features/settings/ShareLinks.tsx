import { useEffect, useRef, useState } from "react";
import { shareRows } from "./tunerShare";

export function ShareLinks({ pageURL, sharing, hdhr = true }: { pageURL: string; sharing: boolean; hdhr?: boolean }) {
  const rows = shareRows(pageURL, sharing, hdhr);
  const apps = [...new Set(rows.map((row) => row.app))];
  const [copied, setCopied] = useState("");
  const timer = useRef(0);
  useEffect(() => () => window.clearTimeout(timer.current), []);
  return (
    <div className="share-apps">
      {apps.map((app) => (
        <section key={app} className="share-app" aria-labelledby={`share-${app}`}>
          <h3 id={`share-${app}`} className="section-title">
            {app}
          </h3>
          <ul className="share-rows">
            {rows
              .filter((row) => row.app === app)
              .map((row) => {
                const key = `${app} ${row.label}`;
                return (
                  <li key={row.label} className="share-row">
                    <div className="share-row-text">
                      <span>{row.label}</span>
                      <code>{row.value}</code>
                      {row.hint ? <span className="hint">{row.hint}</span> : null}
                    </div>
                    <button
                      type="button"
                      className="btn"
                      aria-label={`Copy ${key}`}
                      onClick={() => {
                        const write = navigator.clipboard?.writeText(row.value);
                        if (!write) return;
                        void write.then(() => {
                          setCopied(key);
                          window.clearTimeout(timer.current);
                          timer.current = window.setTimeout(() => setCopied((current) => (current === key ? "" : current)), 2000);
                        }, () => undefined);
                      }}
                    >
                      {copied === key ? "Copied" : "Copy"}
                    </button>
                  </li>
                );
              })}
          </ul>
        </section>
      ))}
      <p className="sr-only" aria-live="polite">
        {copied ? "Copied." : ""}
      </p>
    </div>
  );
}

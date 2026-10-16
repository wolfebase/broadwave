import { useEffect, useRef, useState, type ReactNode } from "react";
import { ChevronIcon } from "./icons";
import { formatDateTime } from "../i18n/runtime.ts";
import { navigate } from "../app/router";
import type { Recording } from "../types";
import { Progress, SectionHeader } from "./primitives";
import "./shelf.css";

export function Shelf({ title, action, children }: { title: string; action?: ReactNode; children: ReactNode }) {
  const row = useRef<HTMLDivElement>(null);
  const [ends, setEnds] = useState({ left: false, right: false });
  useEffect(() => {
    const el = row.current;
    if (!el) return;
    const sync = () => {
      const left = el.scrollLeft > 4;
      const right = el.scrollLeft + el.clientWidth < el.scrollWidth - 4;
      setEnds((cur) => (cur.left === left && cur.right === right ? cur : { left, right }));
    };
    sync();
    el.addEventListener("scroll", sync, { passive: true });
    const watch = new ResizeObserver(sync);
    watch.observe(el);
    return () => {
      el.removeEventListener("scroll", sync);
      watch.disconnect();
    };
  }, []);
  // A mouse has no sideways swipe. The arrows page the row; keys and touch scroll it already.
  function page(dir: 1 | -1) {
    const el = row.current;
    if (!el) return;
    const reduce = window.matchMedia?.("(prefers-reduced-motion: reduce)").matches;
    el.scrollBy({ left: dir * el.clientWidth * 0.85, behavior: reduce ? "auto" : "smooth" });
  }
  return (
    <section className="shelf">
      <SectionHeader title={title} action={action} />
      <div className="shelf-frame">
        <div ref={row} className="shelf-row">
          {children}
        </div>
        {ends.left ? (
          <button type="button" className="shelf-arrow left glass-icon" tabIndex={-1} aria-hidden="true" onClick={() => page(-1)}>
            <ChevronIcon style={{ transform: "rotate(180deg)" }} />
          </button>
        ) : null}
        {ends.right ? (
          <button type="button" className="shelf-arrow right glass-icon" tabIndex={-1} aria-hidden="true" onClick={() => page(1)}>
            <ChevronIcon />
          </button>
        ) : null}
      </div>
    </section>
  );
}

export function RecordingCard({ rec }: { rec: Recording }) {
  const pct = rec.durationSec && rec.position ? rec.position / rec.durationSec : 0;
  return (
    <button type="button" className="rec-card" onClick={() => navigate(`/play?recording=${rec.id}`)}>
      <span className="rc-poster">
        <img
          src={`/media/poster/${rec.id}`}
          alt=""
          loading="lazy"
          onError={(e) => {
            const img = e.currentTarget;
            if (img.dataset.fallback) {
              img.style.visibility = "hidden";
              return;
            }
            img.dataset.fallback = "1";
            img.src = `/media/art/channel/${rec.channelId}?w=320`;
          }}
        />
        {pct > 0 ? <Progress value={pct} /> : null}
      </span>
      <span className="rc-title">{rec.title}</span>
      <span className="rc-sub">{rec.subtitle || formatDateTime(new Date(rec.startedAt), { month: "short", day: "numeric" })}</span>
    </button>
  );
}

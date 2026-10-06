import type { ReactNode } from "react";
import { navigate } from "../app/router";
import type { Recording } from "../types";
import { Progress, SectionHeader } from "./primitives";
import "./shelf.css";

export function Shelf({ title, action, children }: { title: string; action?: ReactNode; children: ReactNode }) {
  return (
    <section className="shelf">
      <SectionHeader title={title} action={action} />
      <div className="shelf-row">{children}</div>
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
      <span className="rc-sub">{rec.subtitle || new Date(rec.startedAt).toLocaleDateString([], { month: "short", day: "numeric" })}</span>
    </button>
  );
}

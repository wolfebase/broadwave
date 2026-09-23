import type { ReactNode } from "react";
import type { Category } from "../lib/guide";
import type { Channel } from "../types";

export function ChannelBadge({ channel, size = "md" }: { channel: Pick<Channel, "displayNumber" | "displayName">; size?: "sm" | "md" | "lg" }) {
  return (
    <span className={`ch-badge ch-badge-${size}`}>
      <span className="ch-badge-num">{channel.displayNumber}</span>
      <span className="ch-badge-name">{channel.displayName}</span>
    </span>
  );
}

export function LiveDot({ label = "Live" }: { label?: string }) {
  return (
    <span className="live-dot">
      <span className="live-dot-light" aria-hidden="true" />
      {label}
    </span>
  );
}

export function RecDot({ scheduled }: { scheduled?: boolean }) {
  return <span className={scheduled ? "rec-dot scheduled" : "rec-dot"} aria-label={scheduled ? "Will record" : "Recording"} title={scheduled ? "Will record" : "Recording"} />;
}

export function Progress({ value, category }: { value: number; category?: Category }) {
  return (
    <span className="progress" data-cat={category}>
      <span className="progress-fill" style={{ transform: `scaleX(${Math.max(0, Math.min(1, value))})` }} />
    </span>
  );
}

export function Chip({ on, onClick, children, count }: { on?: boolean; onClick?: () => void; children: ReactNode; count?: number }) {
  return (
    <button type="button" className={on ? "chip on" : "chip"} aria-pressed={on} onClick={onClick}>
      {children}
      {count != null ? <span className="chip-count">{count}</span> : null}
    </button>
  );
}

export function Empty({ title, children, action }: { title: string; children?: ReactNode; action?: ReactNode }) {
  return (
    <div className="empty-state">
      <h3>{title}</h3>
      {children ? <p>{children}</p> : null}
      {action}
    </div>
  );
}

export function SectionHeader({ title, action }: { title: string; action?: ReactNode }) {
  return (
    <div className="section-header">
      <h2>{title}</h2>
      {action}
    </div>
  );
}

import type { ReactNode } from "react";
import { uiMessages } from "../i18n/messages/ui.ts";
import { bind } from "../i18n/runtime.ts";
import type { Category } from "../lib/guide";
import type { Channel } from "../types";

const t = bind(uiMessages);

export function ChannelBadge({ channel, size = "md" }: { channel: Pick<Channel, "displayNumber" | "displayName" | "standard">; size?: "sm" | "md" | "lg" }) {
  return (
    <span className={`ch-badge ch-badge-${size}`}>
      <span className="ch-badge-num">{channel.displayNumber}</span>
      <span className="ch-badge-name">{channel.displayName}</span>
      {channel.standard === "atsc3" ? <Atsc3Tag /> : null}
    </span>
  );
}

export function Atsc3Tag() {
  return (
    <span className="tag" aria-label={t("ui.atsc3")}>
      {t("ui.atsc3Short")}
    </span>
  );
}

export function LiveDot({ label }: { label?: string }) {
  return (
    <span className="live-dot">
      <span className="live-dot-light" aria-hidden="true" />
      {label ?? t("ui.live")}
    </span>
  );
}

export function RecDot({ scheduled }: { scheduled?: boolean }) {
  const label = scheduled ? t("ui.willRecord") : t("ui.recording");
  return <span className={scheduled ? "rec-dot scheduled" : "rec-dot"} role="img" aria-label={label} title={label} />;
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
      <h2>{title}</h2>
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

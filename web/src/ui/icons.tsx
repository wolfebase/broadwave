import type { SVGProps } from "react";

type P = SVGProps<SVGSVGElement>;

function Base({ children, ...rest }: P) {
  return (
    <svg viewBox="0 0 24 24" width="1em" height="1em" fill="none" stroke="currentColor" strokeWidth={1.8} strokeLinecap="round" strokeLinejoin="round" aria-hidden="true" {...rest}>
      {children}
    </svg>
  );
}

export const HomeIcon = (p: P) => (
  <Base {...p}>
    <path d="M4 11.5 12 5l8 6.5V19a1 1 0 0 1-1 1h-4.5v-5h-5v5H5a1 1 0 0 1-1-1z" />
  </Base>
);
export const GuideIcon = (p: P) => (
  <Base {...p}>
    <rect x="3.5" y="4.5" width="17" height="15" rx="2.5" />
    <path d="M3.5 9.5h17M9 9.5v10M14.5 14.5h6" />
  </Base>
);
export const SportsIcon = (p: P) => (
  <Base {...p}>
    <circle cx="12" cy="12" r="8.5" />
    <path d="M12 3.5v17M3.5 12h17M6 6c3 2.2 3 9.8 0 12M18 6c-3 2.2-3 9.8 0 12" />
  </Base>
);
export const RecordingsIcon = (p: P) => (
  <Base {...p}>
    <rect x="3.5" y="7" width="17" height="12.5" rx="2.5" />
    <path d="M6.5 4.5h11" />
    <path d="m10.5 10.5 4 2.75-4 2.75z" fill="currentColor" />
  </Base>
);
export const ScheduleIcon = (p: P) => (
  <Base {...p}>
    <rect x="3.5" y="5" width="17" height="15" rx="2.5" />
    <path d="M3.5 10h17M8 3v4M16 3v4" />
    <circle cx="12" cy="15" r="1.6" fill="currentColor" stroke="none" />
  </Base>
);
export const SettingsIcon = (p: P) => (
  <Base {...p}>
    <circle cx="12" cy="12" r="3" />
    <path d="M19.4 15a1.7 1.7 0 0 0 .3 1.8l.1.1a2 2 0 1 1-2.8 2.8l-.1-.1a1.7 1.7 0 0 0-1.8-.3 1.7 1.7 0 0 0-1 1.5V21a2 2 0 1 1-4 0v-.1a1.7 1.7 0 0 0-1.1-1.5 1.7 1.7 0 0 0-1.8.3l-.1.1a2 2 0 1 1-2.8-2.8l.1-.1a1.7 1.7 0 0 0 .3-1.8 1.7 1.7 0 0 0-1.5-1H3a2 2 0 1 1 0-4h.1a1.7 1.7 0 0 0 1.5-1.1 1.7 1.7 0 0 0-.3-1.8l-.1-.1a2 2 0 1 1 2.8-2.8l.1.1a1.7 1.7 0 0 0 1.8.3H9a1.7 1.7 0 0 0 1-1.5V3a2 2 0 1 1 4 0v.1a1.7 1.7 0 0 0 1 1.5 1.7 1.7 0 0 0 1.8-.3l.1-.1a2 2 0 1 1 2.8 2.8l-.1.1a1.7 1.7 0 0 0-.3 1.8V9a1.7 1.7 0 0 0 1.5 1H21a2 2 0 1 1 0 4h-.1a1.7 1.7 0 0 0-1.5 1z" />
  </Base>
);
export const PlayIcon = (p: P) => (
  <Base {...p} stroke="none">
    <path d="M8 5.2v13.6a.8.8 0 0 0 1.2.7l10.6-6.8a.8.8 0 0 0 0-1.4L9.2 4.5A.8.8 0 0 0 8 5.2z" fill="currentColor" />
  </Base>
);
export const PauseIcon = (p: P) => (
  <Base {...p} stroke="none">
    <rect x="6.5" y="5" width="4" height="14" rx="1.2" fill="currentColor" />
    <rect x="13.5" y="5" width="4" height="14" rx="1.2" fill="currentColor" />
  </Base>
);
export const BackIcon = (p: P) => (
  <Base {...p}>
    <path d="M15 5.5 8.5 12l6.5 6.5" />
  </Base>
);
export const CloseIcon = (p: P) => (
  <Base {...p}>
    <path d="M6 6l12 12M18 6 6 18" />
  </Base>
);
export const RecordIcon = (p: P) => (
  <Base {...p} stroke="none">
    <circle cx="12" cy="12" r="6.5" fill="currentColor" />
  </Base>
);
export const StarIcon = ({ filled, ...p }: P & { filled?: boolean }) => (
  <Base {...p}>
    <path d="m12 3.8 2.5 5.2 5.7.8-4.1 4 1 5.6-5.1-2.7-5.1 2.7 1-5.6-4.1-4 5.7-.8z" fill={filled ? "currentColor" : "none"} />
  </Base>
);
export const SearchIcon = (p: P) => (
  <Base {...p}>
    <circle cx="11" cy="11" r="6.5" />
    <path d="m20 20-4.2-4.2" />
  </Base>
);
export const ExpandIcon = (p: P) => (
  <Base {...p}>
    <path d="M4 9V4h5M20 9V4h-5M4 15v5h5M20 15v5h-5" />
  </Base>
);
export const PipIcon = (p: P) => (
  <Base {...p}>
    <rect x="3" y="5" width="18" height="14" rx="2.5" />
    <rect x="12" y="11.5" width="6.5" height="5" rx="1.2" fill="currentColor" stroke="none" />
  </Base>
);
export const SyncIcon = (p: P) => (
  <Base {...p}>
    <path d="M4 12a8 8 0 0 1 13.7-5.6L20 8.5M20 4v4.5h-4.5M20 12a8 8 0 0 1-13.7 5.6L4 15.5M4 20v-4.5h4.5" />
  </Base>
);
export const InfoIcon = (p: P) => (
  <Base {...p}>
    <circle cx="12" cy="12" r="8.5" />
    <path d="M12 11v5.5M12 7.8v.2" />
  </Base>
);
export const ListIcon = (p: P) => (
  <Base {...p}>
    <path d="M8 6.5h12M8 12h12M8 17.5h12M4 6.5h.01M4 12h.01M4 17.5h.01" />
  </Base>
);
export const VolumeIcon = ({ muted, ...p }: P & { muted?: boolean }) => (
  <Base {...p}>
    <path d="M4 9.5h3.5L12 5.5v13l-4.5-4H4z" />
    {muted ? <path d="m16 9.5 5 5M21 9.5l-5 5" /> : <path d="M15.5 9a4 4 0 0 1 0 6M18 6.5a7.5 7.5 0 0 1 0 11" />}
  </Base>
);
export const ChevronIcon = (p: P) => (
  <Base {...p}>
    <path d="m9 5.5 6.5 6.5L9 18.5" />
  </Base>
);

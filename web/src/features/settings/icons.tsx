import type { SVGProps } from "react";

type P = SVGProps<SVGSVGElement>;

function Base({ children, ...rest }: P) {
  return (
    <svg viewBox="0 0 24 24" width="1em" height="1em" fill="none" stroke="currentColor" strokeWidth={1.8} strokeLinecap="round" strokeLinejoin="round" aria-hidden="true" {...rest}>
      {children}
    </svg>
  );
}

export const AppearanceIcon = (p: P) => (
  <Base {...p}>
    <circle cx="12" cy="12" r="8.5" />
    <path d="M12 3.5a8.5 8.5 0 0 0 0 17z" fill="currentColor" stroke="none" />
  </Base>
);

export const AntennaIcon = (p: P) => (
  <Base {...p}>
    <path d="M12 11v9.5M8.5 20.5h7" />
    <circle cx="12" cy="9" r="2" />
    <path d="M7.8 13.2a6 6 0 0 1 0-8.4M16.2 4.8a6 6 0 0 1 0 8.4M5 16a10 10 0 0 1 0-14M19 2a10 10 0 0 1 0 14" />
  </Base>
);

export const PlaybackIcon = (p: P) => (
  <Base {...p}>
    <rect x="3" y="4.5" width="18" height="13" rx="2.5" />
    <path d="m10.5 8.5 4 2.5-4 2.5z" fill="currentColor" />
    <path d="M8 20.5h8" />
  </Base>
);

export const GuideDataIcon = (p: P) => (
  <Base {...p}>
    <rect x="3.5" y="4.5" width="17" height="15" rx="2.5" />
    <path d="M3.5 9.5h17M9 9.5v10M14.5 14.5h6" />
  </Base>
);

export const DiskIcon = (p: P) => (
  <Base {...p}>
    <ellipse cx="12" cy="6" rx="7.5" ry="2.5" />
    <path d="M4.5 6v12c0 1.4 3.4 2.5 7.5 2.5s7.5-1.1 7.5-2.5V6M4.5 12c0 1.4 3.4 2.5 7.5 2.5s7.5-1.1 7.5-2.5" />
  </Base>
);

export const DevicesIcon = (p: P) => (
  <Base {...p}>
    <rect x="2.5" y="4.5" width="14" height="10" rx="1.8" />
    <path d="M6.5 18.5h6M9.5 14.5v4" />
    <rect x="16.5" y="9" width="5" height="10.5" rx="1.4" />
  </Base>
);

export const LinkIcon = (p: P) => (
  <Base {...p}>
    <path d="M10 14a4 4 0 0 0 5.7 0l3-3a4 4 0 0 0-5.7-5.7l-1.2 1.2" />
    <path d="M14 10a4 4 0 0 0-5.7 0l-3 3a4 4 0 0 0 5.7 5.7l1.2-1.2" />
  </Base>
);

export const BackupIcon = (p: P) => (
  <Base {...p}>
    <path d="M4 12a8 8 0 1 0 2.3-5.6" />
    <path d="M4 4.5V8h3.5M12 8v4.5l3 1.8" />
  </Base>
);

export const AboutIcon = (p: P) => (
  <Base {...p}>
    <circle cx="12" cy="12" r="8.5" />
    <path d="M12 11v5.5" />
    <circle cx="12" cy="7.8" r="0.6" fill="currentColor" />
  </Base>
);

export const ChevronRight = (p: P) => (
  <Base {...p}>
    <path d="m9.5 6 6 6-6 6" />
  </Base>
);

export const ChevronLeft = (p: P) => (
  <Base {...p}>
    <path d="m14.5 6-6 6 6 6" />
  </Base>
);

export const TvIcon = (p: P) => (
  <Base {...p}>
    <rect x="3" y="5" width="18" height="12" rx="2" />
    <path d="M8 20h8" />
  </Base>
);

export const PhoneIcon = (p: P) => (
  <Base {...p}>
    <rect x="7" y="3" width="10" height="18" rx="2.2" />
    <path d="M11 18h2" />
  </Base>
);

export const SpeakerIcon = (p: P) => (
  <Base {...p}>
    <rect x="6" y="3" width="12" height="18" rx="2.5" />
    <circle cx="12" cy="14" r="3.2" />
    <circle cx="12" cy="7.2" r="0.8" fill="currentColor" />
  </Base>
);

export const ServerIcon = (p: P) => (
  <Base {...p}>
    <rect x="4" y="4" width="16" height="7" rx="1.8" />
    <rect x="4" y="13" width="16" height="7" rx="1.8" />
    <path d="M8 7.5h.01M8 16.5h.01" />
  </Base>
);

export const SearchSmallIcon = (p: P) => (
  <Base {...p}>
    <circle cx="10.5" cy="10.5" r="6" />
    <path d="m15 15 5 5" />
  </Base>
);

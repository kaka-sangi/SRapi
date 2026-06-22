import * as React from "react";
import { cn } from "@/lib/cn";

export type IllustrationKey =
  | "logs"
  | "accounts"
  | "users"
  | "search"
  | "cog"
  | "bell"
  | "inbox"
  | "chart";

export interface IllustratedEmptyStateProps {
  illust: IllustrationKey;
  title: string;
  description?: string;
  action?: React.ReactNode;
  className?: string;
}

/**
 * Richer than the bare `EmptyState` — pairs an abstract geometric SVG with a
 * title/description/action stack inside the same dashed bordered container.
 */
export function IllustratedEmptyState({
  illust,
  title,
  description,
  action,
  className,
}: IllustratedEmptyStateProps) {
  return (
    <div
      className={cn(
        "mx-auto flex max-w-md flex-col items-center justify-center gap-3 p-8 text-center",
        className,
      )}
    >
      <div className="text-srapi-primary/80" aria-hidden>
        {illustrations[illust]}
      </div>
      <div className="text-base font-semibold tracking-tight text-srapi-text-primary">
        {title}
      </div>
      {description ? (
        <p className="max-w-sm text-sm leading-relaxed text-srapi-text-secondary">
          {description}
        </p>
      ) : null}
      {action ? <div className="mt-1.5">{action}</div> : null}
    </div>
  );
}
IllustratedEmptyState.displayName = "IllustratedEmptyState";

// ----- illustrations -----
//
// Each illustration is a 96x96 abstract SVG that consumes srapi color tokens:
// strokes use `currentColor` (driven by the wrapper's text color), accents fill
// `--color-srapi-accent-soft`, with secondary stroke tokens for variety.

const stroke = "currentColor";
const accentSoft = "var(--color-srapi-accent-soft)";
const borderToken = "var(--color-srapi-border-strong)";
const tertiary = "var(--color-srapi-text-tertiary)";

const logsIllust = (
  <svg width={96} height={96} viewBox="0 0 96 96" fill="none">
    <rect x="14" y="20" width="68" height="6" rx="3" stroke={borderToken} strokeWidth="1.5" fill="none" />
    <rect x="14" y="34" width="56" height="6" rx="3" fill={accentSoft} stroke={stroke} strokeWidth="1.5" />
    <rect x="14" y="48" width="64" height="6" rx="3" stroke={borderToken} strokeWidth="1.5" fill="none" />
    <rect x="14" y="62" width="44" height="6" rx="3" stroke={borderToken} strokeWidth="1.5" fill="none" />
    <circle cx="76" cy="65" r="3" fill={stroke} />
  </svg>
);

const accountsIllust = (
  <svg width={96} height={96} viewBox="0 0 96 96" fill="none">
    <circle cx="36" cy="44" r="22" fill={accentSoft} stroke={stroke} strokeWidth="1.5" />
    <circle cx="60" cy="44" r="22" stroke={borderToken} strokeWidth="1.5" fill="none" />
    <circle cx="48" cy="64" r="18" stroke={stroke} strokeWidth="1.5" fill="none" opacity="0.6" />
  </svg>
);

const usersIllust = (
  <svg width={96} height={96} viewBox="0 0 96 96" fill="none">
    <circle cx="36" cy="38" r="10" fill={accentSoft} stroke={stroke} strokeWidth="1.5" />
    <path d="M20 72 C20 60 28 54 36 54 C44 54 52 60 52 72" stroke={stroke} strokeWidth="1.5" fill="none" />
    <circle cx="62" cy="42" r="8" stroke={borderToken} strokeWidth="1.5" fill="none" />
    <path d="M50 72 C50 62 56 58 62 58 C68 58 74 62 74 72" stroke={borderToken} strokeWidth="1.5" fill="none" />
  </svg>
);

const searchIllust = (
  <svg width={96} height={96} viewBox="0 0 96 96" fill="none">
    <circle cx="40" cy="40" r="18" fill={accentSoft} stroke={stroke} strokeWidth="1.5" />
    <path d="M54 54 L70 70" stroke={stroke} strokeWidth="2.5" strokeLinecap="round" />
    <rect x="30" y="68" width="44" height="4" rx="2" fill={borderToken} />
    <rect x="30" y="76" width="30" height="4" rx="2" fill={borderToken} />
    <rect x="30" y="84" width="20" height="4" rx="2" fill={borderToken} />
  </svg>
);

const cogIllust = (
  <svg width={96} height={96} viewBox="0 0 96 96" fill="none">
    <path
      d="M48 22 L52 22 L54 30 L60 32 L66 28 L70 32 L66 38 L68 44 L76 46 L76 50 L68 52 L66 58 L70 64 L66 68 L60 64 L54 66 L52 74 L48 74 L44 74 L42 66 L36 64 L30 68 L26 64 L30 58 L28 52 L20 50 L20 46 L28 44 L30 38 L26 32 L30 28 L36 32 L42 30 L44 22 Z"
      fill={accentSoft}
      stroke={stroke}
      strokeWidth="1.5"
      strokeLinejoin="round"
    />
    <circle cx="48" cy="48" r="8" stroke={stroke} strokeWidth="1.5" fill="none" />
    <circle cx="14" cy="14" r="2" fill={tertiary} />
    <circle cx="82" cy="14" r="2" fill={tertiary} />
    <circle cx="14" cy="82" r="2" fill={tertiary} />
    <circle cx="82" cy="82" r="2" fill={tertiary} />
  </svg>
);

const bellIllust = (
  <svg width={96} height={96} viewBox="0 0 96 96" fill="none">
    <path
      d="M32 56 C32 40 38 28 48 28 C58 28 64 40 64 56 L68 64 L28 64 Z"
      fill={accentSoft}
      stroke={stroke}
      strokeWidth="1.5"
      strokeLinejoin="round"
    />
    <path d="M42 68 C42 72 44 74 48 74 C52 74 54 72 54 68" stroke={stroke} strokeWidth="1.5" fill="none" />
    <path d="M22 36 C20 34 18 30 18 26" stroke={borderToken} strokeWidth="1.5" strokeLinecap="round" fill="none" />
    <path d="M16 50 C14 48 12 44 12 40" stroke={borderToken} strokeWidth="1.5" strokeLinecap="round" fill="none" />
    <path d="M74 36 C76 34 78 30 78 26" stroke={borderToken} strokeWidth="1.5" strokeLinecap="round" fill="none" />
    <circle cx="48" cy="24" r="2" fill={stroke} />
  </svg>
);

const inboxIllust = (
  <svg width={96} height={96} viewBox="0 0 96 96" fill="none">
    <path
      d="M18 50 L28 28 L68 28 L78 50 L78 70 L18 70 Z"
      fill={accentSoft}
      stroke={stroke}
      strokeWidth="1.5"
      strokeLinejoin="round"
    />
    <path d="M18 50 L34 50 L38 58 L58 58 L62 50 L78 50" stroke={stroke} strokeWidth="1.5" fill="none" strokeLinejoin="round" />
    <rect x="38" y="14" width="20" height="14" rx="2" stroke={borderToken} strokeWidth="1.5" fill="none" />
    <line x1="42" y1="20" x2="54" y2="20" stroke={borderToken} strokeWidth="1.5" strokeLinecap="round" />
  </svg>
);

const chartIllust = (
  <svg width={96} height={96} viewBox="0 0 96 96" fill="none">
    <rect x="20" y="58" width="12" height="20" rx="2" fill={accentSoft} stroke={stroke} strokeWidth="1.5" />
    <rect x="40" y="46" width="12" height="32" rx="2" fill={accentSoft} stroke={stroke} strokeWidth="1.5" />
    <rect x="60" y="32" width="12" height="46" rx="2" fill={accentSoft} stroke={stroke} strokeWidth="1.5" />
    <path
      d="M18 64 C28 58 38 50 48 42 C58 34 68 28 78 22"
      stroke={stroke}
      strokeWidth="1.5"
      fill="none"
      strokeLinecap="round"
    />
    <circle cx="78" cy="22" r="3" fill={stroke} />
  </svg>
);

const illustrations: Record<IllustrationKey, React.ReactElement> = {
  logs: logsIllust,
  accounts: accountsIllust,
  users: usersIllust,
  search: searchIllust,
  cog: cogIllust,
  bell: bellIllust,
  inbox: inboxIllust,
  chart: chartIllust,
};

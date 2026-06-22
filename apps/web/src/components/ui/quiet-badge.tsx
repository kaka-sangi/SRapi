import { cn } from "@/lib/cn";

export type QuietStatus = "active" | "limited" | "disabled" | "error";

const GLYPH: Record<QuietStatus, string> = {
  active: "●",
  limited: "■",
  disabled: "○",
  error: "■",
};

const TONE: Record<QuietStatus, string> = {
  active: "text-srapi-success",
  limited: "text-srapi-warning",
  disabled: "text-srapi-text-tertiary",
  error: "text-srapi-error",
};

/**
 * Quiet status marker: soft tinted pill, no border. The colored glyph carries
 * the semantic; the label stays in secondary text so it never screams.
 */
export function QuietBadge({
  status,
  label,
  className,
}: {
  status: QuietStatus;
  label: string;
  className?: string;
}) {
  return (
    <span
      className={cn(
        "inline-flex items-center gap-1.5 rounded-full bg-srapi-card-muted px-2.5 py-0.5 text-[11px] font-medium text-srapi-text-secondary",
        className,
      )}
    >
      <span aria-hidden className={cn("text-[0.7em] leading-none", TONE[status])}>
        {GLYPH[status]}
      </span>
      {label}
    </span>
  );
}

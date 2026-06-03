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
 * §4.4 静默状态标记：11px mono，无高对比大背景，依靠 1px 砂岩边框包裹；
 * 仅状态字形(glyph)着色，文字保持主色，克制不抢眼。
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
        "inline-flex items-center gap-1.5 rounded-md border border-srapi-border px-2 py-0.5 font-mono text-2xs text-srapi-text-secondary",
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

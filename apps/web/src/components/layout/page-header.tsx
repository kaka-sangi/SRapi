import { cn } from "@/lib/cn";

export function PageHeader({
  eyebrow,
  title,
  description,
  actions,
  className,
}: {
  eyebrow?: string;
  title: string;
  description?: string;
  actions?: React.ReactNode;
  className?: string;
}) {
  return (
    <div
      className={cn(
        // Editorial header: column title, lead, optional toolbar. The bottom
        // rule is intentionally hairline + warm so the page feels like one
        // continuous sheet of paper, not a stack of boxes.
        "relative flex flex-col gap-4 pb-6 sm:flex-row sm:items-end sm:justify-between sm:gap-8",
        "after:pointer-events-none after:absolute after:inset-x-0 after:bottom-0 after:h-px after:bg-gradient-to-r after:from-srapi-border-strong after:via-srapi-border after:to-transparent",
        className,
      )}
    >
      <div className="min-w-0">
        {eyebrow && (
          <div className="mb-3 flex items-center gap-2 font-mono text-2xs uppercase tracking-[0.18em] text-srapi-text-tertiary">
            <span className="block h-px w-6 bg-srapi-border-strong" aria-hidden />
            <span>{eyebrow}</span>
          </div>
        )}
        <h1
          className="font-serif text-3xl leading-[1.08] tracking-tight text-srapi-text-primary sm:text-4xl"
          title={title}
        >
          {title}
        </h1>
        {description && (
          <p className="mt-2.5 max-w-prose text-sm leading-relaxed text-srapi-text-secondary">
            {description}
          </p>
        )}
      </div>
      {actions && (
        <div className="flex shrink-0 flex-wrap items-center gap-2 sm:justify-end">
          {actions}
        </div>
      )}
    </div>
  );
}

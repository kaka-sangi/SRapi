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
        // Modern SaaS header: bold sans-serif headline + soft subtitle.
        // No bottom rule (cards below provide their own visual edges) — the
        // open space between the header and content is the visual rest.
        "flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between sm:gap-8",
        className,
      )}
    >
      <div className="min-w-0 flex-1">
        {eyebrow && (
          <div className="mb-2 inline-flex items-center gap-1.5 rounded-full bg-srapi-accent-soft px-2.5 py-1 text-[11px] font-medium uppercase tracking-[0.12em] text-srapi-primary">
            {eyebrow}
          </div>
        )}
        <h1
          className="text-3xl font-semibold leading-tight tracking-tight text-srapi-text-primary sm:text-[2rem]"
          title={title}
        >
          {title}
        </h1>
        {description && (
          <p className="mt-2 max-w-2xl text-sm leading-relaxed text-srapi-text-secondary">
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

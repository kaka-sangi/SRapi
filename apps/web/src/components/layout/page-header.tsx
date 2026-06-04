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
        "flex flex-col gap-3 border-b border-srapi-border pb-5 sm:flex-row sm:items-start sm:justify-between sm:gap-6",
        className,
      )}
    >
      <div className="min-w-0">
        {eyebrow && (
          <div className="mb-2 font-mono text-2xs uppercase tracking-wide text-srapi-text-tertiary">
            {eyebrow}
          </div>
        )}
        <h1 className="font-serif text-3xl text-srapi-text-primary">{title}</h1>
        {description && (
          <p className="mt-1.5 max-w-prose text-sm text-srapi-text-secondary">{description}</p>
        )}
      </div>
      {actions && (
        <div className="flex shrink-0 flex-wrap items-center gap-2 sm:justify-end sm:pt-0.5">
          {actions}
        </div>
      )}
    </div>
  );
}

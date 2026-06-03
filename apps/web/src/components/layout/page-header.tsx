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
    <div className={cn("flex flex-wrap items-end justify-between gap-4", className)}>
      <div className="min-w-0">
        {eyebrow && (
          <div className="mb-2.5 font-mono text-2xs uppercase text-srapi-text-tertiary">
            {eyebrow}
          </div>
        )}
        <h1 className="font-serif text-3xl text-srapi-text-primary">{title}</h1>
        {description && (
          <p className="mt-2 max-w-prose text-sm text-srapi-text-secondary">{description}</p>
        )}
      </div>
      {actions && <div className="flex shrink-0 items-center gap-2">{actions}</div>}
    </div>
  );
}

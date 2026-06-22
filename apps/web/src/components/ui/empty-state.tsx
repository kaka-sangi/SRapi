import type { LucideIcon } from "lucide-react";
import { cn } from "@/lib/cn";

export function EmptyState({
  icon: Icon,
  title,
  description,
  action,
  className,
}: {
  icon?: LucideIcon;
  title: string;
  description?: string;
  action?: React.ReactNode;
  className?: string;
}) {
  return (
    <div
      className={cn(
        "relative mx-auto flex max-w-md flex-col items-center justify-center gap-4 px-6 py-20 text-center",
        className,
      )}
    >
      {/* Soft radial halo behind the icon — calm, paper-warm */}
      <div
        className="pointer-events-none absolute left-1/2 top-12 -translate-x-1/2 select-none"
        aria-hidden
      >
        <div className="h-32 w-32 rounded-full bg-srapi-primary/5 blur-2xl" />
      </div>
      {Icon && (
        <div className="relative grid size-14 place-items-center rounded-2xl border border-srapi-border bg-srapi-card text-srapi-text-secondary tactile-card">
          <Icon className="size-6" aria-hidden />
        </div>
      )}
      <div className="font-serif text-xl tracking-tight text-srapi-text-primary">{title}</div>
      {description && (
        <p className="max-w-sm text-sm leading-relaxed text-srapi-text-secondary">{description}</p>
      )}
      {action && <div className="mt-1.5">{action}</div>}
    </div>
  );
}

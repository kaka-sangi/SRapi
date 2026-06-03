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
        "flex flex-col items-center justify-center gap-3 px-6 py-16 text-center",
        className,
      )}
    >
      {Icon && (
        <div className="grid size-11 place-items-center rounded-xl border border-srapi-border bg-srapi-card-muted text-srapi-text-secondary">
          <Icon className="size-5" aria-hidden />
        </div>
      )}
      <div className="font-serif text-lg text-srapi-text-primary">{title}</div>
      {description && (
        <p className="max-w-sm text-sm text-srapi-text-secondary">{description}</p>
      )}
      {action && <div className="mt-1">{action}</div>}
    </div>
  );
}

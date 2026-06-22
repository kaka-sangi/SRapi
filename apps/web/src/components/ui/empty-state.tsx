import type { LucideIcon } from "lucide-react";
import { cn } from "@/lib/cn";
import { IconBubble } from "./icon-bubble";

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
        "mx-auto flex max-w-md flex-col items-center justify-center gap-3 p-8 text-center",
        className,
      )}
    >
      {Icon && (
        <IconBubble tone="accent" className="size-12 [&>svg]:size-5">
          <Icon aria-hidden />
        </IconBubble>
      )}
      <div className="text-base font-semibold tracking-tight text-srapi-text-primary">
        {title}
      </div>
      {description && (
        <p className="max-w-sm text-sm leading-relaxed text-srapi-text-secondary">
          {description}
        </p>
      )}
      {action && <div className="mt-1.5">{action}</div>}
    </div>
  );
}

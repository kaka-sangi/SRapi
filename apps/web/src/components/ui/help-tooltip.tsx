"use client";

import { HelpCircle } from "lucide-react";
import { Tooltip, TooltipTrigger, TooltipContent } from "@/components/ui/tooltip";
import { cn } from "@/lib/cn";

export function HelpTooltip({
  content,
  side = "top",
  className,
  iconClassName,
}: {
  content: React.ReactNode;
  side?: "top" | "right" | "bottom" | "left";
  className?: string;
  iconClassName?: string;
}) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <button
          type="button"
          tabIndex={-1}
          className={cn(
            "inline-flex size-4 shrink-0 items-center justify-center rounded-full text-srapi-text-tertiary transition-colors hover:text-srapi-text-secondary",
            className,
          )}
        >
          <HelpCircle className={cn("size-3.5", iconClassName)} />
        </button>
      </TooltipTrigger>
      <TooltipContent side={side} className="max-w-72 text-wrap">
        {content}
      </TooltipContent>
    </Tooltip>
  );
}

export function LabelWithHelp({
  htmlFor,
  children,
  help,
  className,
}: {
  htmlFor?: string;
  children: React.ReactNode;
  help?: string;
  className?: string;
}) {
  return (
    <div className={cn("flex items-center gap-1.5", className)}>
      <label
        htmlFor={htmlFor}
        className="text-sm font-medium text-srapi-text-primary"
      >
        {children}
      </label>
      {help ? <HelpTooltip content={help} /> : null}
    </div>
  );
}

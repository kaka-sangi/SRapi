import * as React from "react";
import { cn } from "@/lib/cn";

export type SegmentedControlSize = "sm" | "md";

export interface SegmentedControlOption<T extends string> {
  value: T;
  label: React.ReactNode;
  icon?: React.ReactNode;
}

export interface SegmentedControlProps<T extends string> {
  value: T;
  onChange: (next: T) => void;
  options: Array<SegmentedControlOption<T>>;
  size?: SegmentedControlSize;
  ariaLabel?: string;
  className?: string;
}

const sizeClasses: Record<SegmentedControlSize, string> = {
  sm: "px-2.5 py-1 text-xs",
  md: "px-3 py-1.5 text-xs font-medium",
};

export function SegmentedControl<T extends string>({
  value,
  onChange,
  options,
  size = "md",
  ariaLabel,
  className,
}: SegmentedControlProps<T>) {
  return (
    <div
      role="tablist"
      aria-label={ariaLabel}
      className={cn(
        "inline-flex items-center gap-0.5 rounded-xl border border-srapi-border bg-srapi-card/80 p-1",
        className,
      )}
    >
      {options.map((option) => {
        const active = option.value === value;
        return (
          <button
            key={option.value}
            type="button"
            role="tab"
            aria-selected={active}
            onClick={() => onChange(option.value)}
            className={cn(
              "inline-flex items-center gap-1.5 rounded-lg transition-colors duration-150 focus:outline-none focus-visible:ring-2 focus-visible:ring-srapi-primary/40",
              sizeClasses[size],
              active
                ? "bg-srapi-card-muted text-srapi-text-secondary shadow-[0_1px_2px_rgba(26,24,20,0.04)]"
                : "text-srapi-text-tertiary hover:text-srapi-text-secondary",
            )}
          >
            {option.icon ? (
              <span className="grid place-items-center [&>svg]:size-3.5">{option.icon}</span>
            ) : null}
            <span>{option.label}</span>
          </button>
        );
      })}
    </div>
  );
}
SegmentedControl.displayName = "SegmentedControl";

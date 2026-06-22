"use client";

import { CheckCircle2 } from "lucide-react";
import { cn } from "@/lib/cn";
import { STEPS, type Step } from "./presets";

/**
 * Horizontal pill-shaped step indicator. Mirrors the new SegmentedControl
 * language but is read-only — clicking a future step is a no-op since the
 * wizard flows strictly forward.
 */
export function StepIndicator({
  current,
  t,
}: {
  current: Step;
  t: (key: string) => string;
}) {
  const labels: Record<Step, string> = {
    platform: t("adminQuickSetup.stepPlatform"),
    credentials: t("adminQuickSetup.stepCredentials"),
    result: t("adminQuickSetup.stepDone"),
  };
  const currentIdx = STEPS.indexOf(current);

  return (
    <div
      role="tablist"
      aria-label="Steps"
      className="inline-flex items-center gap-0.5 rounded-xl border border-srapi-border bg-srapi-card/80 p-1"
    >
      {STEPS.map((s, i) => {
        const done = i < currentIdx;
        const active = i === currentIdx;
        return (
          <div
            key={s}
            role="tab"
            aria-selected={active}
            className={cn(
              "inline-flex items-center gap-1.5 rounded-lg px-3 py-1.5 text-xs font-medium transition-colors",
              active && "bg-srapi-accent-soft text-srapi-primary shadow-[0_1px_2px_rgba(26,24,20,0.04)]",
              done && !active && "text-srapi-success",
              !done && !active && "text-srapi-text-tertiary",
            )}
          >
            <span
              className={cn(
                "grid size-5 place-items-center rounded-full text-[10px] font-semibold transition-colors",
                active && "bg-srapi-primary text-white",
                done && !active && "bg-srapi-success/15 text-srapi-success",
                !done && !active && "bg-srapi-card-muted text-srapi-text-tertiary",
              )}
            >
              {done ? <CheckCircle2 className="size-3" /> : i + 1}
            </span>
            <span>{labels[s]}</span>
          </div>
        );
      })}
    </div>
  );
}

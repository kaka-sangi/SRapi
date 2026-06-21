"use client";

import { CheckCircle2 } from "lucide-react";
import { cn } from "@/lib/cn";
import { STEPS, type Step } from "./presets";

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
    <div className="flex items-center gap-2">
      {STEPS.map((s, i) => {
        const done = i < currentIdx;
        const active = i === currentIdx;
        return (
          <div key={s} className="flex items-center gap-2">
            {i > 0 && (
              <div
                className={cn(
                  "h-px w-8 transition-colors",
                  done ? "bg-srapi-success" : "bg-srapi-border",
                )}
              />
            )}
            <div className="flex items-center gap-1.5">
              <div
                className={cn(
                  "flex size-6 items-center justify-center rounded-full text-2xs font-semibold transition-colors",
                  done && "bg-srapi-success/15 text-srapi-success",
                  active &&
                    "bg-srapi-text-primary text-srapi-bg",
                  !done && !active &&
                    "bg-srapi-card-muted text-srapi-text-tertiary",
                )}
              >
                {done ? (
                  <CheckCircle2 className="size-3.5" />
                ) : (
                  i + 1
                )}
              </div>
              <span
                className={cn(
                  "text-xs transition-colors",
                  active
                    ? "font-medium text-srapi-text-primary"
                    : "text-srapi-text-tertiary",
                )}
              >
                {labels[s]}
              </span>
            </div>
          </div>
        );
      })}
    </div>
  );
}

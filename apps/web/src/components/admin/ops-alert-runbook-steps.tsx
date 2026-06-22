"use client";

import { useMemo, useState } from "react";
import { Check, ListChecks } from "lucide-react";
import type { OpsAlertRunbookStep } from "@/lib/admin-ops-alert-evidence";
import { useLanguage } from "@/context/LanguageContext";
import { cn } from "@/lib/cn";
import { DataPill } from "@/components/ui/data-pill";
import { SectionTitle } from "@/components/ui/section-title";

/**
 * Runbook checklist for an alert's recommended response steps. Compact mode
 * trims to the first three steps for inline previews; full mode renders every
 * step as a tactile Card whose checkbox tracks local progress so an on-call
 * operator can mentally tick them off without losing place when toggling tabs.
 */
export function OpsAlertRunbookSteps({
  steps,
  compact = false,
}: {
  steps: OpsAlertRunbookStep[];
  compact?: boolean;
}) {
  const { t } = useLanguage();
  const [checked, setChecked] = useState<Set<string>>(() => new Set());
  const visibleSteps = compact ? steps.slice(0, 3) : steps;
  const done = useMemo(
    () => visibleSteps.filter((s) => checked.has(s)).length,
    [visibleSteps, checked],
  );
  if (steps.length === 0) return null;

  function toggle(step: OpsAlertRunbookStep) {
    setChecked((prev) => {
      const next = new Set(prev);
      if (next.has(step)) next.delete(step);
      else next.add(step);
      return next;
    });
  }

  // Compact rendering keeps the legacy lightweight ordered list so the inline
  // preview inside the alerts table stays dense and stripe-aligned.
  if (compact) {
    return (
      <div className="min-w-0 space-y-1.5">
        <div className="text-xs font-semibold uppercase tracking-[0.12em] text-srapi-text-tertiary">
          {t("adminOps.runbook.title")}
        </div>
        <ol className="space-y-1">
          {visibleSteps.map((step, index) => (
            <li key={step} className="flex gap-2 text-xs text-srapi-text-secondary">
              <span className="text-[11px] font-medium tabular text-srapi-text-tertiary">
                {index + 1}
              </span>
              <span>{t(`adminOps.runbook.steps.${step}`)}</span>
            </li>
          ))}
        </ol>
      </div>
    );
  }

  return (
    <div className="min-w-0 space-y-2.5">
      <SectionTitle
        icon={<ListChecks className="size-3.5" />}
        label={t("adminOps.runbook.title")}
        action={
          <DataPill tone={done === visibleSteps.length ? "success" : "neutral"} size="sm">
            {done} / {visibleSteps.length}
          </DataPill>
        }
      />
      <ol className="space-y-1.5">
        {visibleSteps.map((step, index) => {
          const isDone = checked.has(step);
          return (
            <li key={step}>
              <button
                type="button"
                onClick={() => toggle(step)}
                aria-pressed={isDone}
                className={cn(
                  "group flex w-full items-start gap-2.5 rounded-xl border border-srapi-border bg-srapi-card px-3 py-2 text-left transition-colors hover:border-srapi-border-strong",
                  isDone && "border-srapi-success/30 bg-srapi-success/5",
                )}
              >
                <span
                  className={cn(
                    "mt-0.5 grid size-[18px] shrink-0 place-items-center rounded-md border transition-colors",
                    isDone
                      ? "border-srapi-success bg-srapi-success text-white"
                      : "border-srapi-border-strong bg-srapi-card text-transparent group-hover:border-srapi-primary/50",
                  )}
                >
                  <Check className="size-3" strokeWidth={3} />
                </span>
                <span className="text-[11px] font-medium tabular text-srapi-text-tertiary">
                  {index + 1}
                </span>
                <span
                  className={cn(
                    "flex-1 text-xs leading-relaxed",
                    isDone
                      ? "text-srapi-text-tertiary line-through decoration-srapi-text-tertiary/40"
                      : "text-srapi-text-secondary",
                  )}
                >
                  {t(`adminOps.runbook.steps.${step}`)}
                </span>
              </button>
            </li>
          );
        })}
      </ol>
    </div>
  );
}

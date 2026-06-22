"use client";

import type { OpsAlertRunbookStep } from "@/lib/admin-ops-alert-evidence";
import { useLanguage } from "@/context/LanguageContext";

export function OpsAlertRunbookSteps({
  steps,
  compact = false,
}: {
  steps: OpsAlertRunbookStep[];
  compact?: boolean;
}) {
  const { t } = useLanguage();
  if (steps.length === 0) return null;
  const visibleSteps = compact ? steps.slice(0, 3) : steps;

  return (
    <div className="min-w-0 space-y-1.5">
      <div className="text-xs font-semibold uppercase tracking-[0.12em] text-srapi-text-tertiary">
        {t("adminOps.runbook.title")}
      </div>
      <ol className="space-y-1">
        {visibleSteps.map((step, index) => (
          <li key={step} className="flex gap-2 text-xs text-srapi-text-secondary">
            <span className="text-[11px] font-medium tabular text-srapi-text-tertiary">{index + 1}</span>
            <span>{t(`adminOps.runbook.steps.${step}`)}</span>
          </li>
        ))}
      </ol>
    </div>
  );
}

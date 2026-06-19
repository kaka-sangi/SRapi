import Link from "next/link";
import { useLanguage } from "@/context/LanguageContext";
import type { AccountHealthSnapshot } from "@/lib/sdk-types";
import { cn } from "@/lib/cn";
import { accountHealthNeedsInvestigation } from "@/lib/admin-account-health-investigation";
import { latestQuotaWindows, quotaWindowDisplayLabel, quotaWindowTiming } from "@/lib/quota-display";

export function HealthSummaryStrip({
  healthById,
  total,
}: {
  healthById: Map<string, AccountHealthSnapshot>;
  total: number;
}) {
  const { t } = useLanguage();
  if (healthById.size === 0 || total === 0) return null;
  const entries = [...healthById.values()];
  const healthy = entries.filter((h) => h.circuit_state === "closed" && h.success_rate >= 0.9).length;
  const degraded = entries.filter((h) => h.circuit_state === "closed" && h.success_rate < 0.9 && h.success_rate > 0).length;
  const tripped = entries.filter((h) => h.circuit_state !== "closed").length;
  return (
    <div className="mb-4 flex items-center gap-4 font-mono text-2xs text-srapi-text-tertiary">
      <span>{healthy} {t("dashboard.healthyAccounts")}</span>
      {degraded > 0 && <span>{degraded} {t("dashboard.degradedAccounts")}</span>}
      {tripped > 0 && <span>{tripped} {t("dashboard.trippedAccounts")}</span>}
      <span className="ml-auto">{t("dashboard.total")} {total}</span>
    </div>
  );
}

export function AccountHealthCell({
  health,
  investigationHref,
}: {
  health?: AccountHealthSnapshot;
  investigationHref?: string | null;
}) {
  const { t } = useLanguage();
  if (!health) return <span className="text-2xs text-srapi-text-tertiary">—</span>;
  const rate = health.success_rate;
  const circuit = health.circuit_state;
  const isOpen = circuit === "open";
  const isHalfOpen = circuit === "half-open";
  const p50 = Math.round(health.latency_p50_ms);
  // Explain the routing state in plain language: an "open" circuit means the
  // account is benched — a common reason requests get 'no available account'.
  const circuitTitle = isOpen
    ? t("adminAccounts.circuitOpen")
    : isHalfOpen
      ? t("adminAccounts.circuitHalfOpen")
      : t("adminAccounts.circuitClosed");
  const content = (
    <>
      <span
        title={circuitTitle}
        className={cn(
          "inline-block size-1.5 shrink-0 rounded-full",
          isOpen ? "bg-srapi-error" : isHalfOpen ? "bg-srapi-warning" : rate >= 0.95 ? "bg-srapi-success" : rate >= 0.8 ? "bg-srapi-warning" : "bg-srapi-error",
        )}
      />
      <span className="text-srapi-text-secondary">{Math.round(rate * 100)}%</span>
      {p50 > 0 ? (
        <span className="text-srapi-text-tertiary">{p50}ms</span>
      ) : null}
      {health.error_class ? (
        <span className="max-w-[5rem] truncate text-srapi-text-tertiary" title={health.error_class}>{health.error_class}</span>
      ) : null}
    </>
  );
  const className = "flex min-w-0 items-center gap-1.5 font-mono text-2xs tabular";
  if (investigationHref && accountHealthNeedsInvestigation(health)) {
    return (
      <Link
        href={investigationHref}
        className={`${className} rounded-sm underline-offset-2 hover:text-srapi-text-primary hover:underline`}
        aria-label={t("adminAccounts.investigateErrors")}
      >
        {content}
      </Link>
    );
  }
  return (
    <div className={className}>
      {content}
    </div>
  );
}

export function AccountQuotaCell({ health }: { health?: AccountHealthSnapshot }) {
  const { t } = useLanguage();
  if (!health) return <span className="text-2xs text-srapi-text-tertiary">—</span>;
  const windows = latestQuotaWindows(health.quota_windows ?? []);
  if (windows.length > 0) {
    const title = windows
      .map(
        (window) =>
          `${quotaWindowDisplayLabel(window, t)} ${Math.round(window.remainingPercent)}% · ${quotaWindowTiming(window, t)}`,
      )
      .join("\n");
    return (
      <span className="flex min-w-0 flex-col gap-1" title={title}>
        {windows.map((window) => {
          const ratio = window.remainingPercent / 100;
          const exhausted = window.remainingPercent <= 0;
          const pct = Math.round(window.remainingPercent);
          return (
            <span
              key={window.snapshot.quota_type}
              className="grid grid-cols-[2.5rem_minmax(2rem,1fr)_2.5rem] items-center gap-1.5"
            >
              <span className="truncate font-mono text-[10px] uppercase leading-none text-srapi-text-tertiary">
                {quotaWindowDisplayLabel(window, t)}
              </span>
              <span className="relative h-1.5 overflow-hidden rounded-full bg-srapi-border">
                <span
                  className={cn(
                    "absolute inset-y-0 left-0 rounded-full transition-all",
                    exhausted
                      ? "bg-srapi-error"
                      : ratio <= 0.2
                        ? "bg-srapi-warning"
                        : "bg-srapi-success",
                  )}
                  style={{ width: `${Math.max(pct, 2)}%` }}
                />
              </span>
              <span
                className={cn(
                  "text-right font-mono text-[10px] leading-none tabular text-srapi-text-tertiary",
                  exhausted
                    ? "text-srapi-error"
                    : window.remainingPercent <= 20
                      ? "text-srapi-warning"
                      : undefined,
                )}
              >
                {pct}%
              </span>
            </span>
          );
        })}
      </span>
    );
  }
  const ratio = health.quota_remaining_ratio;
  const exhausted = health.quota_exhausted;
  const pct = Math.round(ratio * 100);
  return (
    <span className="flex items-center gap-1.5">
      <span className="relative h-1.5 w-12 overflow-hidden rounded-full bg-srapi-border">
        <span
          className={cn(
            "absolute inset-y-0 left-0 rounded-full transition-all",
            exhausted ? "bg-srapi-error" : ratio <= 0.2 ? "bg-srapi-warning" : "bg-srapi-success",
          )}
          style={{ width: `${Math.max(pct, 2)}%` }}
        />
      </span>
      <span className={cn(
        "font-mono text-2xs tabular",
        exhausted ? "text-srapi-error" : ratio <= 0.2 ? "text-srapi-warning" : "text-srapi-text-secondary",
      )}>
        {pct}%
      </span>
    </span>
  );
}

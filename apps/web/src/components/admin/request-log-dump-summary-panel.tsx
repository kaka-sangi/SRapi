"use client";

import { useLanguage } from "@/context/LanguageContext";
import type { RequestDumpSummary } from "@/lib/request-log-dump-summary";

export function RequestDumpSummaryGrid({ summary }: { summary: RequestDumpSummary }) {
  const { t } = useLanguage();
  const outcome =
    summary.success === true
      ? t("adminRequestLogFiles.outcomeSuccess")
      : summary.success === false
        ? t("adminRequestLogFiles.outcomeError")
        : "—";
  const tone =
    summary.success === true
      ? "text-srapi-success"
      : summary.success === false
        ? "text-srapi-error"
        : "text-srapi-text-secondary";
  const items = [
    { label: t("adminRequestLogFiles.outcome"), value: outcome, className: tone },
    { label: t("adminRequestLogFiles.statusCode"), value: formatOptional(summary.statusCode) },
    { label: t("adminRequestLogFiles.errorClass"), value: summary.errorClass || "—" },
    {
      label: t("adminRequestLogFiles.latency"),
      value:
        summary.latencyMS === undefined
          ? "—"
          : t("adminRequestLogFiles.latencyMs", { value: summary.latencyMS }),
    },
    {
      label: t("adminRequestLogFiles.attempts"),
      value: t("adminRequestLogFiles.attemptsValue", {
        requests: summary.attemptCount,
        responses: summary.responseCount,
      }),
    },
    { label: t("adminRequestLogFiles.sourceProtocol"), value: summary.sourceProtocol || "—" },
    { label: t("adminRequestLogFiles.sourceEndpoint"), value: summary.sourceEndpoint || "—" },
    { label: t("adminRequestLogFiles.accountId"), value: summary.accountID || "—" },
  ];

  return (
    <div className="rounded-md border border-srapi-border bg-srapi-card-muted px-3 py-2">
      <div className="mb-2 flex flex-wrap items-center justify-between gap-2">
        <span className="font-mono text-2xs uppercase text-srapi-text-tertiary">
          {t("adminRequestLogFiles.diagnosticSummary")}
        </span>
        {summary.hasSummary ? null : (
          <span className="font-mono text-2xs text-srapi-warning">
            {t("adminRequestLogFiles.summaryMissing")}
          </span>
        )}
      </div>
      <dl className="grid gap-x-3 gap-y-2 sm:grid-cols-2 lg:grid-cols-4">
        {items.map((item) => (
          <div key={item.label} className="min-w-0">
            <dt className="font-mono text-2xs uppercase text-srapi-text-tertiary">
              {item.label}
            </dt>
            <dd
              className={`mt-0.5 truncate font-mono text-xs ${item.className ?? "text-srapi-text-primary"}`}
              title={String(item.value)}
            >
              {item.value}
            </dd>
          </div>
        ))}
      </dl>
    </div>
  );
}

function formatOptional(value: number | string | undefined): string {
  if (value === undefined || value === "") return "—";
  return String(value);
}

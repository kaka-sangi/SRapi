"use client";

import { useState } from "react";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  Table,
  TableScroll,
  TableHeader,
  TableBody,
  TableRow,
  TableHead,
  TableCell,
} from "@/components/ui/table";
import { SegmentedControl } from "@/components/ui/segmented-control";
import { QuietBadge } from "@/components/ui/quiet-badge";
import { DataTooltip, type DataTooltipRow } from "@/components/ui/data-tooltip";
import { DialogListSkeleton } from "@/components/charts/chart-skeleton";
import { PageQueryState } from "@/components/layout/page-query-state";
import { useApiKeyUsage } from "@/hooks/queries";
import { useAdminApiKeyUsage } from "@/hooks/admin-queries";
import { useLanguage } from "@/context/LanguageContext";
import { formatMoney, formatDateTime } from "@/lib/admin-format";
import { cn } from "@/lib/cn";
import type { GatewayUsageResponse } from "@/lib/sdk-types";

const WINDOWS = [7, 30, 90];

export function ApiKeyUsageDialog({
  keyId,
  keyName,
  variant,
  open,
  onOpenChange,
}: {
  keyId: string | null;
  keyName: string;
  variant: "me" | "admin";
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useLanguage();
  const [days, setDays] = useState(30);

  const meUsage = useApiKeyUsage(keyId, days, open && variant === "me");
  const adminUsage = useAdminApiKeyUsage(keyId, days, open && variant === "admin");
  const query = variant === "admin" ? adminUsage : meUsage;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle className="text-lg font-semibold tracking-tight">
            {t("apiKeys.usageTitle")}
            {keyName ? <span className="text-srapi-text-tertiary"> · {keyName}</span> : null}
          </DialogTitle>
        </DialogHeader>

        <div className="mt-2 flex items-center gap-2">
          <span className="text-xs text-srapi-text-tertiary">{t("apiKeys.usageWindowLabel")}</span>
          <SegmentedControl
            value={String(days)}
            onChange={(next) => setDays(Number(next))}
            options={WINDOWS.map((w) => ({
              value: String(w),
              label: t("apiKeys.usageWindowDays", { days: w }),
            }))}
            ariaLabel={t("apiKeys.usageWindowLabel")}
            size="sm"
          />
        </div>

        <div className="mt-3 max-h-[60vh] overflow-y-auto">
          <PageQueryState query={query} skeleton={<DialogListSkeleton rows={5} />}>
            {(usage) => <UsageBody usage={usage} />}
          </PageQueryState>
        </div>
      </DialogContent>
    </Dialog>
  );
}

function UsageBody({ usage }: { usage: GatewayUsageResponse }) {
  const { t } = useLanguage();
  const totals = usage.usage;
  const currency = totals.currency;

  // Cost breakdown rows are reused by Cost/Tokens tooltips so we build once.
  const costRows: DataTooltipRow[] = [
    { label: t("usage.costIn"), value: formatMoney(totals.input_cost ?? "0", currency) },
    { label: t("usage.costOut"), value: formatMoney(totals.output_cost ?? "0", currency) },
    { label: t("usage.costCacheRead"), value: formatMoney(totals.cache_read_cost ?? "0", currency) },
    { label: t("usage.costCacheWrite"), value: formatMoney(totals.cache_write_cost ?? "0", currency) },
  ];
  const successRate =
    totals.requests > 0 ? Math.round((totals.success_count / totals.requests) * 100) : 0;
  const errorRate =
    totals.requests > 0 ? Math.round((totals.error_count / totals.requests) * 100) : 0;

  return (
    <div className="space-y-5">
      <dl className="grid grid-cols-2 gap-3 sm:grid-cols-5">
        <Stat
          label={t("apiKeys.usageRequests")}
          value={totals.requests.toLocaleString()}
          tier="primary"
          tooltip={{
            title: t("apiKeys.usageRequests"),
            rows: [
              { label: t("apiKeys.usageSuccess"), value: totals.success_count.toLocaleString(), tone: "success" },
              { label: t("apiKeys.usageErrors"), value: totals.error_count.toLocaleString(), tone: totals.error_count > 0 ? "error" : "muted" },
            ],
          }}
        />
        <Stat
          label={t("apiKeys.usageSuccess")}
          value={totals.success_count.toLocaleString()}
          tier="secondary"
          tone={totals.success_count > 0 ? "good" : undefined}
          tooltip={{
            title: t("apiKeys.usageSuccess"),
            primary: `${successRate}%`,
            rows: [
              { label: t("apiKeys.usageRequests"), value: totals.requests.toLocaleString() },
              { label: t("apiKeys.usageSuccess"), value: totals.success_count.toLocaleString(), tone: "success" },
            ],
          }}
        />
        <Stat
          label={t("apiKeys.usageErrors")}
          value={totals.error_count.toLocaleString()}
          tier="secondary"
          tone={totals.error_count > 0 ? "bad" : undefined}
          tooltip={{
            title: t("apiKeys.usageErrors"),
            primary: `${errorRate}%`,
            rows: [
              { label: t("apiKeys.usageRequests"), value: totals.requests.toLocaleString() },
              { label: t("apiKeys.usageErrors"), value: totals.error_count.toLocaleString(), tone: totals.error_count > 0 ? "error" : "muted" },
            ],
          }}
        />
        <Stat
          label={t("apiKeys.usageTokens")}
          value={totals.total_tokens.toLocaleString()}
          tier="tertiary"
          tooltip={{
            title: t("apiKeys.usageTokens"),
            primary: totals.total_tokens.toLocaleString(),
            rows: costRows,
          }}
        />
        <Stat
          label={t("apiKeys.usageCost")}
          value={formatMoney(totals.cost, currency)}
          tier="primary"
          tooltip={{
            title: t("apiKeys.usageCost"),
            primary: formatMoney(totals.cost, currency),
            rows: costRows,
          }}
        />
      </dl>
      <div className="grid grid-cols-2 gap-2 text-xs text-srapi-text-tertiary sm:grid-cols-4">
        <span>{t("usage.costIn")} {formatMoney(totals.input_cost ?? "0", currency)}</span>
        <span>{t("usage.costOut")} {formatMoney(totals.output_cost ?? "0", currency)}</span>
        <span>{t("usage.costCacheRead")} {formatMoney(totals.cache_read_cost ?? "0", currency)}</span>
        <span>{t("usage.costCacheWrite")} {formatMoney(totals.cache_write_cost ?? "0", currency)}</span>
      </div>

      {usage.model_stats.length === 0 && usage.recent_requests.length === 0 ? (
        <p className="py-6 text-center text-sm text-srapi-text-tertiary">{t("apiKeys.usageEmpty")}</p>
      ) : null}

      {usage.model_stats.length > 0 ? (
        <section>
          <h4 className="mb-2 text-xs font-semibold uppercase tracking-[0.12em] text-srapi-text-tertiary">
            {t("apiKeys.usageModelsTitle")}
          </h4>
          <TableScroll minWidth={420}>
            <Table>
              <TableHeader>
                <tr>
                  <TableHead>{t("apiKeys.usageModel")}</TableHead>
                  <TableHead>{t("apiKeys.usageRequests")}</TableHead>
                  <TableHead>{t("apiKeys.usageTokens")}</TableHead>
                  <TableHead>{t("apiKeys.usageCost")}</TableHead>
                </tr>
              </TableHeader>
              <TableBody>
                {usage.model_stats.map((m) => (
                  <TableRow key={m.model}>
                    <TableCell className="font-mono text-xs text-srapi-text-primary">{m.model}</TableCell>
                    <TableCell className="tabular">{m.requests.toLocaleString()}</TableCell>
                    <TableCell className="tabular">{m.total_tokens.toLocaleString()}</TableCell>
                    <TableCell className="tabular">{formatMoney(m.cost, m.currency)}</TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </TableScroll>
        </section>
      ) : null}

      {usage.recent_requests.length > 0 ? (
        <section>
          <h4 className="mb-2 text-xs font-semibold uppercase tracking-[0.12em] text-srapi-text-tertiary">
            {t("apiKeys.usageRecentTitle")}
          </h4>
          <TableScroll minWidth={520}>
            <Table>
              <TableHeader>
                <tr>
                  <TableHead>{t("apiKeys.usageTime")}</TableHead>
                  <TableHead>{t("apiKeys.usageModel")}</TableHead>
                  <TableHead>{t("apiKeys.usageTokens")}</TableHead>
                  <TableHead>{t("apiKeys.usageLatency")}</TableHead>
                  <TableHead>{t("apiKeys.usageStatus")}</TableHead>
                </tr>
              </TableHeader>
              <TableBody>
                {usage.recent_requests.slice(0, 20).map((req) => (
                  <TableRow key={req.request_id}>
                    <TableCell className="text-[12px] tabular text-srapi-text-tertiary">
                      {formatDateTime(req.created_at)}
                    </TableCell>
                    <TableCell className="font-mono text-xs text-srapi-text-secondary">{req.model}</TableCell>
                    <TableCell className="tabular">{req.total_tokens.toLocaleString()}</TableCell>
                    <TableCell className="tabular">{req.latency_ms}ms</TableCell>
                    <TableCell>
                      <QuietBadge
                        status={req.success ? "active" : "error"}
                        label={req.success ? t("apiKeys.usageOk") : t("apiKeys.usageFailed")}
                      />
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </TableScroll>
        </section>
      ) : null}
    </div>
  );
}

function Stat({
  label,
  value,
  tier = "tertiary",
  tone,
  tooltip,
}: {
  label: string;
  value: string;
  tier?: "primary" | "secondary" | "tertiary";
  tone?: "good" | "warn" | "bad";
  tooltip?: { title?: string; primary?: React.ReactNode; rows?: DataTooltipRow[] };
}) {
  const tierClass =
    tier === "primary"
      ? "metric-primary"
      : tier === "secondary"
        ? "metric-secondary"
        : "metric-tertiary";
  const toneClass =
    tone === "good"
      ? "metric-strong-good"
      : tone === "warn"
        ? "metric-strong-warn"
        : tone === "bad"
          ? "metric-strong-bad"
          : undefined;
  const tile = (
    <div className="block rounded-xl border border-srapi-border bg-srapi-card-muted px-3 py-2 text-left transition-colors hover:bg-srapi-card-muted/80">
      <dt className="text-[11px] font-semibold uppercase tracking-[0.12em] text-srapi-text-tertiary">{label}</dt>
      <dd className={cn("mt-0.5 tabular tracking-tight", tierClass, toneClass)}>{value}</dd>
    </div>
  );
  if (!tooltip) return tile;
  return (
    <DataTooltip title={tooltip.title} primary={tooltip.primary} rows={tooltip.rows}>
      {tile}
    </DataTooltip>
  );
}

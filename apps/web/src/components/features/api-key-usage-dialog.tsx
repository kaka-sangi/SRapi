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
import { Button } from "@/components/ui/button";
import { QuietBadge } from "@/components/ui/quiet-badge";
import { Skeleton } from "@/components/ui/skeleton";
import { PageQueryState } from "@/components/layout/page-query-state";
import { useApiKeyUsage } from "@/hooks/queries";
import { useAdminApiKeyUsage } from "@/hooks/admin-queries";
import { useLanguage } from "@/context/LanguageContext";
import { formatMoney, formatDateTime } from "@/lib/admin-format";
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
          <DialogTitle>
            {t("apiKeys.usageTitle")}
            {keyName ? <span className="text-srapi-text-tertiary"> · {keyName}</span> : null}
          </DialogTitle>
        </DialogHeader>

        <div className="mt-2 flex items-center gap-2">
          <span className="text-2xs text-srapi-text-tertiary">{t("apiKeys.usageWindowLabel")}</span>
          {WINDOWS.map((w) => (
            <Button
              key={w}
              variant={days === w ? "primary" : "outline"}
              size="sm"
              onClick={() => setDays(w)}
            >
              {t("apiKeys.usageWindowDays", { days: w })}
            </Button>
          ))}
        </div>

        <div className="mt-3 max-h-[60vh] overflow-y-auto">
          <PageQueryState query={query} skeleton={<Skeleton className="h-40 rounded-xl" />}>
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

  return (
    <div className="space-y-5">
      <dl className="grid grid-cols-2 gap-3 sm:grid-cols-5">
        <Stat label={t("apiKeys.usageRequests")} value={totals.requests.toLocaleString()} />
        <Stat label={t("apiKeys.usageSuccess")} value={totals.success_count.toLocaleString()} />
        <Stat label={t("apiKeys.usageErrors")} value={totals.error_count.toLocaleString()} />
        <Stat label={t("apiKeys.usageTokens")} value={totals.total_tokens.toLocaleString()} />
        <Stat label={t("apiKeys.usageCost")} value={formatMoney(totals.cost, currency)} />
      </dl>

      {usage.model_stats.length === 0 && usage.recent_requests.length === 0 ? (
        <p className="py-6 text-center text-sm text-srapi-text-tertiary">{t("apiKeys.usageEmpty")}</p>
      ) : null}

      {usage.model_stats.length > 0 ? (
        <section>
          <h4 className="mb-2 text-2xs font-medium uppercase tracking-wide text-srapi-text-tertiary">
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
                    <TableCell className="font-mono text-2xs text-srapi-text-primary">{m.model}</TableCell>
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
          <h4 className="mb-2 text-2xs font-medium uppercase tracking-wide text-srapi-text-tertiary">
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
                    <TableCell className="font-mono text-2xs text-srapi-text-tertiary tabular">
                      {formatDateTime(req.created_at)}
                    </TableCell>
                    <TableCell className="font-mono text-2xs text-srapi-text-secondary">{req.model}</TableCell>
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

function Stat({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-xl border border-srapi-border bg-srapi-card-muted px-3 py-2">
      <dt className="text-2xs text-srapi-text-tertiary">{label}</dt>
      <dd className="mt-0.5 font-mono text-sm text-srapi-text-primary tabular">{value}</dd>
    </div>
  );
}

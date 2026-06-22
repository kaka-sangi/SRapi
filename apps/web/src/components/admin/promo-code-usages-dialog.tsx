"use client";

import { useMemo } from "react";
import { Ticket, Users, TrendingDown, BadgePercent } from "lucide-react";
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
import { DataTooltip } from "@/components/ui/data-tooltip";
import { DataPill } from "@/components/ui/data-pill";
import { IconBubble } from "@/components/ui/icon-bubble";
import { IllustratedEmptyState } from "@/components/ui/illustrated-empty-state";
import { DialogListSkeleton } from "@/components/charts/chart-skeleton";
import { PageQueryState } from "@/components/layout/page-query-state";
import { useAdminPromoCodeUsages } from "@/hooks/admin-queries";
import { useLanguage } from "@/context/LanguageContext";
import { formatMoney, formatDateTime } from "@/lib/admin-format";

export function PromoCodeUsagesDialog({
  promoId,
  code,
  open,
  onOpenChange,
}: {
  promoId: string | null;
  code: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useLanguage();
  const query = useAdminPromoCodeUsages(promoId, open);

  // Roll-up totals across the loaded page so the operator sees aggregate impact
  // — discount given, average per redemption, distinct users — instead of just
  // a list of opaque rows.
  const totals = useMemo(() => {
    const rows = query.data?.data ?? [];
    if (rows.length === 0) return null;
    const currency = rows[0]?.currency ?? "USD";
    let discount = 0;
    let final = 0;
    const users = new Set<string>();
    for (const u of rows) {
      discount += Number(u.discount_amount) || 0;
      final += Number(u.final_amount) || 0;
      users.add(String(u.user_id));
    }
    return {
      count: rows.length,
      discount,
      final,
      uniqueUsers: users.size,
      avgDiscount: discount / rows.length,
      currency,
    };
  }, [query.data]);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2.5 text-lg font-semibold tracking-tight">
            <IconBubble tone="accent" size="md">
              <Ticket />
            </IconBubble>
            <span>{t("adminPromos.usagesTitle")}</span>
            {code ? (
              <DataPill tone="accent" size="md" className="font-mono">
                {code}
              </DataPill>
            ) : null}
          </DialogTitle>
        </DialogHeader>

        {totals ? (
          <div className="mt-2 flex flex-wrap items-center gap-2">
            <DataTooltip
              title={t("adminPromos.usagesTitle")}
              primary={<span className="tabular">{totals.count}</span>}
              rows={[
                {
                  label: t("adminPromos.usagesUser"),
                  value: String(totals.uniqueUsers),
                },
                {
                  label: t("adminPromos.usagesFinal"),
                  value: formatMoney(totals.final, totals.currency),
                  tone: "muted",
                },
              ]}
            >
              <DataPill tone="accent" size="sm" className="metric-secondary cursor-help">
                <BadgePercent className="size-3" />
                {totals.count}
              </DataPill>
            </DataTooltip>
            <DataTooltip
              title={t("adminPromos.usagesUser")}
              primary={<span className="tabular">{totals.uniqueUsers}</span>}
              rows={[
                {
                  label: t("adminPromos.usagesTitle").toLowerCase(),
                  value: String(totals.count),
                  tone: "muted",
                },
              ]}
            >
              <DataPill tone="neutral" size="sm" className="metric-tertiary cursor-help">
                <Users className="size-3" />
                {totals.uniqueUsers}
              </DataPill>
            </DataTooltip>
            <DataTooltip
              title={t("adminPromos.usagesDiscount")}
              primary={
                <span className="tabular">
                  {formatMoney(totals.discount, totals.currency)}
                </span>
              }
              rows={[
                {
                  label: "avg / redemption",
                  value: formatMoney(totals.avgDiscount, totals.currency),
                  tone: "muted",
                },
                {
                  label: t("adminPromos.usagesFinal"),
                  value: formatMoney(totals.final, totals.currency),
                },
              ]}
            >
              <DataPill tone="warning" size="sm" className="metric-tertiary cursor-help">
                <TrendingDown className="size-3" />
                {formatMoney(totals.discount, totals.currency)}
              </DataPill>
            </DataTooltip>
          </div>
        ) : null}

        <div className="mt-3 max-h-[60vh] overflow-y-auto">
          <PageQueryState
            query={query}
            skeleton={<DialogListSkeleton rows={4} />}
            isEmpty={(d) => d.data.length === 0}
          >
            {(result) =>
              result.data.length === 0 ? (
                <IllustratedEmptyState
                  illust="inbox"
                  title={t("adminPromos.usagesEmpty")}
                />
              ) : (
                <TableScroll minWidth={520}>
                <Table>
                  <TableHeader>
                    <tr>
                      <TableHead>{t("adminPromos.usagesTime")}</TableHead>
                      <TableHead>{t("adminPromos.usagesUser")}</TableHead>
                      <TableHead>{t("adminPromos.usagesOrder")}</TableHead>
                      <TableHead>{t("adminPromos.usagesDiscount")}</TableHead>
                      <TableHead>{t("adminPromos.usagesFinal")}</TableHead>
                    </tr>
                  </TableHeader>
                  <TableBody>
                    {result.data.map((usage) => (
                      <TableRow key={usage.id}>
                        <TableCell className="metric-tertiary text-[12px]">
                          {formatDateTime(usage.applied_at)}
                        </TableCell>
                        <TableCell className="metric-secondary text-xs">
                          #{usage.user_id}
                        </TableCell>
                        <TableCell className="metric-tertiary text-xs">
                          {usage.order_no}
                        </TableCell>
                        <TableCell>
                          <DataTooltip
                            title={t("adminPromos.usagesDiscount")}
                            primary={
                              <span className="tabular">
                                {formatMoney(usage.discount_amount, usage.currency)}
                              </span>
                            }
                            rows={[
                              {
                                label: t("adminPromos.usagesFinal"),
                                value: formatMoney(usage.final_amount, usage.currency),
                              },
                              {
                                label: t("adminPromos.usagesTime"),
                                value: formatDateTime(usage.applied_at),
                                tone: "muted",
                              },
                            ]}
                          >
                            <span className="metric-primary cursor-help text-sm">
                              {formatMoney(usage.discount_amount, usage.currency)}
                            </span>
                          </DataTooltip>
                        </TableCell>
                        <TableCell className="metric-primary text-sm">
                          {formatMoney(usage.final_amount, usage.currency)}
                        </TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              </TableScroll>
              )
            }
          </PageQueryState>
        </div>
      </DialogContent>
    </Dialog>
  );
}

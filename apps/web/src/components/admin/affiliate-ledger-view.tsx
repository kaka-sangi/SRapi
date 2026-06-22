"use client";

import { useMemo } from "react";
import { Coins } from "lucide-react";
import type { UseQueryResult } from "@tanstack/react-query";
import { SectionHero } from "@/components/visual/section-hero";
import {
  AdminListView,
  ListCount,
  type Column,
  type AdminListResult,
} from "./admin-list-view";
import { DataTooltip } from "@/components/ui/data-tooltip";
import { DataPill } from "@/components/ui/data-pill";
import { QuietBadge } from "@/components/ui/quiet-badge";
import {
  InlineDetailGrid,
  type InlineDetailSection,
} from "@/components/ui/inline-detail-grid";
import { IllustratedEmptyState } from "@/components/ui/illustrated-empty-state";
import { useUserEmailLookup } from "@/hooks/use-user-email-lookup";
import { useLanguage } from "@/context/LanguageContext";
import { formatMoney, formatDateTime } from "@/lib/admin-format";
import { quietStatusFor, statusLabel } from "@/lib/status-badge";
import type { AffiliateLedgerEntry } from "@/lib/sdk-types";

/**
 * Shared view for the affiliate rebate / transfer ledgers — identical shape,
 * different title + dataset.
 */
export function AffiliateLedgerView({
  query,
  title,
  subtitle,
}: {
  query: UseQueryResult<AdminListResult<AffiliateLedgerEntry>>;
  title: string;
  subtitle: string;
}) {
  const { t } = useLanguage();
  const userLookup = useUserEmailLookup();

  // Roll-up across the current page so the hero band shows aggregate impact —
  // total amount accrued, distinct inviters, and the pending-vs-settled split.
  const summary = useMemo(() => {
    const rows = query.data?.data ?? [];
    if (rows.length === 0) return null;
    const currency = rows[0]?.currency ?? "USD";
    const inviters = new Set<string>();
    let total = 0;
    let pending = 0;
    let settled = 0;
    for (const r of rows) {
      inviters.add(String(r.user_id));
      total += Number(r.amount) || 0;
      if (r.status === "pending") pending += 1;
      else if (r.status === "settled") settled += 1;
    }
    return { total, count: rows.length, inviters: inviters.size, pending, settled, currency };
  }, [query.data]);

  const columns: Column<AffiliateLedgerEntry>[] = [
    {
      key: "user",
      header: t("adminAffiliates.inviter"),
      render: (r) => (
        <span className="text-srapi-text-secondary">{userLookup.get(r.user_id)}</span>
      ),
    },
    {
      key: "type",
      header: t("adminUsers.roles"),
      hideOnMobile: true,
      render: (r) => (
        <DataPill tone="neutral" size="sm">
          {r.type || "—"}
        </DataPill>
      ),
    },
    {
      key: "status",
      header: t("adminCommon.status"),
      hideOnMobile: true,
      render: (r) => (
        <QuietBadge
          status={quietStatusFor(r.status)}
          label={statusLabel(t, r.status)}
        />
      ),
    },
    {
      key: "amount",
      header: t("adminAffiliates.amount"),
      align: "right",
      render: (r) => {
        const amount = Number(r.amount);
        const positive = Number.isFinite(amount) && amount >= 0;
        return (
          <DataTooltip
            title={t("adminAffiliates.amount")}
            primary={
              <span className="tabular">{formatMoney(r.amount, r.currency)}</span>
            }
            rows={[
              {
                label: t("adminCommon.status"),
                value: statusLabel(t, r.status),
                tone: "muted",
              },
              {
                label: t("adminAffiliates.date"),
                value: formatDateTime(r.created_at),
                tone: "muted",
              },
              ...(r.settled_at
                ? [
                    {
                      label: "settled",
                      value: formatDateTime(r.settled_at),
                      tone: "success" as const,
                    },
                  ]
                : []),
            ]}
            footer={r.reference_id ? `ref: ${r.reference_id}` : undefined}
          >
            <span
              className={
                "metric-primary cursor-help text-sm " +
                (positive ? "" : "metric-strong-bad")
              }
            >
              {formatMoney(r.amount, r.currency)}
            </span>
          </DataTooltip>
        );
      },
    },
    {
      key: "date",
      header: t("adminAffiliates.date"),
      align: "right",
      hideOnMobile: true,
      render: (r) => (
        <span className="metric-tertiary text-[12px]">{formatDateTime(r.created_at)}</span>
      ),
    },
  ];

  const headerMetrics = summary
    ? [
        {
          label: t("adminAffiliates.amount"),
          value: formatMoney(summary.total, summary.currency),
          tone: summary.total >= 0 ? ("success" as const) : ("error" as const),
        },
        {
          label: t("adminAffiliates.inviter"),
          value: String(summary.inviters),
        },
        ...(summary.pending > 0
          ? [
              {
                label: "pending",
                value: String(summary.pending),
                tone: "warning" as const,
              },
            ]
          : []),
      ]
    : undefined;

  return (
    <>
      <SectionHero
        eyebrow={`${t("nav.sectionAdmin")} · ${t("nav.adminAffiliates")}`}
        title={title}
        description={subtitle}
        metrics={headerMetrics}
        actions={
          query.data ? (
            <ListCount total={query.data.pagination?.total ?? query.data.data.length} />
          ) : undefined
        }
      />
      <AdminListView
        query={query}
        columns={columns}
        getRowId={(r) => r.id}
        emptyIcon={Coins}
        emptyTitle={t("adminAffiliates.emptyTitle")}
        emptyBody={t("adminAffiliates.emptyBody")}
        emptyContent={
          <IllustratedEmptyState
            illust="chart"
            title={t("adminAffiliates.emptyTitle")}
            description={t("adminAffiliates.emptyBody")}
          />
        }
        minWidth={560}
        rowSeverity={(r) =>
          r.status === "canceled"
            ? "error"
            : r.status === "pending"
              ? "warning"
              : r.status === "settled"
                ? "success"
                : undefined
        }
        expandRow={(r) => {
          const amount = Number(r.amount);
          const positive = Number.isFinite(amount) && amount >= 0;
          const identity: InlineDetailSection = {
            title: t("adminAffiliates.inviter"),
            rows: [
              {
                label: t("adminAffiliates.inviter"),
                value: userLookup.get(r.user_id) ?? `#${r.user_id}`,
              },
              {
                label: t("adminAffiliates.invitee"),
                value: userLookup.get(r.related_user_id) ?? `#${r.related_user_id}`,
              },
              ...(r.payment_order_id
                ? [
                    {
                      label: "payment order",
                      value: String(r.payment_order_id),
                      mono: true,
                      tone: "muted" as const,
                    },
                  ]
                : []),
              ...(r.subscription_id
                ? [
                    {
                      label: "subscription",
                      value: String(r.subscription_id),
                      mono: true,
                      tone: "muted" as const,
                    },
                  ]
                : []),
            ],
          };
          const ledger: InlineDetailSection = {
            title: t("adminAffiliates.amount"),
            rows: [
              {
                label: t("adminAffiliates.amount"),
                value: formatMoney(r.amount, r.currency),
                tone: positive ? "success" : "error",
              },
              {
                label: t("adminCommon.currency"),
                value: r.currency,
                tone: "muted",
              },
              {
                label: t("adminCommon.status"),
                value: statusLabel(t, r.status),
              },
              {
                label: t("adminUsers.roles"),
                value: r.type ?? "—",
                tone: "muted",
              },
            ],
          };
          const timing: InlineDetailSection = {
            title: t("adminAffiliates.date"),
            rows: [
              { label: "created", value: formatDateTime(r.created_at) },
              ...(r.settled_at
                ? [{ label: "settled", value: formatDateTime(r.settled_at), tone: "success" as const }]
                : []),
              { label: "updated", value: formatDateTime(r.updated_at), tone: "muted" },
              ...(r.reference_id
                ? [
                    {
                      label: "reference",
                      value: r.reference_id,
                      mono: true,
                      tone: "muted" as const,
                    },
                  ]
                : []),
            ],
          };
          return <InlineDetailGrid sections={[identity, ledger, timing]} />;
        }}
      />
    </>
  );
}


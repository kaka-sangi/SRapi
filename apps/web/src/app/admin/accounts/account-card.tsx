import type { ReactNode } from "react";
import type { UseQueryResult } from "@tanstack/react-query";
import { SearchX, Server } from "lucide-react";
import { useLanguage } from "@/context/LanguageContext";
import { runtimeClassLabel } from "@/lib/admin-account-form";
import { formatCompactNumber, formatMoney, formatPercent } from "@/lib/admin-format";
import { PageQueryState } from "@/components/layout/page-query-state";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Checkbox } from "@/components/ui/checkbox";
import { DataPill } from "@/components/ui/data-pill";
import { DataTooltip } from "@/components/ui/data-tooltip";
import { EmptyState } from "@/components/ui/empty-state";
import { Pagination } from "@/components/ui/pagination";
import { Skeleton } from "@/components/ui/skeleton";
import { type AdminListResult } from "@/lib/admin-api";
import type { ProviderAccount, AccountHealthSnapshot, AccountUsageToday } from "@/lib/sdk-types";
import { cn } from "@/lib/cn";
import { AccountHealthCell, AccountQuotaCell } from "./account-health-cells";
import {
  accountCapacityFacts,
  accountEndpointCapabilityFacts,
  accountIdentitySummary,
  accountModelPolicyLabel,
  accountProfileFacts,
  type AccountSelection,
  type AccountPagination,
} from "./account-types";
import { TokenExpiryChip } from "./token-expiry-chip";

const EMPTY_FILL = "min-h-[55vh] justify-center";
type AccountUsageTodayWithId = AccountUsageToday & { account_id: string };

export function AccountsCardView({
  query,
  providerNameById,
  groupNameById,
  healthById,
  todayByAccountId,
  healthInvestigationHref,
  toolbar,
  selection,
  pagination,
  isFiltered,
  onClearFilters,
  emptyAction,
  onDetail,
  renderActions,
  renderStatus,
}: {
  query: UseQueryResult<AdminListResult<ProviderAccount>>;
  providerNameById: Map<string, string>;
  groupNameById: Map<string, string>;
  healthById: Map<string, AccountHealthSnapshot>;
  todayByAccountId: Map<string, AccountUsageTodayWithId>;
  healthInvestigationHref: (health?: AccountHealthSnapshot) => string | null;
  toolbar: ReactNode;
  selection?: AccountSelection;
  pagination: AccountPagination;
  isFiltered: boolean;
  onClearFilters: () => void;
  emptyAction?: ReactNode;
  onDetail?: (account: ProviderAccount) => void;
  renderActions: (account: ProviderAccount) => ReactNode;
  renderStatus: (account: ProviderAccount) => ReactNode;
}) {
  const { t } = useLanguage();

  return (
    <Card className="overflow-hidden">
      {toolbar}
      {selection && selection.selected.size > 0 ? (
        <AccountBulkBar
          count={selection.selected.size}
          onClear={() => selection.onTogglePage([...selection.selected], false)}
        >
          {selection.bulkActions}
        </AccountBulkBar>
      ) : null}
      <PageQueryState
        query={query}
        isEmpty={(d) => d.data.length === 0}
        skeleton={<AccountCardSkeleton />}
      >
        {(data) =>
          data.data.length === 0 ? (
            isFiltered ? (
              <EmptyState
                className={EMPTY_FILL}
                icon={SearchX}
                title={t("adminCommon.noResults")}
                description={t("adminCommon.noResultsBody")}
                action={
                  <Button variant="outline" size="sm" onClick={onClearFilters}>
                    {t("adminCommon.clearFilters")}
                  </Button>
                }
              />
            ) : (
              <EmptyState
                className={EMPTY_FILL}
                icon={Server}
                title={t("adminAccounts.emptyTitle")}
                description={t("adminAccounts.emptyBody")}
                action={emptyAction}
              />
            )
          ) : (
            <AccountCardGrid
              accounts={data.data}
              providerNameById={providerNameById}
              groupNameById={groupNameById}
              healthById={healthById}
              todayByAccountId={todayByAccountId}
              healthInvestigationHref={healthInvestigationHref}
              selection={selection}
              onDetail={onDetail}
              renderActions={renderActions}
              renderStatus={renderStatus}
            />
          )
        }
      </PageQueryState>
      {pagination.total > pagination.pageSize ? (
        <div className="border-srapi-border border-t">
          <Pagination
            page={pagination.page}
            pageSize={pagination.pageSize}
            total={pagination.total}
            onPageChange={pagination.onPageChange}
            labelFor={(from, to, total) => t("adminCommon.pageLabel", { from, to, total })}
            labelPrev={t("adminCommon.previousPage")}
            labelNext={t("adminCommon.nextPage")}
          />
        </div>
      ) : null}
    </Card>
  );
}

function AccountCardGrid({
  accounts,
  providerNameById,
  groupNameById,
  healthById,
  todayByAccountId,
  healthInvestigationHref,
  selection,
  onDetail,
  renderActions,
  renderStatus,
}: {
  accounts: ProviderAccount[];
  providerNameById: Map<string, string>;
  groupNameById: Map<string, string>;
  healthById: Map<string, AccountHealthSnapshot>;
  todayByAccountId: Map<string, AccountUsageTodayWithId>;
  healthInvestigationHref: (health?: AccountHealthSnapshot) => string | null;
  selection?: AccountSelection;
  onDetail?: (account: ProviderAccount) => void;
  renderActions: (account: ProviderAccount) => ReactNode;
  renderStatus: (account: ProviderAccount) => ReactNode;
}) {
  const pageIds = accounts.map((a) => a.id);
  const allOnPage = pageIds.length > 0 && pageIds.every((id) => selection?.selected.has(id));
  const someOnPage = pageIds.some((id) => selection?.selected.has(id));

  return (
    <div>
      {selection ? (
        <div className="border-srapi-border flex items-center gap-2 border-b px-4 py-2.5">
          <Checkbox
            aria-label="select all"
            checked={allOnPage}
            indeterminate={!allOnPage && someOnPage}
            onChange={(e) => selection.onTogglePage(pageIds, e.target.checked)}
          />
        </div>
      ) : null}
      {/* Card grid: 2-up on sm, 3-up on xl, 4-up on 2xl. With a fleet of 20+
          accounts the 4-up density on wide displays keeps the operator out of
          scroll-and-search hell. The skeleton below mirrors the same breakpoints
          to avoid layout shift. */}
      <div className="grid gap-3 p-3 sm:grid-cols-2 xl:grid-cols-3 2xl:grid-cols-4">
        {accounts.map((account) => (
          <AccountCard
            key={account.id}
            account={account}
            providerName={providerNameById.get(String(account.provider_id)) || account.provider_id}
            groupNameById={groupNameById}
            health={healthById.get(account.id)}
            today={todayByAccountId.get(account.id)}
            investigationHref={healthInvestigationHref(healthById.get(account.id))}
            selected={selection?.selected.has(account.id) ?? false}
            onSelect={selection ? () => selection.onToggle(account.id) : undefined}
            onDetail={onDetail ? () => onDetail(account) : undefined}
            actions={renderActions(account)}
            status={renderStatus(account)}
          />
        ))}
      </div>
    </div>
  );
}

function AccountCard({
  account,
  providerName,
  groupNameById,
  health,
  today,
  investigationHref,
  selected,
  onSelect,
  onDetail,
  actions,
  status,
}: {
  account: ProviderAccount;
  providerName: string;
  groupNameById: Map<string, string>;
  health?: AccountHealthSnapshot;
  today?: AccountUsageTodayWithId;
  investigationHref?: string | null;
  selected: boolean;
  onSelect?: () => void;
  onDetail?: () => void;
  actions: ReactNode;
  status: ReactNode;
}) {
  const { t } = useLanguage();
  const identity = accountIdentitySummary(t, account);
  const modelPolicy = accountModelPolicyLabel(t, account.metadata);
  const capacityFacts = accountCapacityFacts(t, account);
  const endpointFacts = accountEndpointCapabilityFacts(t, account);
  const profileFacts = accountProfileFacts(t, account).filter((fact) =>
    ["plan", "org"].includes(fact.key),
  );
  const proxyLabel = account.proxy_id
    ? t("adminAccounts.proxyConfigured")
    : t("adminAccounts.noProxy");
  const groups = account.group_ids ?? [];
  const groupNames = groups.map((id) => groupNameById.get(String(id)) ?? `#${id}`);
  const hasTodayUsage = Boolean(today && today.requests > 0);
  const hasIdentity = identity.primary !== account.name || identity.secondary.length > 0;
  const routeLabel = `P${account.priority ?? 0} / W${account.weight ?? 1}`;
  // Pre-compute «more details» bag for the "+N" pill that opens a DataTooltip
  // listing every secondary fact (capacity, profile, all groups, endpoints,
  // identity secondaries). Visible pills stay minimal — modelPolicy / proxy
  // / route only — for a clean card.
  const detailRows = [
    ...endpointFacts.map((f) => ({
      label: f.label,
      value: f.value,
      tone: (f.tone === "enabled" ? "success" : "muted") as "success" | "muted",
    })),
    ...capacityFacts.map((f) => ({ label: f.label, value: f.value, tone: "muted" as const })),
    ...profileFacts.map((f) => ({ label: f.label, value: f.value, tone: "muted" as const })),
    ...identity.secondary.map((v) => ({ label: t("adminAccounts.identityLabel") ?? "Identity", value: v, tone: "muted" as const })),
    ...groupNames.map((n) => ({ label: t("nav.adminGroups") ?? "Group", value: n, tone: "muted" as const })),
  ];
  const detailCount = detailRows.length;

  // Today summary numeric for hierarchy
  const todayTokens = today ? (today.total_tokens || today.input_tokens + today.output_tokens) : 0;
  return (
    <article
      className={cn(
        "group flex flex-col rounded-xl border border-srapi-border bg-srapi-card shadow-[0_1px_2px_rgba(26,24,20,0.04)] transition-all duration-200",
        account.status === "disabled" && "opacity-55",
        selected && "border-srapi-primary/50 bg-srapi-accent-soft/40 ring-1 ring-srapi-primary/30",
        onDetail && "cursor-pointer hover:border-srapi-border-strong hover:shadow-sm",
      )}
      onClick={(e) => {
        if (!onDetail) return;
        const target = e.target as HTMLElement;
        if (target.closest("button, input, a, [role=menuitem]")) return;
        onDetail();
      }}
    >
      {/* §1 Header — name + provider + identity (single tight stack) */}
      <div className="flex items-start gap-3 px-5 pt-5 pb-4">
        {onSelect ? (
          <Checkbox
            aria-label="select row"
            checked={selected}
            onChange={() => onSelect()}
            className="mt-1"
          />
        ) : null}
        <div className="min-w-0 flex-1">
          <div className="flex items-start justify-between gap-3">
            <h3 className="metric-primary truncate text-base">{account.name}</h3>
            <div className="shrink-0">{actions}</div>
          </div>
          <div className="mt-1 flex min-w-0 items-center gap-1.5 text-xs text-srapi-text-secondary">
            <span className="truncate font-medium">{providerName}</span>
            <span className="text-srapi-border-strong">·</span>
            <span className="metric-tertiary truncate text-[11px]">
              {runtimeClassLabel(t, account.runtime_class)}
            </span>
            {hasIdentity ? (
              <>
                <span className="text-srapi-border-strong">·</span>
                <DataTooltip
                  title={t("adminAccounts.identityLabel") ?? "Identity"}
                  primary={identity.primary}
                  rows={identity.secondary.map((v) => ({ label: "Alt", value: v, tone: "muted" as const }))}
                >
                  <span className="metric-tertiary max-w-[10rem] cursor-help truncate text-[11px] underline decoration-srapi-border-strong decoration-dotted underline-offset-2">
                    {identity.primary}
                  </span>
                </DataTooltip>
              </>
            ) : null}
          </div>
          {/* status row inline — pure pip + label, kept compact */}
          <div className="mt-2.5 flex flex-wrap items-center gap-1.5">
            {status}
            {account.risk_level ? (
              <DataPill tone="warning" size="sm">{account.risk_level}</DataPill>
            ) : null}
            <TokenExpiryChip account={account} />
          </div>
        </div>
      </div>

      {/* §2 KPI strip — 3 columns, each with DataTooltip revealing details */}
      <div className="grid grid-cols-3 divide-x divide-srapi-border/50 border-y border-srapi-border/60 bg-srapi-card-muted/40">
        {/* Health */}
        <DataTooltip
          title={t("adminAccounts.healthTitle")}
          primary={health ? `${Math.round((health.success_rate ?? 0) * 100)}%` : "—"}
          rows={
            health
              ? [
                  { label: "Circuit", value: health.circuit_state, tone: health.circuit_state === "open" ? "error" : health.circuit_state === "half-open" ? "warning" : "success" },
                  { label: "p50", value: `${Math.round(health.latency_p50_ms ?? 0)} ms` },
                  ...(health.error_class ? [{ label: "Last error", value: health.error_class, tone: "error" as const }] : []),
                ]
              : undefined
          }
        >
          <div className="cursor-help px-4 py-3">
            <div className="text-[10px] font-semibold uppercase tracking-[0.12em] text-srapi-text-tertiary">
              {t("adminAccounts.healthTitle")}
            </div>
            <div className="mt-1">
              <AccountHealthCell health={health} investigationHref={investigationHref} />
            </div>
          </div>
        </DataTooltip>
        {/* Quota */}
        <DataTooltip
          title={t("adminAccounts.quotaTitle")}
          primary={health ? `${Math.round((health.quota_remaining_ratio ?? 0) * 100)}%` : "—"}
          rows={
            health?.quota_exhausted
              ? [{ label: "Status", value: "Exhausted", tone: "error" as const }]
              : undefined
          }
          footer={(health?.quota_windows ?? []).length > 0 ? `${(health!.quota_windows ?? []).length} window(s)` : undefined}
        >
          <div className="cursor-help px-4 py-3">
            <div className="text-[10px] font-semibold uppercase tracking-[0.12em] text-srapi-text-tertiary">
              {t("adminAccounts.quotaTitle")}
            </div>
            <div className="mt-1">
              <AccountQuotaCell health={health} />
            </div>
          </div>
        </DataTooltip>
        {/* Today */}
        <DataTooltip
          title={t("adminAccounts.today")}
          primary={hasTodayUsage && today ? formatCompactNumber(today.requests) + " req" : t("adminAccounts.todayIdle")}
          rows={
            hasTodayUsage && today
              ? [
                  { label: "Tokens", value: formatCompactNumber(todayTokens) },
                  { label: "Cost", value: formatMoney(today.cost, today.currency) },
                  { label: "Success", value: formatPercent(today.success_rate), tone: today.success_rate >= 0.95 ? "success" : today.success_rate >= 0.8 ? "warning" : "error" },
                ]
              : undefined
          }
        >
          <div className="cursor-help px-4 py-3">
            <div className="text-[10px] font-semibold uppercase tracking-[0.12em] text-srapi-text-tertiary">
              {t("adminAccounts.today")}
            </div>
            <div className="mt-1 flex min-w-0 items-baseline gap-1.5 text-xs tabular">
              {hasTodayUsage && today ? (
                <>
                  <span className="metric-primary text-sm">
                    {formatCompactNumber(today.requests)}
                  </span>
                  <span className="metric-tertiary text-[11px]">
                    {formatMoney(today.cost, today.currency)}
                  </span>
                </>
              ) : (
                <span className="metric-tertiary text-[11px]">
                  {t("adminAccounts.todayIdle")}
                </span>
              )}
            </div>
          </div>
        </DataTooltip>
      </div>

      {/* §3 Chip strip — minimal: modelPolicy + proxy + routeLabel + groups + «+N more» */}
      <div className="flex min-w-0 flex-wrap items-center gap-1.5 px-5 py-3 mt-auto">
        <DataPill tone="neutral" size="sm" className="max-w-[10rem] truncate">{modelPolicy}</DataPill>
        <DataPill tone="neutral" size="sm">{routeLabel}</DataPill>
        <DataPill tone={account.proxy_id ? "accent" : "neutral"} size="sm">{proxyLabel}</DataPill>
        {groupNames.length === 0 ? (
          <DataPill tone="neutral" size="sm">{t("adminAccounts.ungrouped")}</DataPill>
        ) : groupNames.length <= 2 ? (
          groupNames.map((name) => (
            <DataPill key={name} tone="neutral" size="sm" className="max-w-[7rem] truncate">{name}</DataPill>
          ))
        ) : (
          <DataTooltip
            title={t("nav.adminGroups") ?? "Groups"}
            primary={groupNames.length + " groups"}
            rows={groupNames.map((n) => ({ label: "·", value: n, tone: "muted" as const }))}
          >
            <DataPill tone="accent" size="sm" className="cursor-help">
              {groupNames.length} groups
            </DataPill>
          </DataTooltip>
        )}
        {detailCount > 0 ? (
          <DataTooltip
            title={t("adminAccounts.detailsTitle") ?? "Details"}
            rows={detailRows}
          >
            <DataPill tone="neutral" size="sm" className="cursor-help">
              +{detailCount} more
            </DataPill>
          </DataTooltip>
        ) : null}
      </div>
    </article>
  );
}

function AccountBulkBar({
  count,
  onClear,
  children,
}: {
  count: number;
  onClear: () => void;
  children?: ReactNode;
}) {
  const { t } = useLanguage();
  return (
    <div className="flex flex-wrap items-center gap-3 border-b border-srapi-border bg-srapi-card-muted px-4 py-2.5">
      <span className="text-xs font-medium text-srapi-text-secondary">
        {t("adminCommon.selectedCount", { count })}
      </span>
      <button
        type="button"
        onClick={onClear}
        className="text-xs text-srapi-text-tertiary underline-offset-2 hover:text-srapi-text-primary hover:underline"
      >
        {t("adminCommon.clearSelection")}
      </button>
      <div className="ml-auto flex flex-wrap items-center gap-2">{children}</div>
    </div>
  );
}

function AccountCardSkeleton() {
  return (
    <div className="min-h-[55vh] p-3">
      <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-3 2xl:grid-cols-4">
        {Array.from({ length: 6 }).map((_, i) => (
          <div key={i} className="rounded-xl border border-srapi-border bg-srapi-card">
            <div className="px-5 pt-5 pb-3">
              <div className="flex items-start justify-between gap-3">
                <div className="space-y-2">
                  <Skeleton className="h-4 w-36" />
                  <Skeleton className="h-3 w-28" />
                </div>
                <Skeleton className="size-7 rounded-lg" />
              </div>
            </div>
            <div className="border-t border-srapi-border/60 px-5 py-3">
              <Skeleton className="h-5 w-20" />
            </div>
            <div className="grid grid-cols-2 gap-px border-t border-srapi-border/60">
              <div className="px-5 py-3">
                <Skeleton className="h-6 w-full" />
              </div>
              <div className="px-5 py-3">
                <Skeleton className="h-6 w-full" />
              </div>
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}

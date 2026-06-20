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
import { EmptyState } from "@/components/ui/empty-state";
import { Pagination } from "@/components/ui/pagination";
import { Skeleton } from "@/components/ui/skeleton";
import { type AdminListResult } from "@/lib/admin-api";
import type { ProviderAccount, AccountHealthSnapshot, AccountUsageToday } from "@/lib/sdk-types";
import { cn } from "@/lib/cn";
import { AccountHealthCell, AccountQuotaCell } from "./account-health-cells";
import {
  accountCapacityFacts,
  accountIdentitySummary,
  accountMetadataFacts,
  accountModelPolicyLabel,
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
    <Card className="anim-rise-sm overflow-hidden">
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
        <div className="border-t border-srapi-border">
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
        <div className="flex items-center gap-2 border-b border-srapi-border px-4 py-2.5">
          <Checkbox
            aria-label="select all"
            checked={allOnPage}
            indeterminate={!allOnPage && someOnPage}
            onChange={(e) => selection.onTogglePage(pageIds, e.target.checked)}
          />
        </div>
      ) : null}
      <div className="grid gap-3 p-3 sm:grid-cols-2 xl:grid-cols-3">
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
  const metadataFacts = accountMetadataFacts(t, account);
  const operationalFacts = metadataFacts
    .filter(
      (fact) =>
        ![
          "email",
          "plan",
          "upstream-id",
          "client",
          "max-concurrency",
          "max-sessions",
          "rpm",
        ].includes(fact.key),
    )
    .slice(0, 3);
  const proxyLabel = account.proxy_id ? t("adminAccounts.proxyConfigured") : t("adminAccounts.noProxy");
  const groups = account.group_ids ?? [];
  const visibleGroups = groups.slice(0, 3).map((id) => groupNameById.get(String(id)) ?? `#${id}`);
  const extraGroupCount = Math.max(0, groups.length - visibleGroups.length);
  const hasTodayUsage = Boolean(today && today.requests > 0);
  const hasIdentity = identity.primary !== account.name || identity.secondary.length > 0;
  return (
    <article
      className={cn(
        "tactile-card rounded-lg border border-srapi-border bg-srapi-card transition-colors",
        account.status === "disabled" && "opacity-55",
        selected && "border-srapi-primary/50 bg-srapi-card-muted",
        onDetail && "cursor-pointer hover:border-srapi-border-strong",
      )}
      onClick={(e) => {
        if (!onDetail) return;
        const target = e.target as HTMLElement;
        if (target.closest("button, input, a, [role=menuitem]")) return;
        onDetail();
      }}
    >
      {/* Header */}
      <div className="flex items-start gap-3 px-4 pt-4 pb-3">
        {onSelect ? (
          <Checkbox
            aria-label="select row"
            checked={selected}
            onChange={() => onSelect()}
            className="mt-0.5"
          />
        ) : null}
        <div className="min-w-0 flex-1">
          <div className="flex items-start justify-between gap-2">
            <h3 className="truncate text-sm font-medium text-srapi-text-primary">{account.name}</h3>
            <div className="shrink-0">{actions}</div>
          </div>
          <div className="mt-1.5 flex min-w-0 items-center gap-1.5">
            <span className="truncate text-xs text-srapi-text-secondary">{providerName}</span>
            <span className="shrink-0 text-srapi-border">·</span>
            <span className="truncate text-2xs text-srapi-text-tertiary">{runtimeClassLabel(t, account.runtime_class)}</span>
          </div>
          {hasIdentity ? (
            <div className="mt-1 flex min-w-0 flex-wrap items-center gap-x-1.5 gap-y-1 font-mono text-2xs">
              <span className="min-w-0 max-w-[13rem] truncate text-srapi-text-secondary" title={identity.primary}>
                {identity.primary}
              </span>
              {identity.secondary.slice(0, 2).map((item) => (
                <span
                  key={item}
                  className="max-w-[8rem] truncate text-srapi-text-tertiary"
                  title={item}
                >
                  {item}
                </span>
              ))}
            </div>
          ) : null}
          <div className="mt-2 flex flex-wrap gap-1">
            <span className="rounded-md bg-srapi-bg-muted px-1.5 py-0.5 font-mono text-[10px] text-srapi-text-tertiary">
              {modelPolicy}
            </span>
            <span className="rounded-md bg-srapi-bg-muted px-1.5 py-0.5 font-mono text-[10px] text-srapi-text-tertiary">
              {proxyLabel}
            </span>
            {visibleGroups.length > 0 ? (
              visibleGroups.map((name) => (
                <span
                  key={name}
                  className="max-w-[7rem] truncate rounded-md bg-srapi-bg-muted px-1.5 py-0.5 font-mono text-[10px] text-srapi-text-tertiary"
                  title={name}
                >
                  {name}
                </span>
              ))
            ) : (
              <span className="rounded-md bg-srapi-bg-muted px-1.5 py-0.5 font-mono text-[10px] text-srapi-text-tertiary">
                {t("adminAccounts.ungrouped")}
              </span>
            )}
            {extraGroupCount > 0 ? (
              <span className="rounded-md bg-srapi-bg-muted px-1.5 py-0.5 font-mono text-[10px] text-srapi-text-tertiary">
                +{extraGroupCount}
              </span>
            ) : null}
            {[...capacityFacts, ...operationalFacts].slice(0, 4).map((fact) => (
              <span
                key={fact.key}
                className="max-w-[10rem] truncate rounded-md bg-srapi-bg-muted px-1.5 py-0.5 font-mono text-[10px] text-srapi-text-tertiary"
                title={`${fact.label}: ${fact.value}`}
              >
                {fact.label}: {fact.value}
              </span>
            ))}
          </div>
        </div>
      </div>

      {/* Status row */}
      <div className="flex items-center gap-2 border-t border-srapi-border/50 px-4 py-2.5">
        {status}
        {account.priority != null && account.priority !== 0 ? (
          <span className="font-mono text-2xs text-srapi-text-tertiary">P{account.priority}</span>
        ) : null}
        {account.weight != null && account.weight !== 1 ? (
          <span className="font-mono text-2xs text-srapi-text-tertiary">W{account.weight}</span>
        ) : null}
        {account.risk_level ? (
          <span className="font-mono text-2xs text-srapi-text-tertiary">{account.risk_level}</span>
        ) : null}
        <TokenExpiryInline account={account} />
      </div>

      {/* Metrics */}
      <div className="grid grid-cols-2 gap-px border-t border-srapi-border/50 bg-srapi-border/30">
        <div className="bg-srapi-card px-4 py-2.5">
          <div className="mb-1 font-mono text-[10px] uppercase tracking-wide text-srapi-text-tertiary">{t("adminAccounts.healthTitle")}</div>
          <AccountHealthCell health={health} investigationHref={investigationHref} />
        </div>
        <div className="bg-srapi-card px-4 py-2.5">
          <div className="mb-1 font-mono text-[10px] uppercase tracking-wide text-srapi-text-tertiary">{t("adminAccounts.quotaTitle")}</div>
          <AccountQuotaCell health={health} />
        </div>
      </div>
      <div className="border-t border-srapi-border/50 px-4 py-2.5">
        <div className="mb-1 font-mono text-[10px] uppercase tracking-wide text-srapi-text-tertiary">
          {t("adminAccounts.today")}
        </div>
        {hasTodayUsage && today ? (
          <div className="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1 font-mono text-2xs tabular">
            <span className="text-srapi-text-primary">
              {formatCompactNumber(today.requests)} {t("adminAccounts.usageRequests").toLowerCase()}
            </span>
            <span className="text-srapi-text-secondary">
              {formatCompactNumber(today.total_tokens || today.input_tokens + today.output_tokens)} {t("adminAccounts.usageTokens").toLowerCase()}
            </span>
            <span className="text-srapi-text-secondary">{formatMoney(today.cost, today.currency)}</span>
            <span className="text-srapi-text-tertiary">{formatPercent(today.success_rate)}</span>
          </div>
        ) : (
          <span className="font-mono text-2xs text-srapi-text-tertiary">{t("adminAccounts.todayIdle")}</span>
        )}
      </div>
    </article>
  );
}

function TokenExpiryInline({ account }: { account: ProviderAccount }) {
  return (
    <span className="ml-auto">
      <TokenExpiryChip account={account} />
    </span>
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
    <div className="anim-rise-sm flex flex-wrap items-center gap-3 border-b border-srapi-border bg-srapi-card-muted px-4 py-2.5">
      <span className="font-mono text-2xs text-srapi-text-secondary">
        {t("adminCommon.selectedCount", { count })}
      </span>
      <button
        type="button"
        onClick={onClear}
        className="text-2xs text-srapi-text-tertiary underline-offset-2 hover:text-srapi-text-primary hover:underline"
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
      <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
        {Array.from({ length: 6 }).map((_, i) => (
          <div key={i} className="rounded-lg border border-srapi-border bg-srapi-card">
            <div className="px-4 pt-4 pb-3">
              <div className="flex items-start justify-between gap-3">
                <div className="space-y-2">
                  <Skeleton className="h-4 w-36" />
                  <Skeleton className="h-3 w-28" />
                </div>
                <Skeleton className="size-7 rounded-md" />
              </div>
            </div>
            <div className="border-t border-srapi-border/50 px-4 py-2.5">
              <Skeleton className="h-5 w-20" />
            </div>
            <div className="grid grid-cols-2 gap-px border-t border-srapi-border/50">
              <div className="px-4 py-2.5"><Skeleton className="h-6 w-full" /></div>
              <div className="px-4 py-2.5"><Skeleton className="h-6 w-full" /></div>
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}

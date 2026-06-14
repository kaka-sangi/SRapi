import type { ReactNode } from "react";
import type { UseQueryResult } from "@tanstack/react-query";
import { SearchX, Server } from "lucide-react";
import { useLanguage } from "@/context/LanguageContext";
import { runtimeClassLabel } from "@/lib/admin-account-form";
import { PageQueryState } from "@/components/layout/page-query-state";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Checkbox } from "@/components/ui/checkbox";
import { EmptyState } from "@/components/ui/empty-state";
import { Pagination } from "@/components/ui/pagination";
import { Skeleton } from "@/components/ui/skeleton";
import { type AdminListResult } from "@/lib/admin-api";
import type { ProviderAccount, AccountHealthSnapshot } from "@/lib/sdk-types";
import { cn } from "@/lib/cn";
import { AccountHealthCell, AccountQuotaCell } from "./account-health-cells";
import {
  metadataString,
  type AccountSelection,
  type AccountPagination,
} from "./account-types";

const EMPTY_FILL = "min-h-[55vh] justify-center";

export function AccountsCardView({
  query,
  providerNameById,
  healthById,
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
  healthById: Map<string, AccountHealthSnapshot>;
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
              healthById={healthById}
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
          />
        </div>
      ) : null}
    </Card>
  );
}

function AccountCardGrid({
  accounts,
  providerNameById,
  healthById,
  selection,
  onDetail,
  renderActions,
  renderStatus,
}: {
  accounts: ProviderAccount[];
  providerNameById: Map<string, string>;
  healthById: Map<string, AccountHealthSnapshot>;
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
            health={healthById.get(account.id)}
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
  health,
  selected,
  onSelect,
  onDetail,
  actions,
  status,
}: {
  account: ProviderAccount;
  providerName: string;
  health?: AccountHealthSnapshot;
  selected: boolean;
  onSelect?: () => void;
  onDetail?: () => void;
  actions: ReactNode;
  status: ReactNode;
}) {
  const { t } = useLanguage();
  const baseUrl = metadataString(account.metadata, "base_url");
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
          {baseUrl ? (
            <p className="mt-1 truncate font-mono text-2xs text-srapi-text-tertiary" title={baseUrl}>
              {baseUrl}
            </p>
          ) : null}
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
      </div>

      {/* Metrics */}
      <div className="grid grid-cols-2 gap-px border-t border-srapi-border/50 bg-srapi-border/30">
        <div className="bg-srapi-card px-4 py-2.5">
          <div className="mb-1 font-mono text-[10px] uppercase tracking-wide text-srapi-text-tertiary">{t("adminAccounts.healthTitle")}</div>
          <AccountHealthCell health={health} />
        </div>
        <div className="bg-srapi-card px-4 py-2.5">
          <div className="mb-1 font-mono text-[10px] uppercase tracking-wide text-srapi-text-tertiary">{t("adminAccounts.quotaTitle")}</div>
          <AccountQuotaCell health={health} />
        </div>
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

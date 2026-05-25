"use client";

import { type FormEvent, type ReactNode, type SelectHTMLAttributes, type TextareaHTMLAttributes, useMemo, useState } from "react";
import {
  Activity,
  AlertTriangle,
  BarChart3,
  Coins,
  Cpu,
  DollarSign,
  Key,
  Layers,
  Maximize2,
  Minimize2,
  Plus,
  RefreshCw,
  Save,
  Search,
  Shield,
  Ticket,
  UserPlus,
  Zap,
} from "lucide-react";
import {
  AdminBarList,
  AdminEmptyState,
  AdminErrorState,
  AdminLoadingState,
  AdminPageHeader,
  AdminPaginationSummary,
  AdminSection,
  AdminStatCard,
  AdminStatusBadge,
  AdminTable,
  AdminTrendBars,
} from "@/components/admin/admin-primitives";
import { AdminShell, useAdminLocale } from "@/components/admin/admin-shell";
import {
  Badge,
  Button,
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  Input,
  Label,
} from "@/components/ui";
import {
  useAdminAccountGroups,
  useAdminAccountProxyMutations,
  useAdminAccountProxyQuality,
  useAdminAccountRuntime,
  useAdminAccountGroupMutations,
  useAdminAccountMutations,
  useAdminAccounts,
  useAdminAffiliateInvites,
  useAdminAffiliateRebates,
  useAdminAffiliateTransfers,
  useAdminAnnouncementMutations,
  useAdminAnnouncements,
  useAdminDashboardSnapshot,
  useAdminModels,
  useAdminOps,
  useAdminOpsMutations,
  useAdminPaymentOrderMutations,
  useAdminPaymentOrders,
  useAdminPaymentProviderMutations,
  useAdminPaymentProviders,
  useAdminProviderMutations,
  useAdminPricingRuleMutations,
  useAdminPricingRules,
  useAdminPromoCodeMutations,
  useAdminPromoCodes,
  useAdminProviders,
  useAdminProxies,
  useAdminProxyMutations,
  useAdminRedeemCodes,
  useAdminRedeemMutations,
  useAdminRiskControl,
  useAdminRiskMutations,
  useAdminSettings,
  useAdminSettingsMutation,
  useAdminSubscriptionPlanMutations,
  useAdminSubscriptionPlans,
  useAdminUsageAggregates,
  useAdminUsageLogs,
  useAdminUserMutations,
  useAdminUsers,
  useAdminUserSubscriptionMutations,
  useAdminUserSubscriptions,
} from "@/hooks/admin-queries";
import { adminErrorMessage } from "@/lib/admin-api";
import {
  formatCompactNumber,
  formatDate,
  formatDateTime,
  formatInteger,
  formatMoney,
  formatPercent,
  safeJson,
} from "@/lib/admin-format";
import {
  ACCOUNT_RUNTIME_CLASSES,
  ACCOUNT_STATUSES,
  accountFormFromAccount,
  buildBatchUpdateAccountsBody,
  buildCreateAccountBody,
  buildImportAccountsBody,
  buildUpdateAccountBody,
  canConfirmAccountBatchStatus,
  canConfirmAccountModelDiscovery,
  canConfirmAccountProxyBinding,
  createAccountBatchStatusConfirmation,
  createAccountModelDiscoveryConfirmation,
  createAccountProxyBindingConfirmation,
  type AccountBatchStatusConfirmationState,
  emptyAccountForm,
  type AccountModelDiscoveryConfirmationState,
  type AdminAccountFormState,
  type AccountProxyBindingConfirmationState,
} from "@/lib/admin-account-form";
import {
  ANNOUNCEMENT_AUDIENCES,
  ANNOUNCEMENT_SEVERITIES,
  ANNOUNCEMENT_STATUSES,
  announcementFormFromAnnouncement,
  buildAnnouncementBody,
  canDeleteAnnouncement,
  deleteStateFromAnnouncement,
  emptyAnnouncementForm,
  type AnnouncementDeleteState,
  type AnnouncementFormState,
} from "@/lib/admin-announcement-form";
import {
  PROMO_CODE_STATUSES,
  PROMO_DISCOUNT_TYPES,
  REDEEM_CODE_TYPES,
  buildBatchGenerateRedeemCodesBody,
  buildCreateRedeemCodeBody,
  buildPromoCodeBody,
  canConfirmRedeemDisable,
  canDeletePromoCode,
  emptyPromoCodeForm,
  emptyRedeemBatchForm,
  emptyRedeemCodeForm,
  promoDeleteStateFromCode,
  promoFormFromCode,
  redeemDisableStateFromCode,
  redeemDisableStateFromSelection,
  type PromoCodeFormState,
  type PromoDeleteState,
  type RedeemBatchFormState,
  type RedeemCodeFormState,
  type RedeemDisableState,
} from "@/lib/admin-commerce-code-form";
import {
  SETTINGS_TABS,
  canConfirmSettingsSave,
  createSettingsSaveConfirmation,
  createSettingsDraft,
  materializeSettingsDraft,
  settingsTabRequiresConfirmation,
  updateSettingsValue,
  type AdminSettingsDraft,
  type SettingsSaveConfirmationState,
  type SettingsTab,
} from "@/lib/admin-settings-form";
import {
  OPS_ERROR_OWNERS,
  OPS_SLI_TYPES,
  OPS_SLO_STATUSES,
  buildCreateOpsSloBody,
  buildUpdateOpsSloBody,
  emptyOpsSloForm,
  opsSloFormFromDefinition,
  toggleErrorOwner,
  type OpsSloFormState,
} from "@/lib/admin-ops-slo-form";
import {
  PAYMENT_PROVIDER_STATUSES,
  buildCreatePaymentProviderBody,
  buildRefundPaymentOrderBody,
  emptyPaymentProviderForm,
  isRefundableOrder,
  refundFormFromOrder,
  sumOrderAmounts,
  type PaymentProviderFormState,
  type RefundOrderFormState,
} from "@/lib/admin-orders-form";
import {
  PROVIDER_ADAPTER_TYPES,
  PROVIDER_PROTOCOLS,
  RESOURCE_STATUSES,
  buildCreateProviderBody,
  buildUpdateProviderBody,
  emptyProviderForm,
  providerFormFromProvider,
  type ProviderFormState,
} from "@/lib/admin-provider-form";
import {
  PROXY_STATUSES,
  PROXY_TYPES,
  buildCreateProxyBody,
  buildUpdateProxyBody,
  emptyProxyForm,
  proxyFormFromProxy,
  type ProxyFormState,
} from "@/lib/admin-proxy-form";
import {
  ACCOUNT_GROUP_STATUSES,
  GROUP_STRATEGY_HINTS,
  accountGroupFormFromGroup,
  applyModelScopeSelection,
  applyProviderScopeSelection,
  buildCreateAccountGroupBody,
  buildUpdateAccountGroupBody,
  emptyAccountGroupForm,
  modelScopeLabel,
  providerScopeLabel,
  type AccountGroupFormState,
} from "@/lib/admin-group-form";
import {
  RISK_CONTROL_TABS,
  buildRiskControlConfig,
  canConfirmRiskControlSave,
  createRiskControlSaveConfirmation,
  createRiskControlForm,
  updateRiskControlForm,
  type RiskControlSaveConfirmationState,
  type RiskControlFormState,
  type RiskControlTab,
} from "@/lib/admin-risk-control-form";
import {
  SUBSCRIPTION_PLAN_STATUSES,
  USER_SUBSCRIPTION_STATUSES,
  buildCreatePricingRuleBody,
  buildCreateSubscriptionPlanBody,
  buildCreateUserSubscriptionBody,
  canConfirmPricingRuleCreate,
  createPricingRuleConfirmation,
  emptyPricingRuleForm,
  emptySubscriptionPlanForm,
  emptyUserSubscriptionForm,
  type PricingRuleCreateConfirmationState,
  type PricingRuleFormState,
  type SubscriptionPlanFormState,
  type UserSubscriptionFormState,
} from "@/lib/admin-subscription-form";
import {
  accountToggleIdentifier,
  canConfirmToggle,
  toggleActionFromStatus,
  toggleActionLabel,
  userToggleIdentifier,
} from "@/lib/admin-toggle-confirmation";
import type {
  AccountGroup,
  AccountHealthSnapshot,
  AccountModelDiscovery,
  AccountProxyQuality,
  AccountQuotaSnapshot,
  AccountRpmStatus,
  AdminSettings,
  AdminTestResult,
  AffiliateInviteRecord,
  AffiliateLedgerEntry,
  Announcement,
  PaymentOrder,
  PaymentOrderStatus,
  PaymentProviderInstance,
  PricingRule,
  Provider,
  ProviderAccount,
  ProviderAccountExportItem,
  ProviderAccountImportResult,
  ProviderAccountStatus,
  BatchUpdateAccountsResult,
  OpsSlo,
  OpsSloDefinition,
  ProxyDefinition,
  ProxyDefinitionStatus,
  PromoCode,
  RedeemCodeStatus,
  PromoCodeStatus,
  RiskControlConfig,
  SchedulerStrategyName,
  SubscriptionPlan,
  UsageLog,
  UserSubscription,
  User,
  UserStatus,
} from "../../../../../packages/sdk/typescript/src/types.gen";

function usePageCopy(en: string, zh: string) {
  const { isZh } = useAdminLocale();
  return isZh ? zh : en;
}

function dateLabel(value: string) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return value;
  }
  return new Intl.DateTimeFormat(undefined, { month: "short", day: "2-digit" }).format(date);
}

function schedulerStrategyNameFromSelect(value: string): SchedulerStrategyName | undefined {
  switch (value) {
    case "balanced":
    case "cost_saver":
    case "latency_first":
    case "quota_protect":
    case "sticky_first":
    case "cache_affinity_first":
    case "premium_quality":
      return value;
    default:
      return undefined;
  }
}

type OpsEvidenceSelection =
  | { kind: "alert"; title: string; details: unknown }
  | { kind: "log"; title: string; details: unknown }
  | { kind: "error"; title: string; errorClass: string; owner: string }
  | { kind: "request"; title: string; request: UsageLog };

function MutationError({ error }: { error: unknown }) {
  if (!error) {
    return null;
  }
  return (
    <div className="rounded-xl border border-srapi-error/30 bg-srapi-error/5 p-3 text-xs text-srapi-error">
      {adminErrorMessage(error)}
    </div>
  );
}

function Textarea({
  className,
  ...props
}: TextareaHTMLAttributes<HTMLTextAreaElement>) {
  return (
    <textarea
      className={[
        "w-full rounded-xl border border-srapi-border bg-srapi-bg px-3.5 py-3 font-mono text-xs text-srapi-text-primary",
        "placeholder:text-srapi-text-secondary/50 focus:border-srapi-primary focus:outline-none focus:ring-2 focus:ring-srapi-primary/20",
        className || "",
      ].join(" ")}
      {...props}
    />
  );
}

function Select({
  className,
  ...props
}: SelectHTMLAttributes<HTMLSelectElement>) {
  return (
    <select
      className={[
        "rounded-xl border border-srapi-border bg-srapi-bg px-3.5 py-3 font-mono text-xs text-srapi-text-primary",
        "focus:border-srapi-primary focus:outline-none focus:ring-2 focus:ring-srapi-primary/20",
        className || "",
      ].join(" ")}
      {...props}
    />
  );
}

function toggleIdSelection(values: string[], id: string, checked: boolean) {
  if (checked) {
    return values.includes(id) ? values : [...values, id];
  }
  return values.filter((value) => value !== id);
}

function SettingsToggle({
  label,
  checked,
  onChange,
}: {
  label: string;
  checked: boolean;
  onChange: (checked: boolean) => void;
}) {
  return (
    <label className="flex items-center justify-between gap-4 rounded-xl border border-srapi-border p-3 text-xs">
      <span className="font-mono font-bold uppercase text-srapi-text-primary">{label}</span>
      <input
        type="checkbox"
        checked={checked}
        onChange={(event) => onChange(event.target.checked)}
      />
    </label>
  );
}

export function AdminDashboardProductionPage() {
  const title = usePageCopy("Admin Dashboard", "管理员仪表盘");
  const description = usePageCopy(
    "Live control-plane snapshot for inventory, traffic, tokens, cost, latency and top consumers.",
    "基于真实管理端接口的库存、流量、Token、成本、延迟和用户用量总览。",
  );
  const loading = usePageCopy("Loading dashboard snapshot...", "正在加载仪表盘快照...");
  const query = useAdminDashboardSnapshot();
  const snapshot = query.data;

  return (
    <AdminShell>
      <AdminPageHeader
        title={title}
        description={description}
        actions={
          <Button type="button" variant="outline" size="sm" onClick={() => void query.refetch()}>
            <RefreshCw size={12} />
            Refresh
          </Button>
        }
      />

      {query.isLoading ? <AdminLoadingState label={loading} /> : null}
      {query.isError ? <AdminErrorState error={query.error} onRetry={() => void query.refetch()} /> : null}

      {snapshot ? (
        <>
          <div className="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-4">
            <AdminStatCard
              label="API Keys"
              value={formatInteger(snapshot.inventory.total_api_keys)}
              detail={`${formatInteger(snapshot.inventory.active_api_keys)} active`}
              icon={<Key size={16} />}
            />
            <AdminStatCard
              label="Accounts"
              value={formatInteger(snapshot.inventory.total_accounts)}
              detail={`${formatInteger(snapshot.inventory.healthy_accounts)} healthy / ${formatInteger(snapshot.inventory.abnormal_accounts)} abnormal`}
              icon={<Cpu size={16} />}
              tone={snapshot.inventory.abnormal_accounts > 0 ? "warning" : "neutral"}
            />
            <AdminStatCard
              label="Today Requests"
              value={formatInteger(snapshot.traffic.today_requests)}
              detail={`${formatInteger(snapshot.traffic.error_requests)} errors in window`}
              icon={<Activity size={16} />}
            />
            <AdminStatCard
              label="New Users"
              value={formatInteger(snapshot.users.today_new_users)}
              detail={`${formatInteger(snapshot.users.total_users)} total users`}
              icon={<UserPlus size={16} />}
            />
          </div>

          <AdminSection
            title="Token And Cost"
            description="Decimal values come directly from the backend; financial math is not recomputed in the browser."
          >
            <div className="grid grid-cols-1 gap-4 md:grid-cols-5">
              <AdminStatCard
                label="Today Tokens"
                value={formatCompactNumber(snapshot.tokens.today_tokens)}
                detail={`${formatCompactNumber(snapshot.tokens.total_tokens)} all time`}
                icon={<Coins size={16} />}
              />
              <AdminStatCard
                label="Input Tokens"
                value={formatCompactNumber(snapshot.tokens.input_tokens)}
                icon={<BarChart3 size={16} />}
              />
              <AdminStatCard
                label="Output Tokens"
                value={formatCompactNumber(snapshot.tokens.output_tokens)}
                icon={<Zap size={16} />}
              />
              <AdminStatCard
                label="Actual Cost"
                value={formatMoney(snapshot.tokens.costs.actual_cost, snapshot.tokens.costs.currency)}
                icon={<DollarSign size={16} />}
              />
              <AdminStatCard
                label="Standard Cost"
                value={formatMoney(snapshot.tokens.costs.standard_cost, snapshot.tokens.costs.currency)}
                icon={<DollarSign size={16} />}
                tone="success"
              />
            </div>
          </AdminSection>

          <div className="grid grid-cols-1 gap-4 md:grid-cols-3">
            <AdminStatCard
              label="RPM"
              value={formatCompactNumber(snapshot.performance.current_rpm)}
              detail={`Peak ${formatCompactNumber(snapshot.performance.peak_rpm)}`}
              icon={<Activity size={16} />}
            />
            <AdminStatCard
              label="TPM"
              value={formatCompactNumber(snapshot.performance.current_tpm)}
              detail={`Peak ${formatCompactNumber(snapshot.performance.peak_tpm)}`}
              icon={<Zap size={16} />}
            />
            <AdminStatCard
              label="Avg / P95 Latency"
              value={`${formatInteger(snapshot.performance.average_latency_ms)}ms`}
              detail={`P95 ${formatInteger(snapshot.performance.p95_latency_ms)}ms`}
              icon={<Cpu size={16} />}
            />
          </div>

          <div className="grid grid-cols-1 gap-6 xl:grid-cols-2">
            <AdminSection title="Model Distribution" description="Ranked by request count in the selected window.">
              <AdminBarList
                emptyLabel="No model distribution yet"
                items={snapshot.model_distribution.map((item) => ({
                  label: item.model,
                  value: item.request_count,
                  detail: `${formatInteger(item.request_count)} req / ${formatCompactNumber(item.token_count)} tokens`,
                }))}
              />
            </AdminSection>
            <AdminSection title="Token Trend" description="Token volume buckets returned by /admin/dashboard/snapshot.">
              <AdminTrendBars
                emptyLabel="No token trend yet"
                points={snapshot.token_trend.map((point) => ({
                  label: dateLabel(point.bucket_start),
                  value: point.token_count,
                }))}
              />
            </AdminSection>
          </div>

          <AdminSection title="Top User Usage" description="Top 12 users by backend-reported cost.">
            <AdminTable
              empty={<AdminEmptyState title="No user usage in this window" />}
              columns={[
                { key: "user", header: "User" },
                { key: "requests", header: "Requests" },
                { key: "tokens", header: "Tokens" },
                { key: "cost", header: "Cost" },
              ]}
              rows={snapshot.user_usage_trend.slice(0, 12).map((item) => ({
                user: (
                  <div>
                    <div className="font-semibold text-srapi-text-primary">{item.email || item.user_id}</div>
                    <div className="text-[10px] text-srapi-text-secondary">{item.user_id}</div>
                  </div>
                ),
                requests: formatInteger(item.request_count),
                tokens: formatCompactNumber(item.token_count),
                cost: formatMoney(item.cost),
              }))}
              getRowKey={(row, index) => String(snapshot.user_usage_trend[index]?.user_id ?? index)}
            />
          </AdminSection>
        </>
      ) : null}
    </AdminShell>
  );
}

export function AdminOpsProductionPage() {
  const [range, setRange] = useState("1h");
  const [autoRefresh, setAutoRefresh] = useState(true);
  const [isFullscreen, setIsFullscreen] = useState(false);
  const [sloDialogOpen, setSloDialogOpen] = useState(false);
  const [editingSlo, setEditingSlo] = useState<OpsSloDefinition | null>(null);
  const [sloForm, setSloForm] = useState<OpsSloFormState>(() => emptyOpsSloForm());
  const [sloFormError, setSloFormError] = useState<string | null>(null);
  const [evidenceSelection, setEvidenceSelection] = useState<OpsEvidenceSelection | null>(null);
  const [usageModelFilter, setUsageModelFilter] = useState("");
  const interval = autoRefresh ? 30_000 : false;
  const ops = useAdminOps({ bucket: range === "24h" ? "day" : "hour", refetchIntervalMs: interval });
  const usageEvidence = useAdminUsageLogs({ page: 1, model: usageModelFilter });
  const mutations = useAdminOpsMutations();

  const opsQueries = Object.values(ops);
  const anyLoading = opsQueries.some((query) => query.isLoading);
  const firstError = opsQueries.find((query) => query.isError)?.error;
  const overview = ops.overview.data;
  const concurrency = ops.concurrency.data;
  const recentRequests = usageEvidence.data?.data ?? [];
  const openCreateSlo = () => {
    setEditingSlo(null);
    setSloFormError(null);
    setSloForm(emptyOpsSloForm());
    setSloDialogOpen(true);
  };
  const openEditSlo = (slo: OpsSlo) => {
    setEditingSlo(slo.definition);
    setSloFormError(null);
    setSloForm(opsSloFormFromDefinition(slo.definition));
    setSloDialogOpen(true);
  };
  const submitSlo = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setSloFormError(null);
    try {
      if (editingSlo) {
        mutations.updateSlo.mutate(
          { id: editingSlo.id, body: buildUpdateOpsSloBody(sloForm) },
          { onSuccess: () => setSloDialogOpen(false) },
        );
        return;
      }
      mutations.createSlo.mutate(
        buildCreateOpsSloBody(sloForm),
        { onSuccess: () => setSloDialogOpen(false) },
      );
    } catch (error) {
      setSloFormError(error instanceof Error ? error.message : "Invalid SLO form.");
    }
  };

  return (
    <AdminShell>
      <div className={isFullscreen ? "fixed inset-0 z-40 overflow-y-auto bg-srapi-bg p-6" : "space-y-8"}>
        <AdminPageHeader
          title="Operations Center"
          description="SRE-oriented gateway console: traffic, errors, latency, concurrency, realtime slots, SLO burn and logs."
          actions={
            <>
              <Select value={range} onChange={(event) => setRange(event.target.value)}>
                <option value="1h">1h</option>
                <option value="6h">6h</option>
                <option value="24h">24h</option>
              </Select>
              <Button
                type="button"
                variant={autoRefresh ? "accent" : "outline"}
                size="sm"
                onClick={() => setAutoRefresh((value) => !value)}
              >
                <RefreshCw size={12} />
                Auto {autoRefresh ? "On" : "Off"}
              </Button>
              <Button type="button" variant="outline" size="icon" onClick={() => setIsFullscreen((value) => !value)}>
                {isFullscreen ? <Minimize2 size={14} /> : <Maximize2 size={14} />}
              </Button>
            </>
          }
        />

        {anyLoading ? <AdminLoadingState label="Loading operations telemetry..." /> : null}
        {firstError ? <AdminErrorState error={firstError} onRetry={() => {
          opsQueries.forEach((query) => void query.refetch());
          void usageEvidence.refetch();
        }} /> : null}

        {overview ? (
          <div className="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-4">
            <AdminStatCard label="Requests" value={formatInteger(overview.request_count)} detail={`${formatInteger(overview.success_count)} success`} icon={<Activity size={16} />} />
            <AdminStatCard label="Error Rate" value={formatPercent(overview.error_rate)} detail={`${formatInteger(overview.error_count)} errors`} icon={<AlertTriangle size={16} />} tone={overview.error_rate > 0.02 ? "danger" : "success"} />
            <AdminStatCard label="Latency P95 / P99" value={`${formatInteger(overview.latency_p95_ms)}ms`} detail={`P99 ${formatInteger(overview.latency_p99_ms)}ms`} icon={<Cpu size={16} />} />
            <AdminStatCard label="RPM / TPM" value={`${formatCompactNumber(overview.rpm)} / ${formatCompactNumber(overview.tpm)}`} detail={`${formatInteger(overview.active_users)} active users`} icon={<Zap size={16} />} />
          </div>
        ) : null}

        {concurrency ? (
          <AdminSection title="Concurrency" description="Active gateway requests and realtime slots, without API-key labels in the UI.">
            <div className="grid grid-cols-1 gap-4 md:grid-cols-3">
              <AdminStatCard label="Gateway Requests" value={formatInteger(concurrency.active_gateway_requests)} icon={<Activity size={16} />} />
              <AdminStatCard label="Realtime Slots" value={formatInteger(concurrency.active_realtime_slots)} icon={<Zap size={16} />} />
              <AdminStatCard label="API Keys With Activity" value={formatInteger(Object.keys(concurrency.active_by_api_key).length)} icon={<Key size={16} />} />
            </div>
          </AdminSection>
        ) : null}

        <div className="grid grid-cols-1 gap-6 xl:grid-cols-2">
          <AdminSection title="Throughput Trend">
            <AdminTrendBars
              emptyLabel="No throughput data"
              points={(ops.throughput.data?.points ?? []).map((point) => ({
                label: dateLabel(point.bucket_start),
                value: point.request_count,
              }))}
            />
          </AdminSection>
          <AdminSection title="Error Trend">
            <AdminTrendBars
              emptyLabel="No error trend data"
              points={(ops.errorTrend.data?.points ?? []).map((point) => ({
                label: dateLabel(point.bucket_start),
                value: point.error_count,
              }))}
            />
          </AdminSection>
          <AdminSection title="Latency Histogram">
            <AdminBarList
              emptyLabel="No latency histogram"
              items={(ops.latencyHistogram.data?.buckets ?? []).map((bucket) => ({
                label: bucket.label,
                value: bucket.count,
                detail: `${formatInteger(bucket.count)} (${formatPercent(bucket.share)})`,
              }))}
            />
          </AdminSection>
          <AdminSection title="Error Distribution">
            {(ops.errorDistribution.data?.items ?? []).length ? (
              <div className="space-y-2">
                {(ops.errorDistribution.data?.items ?? []).map((item) => (
                  <button
                    key={`${item.error_class}-${item.owner}`}
                    type="button"
                    className="flex w-full items-center justify-between gap-4 rounded-xl border border-srapi-border bg-srapi-card-muted/20 px-4 py-3 text-left text-xs transition hover:border-srapi-primary/50"
                    onClick={() => setEvidenceSelection({
                      kind: "error",
                      title: `${item.error_class} evidence`,
                      errorClass: item.error_class,
                      owner: item.owner,
                    })}
                  >
                    <span className="font-mono text-srapi-text-primary">{item.error_class} / {item.owner}</span>
                    <span className="text-srapi-text-secondary">{formatInteger(item.count)} ({formatPercent(item.share)})</span>
                  </button>
                ))}
              </div>
            ) : (
              <AdminEmptyState title="No error distribution" />
            )}
          </AdminSection>
        </div>

        <AdminSection
          title="SLO Burn"
          description="Definitions and burn-rate evaluations are backed by /api/v1/admin/ops/slo."
          actions={<Button type="button" size="sm" onClick={openCreateSlo}><Plus size={12} />New SLO</Button>}
        >
          <AdminTable
            empty={<AdminEmptyState title="No SLOs configured" />}
            columns={[
              { key: "name", header: "SLO" },
              { key: "filter", header: "Filter" },
              { key: "objective", header: "Objective" },
              { key: "burn", header: "Burn Rate" },
              { key: "budget", header: "Budget Consumed" },
              { key: "status", header: "Status" },
              { key: "actions", header: "" },
            ]}
            rows={(ops.slos.data?.data ?? []).map((slo) => ({
              name: (
                <div>
                  <div className="font-semibold text-srapi-text-primary">{slo.definition.name}</div>
                  <div className="text-[10px] text-srapi-text-secondary">{slo.definition.sli_type} / {slo.definition.window_days}d</div>
                </div>
              ),
              filter: (
                <div className="max-w-[260px] whitespace-normal text-[11px] text-srapi-text-secondary">
                  <div>{slo.definition.filter.source_endpoint || "all endpoints"}</div>
                  <div>{slo.definition.filter.model || "all models"}</div>
                </div>
              ),
              objective: formatPercent(slo.evaluation.objective),
              burn: slo.evaluation.burn_rate.toFixed(2),
              budget: formatPercent(slo.evaluation.error_budget_consumed),
              status: <AdminStatusBadge status={slo.definition.status} />,
              actions: (
                <Button type="button" variant="outline" size="sm" onClick={() => openEditSlo(slo)}>
                  Edit
                </Button>
              ),
            }))}
            getRowKey={(_, index) => ops.slos.data?.data[index]?.definition.id ?? String(index)}
          />
          <MutationError error={mutations.createSlo.error || mutations.updateSlo.error} />
        </AdminSection>

        <div className="grid grid-cols-1 gap-6 xl:grid-cols-2">
          <AdminSection title="Alert Events" description="Acknowledge is a real mutation against /api/v1/admin/ops/alerts/{id}/ack.">
            <AdminTable
              empty={<AdminEmptyState title="No active alert events" />}
              columns={[
                { key: "summary", header: "Summary" },
                { key: "severity", header: "Severity" },
                { key: "status", header: "Status" },
                { key: "actions", header: "" },
              ]}
              rows={(ops.alerts.data?.data ?? []).map((alert) => ({
                summary: (
                  <div className="max-w-[280px] whitespace-normal">
                    <div className="font-semibold text-srapi-text-primary">{alert.summary}</div>
                    <div className="text-[10px] text-srapi-text-secondary">{formatDateTime(alert.started_at)}</div>
                  </div>
                ),
                severity: <AdminStatusBadge status={alert.severity} />,
                status: <AdminStatusBadge status={alert.status} />,
                actions: (
                  <div className="flex flex-wrap justify-end gap-2">
                    <Button
                      type="button"
                      variant="outline"
                      size="sm"
                      onClick={() => setEvidenceSelection({
                        kind: "alert",
                        title: alert.summary,
                        details: alert,
                      })}
                    >
                      Evidence
                    </Button>
                    <Button
                      type="button"
                      variant="outline"
                      size="sm"
                      disabled={alert.status !== "firing" || mutations.acknowledgeAlert.isPending}
                      onClick={() => mutations.acknowledgeAlert.mutate(alert.id)}
                    >
                      Ack
                    </Button>
                  </div>
                ),
              }))}
              getRowKey={(_, index) => ops.alerts.data?.data[index]?.id ?? String(index)}
            />
            <MutationError error={mutations.acknowledgeAlert.error} />
          </AdminSection>

          <AdminSection title="System Logs">
            <AdminTable
              empty={<AdminEmptyState title="No system logs" />}
              columns={[
                { key: "time", header: "Time" },
                { key: "level", header: "Level" },
                { key: "source", header: "Source" },
                { key: "message", header: "Message" },
                { key: "actions", header: "" },
              ]}
              rows={(ops.logs.data?.data ?? []).map((log) => ({
                time: formatDateTime(log.created_at),
                level: <AdminStatusBadge status={log.level} />,
                source: log.source,
                message: <span className="whitespace-normal">{log.message}</span>,
                actions: (
                  <Button
                    type="button"
                    variant="outline"
                    size="sm"
                    onClick={() => setEvidenceSelection({
                      kind: "log",
                      title: `${log.source} log`,
                      details: log,
                    })}
                  >
                    Details
                  </Button>
                ),
              }))}
              getRowKey={(_, index) => ops.logs.data?.data[index]?.id ?? String(index)}
            />
          </AdminSection>
        </div>

        <AdminSection
          title="Recent Request Evidence"
          description="Usage logs connect gateway symptoms to request ids, selected provider/account ids, latency, tokens and error class."
          actions={
            <Input
              value={usageModelFilter}
              onChange={(event) => setUsageModelFilter(event.target.value)}
              placeholder="Filter model"
            />
          }
        >
          {usageEvidence.isLoading ? (
            <div className="rounded-xl border border-srapi-border bg-srapi-card-muted/20 p-4 text-xs text-srapi-text-secondary">
              Loading recent request evidence...
            </div>
          ) : usageEvidence.error ? (
            <div className="space-y-3 rounded-xl border border-srapi-error/30 bg-srapi-error/5 p-4">
              <div className="font-mono text-xs font-bold uppercase tracking-wider text-srapi-error">
                Usage evidence request failed
              </div>
              <p className="text-xs leading-relaxed text-srapi-text-secondary">
                {adminErrorMessage(usageEvidence.error)}
              </p>
              <Button type="button" variant="outline" size="sm" onClick={() => void usageEvidence.refetch()}>
                <RefreshCw size={12} />
                Retry evidence
              </Button>
            </div>
          ) : (
            <>
              <AdminTable
                empty={<AdminEmptyState title="No recent request evidence" />}
                columns={[
                  { key: "request", header: "Request" },
                  { key: "model", header: "Model" },
                  { key: "route", header: "Route" },
                  { key: "latency", header: "Latency" },
                  { key: "cost", header: "Cost" },
                  { key: "status", header: "Status" },
                  { key: "actions", header: "" },
                ]}
                rows={recentRequests.slice(0, 12).map((request) => ({
                  request: <span className="select-all font-mono text-[11px]">{request.request_id}</span>,
                  model: request.model,
                  route: (
                    <div className="font-mono text-[11px] text-srapi-text-secondary">
                      <div>provider {request.provider_id || "-"}</div>
                      <div>account {request.account_id || "-"}</div>
                    </div>
                  ),
                  latency: `${formatInteger(request.latency_ms)}ms`,
                  cost: formatMoney(request.cost, request.currency),
                  status: request.success
                    ? <AdminStatusBadge status="success" />
                    : <AdminStatusBadge status={request.error_class || "failed"} />,
                  actions: (
                    <Button
                      type="button"
                      variant="outline"
                      size="sm"
                      onClick={() => setEvidenceSelection({
                        kind: "request",
                        title: request.request_id,
                        request,
                      })}
                    >
                      Details
                    </Button>
                  ),
                }))}
                getRowKey={(_, index) => recentRequests[index]?.id ?? String(index)}
              />
              <AdminPaginationSummary pagination={usageEvidence.data?.pagination} />
            </>
          )}
        </AdminSection>

        <OpsEvidenceDrawer
          selection={evidenceSelection}
          requests={recentRequests}
          onClose={() => setEvidenceSelection(null)}
        />

        <Dialog open={sloDialogOpen} onOpenChange={setSloDialogOpen}>
          <DialogContent className="max-h-[90vh] max-w-3xl overflow-y-auto">
            <form className="space-y-4" onSubmit={submitSlo}>
              <DialogHeader>
                <DialogTitle>{editingSlo ? "Edit SLO" : "Create SLO"}</DialogTitle>
                <DialogDescription>
                  SLOs define user-facing gateway reliability targets and multi-window burn-rate alert thresholds.
                </DialogDescription>
              </DialogHeader>
              <div className="grid grid-cols-1 gap-3 md:grid-cols-2">
                <div className="space-y-2">
                  <Label htmlFor="ops-slo-name">Name</Label>
                  <Input id="ops-slo-name" required value={sloForm.name} onChange={(event) => setSloForm((value) => ({ ...value, name: event.target.value }))} />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="ops-slo-sli-type">SLI Type</Label>
                  <Select id="ops-slo-sli-type" disabled={Boolean(editingSlo)} value={sloForm.sliType} onChange={(event) => setSloForm((value) => ({ ...value, sliType: event.target.value as OpsSloFormState["sliType"] }))} className="w-full">
                    {OPS_SLI_TYPES.map((type) => <option key={type} value={type}>{type}</option>)}
                  </Select>
                </div>
                <div className="space-y-2">
                  <Label htmlFor="ops-slo-objective">Objective Percent</Label>
                  <Input id="ops-slo-objective" type="number" min="0.001" max="100" step="0.001" value={sloForm.objective} onChange={(event) => setSloForm((value) => ({ ...value, objective: event.target.value }))} />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="ops-slo-window-days">Window Days</Label>
                  <Input id="ops-slo-window-days" type="number" min="1" max="365" value={sloForm.windowDays} onChange={(event) => setSloForm((value) => ({ ...value, windowDays: event.target.value }))} />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="ops-slo-status">Status</Label>
                  <Select id="ops-slo-status" value={sloForm.status} onChange={(event) => setSloForm((value) => ({ ...value, status: event.target.value as OpsSloFormState["status"] }))} className="w-full">
                    {OPS_SLO_STATUSES.map((statusValue) => <option key={statusValue} value={statusValue}>{statusValue}</option>)}
                  </Select>
                </div>
                <div className="space-y-2">
                  <Label htmlFor="ops-slo-policy-name">Alert Policy</Label>
                  <Input id="ops-slo-policy-name" value={sloForm.policyName} onChange={(event) => setSloForm((value) => ({ ...value, policyName: event.target.value }))} />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="ops-slo-source-endpoint">Source Endpoint</Label>
                  <Input id="ops-slo-source-endpoint" value={sloForm.sourceEndpoint} onChange={(event) => setSloForm((value) => ({ ...value, sourceEndpoint: event.target.value }))} />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="ops-slo-model">Model Filter</Label>
                  <Input id="ops-slo-model" value={sloForm.model} onChange={(event) => setSloForm((value) => ({ ...value, model: event.target.value }))} />
                </div>
                <div className="space-y-2 md:col-span-2">
                  <Label htmlFor="ops-slo-provider-id">Provider ID</Label>
                  <Input id="ops-slo-provider-id" value={sloForm.providerId} onChange={(event) => setSloForm((value) => ({ ...value, providerId: event.target.value }))} placeholder="optional provider id" />
                </div>
              </div>
              <div className="space-y-2">
                <Label>Error Owner Exclusions</Label>
                <div className="grid grid-cols-1 gap-2 rounded-xl border border-srapi-border p-3 md:grid-cols-3">
                  {OPS_ERROR_OWNERS.map((owner) => (
                    <label key={owner} className="flex items-center justify-between rounded-lg border border-srapi-border/70 px-3 py-2 text-xs">
                      <span className="font-mono text-srapi-text-primary">{owner}</span>
                      <input
                        type="checkbox"
                        checked={sloForm.errorOwnerExclude.includes(owner)}
                        onChange={(event) => setSloForm((value) => ({
                          ...value,
                          errorOwnerExclude: toggleErrorOwner(value.errorOwnerExclude, owner, event.target.checked),
                        }))}
                      />
                    </label>
                  ))}
                </div>
              </div>
              <div className="space-y-2">
                <Label htmlFor="ops-slo-thresholds">Burn-rate Thresholds JSON</Label>
                <Textarea id="ops-slo-thresholds" rows={10} value={sloForm.thresholdsJson} onChange={(event) => setSloForm((value) => ({ ...value, thresholdsJson: event.target.value }))} />
              </div>
              {sloFormError ? (
                <div className="rounded-xl border border-srapi-error/30 bg-srapi-error/5 p-3 text-xs text-srapi-error">
                  {sloFormError}
                </div>
              ) : null}
              <MutationError error={mutations.createSlo.error || mutations.updateSlo.error} />
              <DialogFooter>
                <Button type="button" variant="outline" onClick={() => setSloDialogOpen(false)}>Cancel</Button>
                <Button type="submit" disabled={mutations.createSlo.isPending || mutations.updateSlo.isPending}>
                  <Save size={12} />
                  Save SLO
                </Button>
              </DialogFooter>
            </form>
          </DialogContent>
        </Dialog>
      </div>
    </AdminShell>
  );
}

type UserFormState = {
  name: string;
  email: string;
  password: string;
  roles: string;
  status: UserStatus;
  balance: string;
  currency: string;
  rpmLimit: string;
};

const emptyUserForm: UserFormState = {
  name: "",
  email: "",
  password: "",
  roles: "user",
  status: "active",
  balance: "0",
  currency: "USD",
  rpmLimit: "",
};

function userToForm(user: User): UserFormState {
  return {
    name: user.name,
    email: user.email,
    password: "",
    roles: user.roles.join(","),
    status: user.status,
    balance: user.balance,
    currency: user.currency,
    rpmLimit: user.rpm_limit?.toString() ?? "",
  };
}

export function AdminUsersProductionPage() {
  const [page, setPage] = useState(1);
  const [q, setQ] = useState("");
  const [status, setStatus] = useState<UserStatus | "all">("all");
  const [userDialogOpen, setUserDialogOpen] = useState(false);
  const [editing, setEditing] = useState<User | null>(null);
  const [form, setForm] = useState<UserFormState>(emptyUserForm);
  const [balanceUser, setBalanceUser] = useState<User | null>(null);
  const [balanceForm, setBalanceForm] = useState({ operation: "increment", amount: "0", note: "" });
  const [toggleUser, setToggleUser] = useState<{ user: User; confirmation: string } | null>(null);
  const users = useAdminUsers({ page, q, status });
  const mutations = useAdminUserMutations();

  const openCreate = () => {
    setEditing(null);
    setForm(emptyUserForm);
    setUserDialogOpen(true);
  };

  const openEdit = (user: User) => {
    setEditing(user);
    setForm(userToForm(user));
    setUserDialogOpen(true);
  };

  const closeUserDialog = () => {
    setUserDialogOpen(false);
    setEditing(null);
    setForm(emptyUserForm);
  };

  const saveUser = (event: FormEvent) => {
    event.preventDefault();
    const roles = form.roles
      .split(",")
      .map((role) => role.trim())
      .filter(Boolean) as User["roles"];
    const rpm_limit = form.rpmLimit ? Number(form.rpmLimit) : null;

    if (editing) {
      mutations.update.mutate(
        {
          id: editing.id,
          body: {
            name: form.name,
            email: form.email,
            roles,
            status: form.status,
            rpm_limit,
            ...(form.password ? { password: form.password } : {}),
          },
        },
        { onSuccess: closeUserDialog },
      );
      return;
    }

    mutations.create.mutate(
      {
        name: form.name,
        email: form.email,
        password: form.password,
        roles,
        status: form.status,
        balance: form.balance,
        currency: form.currency,
        rpm_limit,
      },
      { onSuccess: closeUserDialog },
    );
  };

  const adjustBalance = (event: FormEvent) => {
    event.preventDefault();
    if (!balanceUser) {
      return;
    }
    mutations.balance.mutate(
      {
        id: balanceUser.id,
        body: {
          operation: balanceForm.operation as "set" | "increment" | "decrement",
          amount: balanceForm.amount,
          currency: balanceUser.currency,
          note: balanceForm.note || undefined,
        },
      },
      { onSuccess: () => setBalanceUser(null) },
    );
  };
  const toggleUserAction = toggleUser ? toggleActionFromStatus(toggleUser.user.status) : "disable";
  const toggleUserIdentifier = toggleUser ? userToggleIdentifier(toggleUser.user) : "";
  const canToggleUser = toggleUser ? canConfirmToggle(toggleUserIdentifier, toggleUser.confirmation) : false;
  const confirmUserToggle = () => {
    if (!toggleUser || !canToggleUser) {
      return;
    }
    mutations.toggle.mutate(toggleUser.user, { onSuccess: () => setToggleUser(null) });
  };

  return (
    <AdminShell>
      <AdminPageHeader
        title="User Management"
        description="Search, page, create, edit, disable and adjust balances through the generated admin SDK."
        actions={
          <Button type="button" size="sm" onClick={openCreate}>
            <Plus size={12} />
            Create User
          </Button>
        }
      />

      <AdminSection title="Users" description="Filters are part of the React Query key and are sent to /api/v1/admin/users.">
        <div className="mb-4 flex flex-col gap-3 md:flex-row md:items-center">
          <div className="relative flex-1">
            <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-srapi-text-secondary" />
            <Input className="pl-9" value={q} onChange={(event) => setQ(event.target.value)} placeholder="Search name or email" />
          </div>
          <Select value={status} onChange={(event) => setStatus(event.target.value as UserStatus | "all")}>
            <option value="all">All status</option>
            <option value="active">Active</option>
            <option value="pending">Pending</option>
            <option value="disabled">Disabled</option>
          </Select>
        </div>

        {users.isLoading ? <AdminLoadingState label="Loading users..." /> : null}
        {users.isError ? <AdminErrorState error={users.error} onRetry={() => void users.refetch()} /> : null}
        {users.data ? (
          <>
            <AdminTable
              empty={<AdminEmptyState title="No users matched these filters" />}
              columns={[
                { key: "user", header: "User" },
                { key: "roles", header: "Roles" },
                { key: "balance", header: "Balance" },
                { key: "rpm", header: "RPM Limit" },
                { key: "status", header: "Status" },
                { key: "created", header: "Created" },
                { key: "actions", header: "" },
              ]}
              rows={users.data.data.map((user) => ({
                user: (
                  <div>
                    <div className="font-semibold text-srapi-text-primary">{user.name}</div>
                    <div className="text-[10px] text-srapi-text-secondary">{user.email}</div>
                  </div>
                ),
                roles: user.roles.join(", "),
                balance: formatMoney(user.balance, user.currency),
                rpm: user.rpm_limit ?? "inherit",
                status: <AdminStatusBadge status={user.status} />,
                created: formatDate(user.created_at),
                actions: (
                  <div className="flex justify-end gap-2">
                    <Button type="button" variant="outline" size="sm" onClick={() => openEdit(user)}>
                      Edit
                    </Button>
                    <Button type="button" variant="outline" size="sm" onClick={() => setBalanceUser(user)}>
                      Balance
                    </Button>
                    <Button
                      type="button"
                      variant={user.status === "disabled" ? "outline" : "danger"}
                      size="sm"
                      disabled={mutations.toggle.isPending}
                      onClick={() => setToggleUser({ user, confirmation: "" })}
                    >
                      {user.status === "disabled" ? "Enable" : "Disable"}
                    </Button>
                  </div>
                ),
              }))}
              getRowKey={(_, index) => users.data?.data[index]?.id ?? String(index)}
            />
            <AdminPaginationSummary pagination={users.data.pagination} />
            <div className="mt-4 flex gap-2">
              <Button type="button" variant="outline" size="sm" disabled={page <= 1} onClick={() => setPage((value) => Math.max(1, value - 1))}>
                Previous
              </Button>
              <Button type="button" variant="outline" size="sm" disabled={!users.data.pagination?.has_next} onClick={() => setPage((value) => value + 1)}>
                Next
              </Button>
            </div>
          </>
        ) : null}
      </AdminSection>

      <Dialog open={userDialogOpen} onOpenChange={(open) => !open && closeUserDialog()}>
        <DialogContent>
          <form className="space-y-4" onSubmit={saveUser}>
            <DialogHeader>
              <DialogTitle>{editing ? "Edit User" : "Create User"}</DialogTitle>
              <DialogDescription>Passwords are write-only and are never displayed by the API.</DialogDescription>
            </DialogHeader>
            <div className="grid gap-3">
              <Label>Name</Label>
              <Input required value={form.name} onChange={(event) => setForm((value) => ({ ...value, name: event.target.value }))} />
              <Label>Email</Label>
              <Input required type="email" value={form.email} onChange={(event) => setForm((value) => ({ ...value, email: event.target.value }))} />
              <Label>Password</Label>
              <Input required={!editing} type="password" value={form.password} onChange={(event) => setForm((value) => ({ ...value, password: event.target.value }))} />
              <Label>Roles (comma separated)</Label>
              <Input value={form.roles} onChange={(event) => setForm((value) => ({ ...value, roles: event.target.value }))} />
              <Label>Status</Label>
              <Select value={form.status} onChange={(event) => setForm((value) => ({ ...value, status: event.target.value as UserStatus }))}>
                <option value="active">active</option>
                <option value="pending">pending</option>
                <option value="disabled">disabled</option>
              </Select>
              {!editing ? (
                <>
                  <Label>Initial Balance</Label>
                  <Input value={form.balance} onChange={(event) => setForm((value) => ({ ...value, balance: event.target.value }))} />
                  <Label>Currency</Label>
                  <Input value={form.currency} onChange={(event) => setForm((value) => ({ ...value, currency: event.target.value }))} />
                </>
              ) : null}
              <Label>RPM Limit</Label>
              <Input type="number" min="0" value={form.rpmLimit} onChange={(event) => setForm((value) => ({ ...value, rpmLimit: event.target.value }))} />
            </div>
            <MutationError error={mutations.create.error || mutations.update.error} />
            <DialogFooter>
              <Button type="button" variant="outline" onClick={closeUserDialog}>Cancel</Button>
              <Button type="submit" disabled={mutations.create.isPending || mutations.update.isPending}>
                <Save size={12} />
                Save
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      <Dialog open={balanceUser !== null} onOpenChange={(open) => !open && setBalanceUser(null)}>
        <DialogContent>
          <form className="space-y-4" onSubmit={adjustBalance}>
            <DialogHeader>
              <DialogTitle>Adjust Balance</DialogTitle>
              <DialogDescription>{balanceUser?.email}</DialogDescription>
            </DialogHeader>
            <Label>Operation</Label>
            <Select value={balanceForm.operation} onChange={(event) => setBalanceForm((value) => ({ ...value, operation: event.target.value }))}>
              <option value="increment">increment</option>
              <option value="decrement">decrement</option>
              <option value="set">set</option>
            </Select>
            <Label>Amount</Label>
            <Input required value={balanceForm.amount} onChange={(event) => setBalanceForm((value) => ({ ...value, amount: event.target.value }))} />
            <Label>Note</Label>
            <Textarea value={balanceForm.note} onChange={(event) => setBalanceForm((value) => ({ ...value, note: event.target.value }))} />
            <MutationError error={mutations.balance.error} />
            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => setBalanceUser(null)}>Cancel</Button>
              <Button type="submit" disabled={mutations.balance.isPending}>Apply</Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      <Dialog open={toggleUser !== null} onOpenChange={(open) => !open && setToggleUser(null)}>
        <DialogContent>
          <div className="space-y-4">
            <DialogHeader>
              <DialogTitle>{toggleActionLabel(toggleUserAction, "User")}</DialogTitle>
              <DialogDescription>
                This changes login and API access for {toggleUser?.user.email}. Type the user email to confirm.
              </DialogDescription>
            </DialogHeader>
            <div className="rounded-xl border border-srapi-border bg-srapi-card-muted p-3 text-xs">
              <div className="font-mono font-bold text-srapi-text-primary">{toggleUserIdentifier}</div>
              <div className="mt-1 text-srapi-text-secondary">Current status: {toggleUser?.user.status}</div>
            </div>
            <div className="space-y-2">
              <Label htmlFor="user-toggle-confirmation">Confirmation</Label>
              <Input
                id="user-toggle-confirmation"
                value={toggleUser?.confirmation ?? ""}
                onChange={(event) =>
                  setToggleUser((value) => value ? { ...value, confirmation: event.target.value } : value)
                }
                placeholder={toggleUserIdentifier}
              />
            </div>
            <MutationError error={mutations.toggle.error} />
            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => setToggleUser(null)}>Cancel</Button>
              <Button
                type="button"
                variant={toggleUserAction === "disable" ? "danger" : "primary"}
                disabled={!canToggleUser || mutations.toggle.isPending}
                onClick={confirmUserToggle}
              >
                {toggleActionLabel(toggleUserAction, "User")}
              </Button>
            </DialogFooter>
          </div>
        </DialogContent>
      </Dialog>
    </AdminShell>
  );
}

export function AdminAccountsProductionPage() {
  const [status, setStatus] = useState<ProviderAccountStatus | "all">("all");
  const [dialogOpen, setDialogOpen] = useState(false);
  const [editing, setEditing] = useState<ProviderAccount | null>(null);
  const [form, setForm] = useState<AdminAccountFormState>(() => emptyAccountForm());
  const [formError, setFormError] = useState<string | null>(null);
  const [toggleAccount, setToggleAccount] = useState<{ account: ProviderAccount; confirmation: string } | null>(null);
  const [discoveryConfirmation, setDiscoveryConfirmation] = useState<AccountModelDiscoveryConfirmationState | null>(null);
  const [testResult, setTestResult] = useState<AdminTestResult | null>(null);
  const [modelDiscovery, setModelDiscovery] = useState<AccountModelDiscovery | null>(null);
  const [runtimeAccount, setRuntimeAccount] = useState<ProviderAccount | null>(null);
  const [selectedAccountIds, setSelectedAccountIds] = useState<string[]>([]);
  const [importDialogOpen, setImportDialogOpen] = useState(false);
  const [importJson, setImportJson] = useState("{\n  \"accounts\": []\n}");
  const [importError, setImportError] = useState<string | null>(null);
  const [batchStatus, setBatchStatus] = useState<ProviderAccountStatus>("disabled");
  const [batchConfirmation, setBatchConfirmation] = useState<AccountBatchStatusConfirmationState | null>(null);
  const [exportData, setExportData] = useState<ProviderAccountExportItem[] | null>(null);
  const [importResult, setImportResult] = useState<ProviderAccountImportResult | null>(null);
  const [batchResult, setBatchResult] = useState<BatchUpdateAccountsResult | null>(null);
  const accounts = useAdminAccounts({ status });
  const providers = useAdminProviders();
  const groups = useAdminAccountGroups();
  const mutations = useAdminAccountMutations();
  const runtime = useAdminAccountRuntime(runtimeAccount?.id ?? null);
  const providerName = useMemo(() => {
    const map = new Map<string, string>();
    providers.data?.data.forEach((provider) => map.set(provider.id, provider.display_name));
    return map;
  }, [providers.data]);
  const groupName = useMemo(() => {
    const map = new Map<string, string>();
    groups.data?.data.forEach((group) => map.set(group.id, group.name));
    return map;
  }, [groups.data]);
  const isSaving =
    mutations.create.isPending ||
    mutations.update.isPending ||
    mutations.syncGroups.isPending;

  const openCreate = () => {
    setEditing(null);
    setFormError(null);
    setForm(emptyAccountForm(providers.data?.data[0]?.id ?? ""));
    setDialogOpen(true);
  };
  const openEdit = (account: ProviderAccount) => {
    setEditing(account);
    setFormError(null);
    setForm(accountFormFromAccount(account));
    setDialogOpen(true);
  };
  const saveAccount = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setFormError(null);

    try {
      if (editing) {
        const body = buildUpdateAccountBody(form);
        mutations.update.mutate(
          { id: editing.id, body },
          {
            onSuccess: (account) => {
              mutations.syncGroups.mutate(
                {
                  accountId: account.id,
                  currentGroupIds: editing.group_ids,
                  nextGroupIds: form.groupIds,
                },
                { onSuccess: () => setDialogOpen(false) },
              );
            },
          },
        );
        return;
      }

      const body = buildCreateAccountBody(form);
      mutations.create.mutate(body, {
        onSuccess: (account) => {
          mutations.syncGroups.mutate(
            { accountId: account.id, currentGroupIds: [], nextGroupIds: form.groupIds },
            { onSuccess: () => setDialogOpen(false) },
          );
        },
      });
    } catch (error) {
      setFormError(error instanceof Error ? error.message : "Invalid account form.");
    }
  };
  const toggleAccountAction = toggleAccount ? toggleActionFromStatus(toggleAccount.account.status) : "disable";
  const toggleAccountIdentifier = toggleAccount ? accountToggleIdentifier(toggleAccount.account) : "";
  const canToggleAccount = toggleAccount ? canConfirmToggle(toggleAccountIdentifier, toggleAccount.confirmation) : false;
  const confirmAccountToggle = () => {
    if (!toggleAccount || !canToggleAccount) {
      return;
    }
    mutations.toggle.mutate(toggleAccount.account, { onSuccess: () => setToggleAccount(null) });
  };
  const runAccountTest = (account: ProviderAccount) => {
    setTestResult(null);
    mutations.test.mutate(account, { onSuccess: setTestResult });
  };
  const discoverAccountModels = (account: ProviderAccount, persist: boolean) => {
    setModelDiscovery(null);
    mutations.discoverModels.mutate(
      { account, persist },
      {
        onSuccess: (discovery) => {
          setModelDiscovery(discovery);
          setDiscoveryConfirmation(null);
        },
      },
    );
  };
  const confirmPersistDiscovery = () => {
    if (!discoveryConfirmation || !canConfirmAccountModelDiscovery(discoveryConfirmation)) {
      return;
    }
    const account = accounts.data?.data.find((item) => item.id === discoveryConfirmation.accountId);
    if (!account) {
      return;
    }
    discoverAccountModels(account, true);
  };
  const exportAccounts = () => {
    setExportData(null);
    mutations.exportAccounts.mutate(undefined, { onSuccess: setExportData });
  };
  const importAccounts = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setImportError(null);
    let body: Parameters<typeof mutations.importAccounts.mutate>[0];
    try {
      body = buildImportAccountsBody(importJson);
    } catch (error) {
      setImportError(error instanceof Error ? error.message : "Import JSON is invalid.");
      return;
    }
    setImportResult(null);
    mutations.importAccounts.mutate(body, {
      onSuccess: (result) => {
        setImportResult(result);
        setImportDialogOpen(false);
      },
    });
  };
  const reviewBatchStatus = () => {
    try {
      buildBatchUpdateAccountsBody({ accountIds: selectedAccountIds, status: batchStatus });
    } catch (error) {
      setFormError(error instanceof Error ? error.message : "Batch update is invalid.");
      return;
    }
    setFormError(null);
    setBatchConfirmation(createAccountBatchStatusConfirmation({
      accountIds: selectedAccountIds,
      status: batchStatus,
    }));
  };
  const confirmBatchStatus = () => {
    if (!batchConfirmation || !canConfirmAccountBatchStatus(batchConfirmation)) {
      return;
    }
    const body = buildBatchUpdateAccountsBody({
      accountIds: batchConfirmation.accountIds,
      status: batchConfirmation.status,
    });
    setBatchResult(null);
    mutations.batchUpdate.mutate(body, {
      onSuccess: (result) => {
        setBatchResult(result);
        setSelectedAccountIds([]);
        setBatchConfirmation(null);
      },
    });
  };

  return (
    <AdminShell>
      <AdminPageHeader
        title="Account Pool"
        description="Provider accounts, write-only credentials, runtime classes, routing weights and group bindings from the production admin contract."
        actions={
          <>
            <Button type="button" variant="outline" size="sm" onClick={exportAccounts} disabled={mutations.exportAccounts.isPending}>
              Export
            </Button>
            <Button type="button" variant="outline" size="sm" onClick={() => setImportDialogOpen(true)}>
              Import
            </Button>
            <Button type="button" size="sm" onClick={openCreate} disabled={!providers.data?.data.length}>
              <Plus size={12} />
              New Account
            </Button>
          </>
        }
      />
      {!providers.isLoading && providers.data?.data.length === 0 ? (
        <AdminSection title="Provider Required">
          <AdminEmptyState title="No providers" description="Create a provider before adding provider accounts." />
        </AdminSection>
      ) : null}
      <AdminSection
        title="Accounts"
        actions={
          <>
            <Select value={batchStatus} onChange={(event) => setBatchStatus(event.target.value as ProviderAccountStatus)}>
              {ACCOUNT_STATUSES.map((nextStatus) => <option key={nextStatus} value={nextStatus}>{nextStatus}</option>)}
            </Select>
            <Button type="button" variant="outline" size="sm" disabled={!selectedAccountIds.length || mutations.batchUpdate.isPending} onClick={reviewBatchStatus}>
              Set Selected
            </Button>
            <Select value={status} onChange={(event) => setStatus(event.target.value as ProviderAccountStatus | "all")}>
              <option value="all">All status</option>
              <option value="active">Active</option>
              <option value="needs_reauth">Needs reauth</option>
              <option value="suspended">Suspended</option>
              <option value="dead">Dead</option>
              <option value="disabled">Disabled</option>
            </Select>
          </>
        }
      >
        {accounts.isLoading ? <AdminLoadingState label="Loading accounts..." /> : null}
        {accounts.isError ? <AdminErrorState error={accounts.error} onRetry={() => void accounts.refetch()} /> : null}
        {groups.isError ? <AdminErrorState error={groups.error} onRetry={() => void groups.refetch()} /> : null}
        {accounts.data ? (
          <>
            <AdminTable
              empty={<AdminEmptyState title="No provider accounts" />}
              columns={[
                { key: "select", header: "" },
                { key: "name", header: "Account" },
                { key: "provider", header: "Provider" },
                { key: "runtime", header: "Runtime" },
                { key: "weight", header: "Priority / Weight" },
                { key: "groups", header: "Groups" },
                { key: "meta", header: "Metadata" },
                { key: "status", header: "Status" },
                { key: "actions", header: "" },
              ]}
              rows={accounts.data.data.map((account) => ({
                select: (
                  <input
                    type="checkbox"
                    checked={selectedAccountIds.includes(account.id)}
                    onChange={(event) => setSelectedAccountIds((value) => toggleIdSelection(value, account.id, event.target.checked))}
                    aria-label={`Select ${account.name}`}
                  />
                ),
                name: account.name,
                provider: providerName.get(account.provider_id) ?? account.provider_id,
                runtime: (
                  <Button type="button" variant="ghost" size="sm" onClick={() => setRuntimeAccount(account)}>
                    {account.runtime_class}
                  </Button>
                ),
                weight: `${account.priority} / ${account.weight}`,
                groups: account.group_ids.length
                  ? account.group_ids.map((id) => groupName.get(id) ?? id).join(", ")
                  : "-",
                meta: <code className="max-w-[220px] truncate text-[10px]">{safeJson(account.metadata ?? {})}</code>,
                status: <AdminStatusBadge status={account.status} />,
                actions: (
                  <div className="flex flex-wrap justify-end gap-2">
                    <Button type="button" variant="outline" size="sm" disabled={mutations.test.isPending} onClick={() => runAccountTest(account)}>
                      Test
                    </Button>
                    <Button type="button" variant="outline" size="sm" disabled={mutations.discoverModels.isPending} onClick={() => discoverAccountModels(account, false)}>
                      Discover
                    </Button>
                    <Button type="button" variant="outline" size="sm" onClick={() => openEdit(account)}>
                      Edit
                    </Button>
                    <Button type="button" variant="outline" size="sm" disabled={mutations.clearError.isPending} onClick={() => mutations.clearError.mutate(account)}>
                      Clear Error
                    </Button>
                    <Button type="button" variant="outline" size="sm" disabled={mutations.recover.isPending} onClick={() => mutations.recover.mutate(account)}>
                      Recover
                    </Button>
                    <Button type="button" variant="outline" size="sm" disabled={mutations.discoverModels.isPending} onClick={() => setDiscoveryConfirmation(createAccountModelDiscoveryConfirmation(account))}>
                      Persist Models
                    </Button>
                    <Button
                      type="button"
                      variant={account.status === "disabled" ? "outline" : "danger"}
                      size="sm"
                      disabled={mutations.toggle.isPending}
                      onClick={() => setToggleAccount({ account, confirmation: "" })}
                    >
                      {account.status === "disabled" ? "Enable" : "Disable"}
                    </Button>
                  </div>
                ),
              }))}
              getRowKey={(_, index) => accounts.data?.data[index]?.id ?? String(index)}
            />
            <AdminPaginationSummary pagination={accounts.data.pagination} />
            <MutationError
              error={
                mutations.toggle.error ||
                mutations.create.error ||
                mutations.update.error ||
                mutations.syncGroups.error ||
                mutations.test.error ||
                mutations.discoverModels.error ||
                mutations.clearError.error ||
                mutations.recover.error ||
                mutations.exportAccounts.error ||
                mutations.importAccounts.error ||
                mutations.batchUpdate.error
              }
            />
            <AccountOperationResultPanel testResult={testResult} discovery={modelDiscovery} />
            <AccountBulkResultPanel exportData={exportData} importResult={importResult} batchResult={batchResult} />
          </>
        ) : null}
      </AdminSection>
      <Dialog open={importDialogOpen} onOpenChange={setImportDialogOpen}>
        <DialogContent className="max-h-[90vh] max-w-3xl overflow-y-auto">
          <form className="space-y-4" onSubmit={importAccounts}>
            <DialogHeader>
              <DialogTitle>Import Accounts</DialogTitle>
              <DialogDescription>
                Import provider account metadata plus optional write-only credentials. Export files intentionally do not contain credentials.
              </DialogDescription>
            </DialogHeader>
            <Textarea rows={16} value={importJson} onChange={(event) => setImportJson(event.target.value)} />
            {importError ? <div className="rounded-xl border border-srapi-error/30 bg-srapi-error/5 p-3 text-xs text-srapi-error">{importError}</div> : null}
            <MutationError error={mutations.importAccounts.error} />
            <DialogFooter>
              <Button type="button" variant="ghost" onClick={() => setImportDialogOpen(false)}>Cancel</Button>
              <Button type="submit" disabled={mutations.importAccounts.isPending}>Import Accounts</Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
      <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
        <DialogContent className="max-h-[90vh] max-w-3xl overflow-y-auto">
          <form className="space-y-4" onSubmit={saveAccount}>
            <DialogHeader>
              <DialogTitle>{editing ? "Edit Provider Account" : "Create Provider Account"}</DialogTitle>
              <DialogDescription>
                Credentials are write-only. Leave the credential JSON blank while editing to keep the encrypted value unchanged.
              </DialogDescription>
            </DialogHeader>
            <div className="grid grid-cols-1 gap-3 md:grid-cols-2">
              <div className="space-y-2">
                <Label htmlFor="account-provider">Provider</Label>
                <Select
                  id="account-provider"
                  required
                  disabled={Boolean(editing)}
                  value={form.providerId}
                  onChange={(event) => setForm((value) => ({ ...value, providerId: event.target.value }))}
                  className="w-full"
                >
                  <option value="">Select provider</option>
                  {providers.data?.data.map((provider) => (
                    <option key={provider.id} value={provider.id}>
                      {provider.display_name} ({provider.name})
                    </option>
                  ))}
                </Select>
              </div>
              <div className="space-y-2">
                <Label htmlFor="account-name">Name</Label>
                <Input
                  id="account-name"
                  required
                  value={form.name}
                  onChange={(event) => setForm((value) => ({ ...value, name: event.target.value }))}
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="account-runtime">Runtime Class</Label>
                <Select
                  id="account-runtime"
                  value={form.runtimeClass}
                  onChange={(event) => setForm((value) => ({ ...value, runtimeClass: event.target.value as AdminAccountFormState["runtimeClass"] }))}
                  className="w-full"
                >
                  {ACCOUNT_RUNTIME_CLASSES.map((runtimeClass) => (
                    <option key={runtimeClass} value={runtimeClass}>{runtimeClass}</option>
                  ))}
                </Select>
              </div>
              <div className="space-y-2">
                <Label htmlFor="account-upstream-client">Upstream Client</Label>
                <Input
                  id="account-upstream-client"
                  value={form.upstreamClient}
                  onChange={(event) => setForm((value) => ({ ...value, upstreamClient: event.target.value }))}
                  placeholder="claude_code_cli"
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="account-priority">Priority</Label>
                <Input
                  id="account-priority"
                  type="number"
                  value={form.priority}
                  onChange={(event) => setForm((value) => ({ ...value, priority: event.target.value }))}
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="account-weight">Weight</Label>
                <Input
                  id="account-weight"
                  type="number"
                  min="0"
                  step="0.01"
                  value={form.weight}
                  onChange={(event) => setForm((value) => ({ ...value, weight: event.target.value }))}
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="account-status">Status</Label>
                <Select
                  id="account-status"
                  value={form.status}
                  onChange={(event) => setForm((value) => ({ ...value, status: event.target.value as ProviderAccountStatus }))}
                  className="w-full"
                >
                  {ACCOUNT_STATUSES.map((nextStatus) => (
                    <option key={nextStatus} value={nextStatus}>{nextStatus}</option>
                  ))}
                </Select>
              </div>
              <div className="space-y-2">
                <Label htmlFor="account-proxy">Proxy ID</Label>
                <Input
                  id="account-proxy"
                  value={form.proxyId}
                  onChange={(event) => setForm((value) => ({ ...value, proxyId: event.target.value }))}
                  placeholder="optional proxy registry id"
                />
              </div>
            </div>
            <div className="space-y-2">
              <Label htmlFor="account-groups">Groups</Label>
              <div id="account-groups" className="grid grid-cols-1 gap-2 rounded-xl border border-srapi-border p-3 md:grid-cols-2">
                {groups.data?.data.length ? groups.data.data.map((group) => (
                  <label key={group.id} className="flex items-center justify-between gap-3 rounded-lg border border-srapi-border/70 px-3 py-2 text-xs">
                    <span>
                      <span className="block font-mono font-bold text-srapi-text-primary">{group.name}</span>
                      <span className="block text-[10px] text-srapi-text-secondary">{group.strategy_hint || group.id}</span>
                    </span>
                    <input
                      type="checkbox"
                      checked={form.groupIds.includes(group.id)}
                      onChange={(event) =>
                        setForm((value) => ({
                          ...value,
                          groupIds: toggleIdSelection(value.groupIds, group.id, event.target.checked),
                        }))
                      }
                    />
                  </label>
                )) : (
                  <span className="text-xs text-srapi-text-secondary">No account groups configured.</span>
                )}
              </div>
            </div>
            <div className="space-y-2">
              <Label htmlFor="account-credential">Credential JSON</Label>
              <Textarea
                id="account-credential"
                rows={6}
                required={!editing}
                value={form.credential}
                onChange={(event) => setForm((value) => ({ ...value, credential: event.target.value }))}
                placeholder={editing ? "Leave blank to keep encrypted credential" : '{ "api_key": "sk-..." }'}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="account-metadata">Metadata JSON</Label>
              <Textarea
                id="account-metadata"
                rows={6}
                value={form.metadata}
                onChange={(event) => setForm((value) => ({ ...value, metadata: event.target.value }))}
              />
            </div>
            {formError ? (
              <div className="rounded-xl border border-srapi-error/30 bg-srapi-error/5 p-3 text-xs text-srapi-error">
                {formError}
              </div>
            ) : null}
            <MutationError error={mutations.create.error || mutations.update.error || mutations.syncGroups.error} />
            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => setDialogOpen(false)}>Cancel</Button>
              <Button type="submit" disabled={isSaving}>
                <Save size={12} />
                Save Account
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
      <Dialog open={Boolean(runtimeAccount)} onOpenChange={(open) => !open && setRuntimeAccount(null)}>
        <DialogContent className="max-h-[90vh] max-w-5xl overflow-y-auto">
          <DialogHeader>
            <DialogTitle>Account Runtime Evidence</DialogTitle>
            <DialogDescription>
              Health, quota, RPM and proxy quality are loaded on demand from production account diagnostics endpoints.
            </DialogDescription>
          </DialogHeader>
          {runtimeAccount ? (
            <AccountRuntimeDetails
              account={runtimeAccount}
              health={runtime.health.data}
              quota={runtime.quota.data?.data}
              rpm={runtime.rpm.data}
              proxyQuality={runtime.proxyQuality.data}
              loading={
                runtime.health.isLoading ||
                runtime.quota.isLoading ||
                runtime.rpm.isLoading ||
                runtime.proxyQuality.isLoading
              }
              error={
                runtime.health.error ||
                runtime.quota.error ||
                runtime.rpm.error ||
                runtime.proxyQuality.error
              }
            />
          ) : null}
        </DialogContent>
      </Dialog>
      <Dialog open={toggleAccount !== null} onOpenChange={(open) => !open && setToggleAccount(null)}>
        <DialogContent>
          <div className="space-y-4">
            <DialogHeader>
              <DialogTitle>{toggleActionLabel(toggleAccountAction, "Account")}</DialogTitle>
              <DialogDescription>
                This changes routing eligibility for {toggleAccount?.account.name}. Type the account name to confirm.
              </DialogDescription>
            </DialogHeader>
            <div className="rounded-xl border border-srapi-border bg-srapi-card-muted p-3 text-xs">
              <div className="font-mono font-bold text-srapi-text-primary">{toggleAccountIdentifier}</div>
              <div className="mt-1 text-srapi-text-secondary">Current status: {toggleAccount?.account.status}</div>
            </div>
            <div className="space-y-2">
              <Label htmlFor="account-toggle-confirmation">Confirmation</Label>
              <Input
                id="account-toggle-confirmation"
                value={toggleAccount?.confirmation ?? ""}
                onChange={(event) =>
                  setToggleAccount((value) => value ? { ...value, confirmation: event.target.value } : value)
                }
                placeholder={toggleAccountIdentifier}
              />
            </div>
            <MutationError error={mutations.toggle.error} />
            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => setToggleAccount(null)}>Cancel</Button>
              <Button
                type="button"
                variant={toggleAccountAction === "disable" ? "danger" : "primary"}
                disabled={!canToggleAccount || mutations.toggle.isPending}
                onClick={confirmAccountToggle}
              >
                {toggleActionLabel(toggleAccountAction, "Account")}
              </Button>
            </DialogFooter>
          </div>
        </DialogContent>
      </Dialog>
      <Dialog open={Boolean(discoveryConfirmation)} onOpenChange={(open) => !open && setDiscoveryConfirmation(null)}>
        <DialogContent>
          <div className="space-y-4">
            <DialogHeader>
              <DialogTitle>Persist Discovered Models</DialogTitle>
              <DialogDescription>
                This writes discovered upstream model IDs to account metadata as supported_models. Preview discovery first when changing production routing.
              </DialogDescription>
            </DialogHeader>
            <div className="rounded-xl border border-srapi-border bg-srapi-card-muted p-3 text-xs">
              <div className="font-mono font-bold text-srapi-text-primary">{discoveryConfirmation?.phrase}</div>
              <div className="mt-1 text-srapi-text-secondary">Account: {discoveryConfirmation?.accountName}</div>
            </div>
            <div className="space-y-2">
              <Label htmlFor="account-discovery-confirmation">Confirmation</Label>
              <Input
                id="account-discovery-confirmation"
                value={discoveryConfirmation?.confirmation ?? ""}
                onChange={(event) =>
                  setDiscoveryConfirmation((value) => value ? { ...value, confirmation: event.target.value } : value)
                }
                placeholder={discoveryConfirmation?.phrase}
              />
            </div>
            <MutationError error={mutations.discoverModels.error} />
            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => setDiscoveryConfirmation(null)}>Cancel</Button>
              <Button
                type="button"
                variant="danger"
                disabled={!canConfirmAccountModelDiscovery(discoveryConfirmation) || mutations.discoverModels.isPending}
                onClick={confirmPersistDiscovery}
              >
                Persist Models
              </Button>
            </DialogFooter>
          </div>
        </DialogContent>
      </Dialog>
      <Dialog open={Boolean(batchConfirmation)} onOpenChange={(open) => !open && setBatchConfirmation(null)}>
        <DialogContent>
          <div className="space-y-4">
            <DialogHeader>
              <DialogTitle>Batch Update Account Status</DialogTitle>
              <DialogDescription>
                This changes routing eligibility for selected accounts. Type the confirmation phrase before applying the batch update.
              </DialogDescription>
            </DialogHeader>
            <div className="rounded-xl border border-srapi-border bg-srapi-card-muted p-3 text-xs">
              <div className="font-mono font-bold text-srapi-text-primary">{batchConfirmation?.phrase}</div>
              <div className="mt-1 text-srapi-text-secondary">
                {formatInteger(batchConfirmation?.accountIds.length)} accounts {"->"} {batchConfirmation?.status}
              </div>
            </div>
            <div className="space-y-2">
              <Label htmlFor="account-batch-confirmation">Confirmation</Label>
              <Input
                id="account-batch-confirmation"
                value={batchConfirmation?.confirmation ?? ""}
                onChange={(event) => setBatchConfirmation((value) => value ? { ...value, confirmation: event.target.value } : value)}
                placeholder={batchConfirmation?.phrase}
              />
            </div>
            <MutationError error={mutations.batchUpdate.error} />
            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => setBatchConfirmation(null)}>Cancel</Button>
              <Button
                type="button"
                variant="danger"
                disabled={!canConfirmAccountBatchStatus(batchConfirmation) || mutations.batchUpdate.isPending}
                onClick={confirmBatchStatus}
              >
                Apply Batch
              </Button>
            </DialogFooter>
          </div>
        </DialogContent>
      </Dialog>
    </AdminShell>
  );
}

export function AdminGroupsProductionPage() {
  const groups = useAdminAccountGroups();
  const providers = useAdminProviders();
  const models = useAdminModels();
  const mutations = useAdminAccountGroupMutations();
  const [open, setOpen] = useState(false);
  const [editing, setEditing] = useState<AccountGroup | null>(null);
  const [form, setForm] = useState<AccountGroupFormState>(() => emptyAccountGroupForm());
  const [formError, setFormError] = useState<string | null>(null);

  const openCreate = () => {
    setEditing(null);
    setForm(emptyAccountGroupForm());
    setFormError(null);
    setOpen(true);
  };
  const openEdit = (group: AccountGroup) => {
    setEditing(group);
    setForm(accountGroupFormFromGroup(group));
    setFormError(null);
    setOpen(true);
  };
  const save = (event: FormEvent) => {
    event.preventDefault();
    setFormError(null);
    try {
      if (editing) {
        const body = buildUpdateAccountGroupBody(form);
        mutations.update.mutate({ id: editing.id, body }, { onSuccess: () => setOpen(false) });
        return;
      }
      const body = buildCreateAccountGroupBody(form);
      mutations.create.mutate(body, { onSuccess: () => setOpen(false) });
    } catch (error) {
      setFormError(error instanceof Error ? error.message : "Account group is invalid.");
    }
  };

  return (
    <AdminShell>
      <AdminPageHeader
        title="Group Management"
        description="Account groups define provider/model scope and scheduler strategy hints from production catalog data."
        actions={<Button type="button" size="sm" onClick={openCreate}><Plus size={12} />Create Group</Button>}
      />
      <AdminSection title="Groups">
        {groups.isLoading ? <AdminLoadingState label="Loading groups..." /> : null}
        {groups.isError ? <AdminErrorState error={groups.error} onRetry={() => void groups.refetch()} /> : null}
        {providers.isError ? <AdminErrorState error={providers.error} onRetry={() => void providers.refetch()} /> : null}
        {models.isError ? <AdminErrorState error={models.error} onRetry={() => void models.refetch()} /> : null}
        {groups.data ? (
          <AdminTable
            empty={<AdminEmptyState title="No account groups" />}
            columns={[
              { key: "name", header: "Name" },
              { key: "strategy", header: "Strategy" },
              { key: "provider", header: "Provider Scope" },
              { key: "model", header: "Model Scope" },
              { key: "status", header: "Status" },
              { key: "actions", header: "" },
            ]}
            rows={groups.data.data.map((group) => ({
              name: (
                <div>
                  <div className="font-semibold text-srapi-text-primary">{group.name}</div>
                  <div className="max-w-[260px] truncate text-[10px] text-srapi-text-secondary">{group.description}</div>
                </div>
              ),
              strategy: group.strategy_hint || "-",
              provider: providerScopeLabel(group.provider_scope, providers.data?.data ?? []),
              model: modelScopeLabel(group.model_scope, models.data?.data ?? []),
              status: <AdminStatusBadge status={group.status} />,
              actions: <Button type="button" variant="outline" size="sm" onClick={() => openEdit(group)}>Edit</Button>,
            }))}
            getRowKey={(_, index) => groups.data?.data[index]?.id ?? String(index)}
          />
        ) : null}
      </AdminSection>

      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent className="max-w-2xl">
          <form className="space-y-4" onSubmit={save}>
            <DialogHeader>
              <DialogTitle>{editing ? "Edit Group" : "Create Group"}</DialogTitle>
              <DialogDescription>Use catalog selectors for common scopes, or adjust JSON for advanced rules.</DialogDescription>
            </DialogHeader>
            <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
              <div>
                <Label htmlFor="group-name">Name</Label>
                <Input id="group-name" required value={form.name} onChange={(event) => setForm((value) => ({ ...value, name: event.target.value }))} />
              </div>
              <div>
                <Label htmlFor="group-status">Status</Label>
                <Select id="group-status" value={form.status} onChange={(event) => setForm((value) => ({ ...value, status: event.target.value as AccountGroupFormState["status"] }))}>
                  {ACCOUNT_GROUP_STATUSES.map((status) => <option key={status} value={status}>{status}</option>)}
                </Select>
              </div>
              <div className="md:col-span-2">
                <Label htmlFor="group-description">Description</Label>
                <Input id="group-description" value={form.description} onChange={(event) => setForm((value) => ({ ...value, description: event.target.value }))} />
              </div>
              <div>
                <Label htmlFor="group-strategy">Strategy Hint</Label>
                <Select id="group-strategy" value={form.strategyHint} onChange={(event) => setForm((value) => ({ ...value, strategyHint: event.target.value }))}>
                  {GROUP_STRATEGY_HINTS.map((strategy) => <option key={strategy} value={strategy}>{strategy}</option>)}
                </Select>
              </div>
              <div>
                <Label htmlFor="group-provider-selector">Provider Scope</Label>
                <Select id="group-provider-selector" value={form.selectedProviderId} onChange={(event) => setForm((value) => applyProviderScopeSelection(value, event.target.value))}>
                  <option value="">All providers</option>
                  {providers.data?.data.map((provider) => <option key={provider.id} value={provider.id}>{provider.display_name}</option>)}
                </Select>
              </div>
              <div>
                <Label htmlFor="group-model-selector">Model Scope</Label>
                <Select id="group-model-selector" value={form.selectedModelName} onChange={(event) => setForm((value) => applyModelScopeSelection(value, event.target.value))}>
                  <option value="">All models</option>
                  {models.data?.data.map((model) => <option key={model.id} value={model.canonical_name}>{model.canonical_name}</option>)}
                </Select>
              </div>
            </div>
            <div>
              <Label htmlFor="group-provider-json">Provider Scope JSON</Label>
              <Textarea id="group-provider-json" rows={5} value={form.providerScopeJson} onChange={(event) => setForm((value) => ({ ...value, providerScopeJson: event.target.value }))} />
            </div>
            <div>
              <Label htmlFor="group-model-json">Model Scope JSON</Label>
              <Textarea id="group-model-json" rows={5} value={form.modelScopeJson} onChange={(event) => setForm((value) => ({ ...value, modelScopeJson: event.target.value }))} />
            </div>
            {formError ? <div className="rounded-xl border border-srapi-error/30 bg-srapi-error/5 p-3 text-xs text-srapi-error">{formError}</div> : null}
            <MutationError error={mutations.create.error || mutations.update.error} />
            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => setOpen(false)}>Cancel</Button>
              <Button type="submit" disabled={mutations.create.isPending || mutations.update.isPending}>Save</Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </AdminShell>
  );
}

function SimpleResourceTable<T>({
  title,
  description,
  loading,
  error,
  onRetry,
  rows,
  columns,
  getRowKey,
  pagination,
}: {
  title: string;
  description?: string;
  loading: boolean;
  error: unknown;
  onRetry: () => void;
  rows: T[] | undefined;
  columns: Array<{ key: string; header: ReactNode; render: (row: T) => ReactNode }>;
  getRowKey: (row: T) => string;
  pagination?: { page: number; page_size: number; total: number; has_next: boolean };
}) {
  return (
    <AdminSection title={title} description={description}>
      {loading ? <AdminLoadingState label={`Loading ${title.toLowerCase()}...`} /> : null}
      {error ? <AdminErrorState error={error} onRetry={onRetry} /> : null}
      {rows ? (
        <>
          <AdminTable
            empty={<AdminEmptyState title={`No ${title.toLowerCase()} records`} />}
            columns={columns.map(({ key, header }) => ({ key, header }))}
            rows={rows.map((row) =>
              Object.fromEntries(columns.map((column) => [column.key, column.render(row)])),
            )}
            getRowKey={(_, index) => getRowKey(rows[index])}
          />
          <AdminPaginationSummary pagination={pagination} />
        </>
      ) : null}
    </AdminSection>
  );
}

function SubscriptionPlanFormFields({
  form,
  onChange,
}: {
  form: SubscriptionPlanFormState;
  onChange: (form: SubscriptionPlanFormState) => void;
}) {
  const patch = (value: Partial<SubscriptionPlanFormState>) => onChange({ ...form, ...value });
  return (
    <>
      <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
        <div>
          <Label htmlFor="plan-name">Name</Label>
          <Input id="plan-name" value={form.name} onChange={(event) => patch({ name: event.target.value })} />
        </div>
        <div>
          <Label htmlFor="plan-status">Status</Label>
          <Select id="plan-status" value={form.status} onChange={(event) => patch({ status: event.target.value as SubscriptionPlanFormState["status"] })}>
            {SUBSCRIPTION_PLAN_STATUSES.map((status) => <option key={status} value={status}>{status}</option>)}
          </Select>
        </div>
        <div>
          <Label htmlFor="plan-price">Price</Label>
          <Input id="plan-price" inputMode="decimal" value={form.price} onChange={(event) => patch({ price: event.target.value })} />
        </div>
        <div>
          <Label htmlFor="plan-currency">Currency</Label>
          <Input id="plan-currency" value={form.currency} onChange={(event) => patch({ currency: event.target.value })} />
        </div>
        <div>
          <Label htmlFor="plan-validity">Validity Days</Label>
          <Input id="plan-validity" type="number" min="1" value={form.validityDays} onChange={(event) => patch({ validityDays: event.target.value })} />
        </div>
        <div>
          <Label htmlFor="plan-sort">Sort Order</Label>
          <Input id="plan-sort" type="number" value={form.sortOrder} onChange={(event) => patch({ sortOrder: event.target.value })} />
        </div>
        <SettingsToggle label="For Sale" checked={form.forSale} onChange={(checked) => patch({ forSale: checked })} />
        <div>
          <Label htmlFor="plan-description">Description</Label>
          <Input id="plan-description" value={form.description} onChange={(event) => patch({ description: event.target.value })} />
        </div>
      </div>
      <div>
        <Label htmlFor="plan-entitlements">Entitlements JSON</Label>
        <Textarea id="plan-entitlements" rows={8} value={form.entitlementsJson} onChange={(event) => patch({ entitlementsJson: event.target.value })} />
      </div>
    </>
  );
}

function PaymentProviderFormFields({
  form,
  onChange,
}: {
  form: PaymentProviderFormState;
  onChange: (form: PaymentProviderFormState) => void;
}) {
  const patch = (value: Partial<PaymentProviderFormState>) => onChange({ ...form, ...value });
  return (
    <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
      <div>
        <Label htmlFor="payment-provider-code">Provider Code</Label>
        <Input
          id="payment-provider-code"
          placeholder="stripe"
          value={form.provider}
          onChange={(event) => patch({ provider: event.target.value })}
        />
      </div>
      <div>
        <Label htmlFor="payment-provider-name">Display Name</Label>
        <Input
          id="payment-provider-name"
          placeholder="Stripe production"
          value={form.name}
          onChange={(event) => patch({ name: event.target.value })}
        />
      </div>
      <div>
        <Label htmlFor="payment-provider-status">Status</Label>
        <Select id="payment-provider-status" value={form.status} onChange={(event) => patch({ status: event.target.value as PaymentProviderFormState["status"] })}>
          {PAYMENT_PROVIDER_STATUSES.map((status) => <option key={status} value={status}>{status}</option>)}
        </Select>
      </div>
      <div>
        <Label htmlFor="payment-provider-sort">Sort Order</Label>
        <Input id="payment-provider-sort" type="number" value={form.sortOrder} onChange={(event) => patch({ sortOrder: event.target.value })} />
      </div>
      <div className="md:col-span-2">
        <Label htmlFor="payment-provider-methods">Supported Methods</Label>
        <Textarea
          id="payment-provider-methods"
          rows={3}
          value={form.supportedMethodsText}
          onChange={(event) => patch({ supportedMethodsText: event.target.value })}
        />
      </div>
      <div className="md:col-span-2">
        <Label htmlFor="payment-provider-config">Config JSON</Label>
        <Textarea id="payment-provider-config" rows={8} value={form.configJson} onChange={(event) => patch({ configJson: event.target.value })} />
      </div>
      <div>
        <Label htmlFor="payment-provider-limits">Limits JSON</Label>
        <Textarea id="payment-provider-limits" rows={5} value={form.limitsJson} onChange={(event) => patch({ limitsJson: event.target.value })} />
      </div>
      <div>
        <Label htmlFor="payment-provider-metadata">Metadata JSON</Label>
        <Textarea id="payment-provider-metadata" rows={5} value={form.metadataJson} onChange={(event) => patch({ metadataJson: event.target.value })} />
      </div>
    </div>
  );
}

function ProviderRegistryFormFields({
  form,
  onChange,
  editing,
}: {
  form: ProviderFormState;
  onChange: (form: ProviderFormState) => void;
  editing: boolean;
}) {
  const patch = (value: Partial<ProviderFormState>) => onChange({ ...form, ...value });
  return (
    <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
      <div>
        <Label htmlFor="provider-name">Provider Name</Label>
        <Input
          id="provider-name"
          disabled={editing}
          placeholder="openai_main"
          value={form.name}
          onChange={(event) => patch({ name: event.target.value })}
        />
      </div>
      <div>
        <Label htmlFor="provider-display-name">Display Name</Label>
        <Input
          id="provider-display-name"
          value={form.displayName}
          onChange={(event) => patch({ displayName: event.target.value })}
        />
      </div>
      <div>
        <Label htmlFor="provider-adapter">Adapter</Label>
        <Select id="provider-adapter" value={form.adapterType} onChange={(event) => patch({ adapterType: event.target.value as ProviderFormState["adapterType"] })}>
          {PROVIDER_ADAPTER_TYPES.map((adapter) => <option key={adapter} value={adapter}>{adapter}</option>)}
        </Select>
      </div>
      <div>
        <Label htmlFor="provider-protocol">Protocol</Label>
        <Select id="provider-protocol" value={form.protocol} onChange={(event) => patch({ protocol: event.target.value as ProviderFormState["protocol"] })}>
          {PROVIDER_PROTOCOLS.map((protocol) => <option key={protocol} value={protocol}>{protocol}</option>)}
        </Select>
      </div>
      <div>
        <Label htmlFor="provider-status">Status</Label>
        <Select id="provider-status" value={form.status} onChange={(event) => patch({ status: event.target.value as ProviderFormState["status"] })}>
          {RESOURCE_STATUSES.map((status) => <option key={status} value={status}>{status}</option>)}
        </Select>
      </div>
      <div className="md:col-span-2">
        <Label htmlFor="provider-capabilities">Capabilities JSON</Label>
        <Textarea
          id="provider-capabilities"
          rows={6}
          value={form.capabilitiesJson}
          onChange={(event) => patch({ capabilitiesJson: event.target.value })}
        />
      </div>
      <div className="md:col-span-2">
        <Label htmlFor="provider-config-schema">Config Schema JSON</Label>
        <Textarea
          id="provider-config-schema"
          rows={8}
          value={form.configSchemaJson}
          onChange={(event) => patch({ configSchemaJson: event.target.value })}
        />
      </div>
    </div>
  );
}

function ProviderTestResultPanel({ result }: { result: AdminTestResult | null }) {
  if (!result) {
    return null;
  }
  return (
    <div className="rounded-2xl border border-srapi-border bg-srapi-card-muted/30 p-4 text-xs">
      <div className="mb-3 flex flex-wrap items-center justify-between gap-2">
        <div className="font-mono font-bold uppercase tracking-wider text-srapi-text-secondary">
          Provider Test Result
        </div>
        <AdminStatusBadge status={result.status} />
      </div>
      <div className="grid grid-cols-1 gap-3 md:grid-cols-3">
        <div>
          <div className="font-mono font-bold uppercase text-srapi-text-secondary">Checked</div>
          <div className="mt-1 text-srapi-text-primary">{formatDateTime(result.checked_at)}</div>
        </div>
        <div>
          <div className="font-mono font-bold uppercase text-srapi-text-secondary">Latency</div>
          <div className="mt-1 text-srapi-text-primary">{result.latency_ms ? `${result.latency_ms} ms` : "n/a"}</div>
        </div>
        <div>
          <div className="font-mono font-bold uppercase text-srapi-text-secondary">Message</div>
          <div className="mt-1 text-srapi-text-primary">{result.message || "No message"}</div>
        </div>
      </div>
      {result.checks ? (
        <pre className="mt-4 max-h-40 overflow-auto rounded-xl bg-srapi-bg p-3 font-mono text-[11px] text-srapi-text-secondary">
          {safeJson(result.checks)}
        </pre>
      ) : null}
    </div>
  );
}

function AccountOperationResultPanel({
  testResult,
  discovery,
}: {
  testResult: AdminTestResult | null;
  discovery: AccountModelDiscovery | null;
}) {
  if (!testResult && !discovery) {
    return null;
  }
  return (
    <div className="grid grid-cols-1 gap-4 xl:grid-cols-2">
      {testResult ? (
        <div className="rounded-2xl border border-srapi-border bg-srapi-card-muted/30 p-4 text-xs">
          <div className="mb-3 flex flex-wrap items-center justify-between gap-2">
            <div className="font-mono font-bold uppercase tracking-wider text-srapi-text-secondary">
              Account Test
            </div>
            <AdminStatusBadge status={testResult.status} />
          </div>
          <div className="grid grid-cols-1 gap-3 md:grid-cols-3">
            <div>
              <div className="font-mono font-bold uppercase text-srapi-text-secondary">Checked</div>
              <div className="mt-1 text-srapi-text-primary">{formatDateTime(testResult.checked_at)}</div>
            </div>
            <div>
              <div className="font-mono font-bold uppercase text-srapi-text-secondary">Latency</div>
              <div className="mt-1 text-srapi-text-primary">{testResult.latency_ms ? `${testResult.latency_ms} ms` : "n/a"}</div>
            </div>
            <div>
              <div className="font-mono font-bold uppercase text-srapi-text-secondary">Message</div>
              <div className="mt-1 text-srapi-text-primary">{testResult.message || "No message"}</div>
            </div>
          </div>
          {testResult.checks ? (
            <pre className="mt-4 max-h-40 overflow-auto rounded-xl bg-srapi-bg p-3 font-mono text-[11px] text-srapi-text-secondary">
              {safeJson(testResult.checks)}
            </pre>
          ) : null}
        </div>
      ) : null}
      {discovery ? (
        <div className="rounded-2xl border border-srapi-border bg-srapi-card-muted/30 p-4 text-xs">
          <div className="mb-3 flex flex-wrap items-center justify-between gap-2">
            <div className="font-mono font-bold uppercase tracking-wider text-srapi-text-secondary">
              Model Discovery
            </div>
            <Badge variant={discovery.persisted ? "success" : "neutral"}>
              {discovery.persisted ? "persisted" : "preview"}
            </Badge>
          </div>
          <div className="grid grid-cols-1 gap-3 md:grid-cols-2">
            <div>
              <div className="font-mono font-bold uppercase text-srapi-text-secondary">Source</div>
              <div className="mt-1 text-srapi-text-primary">{discovery.source}</div>
            </div>
            <div>
              <div className="font-mono font-bold uppercase text-srapi-text-secondary">Checked</div>
              <div className="mt-1 text-srapi-text-primary">{formatDateTime(discovery.checked_at)}</div>
            </div>
          </div>
          <div className="mt-3">
            <div className="font-mono font-bold uppercase text-srapi-text-secondary">Endpoint</div>
            <div className="mt-1 break-all font-mono text-[11px] text-srapi-text-primary">{discovery.endpoint}</div>
          </div>
          <div className="mt-3 max-h-40 overflow-auto rounded-xl bg-srapi-bg p-3 font-mono text-[11px] text-srapi-text-secondary">
            {discovery.model_ids.length ? discovery.model_ids.join("\n") : "No models discovered."}
          </div>
        </div>
      ) : null}
    </div>
  );
}

function OpsEvidenceDrawer({
  selection,
  requests,
  onClose,
}: {
  selection: OpsEvidenceSelection | null;
  requests: UsageLog[];
  onClose: () => void;
}) {
  const filteredRequests = selection?.kind === "error"
    ? requests.filter((request) => request.error_class === selection.errorClass)
    : selection?.kind === "request"
      ? [selection.request]
      : requests.filter((request) => !request.success).slice(0, 8);

  return (
    <Dialog open={Boolean(selection)} onOpenChange={(open) => !open && onClose()}>
      <DialogContent className="max-h-[90vh] max-w-5xl overflow-y-auto">
        <DialogHeader>
          <DialogTitle>{selection?.title ?? "Operations Evidence"}</DialogTitle>
          <DialogDescription>
            Request, account and metadata evidence is read from admin logs and usage records. Secrets and request bodies are not displayed.
          </DialogDescription>
        </DialogHeader>
        {selection?.kind === "alert" || selection?.kind === "log" ? (
          <AdminSection title="Event Details">
            <pre className="max-h-64 overflow-auto rounded-xl bg-srapi-bg p-3 font-mono text-[11px] text-srapi-text-secondary">
              {safeJson(selection.details)}
            </pre>
          </AdminSection>
        ) : null}
        {selection?.kind === "error" ? (
          <div className="grid grid-cols-1 gap-3 md:grid-cols-2">
            <RuntimeMetric label="Error Class" value={selection.errorClass} />
            <RuntimeMetric label="Owner" value={selection.owner} />
          </div>
        ) : null}
        <AdminSection
          title="Recent Request Evidence"
          description="Recent usage records show request id, model, provider/account routing, latency, tokens and error class."
        >
          <AdminTable
            empty={<AdminEmptyState title="No matching request evidence" />}
            columns={[
              { key: "request", header: "Request" },
              { key: "model", header: "Model" },
              { key: "route", header: "Route" },
              { key: "latency", header: "Latency" },
              { key: "tokens", header: "Tokens" },
              { key: "status", header: "Status" },
            ]}
            rows={filteredRequests.map((request) => ({
              request: (
                <span className="select-all font-mono text-[11px]">{request.request_id}</span>
              ),
              model: request.model,
              route: (
                <div className="font-mono text-[11px] text-srapi-text-secondary">
                  <div>provider {request.provider_id || "-"}</div>
                  <div>account {request.account_id || "-"}</div>
                </div>
              ),
              latency: `${formatInteger(request.latency_ms)}ms`,
              tokens: formatInteger(request.total_tokens),
              status: request.success
                ? <AdminStatusBadge status="success" />
                : <AdminStatusBadge status={request.error_class || "failed"} />,
            }))}
            getRowKey={(_, index) => filteredRequests[index]?.id ?? String(index)}
          />
        </AdminSection>
      </DialogContent>
    </Dialog>
  );
}

function RuntimeMetric({
  label,
  value,
  detail,
}: {
  label: string;
  value: ReactNode;
  detail?: ReactNode;
}) {
  return (
    <div className="rounded-xl border border-srapi-border bg-srapi-card-muted/20 p-4">
      <div className="font-mono text-[10px] font-bold uppercase tracking-wider text-srapi-text-secondary">
        {label}
      </div>
      <div className="mt-2 font-serif text-xl font-semibold text-srapi-text-primary">{value}</div>
      {detail ? <div className="mt-1 text-[11px] text-srapi-text-secondary">{detail}</div> : null}
    </div>
  );
}

function AccountRuntimeDetails({
  account,
  health,
  quota,
  rpm,
  proxyQuality,
  loading,
  error,
}: {
  account: ProviderAccount;
  health?: AccountHealthSnapshot;
  quota?: AccountQuotaSnapshot[];
  rpm?: AccountRpmStatus;
  proxyQuality?: AccountProxyQuality;
  loading: boolean;
  error: unknown;
}) {
  return (
    <div className="space-y-4">
      <div className="rounded-2xl border border-srapi-border bg-srapi-card-muted/30 p-4">
        <div className="font-serif text-lg font-medium text-srapi-text-primary">{account.name}</div>
        <div className="mt-1 font-mono text-[11px] text-srapi-text-secondary">
          {account.runtime_class} / provider {account.provider_id}
        </div>
      </div>
      {loading ? <AdminLoadingState label="Loading runtime evidence..." /> : null}
      {error ? <AdminErrorState error={error} /> : null}
      {health ? (
        <>
          <div className="grid grid-cols-1 gap-3 md:grid-cols-3">
            <RuntimeMetric
              label="Success Rate"
              value={formatPercent(health.success_rate)}
              detail={`Error ${formatPercent(health.error_rate)}`}
            />
            <RuntimeMetric
              label="Latency"
              value={`${formatInteger(health.latency_p95_ms)}ms`}
              detail={`P50 ${formatInteger(health.latency_p50_ms)}ms`}
            />
            <RuntimeMetric
              label="Quota Remaining"
              value={formatPercent(health.quota_remaining_ratio)}
              detail={health.quota_exhausted ? "Quota exhausted" : "Quota available"}
            />
            <RuntimeMetric
              label="Rate Limits"
              value={formatInteger(health.rate_limit_count)}
              detail={`${formatInteger(health.timeout_count)} timeouts`}
            />
            <RuntimeMetric
              label="Circuit"
              value={<AdminStatusBadge status={health.circuit_state} />}
              detail={health.cooldown_until ? `Cooldown until ${formatDateTime(health.cooldown_until)}` : "No cooldown"}
            />
            <RuntimeMetric
              label="Snapshot"
              value={formatDateTime(health.snapshot_at)}
              detail={health.error_class || health.cooldown_reason || "No active error class"}
            />
          </div>
        </>
      ) : null}
      {rpm ? (
        <AdminSection title="RPM Window" description="Runtime request window for this provider account.">
          <div className="grid grid-cols-1 gap-3 md:grid-cols-3">
            <RuntimeMetric label="Used" value={formatInteger(rpm.rpm_used)} />
            <RuntimeMetric label="Limit" value={rpm.rpm_limit === null ? "unlimited" : formatInteger(rpm.rpm_limit)} />
            <RuntimeMetric
              label="Window"
              value={`${formatInteger(rpm.window_seconds)}s`}
              detail={rpm.reset_at ? `Reset ${formatDateTime(rpm.reset_at)}` : "No reset time"}
            />
          </div>
        </AdminSection>
      ) : null}
      <AdminSection title="Quota" description="Provider-reported quota snapshots. Values remain decimal strings from the API.">
        {quota ? (
          <AdminTable
            empty={<AdminEmptyState title="No quota snapshots" />}
            columns={[
              { key: "type", header: "Type" },
              { key: "remaining", header: "Remaining" },
              { key: "used", header: "Used" },
              { key: "limit", header: "Limit" },
              { key: "ratio", header: "Remaining" },
              { key: "reset", header: "Reset" },
            ]}
            rows={quota.map((item) => ({
              type: item.quota_type,
              remaining: item.remaining,
              used: item.used,
              limit: item.quota_limit,
              ratio: formatPercent(item.remaining_ratio),
              reset: item.reset_at ? formatDateTime(item.reset_at) : "-",
            }))}
            getRowKey={(_, index) => `${quota[index]?.quota_type ?? "quota"}-${index}`}
          />
        ) : null}
      </AdminSection>
      {proxyQuality ? (
        <AdminSection title="Proxy Quality" description="Proxy evidence for the selected account.">
          <div className="grid grid-cols-1 gap-3 md:grid-cols-4">
            <RuntimeMetric label="Proxy" value={proxyQuality.proxy_id || "none"} />
            <RuntimeMetric label="Success" value={formatPercent(proxyQuality.success_rate)} />
            <RuntimeMetric label="Error" value={formatPercent(proxyQuality.error_rate)} />
            <RuntimeMetric
              label="Latency P95"
              value={`${formatInteger(proxyQuality.latency_p95_ms)}ms`}
              detail={`${formatInteger(proxyQuality.sample_count)} samples`}
            />
          </div>
        </AdminSection>
      ) : null}
    </div>
  );
}

function AccountBulkResultPanel({
  exportData,
  importResult,
  batchResult,
}: {
  exportData: ProviderAccountExportItem[] | null;
  importResult: ProviderAccountImportResult | null;
  batchResult: BatchUpdateAccountsResult | null;
}) {
  if (!exportData && !importResult && !batchResult) {
    return null;
  }
  return (
    <AdminSection title="Bulk Operation Result" description="Results are returned by the admin account contract; exports never include credentials.">
      <div className="grid grid-cols-1 gap-4 xl:grid-cols-3">
        {exportData ? (
          <div className="rounded-xl border border-srapi-border bg-srapi-card-muted/20 p-4">
            <div className="font-mono text-[10px] font-bold uppercase tracking-wider text-srapi-text-secondary">Exported Accounts</div>
            <div className="mt-2 font-serif text-2xl font-semibold text-srapi-text-primary">{formatInteger(exportData.length)}</div>
            <pre className="mt-3 max-h-48 overflow-auto rounded-xl bg-srapi-bg p-3 font-mono text-[11px] text-srapi-text-secondary">
              {JSON.stringify({ accounts: exportData }, null, 2)}
            </pre>
          </div>
        ) : null}
        {importResult ? (
          <div className="rounded-xl border border-srapi-border bg-srapi-card-muted/20 p-4">
            <div className="font-mono text-[10px] font-bold uppercase tracking-wider text-srapi-text-secondary">Import</div>
            <div className="mt-2 text-sm text-srapi-text-primary">
              Created {formatInteger(importResult.created_count)} / skipped {formatInteger(importResult.skipped_count)}
            </div>
            {importResult.errors.length ? (
              <pre className="mt-3 max-h-40 overflow-auto rounded-xl bg-srapi-bg p-3 font-mono text-[11px] text-srapi-error">
                {importResult.errors.join("\n")}
              </pre>
            ) : null}
          </div>
        ) : null}
        {batchResult ? (
          <div className="rounded-xl border border-srapi-border bg-srapi-card-muted/20 p-4">
            <div className="font-mono text-[10px] font-bold uppercase tracking-wider text-srapi-text-secondary">Batch Status</div>
            <div className="mt-2 text-sm text-srapi-text-primary">
              Updated {formatInteger(batchResult.updated_count)}
            </div>
            {batchResult.errors.length ? (
              <pre className="mt-3 max-h-40 overflow-auto rounded-xl bg-srapi-bg p-3 font-mono text-[11px] text-srapi-error">
                {batchResult.errors.join("\n")}
              </pre>
            ) : null}
          </div>
        ) : null}
      </div>
    </AdminSection>
  );
}

function RedeemCodeFormFields({
  form,
  onChange,
}: {
  form: RedeemCodeFormState;
  onChange: (form: RedeemCodeFormState) => void;
}) {
  return (
    <div className="grid gap-3">
      <Label htmlFor="redeem-code">Code</Label>
      <Input id="redeem-code" required value={form.code} onChange={(event) => onChange({ ...form, code: event.target.value })} />
      <CodeValueFields
        type={form.type}
        value={form.value}
        currency={form.currency}
        maxCount={form.maxRedemptions}
        expiresAtLocal={form.expiresAtLocal}
        maxCountLabel="Max Redemptions"
        onChange={(next) => onChange({ ...form, ...next })}
      />
    </div>
  );
}

function RedeemBatchFormFields({
  form,
  onChange,
}: {
  form: RedeemBatchFormState;
  onChange: (form: RedeemBatchFormState) => void;
}) {
  return (
    <div className="grid gap-3">
      <Label htmlFor="redeem-batch-prefix">Prefix</Label>
      <Input id="redeem-batch-prefix" value={form.prefix} onChange={(event) => onChange({ ...form, prefix: event.target.value })} />
      <Label htmlFor="redeem-batch-count">Count</Label>
      <Input id="redeem-batch-count" required type="number" min="1" max="500" value={form.count} onChange={(event) => onChange({ ...form, count: event.target.value })} />
      <CodeValueFields
        type={form.type}
        value={form.value}
        currency={form.currency}
        maxCount={form.maxRedemptions}
        expiresAtLocal={form.expiresAtLocal}
        maxCountLabel="Max Redemptions"
        onChange={(next) => onChange({ ...form, ...next })}
      />
    </div>
  );
}

function CodeValueFields({
  type,
  value,
  currency,
  maxCount,
  expiresAtLocal,
  maxCountLabel,
  onChange,
}: {
  type: RedeemCodeFormState["type"];
  value: string;
  currency: string;
  maxCount: string;
  expiresAtLocal: string;
  maxCountLabel: string;
  onChange: (next: Partial<Pick<RedeemCodeFormState, "type" | "value" | "currency" | "maxRedemptions" | "expiresAtLocal">>) => void;
}) {
  return (
    <>
      <Label htmlFor="redeem-type">Type</Label>
      <Select id="redeem-type" value={type} onChange={(event) => onChange({ type: event.target.value as RedeemCodeFormState["type"] })}>
        {REDEEM_CODE_TYPES.map((nextType) => (
          <option key={nextType} value={nextType}>{nextType}</option>
        ))}
      </Select>
      <Label htmlFor="redeem-value">Value</Label>
      <Input id="redeem-value" required value={value} onChange={(event) => onChange({ value: event.target.value })} />
      <Label htmlFor="redeem-currency">Currency</Label>
      <Input id="redeem-currency" required value={currency} onChange={(event) => onChange({ currency: event.target.value })} />
      <Label htmlFor="redeem-max-count">{maxCountLabel}</Label>
      <Input id="redeem-max-count" required type="number" min="1" value={maxCount} onChange={(event) => onChange({ maxRedemptions: event.target.value })} />
      <Label htmlFor="redeem-expires">Expires At</Label>
      <Input id="redeem-expires" type="datetime-local" value={expiresAtLocal} onChange={(event) => onChange({ expiresAtLocal: event.target.value })} />
    </>
  );
}

function PromoCodeFormFields({
  form,
  onChange,
}: {
  form: PromoCodeFormState;
  onChange: (form: PromoCodeFormState) => void;
}) {
  return (
    <div className="grid gap-3">
      <Label htmlFor="promo-code">Code</Label>
      <Input id="promo-code" required value={form.code} onChange={(event) => onChange({ ...form, code: event.target.value })} />
      <Label htmlFor="promo-discount-type">Discount Type</Label>
      <Select id="promo-discount-type" value={form.discountType} onChange={(event) => onChange({ ...form, discountType: event.target.value as PromoCodeFormState["discountType"] })}>
        {PROMO_DISCOUNT_TYPES.map((type) => (
          <option key={type} value={type}>{type}</option>
        ))}
      </Select>
      <Label htmlFor="promo-discount-value">Discount Value</Label>
      <Input id="promo-discount-value" required value={form.discountValue} onChange={(event) => onChange({ ...form, discountValue: event.target.value })} />
      <Label htmlFor="promo-currency">Currency</Label>
      <Input id="promo-currency" required value={form.currency} onChange={(event) => onChange({ ...form, currency: event.target.value })} />
      <Label htmlFor="promo-max-uses">Max Uses</Label>
      <Input id="promo-max-uses" required type="number" min="1" value={form.maxUses} onChange={(event) => onChange({ ...form, maxUses: event.target.value })} />
      <Label htmlFor="promo-status">Status</Label>
      <Select id="promo-status" value={form.status} onChange={(event) => onChange({ ...form, status: event.target.value as PromoCodeFormState["status"] })}>
        {PROMO_CODE_STATUSES.map((status) => (
          <option key={status} value={status}>{status}</option>
        ))}
      </Select>
      <Label htmlFor="promo-starts">Starts At</Label>
      <Input id="promo-starts" type="datetime-local" value={form.startsAtLocal} onChange={(event) => onChange({ ...form, startsAtLocal: event.target.value })} />
      <Label htmlFor="promo-expires">Expires At</Label>
      <Input id="promo-expires" type="datetime-local" value={form.expiresAtLocal} onChange={(event) => onChange({ ...form, expiresAtLocal: event.target.value })} />
    </div>
  );
}

export function AdminSubscriptionsProductionPage() {
  const plans = useAdminSubscriptionPlans({ page: 1 });
  const subscriptions = useAdminUserSubscriptions({ page: 1 });
  const users = useAdminUsers({ page: 1 });
  const planMutations = useAdminSubscriptionPlanMutations();
  const subMutations = useAdminUserSubscriptionMutations();
  const [planDialogOpen, setPlanDialogOpen] = useState(false);
  const [subscriptionDialogOpen, setSubscriptionDialogOpen] = useState(false);
  const [planForm, setPlanForm] = useState<SubscriptionPlanFormState>(() => emptySubscriptionPlanForm());
  const [subscriptionForm, setSubscriptionForm] = useState<UserSubscriptionFormState>(() => emptyUserSubscriptionForm());
  const [formError, setFormError] = useState<string | null>(null);

  const openPlanDialog = () => {
    setPlanForm(emptySubscriptionPlanForm());
    setFormError(null);
    setPlanDialogOpen(true);
  };
  const openSubscriptionDialog = () => {
    setSubscriptionForm(emptyUserSubscriptionForm(plans.data?.data[0]?.id ?? "", users.data?.data[0]?.id ?? ""));
    setFormError(null);
    setSubscriptionDialogOpen(true);
  };
  const submitPlan = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setFormError(null);
    let body: Parameters<typeof planMutations.create.mutate>[0];
    try {
      body = buildCreateSubscriptionPlanBody(planForm);
    } catch (error) {
      setFormError(error instanceof Error ? error.message : "Subscription plan is invalid.");
      return;
    }
    planMutations.create.mutate(body, {
      onSuccess: () => {
        setPlanDialogOpen(false);
        setPlanForm(emptySubscriptionPlanForm());
      },
    });
  };
  const submitSubscription = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setFormError(null);
    let body: Parameters<typeof subMutations.create.mutate>[0];
    try {
      body = buildCreateUserSubscriptionBody(subscriptionForm);
    } catch (error) {
      setFormError(error instanceof Error ? error.message : "User subscription is invalid.");
      return;
    }
    subMutations.create.mutate(body, {
      onSuccess: () => {
        setSubscriptionDialogOpen(false);
        setSubscriptionForm(emptyUserSubscriptionForm());
      },
    });
  };

  return (
    <AdminShell>
      <AdminPageHeader
        title="Subscription Management"
        description="Plans and user subscriptions backed by the admin subscription contract."
        actions={
          <div className="flex flex-wrap gap-2">
            <Button type="button" variant="outline" size="sm" onClick={openPlanDialog}>
              <Plus size={12} />
              New Plan
            </Button>
            <Button
              type="button"
              variant="outline"
              size="sm"
              disabled={!plans.data?.data.length || !users.data?.data.length}
              onClick={openSubscriptionDialog}
            >
              <UserPlus size={12} />
              Grant Subscription
            </Button>
          </div>
        }
      />
      <SimpleResourceTable<SubscriptionPlan>
        title="Plans"
        loading={plans.isLoading}
        error={plans.error}
        onRetry={() => void plans.refetch()}
        rows={plans.data?.data}
        pagination={plans.data?.pagination}
        getRowKey={(plan) => plan.id}
        columns={[
          { key: "name", header: "Name", render: (plan) => plan.name },
          { key: "price", header: "Price", render: (plan) => formatMoney(plan.price, plan.currency) },
          { key: "validity", header: "Validity", render: (plan) => `${plan.validity_days} days` },
          { key: "sale", header: "For Sale", render: (plan) => <Badge variant={plan.for_sale ? "success" : "neutral"}>{String(plan.for_sale)}</Badge> },
          { key: "status", header: "Status", render: (plan) => <AdminStatusBadge status={plan.status} /> },
        ]}
      />
      <SimpleResourceTable<UserSubscription>
        title="User Subscriptions"
        loading={subscriptions.isLoading}
        error={subscriptions.error}
        onRetry={() => void subscriptions.refetch()}
        rows={subscriptions.data?.data}
        pagination={subscriptions.data?.pagination}
        getRowKey={(subscription) => subscription.id}
        columns={[
          { key: "user", header: "User", render: (subscription) => subscription.user_id },
          { key: "plan", header: "Plan", render: (subscription) => subscription.plan_id },
          { key: "window", header: "Window", render: (subscription) => `${formatDate(subscription.starts_at)} -> ${formatDate(subscription.expires_at)}` },
          { key: "source", header: "Source", render: (subscription) => `${subscription.source_type}:${subscription.source_id}` },
          { key: "status", header: "Status", render: (subscription) => <AdminStatusBadge status={subscription.status} /> },
        ]}
      />
      <MutationError error={planMutations.create.error || subMutations.create.error} />
      <Dialog open={planDialogOpen} onOpenChange={setPlanDialogOpen}>
        <DialogContent>
          <form className="space-y-4" onSubmit={submitPlan}>
            <DialogHeader>
              <DialogTitle>Create Subscription Plan</DialogTitle>
              <DialogDescription>Amounts are sent as decimal strings; entitlements must be a JSON object.</DialogDescription>
            </DialogHeader>
            <SubscriptionPlanFormFields form={planForm} onChange={setPlanForm} />
            {formError ? <div className="rounded-xl border border-srapi-error/30 bg-srapi-error/5 p-3 text-xs text-srapi-error">{formError}</div> : null}
            <MutationError error={planMutations.create.error} />
            <DialogFooter>
              <Button type="button" variant="ghost" onClick={() => setPlanDialogOpen(false)}>Cancel</Button>
              <Button type="submit" disabled={planMutations.create.isPending}>Create Plan</Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
      <Dialog open={subscriptionDialogOpen} onOpenChange={setSubscriptionDialogOpen}>
        <DialogContent>
          <form className="space-y-4" onSubmit={submitSubscription}>
            <DialogHeader>
              <DialogTitle>Grant User Subscription</DialogTitle>
              <DialogDescription>Create a user subscription from an existing plan and user.</DialogDescription>
            </DialogHeader>
            <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
              <div>
                <Label htmlFor="subscription-user">User</Label>
                <Select id="subscription-user" value={subscriptionForm.userId} onChange={(event) => setSubscriptionForm((value) => ({ ...value, userId: event.target.value }))}>
                  <option value="">Select user</option>
                  {users.data?.data.map((user) => <option key={user.id} value={user.id}>{user.email}</option>)}
                </Select>
              </div>
              <div>
                <Label htmlFor="subscription-plan">Plan</Label>
                <Select id="subscription-plan" value={subscriptionForm.planId} onChange={(event) => setSubscriptionForm((value) => ({ ...value, planId: event.target.value }))}>
                  <option value="">Select plan</option>
                  {plans.data?.data.map((plan) => <option key={plan.id} value={plan.id}>{plan.name}</option>)}
                </Select>
              </div>
              <div>
                <Label htmlFor="subscription-status">Status</Label>
                <Select id="subscription-status" value={subscriptionForm.status} onChange={(event) => setSubscriptionForm((value) => ({ ...value, status: event.target.value as UserSubscriptionFormState["status"] }))}>
                  {USER_SUBSCRIPTION_STATUSES.map((status) => <option key={status} value={status}>{status}</option>)}
                </Select>
              </div>
              <div>
                <Label htmlFor="subscription-source-type">Source Type</Label>
                <Input id="subscription-source-type" value={subscriptionForm.sourceType} onChange={(event) => setSubscriptionForm((value) => ({ ...value, sourceType: event.target.value }))} />
              </div>
              <div>
                <Label htmlFor="subscription-starts">Starts At</Label>
                <Input id="subscription-starts" type="datetime-local" value={subscriptionForm.startsAtLocal} onChange={(event) => setSubscriptionForm((value) => ({ ...value, startsAtLocal: event.target.value }))} />
              </div>
              <div>
                <Label htmlFor="subscription-expires">Expires At</Label>
                <Input id="subscription-expires" type="datetime-local" value={subscriptionForm.expiresAtLocal} onChange={(event) => setSubscriptionForm((value) => ({ ...value, expiresAtLocal: event.target.value }))} />
              </div>
              <div className="md:col-span-2">
                <Label htmlFor="subscription-source-id">Source ID</Label>
                <Input id="subscription-source-id" value={subscriptionForm.sourceId} onChange={(event) => setSubscriptionForm((value) => ({ ...value, sourceId: event.target.value }))} />
              </div>
            </div>
            {formError ? <div className="rounded-xl border border-srapi-error/30 bg-srapi-error/5 p-3 text-xs text-srapi-error">{formError}</div> : null}
            <MutationError error={subMutations.create.error} />
            <DialogFooter>
              <Button type="button" variant="ghost" onClick={() => setSubscriptionDialogOpen(false)}>Cancel</Button>
              <Button type="submit" disabled={subMutations.create.isPending}>Grant Subscription</Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </AdminShell>
  );
}

export function AdminAnnouncementsProductionPage() {
  const [status, setStatus] = useState<Announcement["status"] | "all">("all");
  const announcements = useAdminAnnouncements({ status });
  const mutations = useAdminAnnouncementMutations();
  const [open, setOpen] = useState(false);
  const [editing, setEditing] = useState<Announcement | null>(null);
  const [form, setForm] = useState<AnnouncementFormState>(() => emptyAnnouncementForm());
  const [deleteState, setDeleteState] = useState<AnnouncementDeleteState | null>(null);
  const [formError, setFormError] = useState<string | null>(null);

  const openCreate = () => {
    setEditing(null);
    setForm(emptyAnnouncementForm());
    setFormError(null);
    setOpen(true);
  };
  const openEdit = (announcement: Announcement) => {
    setEditing(announcement);
    setForm(announcementFormFromAnnouncement(announcement));
    setFormError(null);
    setOpen(true);
  };
  const save = (event: FormEvent) => {
    event.preventDefault();
    setFormError(null);
    let body: ReturnType<typeof buildAnnouncementBody>;
    try {
      body = buildAnnouncementBody(form);
    } catch (error) {
      setFormError(error instanceof Error ? error.message : "Announcement is invalid.");
      return;
    }
    if (editing) {
      mutations.update.mutate({ id: editing.id, body }, { onSuccess: () => setOpen(false) });
      return;
    }
    mutations.create.mutate(body, { onSuccess: () => setOpen(false) });
  };

  return (
    <AdminShell>
      <AdminPageHeader
        title="Announcement Management"
        description="Create, publish, archive and delete production announcements through /api/v1/admin/announcements."
        actions={<Button type="button" size="sm" onClick={openCreate}><Plus size={12} />Create</Button>}
      />
      <AdminSection
        title="Announcements"
        actions={
          <Select value={status} onChange={(event) => setStatus(event.target.value as Announcement["status"] | "all")}>
            <option value="all">All status</option>
            <option value="draft">Draft</option>
            <option value="published">Published</option>
            <option value="archived">Archived</option>
          </Select>
        }
      >
        {announcements.isLoading ? <AdminLoadingState label="Loading announcements..." /> : null}
        {announcements.isError ? <AdminErrorState error={announcements.error} onRetry={() => void announcements.refetch()} /> : null}
        {announcements.data ? (
          <AdminTable
            empty={<AdminEmptyState title="No announcements" />}
            columns={[
              { key: "title", header: "Title" },
              { key: "audience", header: "Audience" },
              { key: "severity", header: "Severity" },
              { key: "status", header: "Status" },
              { key: "updated", header: "Updated" },
              { key: "actions", header: "" },
            ]}
            rows={announcements.data.data.map((item) => ({
              title: <span className="whitespace-normal font-semibold text-srapi-text-primary">{item.title}</span>,
              audience: item.audience,
              severity: <AdminStatusBadge status={item.severity} />,
              status: <AdminStatusBadge status={item.status} />,
              updated: formatDateTime(item.updated_at),
              actions: (
                <div className="flex justify-end gap-2">
                  <Button type="button" variant="outline" size="sm" onClick={() => openEdit(item)}>Edit</Button>
                  <Button type="button" variant="danger" size="sm" onClick={() => setDeleteState(deleteStateFromAnnouncement(item))}>Delete</Button>
                </div>
              ),
            }))}
            getRowKey={(_, index) => announcements.data?.data[index]?.id ?? String(index)}
          />
        ) : null}
        <MutationError error={mutations.remove.error} />
      </AdminSection>

      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent className="max-w-2xl">
          <form className="space-y-4" onSubmit={save}>
            <DialogHeader>
              <DialogTitle>{editing ? "Edit Announcement" : "Create Announcement"}</DialogTitle>
            </DialogHeader>
            <Label htmlFor="announcement-title">Title</Label>
            <Input id="announcement-title" required value={form.title} onChange={(event) => setForm((value) => ({ ...value, title: event.target.value }))} />
            <Label htmlFor="announcement-content">Content</Label>
            <Textarea id="announcement-content" required rows={8} value={form.content} onChange={(event) => setForm((value) => ({ ...value, content: event.target.value }))} />
            <div className="grid grid-cols-1 gap-3 md:grid-cols-3">
              <Select aria-label="Announcement status" value={form.status} onChange={(event) => setForm((value) => ({ ...value, status: event.target.value as AnnouncementFormState["status"] }))}>
                {ANNOUNCEMENT_STATUSES.map((item) => <option key={item} value={item}>{item}</option>)}
              </Select>
              <Select aria-label="Announcement severity" value={form.severity} onChange={(event) => setForm((value) => ({ ...value, severity: event.target.value as AnnouncementFormState["severity"] }))}>
                {ANNOUNCEMENT_SEVERITIES.map((item) => <option key={item} value={item}>{item}</option>)}
              </Select>
              <Select aria-label="Announcement audience" value={form.audience} onChange={(event) => setForm((value) => ({ ...value, audience: event.target.value as AnnouncementFormState["audience"] }))}>
                {ANNOUNCEMENT_AUDIENCES.map((item) => <option key={item} value={item}>{item}</option>)}
              </Select>
            </div>
            {formError ? <div className="rounded-xl border border-srapi-error/30 bg-srapi-error/5 p-3 text-xs text-srapi-error">{formError}</div> : null}
            <MutationError error={mutations.create.error || mutations.update.error} />
            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => setOpen(false)}>Cancel</Button>
              <Button type="submit">Save</Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
      <Dialog open={Boolean(deleteState)} onOpenChange={(isOpen) => !isOpen && setDeleteState(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Delete Announcement</DialogTitle>
            <DialogDescription>Type the announcement title to confirm deletion.</DialogDescription>
          </DialogHeader>
          {deleteState ? (
            <div className="space-y-4">
              <div className="rounded-xl border border-srapi-border p-4">
                <div className="font-mono text-[11px] font-bold uppercase text-srapi-text-secondary">Title</div>
                <div className="mt-2 select-all text-sm font-semibold text-srapi-text-primary">{deleteState.title}</div>
              </div>
              <div>
                <Label htmlFor="announcement-delete-confirmation">Confirmation</Label>
                <Input
                  id="announcement-delete-confirmation"
                  value={deleteState.confirmation}
                  onChange={(event) => setDeleteState((value) => value ? { ...value, confirmation: event.target.value } : value)}
                />
              </div>
            </div>
          ) : null}
          <MutationError error={mutations.remove.error} />
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => setDeleteState(null)}>Cancel</Button>
            <Button
              type="button"
              variant="danger"
              disabled={!canDeleteAnnouncement(deleteState) || mutations.remove.isPending}
              onClick={() => {
                if (!deleteState || !canDeleteAnnouncement(deleteState)) {
                  return;
                }
                mutations.remove.mutate(deleteState.id, { onSuccess: () => setDeleteState(null) });
              }}
            >
              Delete Announcement
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </AdminShell>
  );
}

export function AdminRedeemProductionPage() {
  const [status, setStatus] = useState<RedeemCodeStatus | "all">("all");
  const [createDialogOpen, setCreateDialogOpen] = useState(false);
  const [batchDialogOpen, setBatchDialogOpen] = useState(false);
  const [createForm, setCreateForm] = useState<RedeemCodeFormState>(() => emptyRedeemCodeForm());
  const [batchForm, setBatchForm] = useState<RedeemBatchFormState>(() => emptyRedeemBatchForm());
  const [disableState, setDisableState] = useState<RedeemDisableState | null>(null);
  const [selectedCodeIds, setSelectedCodeIds] = useState<string[]>([]);
  const [formError, setFormError] = useState<string | null>(null);
  const redeem = useAdminRedeemCodes({ status });
  const mutations = useAdminRedeemMutations();
  const activeCodes = redeem.codes.data?.data.filter((code) => code.status === "active") ?? [];
  const selectedCodes = activeCodes.filter((code) => selectedCodeIds.includes(code.id));
  const createRedeemCode = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setFormError(null);
    try {
      mutations.create.mutate(buildCreateRedeemCodeBody(createForm), {
        onSuccess: () => {
          setCreateDialogOpen(false);
          setCreateForm(emptyRedeemCodeForm());
        },
      });
    } catch (error) {
      setFormError(error instanceof Error ? error.message : "Invalid redeem code form.");
    }
  };
  const batchGenerateRedeemCodes = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setFormError(null);
    try {
      mutations.batchGenerate.mutate(buildBatchGenerateRedeemCodesBody(batchForm), {
        onSuccess: () => {
          setBatchDialogOpen(false);
          setBatchForm(emptyRedeemBatchForm());
          setSelectedCodeIds([]);
        },
      });
    } catch (error) {
      setFormError(error instanceof Error ? error.message : "Invalid batch generation form.");
    }
  };
  const disableRedeemCode = () => {
    if (!disableState || !canConfirmRedeemDisable(disableState)) {
      return;
    }
    mutations.batchDisable.mutate(disableState.ids, {
      onSuccess: () => {
        setDisableState(null);
        setSelectedCodeIds([]);
      },
    });
  };
  const toggleRedeemSelection = (id: string, checked: boolean) => {
    setSelectedCodeIds((current) => toggleIdSelection(current, id, checked));
  };
  const toggleAllActiveRedeemCodes = (checked: boolean) => {
    setSelectedCodeIds(checked ? activeCodes.map((code) => code.id) : []);
  };
  const openBatchDisableRedeemCodes = () => {
    if (!selectedCodes.length) {
      return;
    }
    setDisableState(redeemDisableStateFromSelection(selectedCodes));
  };

  return (
    <AdminShell>
      <AdminPageHeader
        title="Redeem Codes"
        description="Redeem code inventory and counters from the production redeem-code contract."
        actions={
          <div className="flex flex-wrap gap-2">
            <Button type="button" variant="outline" size="sm" onClick={() => setBatchDialogOpen(true)}>
              Batch Generate
            </Button>
            <Button type="button" size="sm" onClick={() => setCreateDialogOpen(true)}>
              <Plus size={12} />
              New Code
            </Button>
          </div>
        }
      />
      {redeem.stats.data ? (
        <div className="grid grid-cols-2 gap-4 md:grid-cols-5">
          <AdminStatCard label="Total" value={formatInteger(redeem.stats.data.total)} icon={<Ticket size={16} />} />
          <AdminStatCard label="Active" value={formatInteger(redeem.stats.data.active)} tone="success" />
          <AdminStatCard label="Redeemed" value={formatInteger(redeem.stats.data.redeemed)} />
          <AdminStatCard label="Disabled" value={formatInteger(redeem.stats.data.disabled)} tone="danger" />
          <AdminStatCard label="Expired" value={formatInteger(redeem.stats.data.expired)} tone="warning" />
        </div>
      ) : null}
      <AdminSection
        title="Codes"
        actions={
          <div className="flex flex-wrap items-center justify-end gap-2">
            <span className="font-mono text-[11px] text-srapi-text-secondary">{selectedCodes.length} selected</span>
            <Button type="button" variant="danger" size="sm" disabled={!selectedCodes.length || mutations.batchDisable.isPending} onClick={openBatchDisableRedeemCodes}>
              Disable Selected
            </Button>
            <Select value={status} onChange={(event) => { setStatus(event.target.value as RedeemCodeStatus | "all"); setSelectedCodeIds([]); }}>
              <option value="all">All status</option>
              <option value="active">Active</option>
              <option value="redeemed">Redeemed</option>
              <option value="disabled">Disabled</option>
              <option value="expired">Expired</option>
            </Select>
          </div>
        }
      >
        {redeem.codes.isLoading ? <AdminLoadingState label="Loading redeem codes..." /> : null}
        {redeem.codes.isError ? <AdminErrorState error={redeem.codes.error} onRetry={() => void redeem.codes.refetch()} /> : null}
        {redeem.codes.data ? (
          <AdminTable
            empty={<AdminEmptyState title="No redeem codes" />}
            columns={[
              {
                key: "select",
                header: (
                  <input
                    aria-label="Select all active redeem codes"
                    type="checkbox"
                    checked={activeCodes.length > 0 && selectedCodes.length === activeCodes.length}
                    disabled={!activeCodes.length}
                    onChange={(event) => toggleAllActiveRedeemCodes(event.target.checked)}
                  />
                ),
              },
              { key: "code", header: "Code" },
              { key: "type", header: "Type" },
              { key: "value", header: "Value" },
              { key: "usage", header: "Usage" },
              { key: "status", header: "Status" },
              { key: "expires", header: "Expires" },
              { key: "actions", header: "" },
            ]}
            rows={redeem.codes.data.data.map((code) => ({
              select: (
                <input
                  aria-label={`Select redeem code ${code.code}`}
                  type="checkbox"
                  checked={selectedCodeIds.includes(code.id)}
                  disabled={code.status !== "active"}
                  onChange={(event) => toggleRedeemSelection(code.id, event.target.checked)}
                />
              ),
              code: <span className="select-all font-semibold text-srapi-text-primary">{code.code}</span>,
              type: code.type,
              value: formatMoney(code.value, code.currency),
              usage: `${code.redeemed_count}/${code.max_redemptions}`,
              status: <AdminStatusBadge status={code.status} />,
              expires: formatDate(code.expires_at),
              actions: (
                <div className="flex justify-end">
                  <Button
                    type="button"
                    variant={code.status === "active" ? "danger" : "outline"}
                    size="sm"
                    disabled={code.status !== "active" || mutations.batchDisable.isPending}
                    onClick={() => setDisableState(redeemDisableStateFromCode(code))}
                  >
                    {code.status === "active" ? "Disable" : "Disabled"}
                  </Button>
                </div>
              ),
            }))}
            getRowKey={(_, index) => redeem.codes.data?.data[index]?.id ?? String(index)}
          />
        ) : null}
        <MutationError error={mutations.create.error || mutations.batchGenerate.error || mutations.batchDisable.error} />
      </AdminSection>
      <Dialog open={createDialogOpen} onOpenChange={(open) => { setCreateDialogOpen(open); if (!open) setFormError(null); }}>
        <DialogContent>
          <form className="space-y-4" onSubmit={createRedeemCode}>
            <DialogHeader>
              <DialogTitle>Create Redeem Code</DialogTitle>
              <DialogDescription>Creates one explicit code through /api/v1/admin/redeem-codes.</DialogDescription>
            </DialogHeader>
            <RedeemCodeFormFields form={createForm} onChange={setCreateForm} />
            {formError ? <div className="rounded-xl border border-srapi-error/30 bg-srapi-error/5 p-3 text-xs text-srapi-error">{formError}</div> : null}
            <MutationError error={mutations.create.error} />
            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => setCreateDialogOpen(false)}>Cancel</Button>
              <Button type="submit" disabled={mutations.create.isPending}>Create Code</Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
      <Dialog open={batchDialogOpen} onOpenChange={(open) => { setBatchDialogOpen(open); if (!open) setFormError(null); }}>
        <DialogContent>
          <form className="space-y-4" onSubmit={batchGenerateRedeemCodes}>
            <DialogHeader>
              <DialogTitle>Batch Generate Redeem Codes</DialogTitle>
              <DialogDescription>Generates between 1 and 500 production redeem codes in one request.</DialogDescription>
            </DialogHeader>
            <RedeemBatchFormFields form={batchForm} onChange={setBatchForm} />
            {formError ? <div className="rounded-xl border border-srapi-error/30 bg-srapi-error/5 p-3 text-xs text-srapi-error">{formError}</div> : null}
            <MutationError error={mutations.batchGenerate.error} />
            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => setBatchDialogOpen(false)}>Cancel</Button>
              <Button type="submit" disabled={mutations.batchGenerate.isPending}>Generate Codes</Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
      <Dialog open={Boolean(disableState)} onOpenChange={(open) => !open && setDisableState(null)}>
        <DialogContent>
          <div className="space-y-4">
            <DialogHeader>
              <DialogTitle>{disableState?.ids.length === 1 ? "Disable Redeem Code" : "Disable Redeem Codes"}</DialogTitle>
              <DialogDescription>
                Type the confirmation phrase exactly. This prevents accidental active-code shutdown.
              </DialogDescription>
            </DialogHeader>
            <div className="rounded-xl border border-srapi-border bg-srapi-card-muted p-3 font-mono text-xs font-bold text-srapi-text-primary">
              {disableState?.label}
            </div>
            <Label htmlFor="redeem-disable-confirmation">Confirmation</Label>
            <Input
              id="redeem-disable-confirmation"
              value={disableState?.confirmation ?? ""}
              onChange={(event) => setDisableState((value) => value ? { ...value, confirmation: event.target.value } : value)}
              placeholder={disableState?.label}
            />
            <MutationError error={mutations.batchDisable.error} />
            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => setDisableState(null)}>Cancel</Button>
              <Button type="button" variant="danger" disabled={!canConfirmRedeemDisable(disableState) || mutations.batchDisable.isPending} onClick={disableRedeemCode}>
                {disableState?.ids.length === 1 ? "Disable Code" : "Disable Codes"}
              </Button>
            </DialogFooter>
          </div>
        </DialogContent>
      </Dialog>
    </AdminShell>
  );
}

export function AdminPromoCodesProductionPage() {
  const [status, setStatus] = useState<PromoCodeStatus | "all">("all");
  const [dialogOpen, setDialogOpen] = useState(false);
  const [editing, setEditing] = useState<PromoCode | null>(null);
  const [form, setForm] = useState<PromoCodeFormState>(() => emptyPromoCodeForm());
  const [deleteState, setDeleteState] = useState<PromoDeleteState | null>(null);
  const [formError, setFormError] = useState<string | null>(null);
  const promos = useAdminPromoCodes({ page: 1, status });
  const mutations = useAdminPromoCodeMutations();
  const openCreate = () => {
    setEditing(null);
    setForm(emptyPromoCodeForm());
    setFormError(null);
    setDialogOpen(true);
  };
  const openEdit = (promo: PromoCode) => {
    setEditing(promo);
    setForm(promoFormFromCode(promo));
    setFormError(null);
    setDialogOpen(true);
  };
  const savePromoCode = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setFormError(null);
    try {
      const body = buildPromoCodeBody(form);
      if (editing) {
        mutations.update.mutate({ id: editing.id, body }, { onSuccess: () => setDialogOpen(false) });
        return;
      }
      mutations.create.mutate(body, {
        onSuccess: () => {
          setDialogOpen(false);
          setForm(emptyPromoCodeForm());
        },
      });
    } catch (error) {
      setFormError(error instanceof Error ? error.message : "Invalid promo code form.");
    }
  };
  const deletePromoCode = () => {
    if (!deleteState || !canDeletePromoCode(deleteState)) {
      return;
    }
    mutations.remove.mutate(deleteState.id, { onSuccess: () => setDeleteState(null) });
  };

  return (
    <AdminShell>
      <AdminPageHeader
        title="Promo Codes"
        description="Promotion codes from /api/v1/admin/promo-codes. No generated discounts are shown."
        actions={
          <Button type="button" size="sm" onClick={openCreate}>
            <Plus size={12} />
            New Promo
          </Button>
        }
      />
      <div className="flex justify-end">
        <Select value={status} onChange={(event) => setStatus(event.target.value as PromoCodeStatus | "all")}>
          <option value="all">All status</option>
          {PROMO_CODE_STATUSES.map((nextStatus) => (
            <option key={nextStatus} value={nextStatus}>{nextStatus}</option>
          ))}
        </Select>
      </div>
      <SimpleResourceTable
        title="Promo Codes"
        loading={promos.isLoading}
        error={promos.error}
        onRetry={() => void promos.refetch()}
        rows={promos.data?.data}
        pagination={promos.data?.pagination}
        getRowKey={(promo) => promo.id}
        columns={[
          { key: "code", header: "Code", render: (promo) => <span className="select-all font-semibold text-srapi-text-primary">{promo.code}</span> },
          { key: "discount", header: "Discount", render: (promo) => promo.discount_type === "percent" ? `${promo.discount_value}%` : formatMoney(promo.discount_value, promo.currency) },
          { key: "usage", header: "Usage", render: (promo) => `${promo.used_count}/${promo.max_uses}` },
          { key: "status", header: "Status", render: (promo) => <AdminStatusBadge status={promo.status} /> },
          { key: "expires", header: "Expires", render: (promo) => formatDate(promo.expires_at) },
          {
            key: "actions",
            header: "",
            render: (promo) => (
              <div className="flex justify-end gap-2">
                <Button type="button" variant="outline" size="sm" onClick={() => openEdit(promo)}>Edit</Button>
                <Button type="button" variant="danger" size="sm" disabled={mutations.remove.isPending} onClick={() => setDeleteState(promoDeleteStateFromCode(promo))}>Delete</Button>
              </div>
            ),
          },
        ]}
      />
      <MutationError error={mutations.create.error || mutations.update.error || mutations.remove.error} />
      <Dialog open={dialogOpen} onOpenChange={(open) => { setDialogOpen(open); if (!open) setFormError(null); }}>
        <DialogContent>
          <form className="space-y-4" onSubmit={savePromoCode}>
            <DialogHeader>
              <DialogTitle>{editing ? "Edit Promo Code" : "Create Promo Code"}</DialogTitle>
              <DialogDescription>Discount values remain decimal strings and are sent through the generated admin SDK.</DialogDescription>
            </DialogHeader>
            <PromoCodeFormFields form={form} onChange={setForm} />
            {formError ? <div className="rounded-xl border border-srapi-error/30 bg-srapi-error/5 p-3 text-xs text-srapi-error">{formError}</div> : null}
            <MutationError error={mutations.create.error || mutations.update.error} />
            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => setDialogOpen(false)}>Cancel</Button>
              <Button type="submit" disabled={mutations.create.isPending || mutations.update.isPending}>
                {editing ? "Save Promo" : "Create Promo"}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
      <Dialog open={Boolean(deleteState)} onOpenChange={(open) => !open && setDeleteState(null)}>
        <DialogContent>
          <div className="space-y-4">
            <DialogHeader>
              <DialogTitle>Delete Promo Code</DialogTitle>
              <DialogDescription>Type the promo code to permanently delete it.</DialogDescription>
            </DialogHeader>
            <div className="rounded-xl border border-srapi-border bg-srapi-card-muted p-3 font-mono text-xs font-bold text-srapi-text-primary">
              {deleteState?.code}
            </div>
            <Label htmlFor="promo-delete-confirmation">Confirmation</Label>
            <Input
              id="promo-delete-confirmation"
              value={deleteState?.confirmation ?? ""}
              onChange={(event) => setDeleteState((value) => value ? { ...value, confirmation: event.target.value } : value)}
              placeholder={deleteState?.code}
            />
            <MutationError error={mutations.remove.error} />
            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => setDeleteState(null)}>Cancel</Button>
              <Button type="button" variant="danger" disabled={!canDeletePromoCode(deleteState) || mutations.remove.isPending} onClick={deletePromoCode}>
                Delete Promo
              </Button>
            </DialogFooter>
          </div>
        </DialogContent>
      </Dialog>
    </AdminShell>
  );
}

export function AdminUsageProductionPage() {
  const [model, setModel] = useState("");
  const logs = useAdminUsageLogs({ page: 1, model });
  const byModel = useAdminUsageAggregates("model");
  const byUser = useAdminUsageAggregates("user");

  return (
    <AdminShell>
      <AdminPageHeader title="Usage Records" description="Global request usage, token and cost records. Request bodies and prompts are never displayed." />
      <div className="grid grid-cols-1 gap-6 xl:grid-cols-2">
        <AdminSection title="By Model">
          <AdminBarList
            emptyLabel="No model aggregates"
            items={(byModel.data?.data ?? []).map((item) => ({
              label: item.aggregate_id,
              value: item.total_tokens,
              detail: `${formatCompactNumber(item.total_tokens)} / ${formatMoney(item.total_cost, item.currency)}`,
            }))}
          />
        </AdminSection>
        <AdminSection title="By User">
          <AdminBarList
            emptyLabel="No user aggregates"
            items={(byUser.data?.data ?? []).map((item) => ({
              label: item.aggregate_id,
              value: item.total_tokens,
              detail: `${formatCompactNumber(item.total_tokens)} / ${formatMoney(item.total_cost, item.currency)}`,
            }))}
          />
        </AdminSection>
      </div>
      <AdminSection
        title="Usage Logs"
        actions={<Input value={model} onChange={(event) => setModel(event.target.value)} placeholder="Filter model" />}
      >
        {logs.isLoading ? <AdminLoadingState label="Loading usage logs..." /> : null}
        {logs.isError ? <AdminErrorState error={logs.error} onRetry={() => void logs.refetch()} /> : null}
        {logs.data ? (
          <AdminTable
            empty={<AdminEmptyState title="No usage logs" />}
            columns={[
              { key: "time", header: "Time" },
              { key: "request", header: "Request" },
              { key: "model", header: "Model" },
              { key: "tokens", header: "Tokens" },
              { key: "latency", header: "Latency" },
              { key: "cost", header: "Cost" },
              { key: "status", header: "Status" },
            ]}
            rows={logs.data.data.map((log) => ({
              time: formatDateTime(log.created_at),
              request: <span className="select-all">{log.request_id}</span>,
              model: log.model,
              tokens: formatInteger(log.total_tokens),
              latency: `${formatInteger(log.latency_ms)}ms`,
              cost: formatMoney(log.cost, log.currency),
              status: <AdminStatusBadge status={log.success ? "success" : log.error_class || "error"} />,
            }))}
            getRowKey={(_, index) => logs.data?.data[index]?.id ?? String(index)}
          />
        ) : null}
      </AdminSection>
    </AdminShell>
  );
}

export function AdminSettingsProductionPage() {
  const settings = useAdminSettings();
  const mutation = useAdminSettingsMutation();
  const [activeTab, setActiveTab] = useState<SettingsTab>("general");
  const [draft, setDraft] = useState<AdminSettingsDraft | null>(null);
  const [saveConfirmation, setSaveConfirmation] = useState<SettingsSaveConfirmationState | null>(null);
  const [formError, setFormError] = useState<string | null>(null);
  const value = draft?.value ?? settings.data ?? null;
  const effectiveDraft = draft ?? (settings.data ? createSettingsDraft(settings.data) : null);

  const submitSettings = (next: AdminSettings) => {
    mutation.mutate(next, {
      onSuccess: (saved) => {
        setDraft(createSettingsDraft(saved));
        setSaveConfirmation(null);
      },
    });
  };

  const save = () => {
    if (!effectiveDraft) {
      return;
    }
    setFormError(null);
    try {
      const next = materializeSettingsDraft(effectiveDraft);
      if (settingsTabRequiresConfirmation(activeTab)) {
        setSaveConfirmation(createSettingsSaveConfirmation(activeTab));
        return;
      }
      submitSettings(next);
    } catch (error) {
      setFormError(error instanceof Error ? error.message : "Invalid settings form.");
    }
  };

  const confirmSave = () => {
    if (!effectiveDraft || !canConfirmSettingsSave(saveConfirmation)) {
      return;
    }
    setFormError(null);
    try {
      submitSettings(materializeSettingsDraft(effectiveDraft));
    } catch (error) {
      setFormError(error instanceof Error ? error.message : "Invalid settings form.");
    }
  };

  const update = (updater: (current: AdminSettings) => AdminSettings) => {
    if (effectiveDraft) {
      setDraft(updateSettingsValue(effectiveDraft, updater));
    }
  };
  const updateDraft = (updater: (current: AdminSettingsDraft) => AdminSettingsDraft) => {
    if (effectiveDraft) {
      setDraft(updater(effectiveDraft));
    }
  };

  return (
    <AdminShell>
      <AdminPageHeader
        title="System Settings"
        description="Typed settings sections from /api/v1/admin/settings. Secrets remain write-only; the UI only displays configured state."
        actions={<Button type="button" size="sm" disabled={!draft || mutation.isPending} onClick={save}><Save size={12} />Save Settings</Button>}
      />
      {settings.isLoading ? <AdminLoadingState label="Loading settings..." /> : null}
      {settings.isError ? <AdminErrorState error={settings.error} onRetry={() => void settings.refetch()} /> : null}
      {value && effectiveDraft ? (
        <>
          <div className="mb-6 overflow-x-auto rounded-2xl border border-srapi-border bg-srapi-card p-1">
            <div className="flex min-w-max gap-1">
              {SETTINGS_TABS.map((tab) => (
                <Button
                  key={tab.id}
                  type="button"
                  size="sm"
                  variant={activeTab === tab.id ? "primary" : "ghost"}
                  onClick={() => { setActiveTab(tab.id); setSaveConfirmation(null); }}
                >
                  {tab.label}
                </Button>
              ))}
            </div>
          </div>

          {activeTab === "general" ? (
            <AdminSection title="General" description="Public console identity and custom menu metadata. Custom menus are stored as a JSON array of objects.">
              <div className="grid grid-cols-1 gap-4 md:grid-cols-3">
                <div><Label htmlFor="settings-site-name">Site Name</Label><Input id="settings-site-name" value={value.general.site_name} onChange={(event) => update((current) => ({ ...current, general: { ...current.general, site_name: event.target.value } }))} /></div>
                <div><Label htmlFor="settings-logo-url">Logo URL</Label><Input id="settings-logo-url" value={value.general.logo_url} onChange={(event) => update((current) => ({ ...current, general: { ...current.general, logo_url: event.target.value } }))} /></div>
                <div><Label htmlFor="settings-version-label">Version Label</Label><Input id="settings-version-label" value={value.general.version_label} onChange={(event) => update((current) => ({ ...current, general: { ...current.general, version_label: event.target.value } }))} /></div>
              </div>
              <div className="mt-4">
                <Label htmlFor="settings-custom-menus">Custom Menus JSON</Label>
                <Textarea id="settings-custom-menus" rows={8} value={effectiveDraft.customMenusJson} onChange={(event) => updateDraft((current) => ({ ...current, customMenusJson: event.target.value }))} />
              </div>
            </AdminSection>
          ) : null}

          {activeTab === "agreement" ? (
            <AdminSection title="Agreement" description="Legal text returned by the typed settings contract.">
              <div className="grid grid-cols-1 gap-4 xl:grid-cols-2">
                <div><Label htmlFor="settings-user-agreement">User Agreement</Label><Textarea id="settings-user-agreement" rows={16} value={value.agreement.user_agreement} onChange={(event) => update((current) => ({ ...current, agreement: { ...current.agreement, user_agreement: event.target.value } }))} /></div>
                <div><Label htmlFor="settings-privacy-policy">Privacy Policy</Label><Textarea id="settings-privacy-policy" rows={16} value={value.agreement.privacy_policy} onChange={(event) => update((current) => ({ ...current, agreement: { ...current.agreement, privacy_policy: event.target.value } }))} /></div>
              </div>
            </AdminSection>
          ) : null}

          {activeTab === "features" ? (
            <AdminSection title="Features" description="Feature flags and enabled provider channel identifiers.">
              <div className="grid grid-cols-1 gap-4 md:grid-cols-3">
                <SettingsToggle label="Channel Monitoring" checked={value.features.channel_monitoring_enabled} onChange={(checked) => update((current) => ({ ...current, features: { ...current.features, channel_monitoring_enabled: checked } }))} />
                <SettingsToggle label="Invitation Rebate" checked={value.features.invitation_rebate_enabled} onChange={(checked) => update((current) => ({ ...current, features: { ...current.features, invitation_rebate_enabled: checked } }))} />
                <SettingsToggle label="Payments" checked={value.features.payments_enabled} onChange={(checked) => update((current) => ({ ...current, features: { ...current.features, payments_enabled: checked } }))} />
              </div>
              <div className="mt-4">
                <Label htmlFor="settings-enabled-channels">Enabled Channels</Label>
                <Textarea id="settings-enabled-channels" rows={8} value={effectiveDraft.enabledChannelsText} onChange={(event) => updateDraft((current) => ({ ...current, enabledChannelsText: event.target.value }))} />
              </div>
            </AdminSection>
          ) : null}

          {activeTab === "security" ? (
            <AdminSection title="Security" description="Security-sensitive settings. Admin API key material is not returned by the API and is not editable here.">
              <div className="grid grid-cols-1 gap-4 md:grid-cols-3">
                <div className="rounded-xl border border-srapi-border p-4">
                  <div className="font-mono text-xs font-bold uppercase">Admin API Key</div>
                  <div className="mt-2"><AdminStatusBadge status={value.security.admin_api_key.configured ? "configured" : "not_configured"} /></div>
                </div>
                <SettingsToggle label="Registration Enabled" checked={value.security.registration_enabled} onChange={(checked) => update((current) => ({ ...current, security: { ...current.security, registration_enabled: checked } }))} />
                <SettingsToggle label="OAuth Enabled" checked={value.security.oauth_enabled} onChange={(checked) => update((current) => ({ ...current, security: { ...current.security, oauth_enabled: checked } }))} />
              </div>
              <div className="mt-4">
                <Label htmlFor="settings-oauth-providers">OAuth Providers</Label>
                <Textarea id="settings-oauth-providers" rows={8} value={effectiveDraft.oauthProvidersText} onChange={(event) => updateDraft((current) => ({ ...current, oauthProvidersText: event.target.value }))} />
              </div>
            </AdminSection>
          ) : null}

          {activeTab === "users" ? (
            <AdminSection title="Users" description="Defaults applied to newly created users and self-service account behavior.">
              <div className="grid grid-cols-1 gap-4 md:grid-cols-3">
                <div><Label htmlFor="settings-default-balance">Default Balance</Label><Input id="settings-default-balance" value={value.users.default_balance} onChange={(event) => update((current) => ({ ...current, users: { ...current.users, default_balance: event.target.value } }))} /></div>
                <div><Label htmlFor="settings-default-group">Default Group</Label><Input id="settings-default-group" value={value.users.default_group} onChange={(event) => update((current) => ({ ...current, users: { ...current.users, default_group: event.target.value } }))} /></div>
                <div><Label htmlFor="settings-rpm-limit-default">Default RPM Limit</Label><Input id="settings-rpm-limit-default" type="number" min="0" value={value.users.rpm_limit_default} onChange={(event) => update((current) => ({ ...current, users: { ...current.users, rpm_limit_default: Number(event.target.value) } }))} /></div>
                <SettingsToggle label="User Self Delete" checked={value.users.user_self_delete_enabled} onChange={(checked) => update((current) => ({ ...current, users: { ...current.users, user_self_delete_enabled: checked } }))} />
              </div>
            </AdminSection>
          ) : null}

          {activeTab === "gateway" ? (
            <AdminSection title="Gateway" description="Gateway overload, rate limit, stream timeout and request shaping controls.">
              <div className="grid grid-cols-1 gap-4 md:grid-cols-3">
                <div><Label htmlFor="settings-overload-cooldown">Overload Cooldown Seconds</Label><Input id="settings-overload-cooldown" type="number" min="0" value={value.gateway.overload_cooldown_seconds} onChange={(event) => update((current) => ({ ...current, gateway: { ...current.gateway, overload_cooldown_seconds: Number(event.target.value) } }))} /></div>
                <div><Label htmlFor="settings-rate-limit-cooldown">429 Cooldown Seconds</Label><Input id="settings-rate-limit-cooldown" type="number" min="0" value={value.gateway.rate_limit_cooldown_seconds} onChange={(event) => update((current) => ({ ...current, gateway: { ...current.gateway, rate_limit_cooldown_seconds: Number(event.target.value) } }))} /></div>
                <div><Label htmlFor="settings-stream-timeout">Stream Timeout Seconds</Label><Input id="settings-stream-timeout" type="number" min="1" value={value.gateway.stream_timeout_seconds} onChange={(event) => update((current) => ({ ...current, gateway: { ...current.gateway, stream_timeout_seconds: Number(event.target.value) } }))} /></div>
                <div><Label htmlFor="settings-beta-strategy">Beta Strategy</Label><Input id="settings-beta-strategy" value={value.gateway.beta_strategy} onChange={(event) => update((current) => ({ ...current, gateway: { ...current.gateway, beta_strategy: event.target.value } }))} /></div>
                <div><Label htmlFor="settings-rollout-shadow-strategy">Shadow Strategy</Label><Select id="settings-rollout-shadow-strategy" value={value.gateway.scheduler_strategy_shadow_strategy ?? ""} onChange={(event) => update((current) => ({ ...current, gateway: { ...current.gateway, scheduler_strategy_shadow_strategy: schedulerStrategyNameFromSelect(event.target.value) } }))}><option value="">None</option><option value="balanced">Balanced</option><option value="cost_saver">Cost Saver</option><option value="latency_first">Latency First</option><option value="quota_protect">Quota Protect</option><option value="sticky_first">Sticky First</option><option value="cache_affinity_first">Cache Affinity</option><option value="premium_quality">Premium Quality</option></Select></div>
                <div><Label htmlFor="settings-rollout-percent">Rollout Percent</Label><Input id="settings-rollout-percent" type="number" min="0" max="100" step="0.01" value={value.gateway.scheduler_strategy_rollout_percent ?? 0} onChange={(event) => update((current) => ({ ...current, gateway: { ...current.gateway, scheduler_strategy_rollout_percent: Number(event.target.value) } }))} /></div>
                <SettingsToggle label="Request Shaper" checked={value.gateway.request_shaper_enabled} onChange={(checked) => update((current) => ({ ...current, gateway: { ...current.gateway, request_shaper_enabled: checked } }))} />
                <SettingsToggle label="Strategy Rollout" checked={value.gateway.scheduler_strategy_rollout_enabled ?? false} onChange={(checked) => update((current) => ({ ...current, gateway: { ...current.gateway, scheduler_strategy_rollout_enabled: checked } }))} />
              </div>
              <div className="mt-4 grid grid-cols-1 gap-4 md:grid-cols-2">
                <div><Label htmlFor="settings-rollout-models">Rollout Models</Label><Textarea id="settings-rollout-models" rows={8} value={effectiveDraft.schedulerRolloutModelsText} onChange={(event) => updateDraft((current) => ({ ...current, schedulerRolloutModelsText: event.target.value }))} /></div>
                <div><Label htmlFor="settings-rollout-api-key-hashes">API Key Prefix Hashes</Label><Textarea id="settings-rollout-api-key-hashes" rows={8} value={effectiveDraft.schedulerRolloutApiKeyHashesText} onChange={(event) => updateDraft((current) => ({ ...current, schedulerRolloutApiKeyHashesText: event.target.value }))} /></div>
              </div>
            </AdminSection>
          ) : null}

          {activeTab === "payment" ? (
            <AdminSection title="Payment" description="Payment feature switches and provider identifiers. Provider credentials remain outside this typed settings payload.">
              <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
                <SettingsToggle label="Payment Enabled" checked={value.payment.enabled} onChange={(checked) => update((current) => ({ ...current, payment: { ...current.payment, enabled: checked } }))} />
                <SettingsToggle label="Subscription Plans" checked={value.payment.subscription_plans_enabled} onChange={(checked) => update((current) => ({ ...current, payment: { ...current.payment, subscription_plans_enabled: checked } }))} />
              </div>
              <div className="mt-4">
                <Label htmlFor="settings-payment-providers">Payment Providers</Label>
                <Textarea id="settings-payment-providers" rows={8} value={effectiveDraft.paymentProvidersText} onChange={(event) => updateDraft((current) => ({ ...current, paymentProvidersText: event.target.value }))} />
              </div>
            </AdminSection>
          ) : null}

          {activeTab === "email" ? (
            <AdminSection title="Email" description="SMTP is represented as configured state only. Templates are editable as a string map.">
              <div className="mb-4 grid grid-cols-1 gap-4 md:grid-cols-2">
                <div className="rounded-xl border border-srapi-border p-4">
                  <div className="font-mono text-xs font-bold uppercase">SMTP</div>
                  <div className="mt-2"><AdminStatusBadge status={value.email.smtp_configured ? "configured" : "not_configured"} /></div>
                </div>
              </div>
              <Label htmlFor="settings-email-templates">Email Templates JSON</Label>
              <Textarea id="settings-email-templates" rows={14} value={effectiveDraft.emailTemplatesJson} onChange={(event) => updateDraft((current) => ({ ...current, emailTemplatesJson: event.target.value }))} />
            </AdminSection>
          ) : null}

          {activeTab === "backup" ? (
            <AdminSection title="Backup" description="Backup scheduling settings and latest backup evidence from the settings contract.">
              <div className="grid grid-cols-1 gap-4 md:grid-cols-3">
                <SettingsToggle label="Backup Enabled" checked={value.backup.enabled} onChange={(checked) => update((current) => ({ ...current, backup: { ...current.backup, enabled: checked } }))} />
                <div><Label htmlFor="settings-retention-days">Retention Days</Label><Input id="settings-retention-days" type="number" min="1" value={value.backup.retention_days} onChange={(event) => update((current) => ({ ...current, backup: { ...current.backup, retention_days: Number(event.target.value) } }))} /></div>
                <AdminStatCard label="Last Backup" value={formatDateTime(value.backup.last_backup_at)} />
              </div>
            </AdminSection>
          ) : null}

          {formError ? (
            <div className="mt-4 rounded-xl border border-srapi-error/30 bg-srapi-error/5 p-3 text-xs text-srapi-error">
              {formError}
            </div>
          ) : null}
          <div className="mt-4">
            <MutationError error={mutation.error} />
          </div>
          <Dialog open={Boolean(saveConfirmation)} onOpenChange={(open) => !open && setSaveConfirmation(null)}>
            <DialogContent>
              <div className="space-y-4">
                <DialogHeader>
                  <DialogTitle>Confirm Settings Save</DialogTitle>
                  <DialogDescription>
                    This tab controls production-sensitive behavior. Type the confirmation phrase before saving.
                  </DialogDescription>
                </DialogHeader>
                <div className="rounded-xl border border-srapi-border bg-srapi-card-muted p-3 font-mono text-xs font-bold text-srapi-text-primary">
                  {saveConfirmation?.phrase}
                </div>
                <Label htmlFor="settings-save-confirmation">Confirmation</Label>
                <Input
                  id="settings-save-confirmation"
                  value={saveConfirmation?.confirmation ?? ""}
                  onChange={(event) => setSaveConfirmation((value) => value ? { ...value, confirmation: event.target.value } : value)}
                  placeholder={saveConfirmation?.phrase}
                />
                <MutationError error={mutation.error} />
                <DialogFooter>
                  <Button type="button" variant="outline" onClick={() => setSaveConfirmation(null)}>Cancel</Button>
                  <Button
                    type="button"
                    variant="danger"
                    disabled={!canConfirmSettingsSave(saveConfirmation) || mutation.isPending}
                    onClick={confirmSave}
                  >
                    Save Settings
                  </Button>
                </DialogFooter>
              </div>
            </DialogContent>
          </Dialog>
        </>
      ) : null}
    </AdminShell>
  );
}

export function AdminChannelsPricingProductionPage() {
  const rules = useAdminPricingRules({ page: 1 });
  const models = useAdminModels();
  const providers = useAdminProviders();
  const mutations = useAdminPricingRuleMutations();
  const [dialogOpen, setDialogOpen] = useState(false);
  const [form, setForm] = useState<PricingRuleFormState>(() => emptyPricingRuleForm());
  const [pendingPricingRule, setPendingPricingRule] = useState<Parameters<typeof mutations.create.mutate>[0] | null>(null);
  const [pricingConfirmation, setPricingConfirmation] = useState<PricingRuleCreateConfirmationState | null>(null);
  const [formError, setFormError] = useState<string | null>(null);
  const modelName = useMemo(() => {
    const map = new Map<string, string>();
    models.data?.data.forEach((model) => map.set(model.id, model.canonical_name));
    return map;
  }, [models.data]);
  const providerName = useMemo(() => {
    const map = new Map<string, string>();
    providers.data?.data.forEach((provider) => map.set(provider.id, provider.display_name));
    return map;
  }, [providers.data]);
  const openCreate = () => {
    setForm(emptyPricingRuleForm(models.data?.data[0]?.id ?? "", providers.data?.data[0]?.id ?? ""));
    setFormError(null);
    setDialogOpen(true);
  };
  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setFormError(null);
    let body: Parameters<typeof mutations.create.mutate>[0];
    try {
      body = buildCreatePricingRuleBody(form);
    } catch (error) {
      setFormError(error instanceof Error ? error.message : "Pricing rule is invalid.");
      return;
    }
    setPendingPricingRule(body);
    setPricingConfirmation(createPricingRuleConfirmation({
      modelLabel: modelName.get(body.model_id) ?? body.model_id,
      providerLabel: providerName.get(body.provider_id) ?? body.provider_id,
    }));
  };
  const confirmCreatePricingRule = () => {
    if (!pendingPricingRule || !canConfirmPricingRuleCreate(pricingConfirmation)) {
      return;
    }
    mutations.create.mutate(pendingPricingRule, {
      onSuccess: () => {
        setDialogOpen(false);
        setForm(emptyPricingRuleForm());
        setPendingPricingRule(null);
        setPricingConfirmation(null);
      },
    });
  };

  return (
    <AdminShell>
      <AdminPageHeader
        title="Channel Pricing"
        description="Provider/model pricing rules from the OpenAPI contract."
        actions={
          <Button
            type="button"
            variant="outline"
            size="sm"
            disabled={!models.data?.data.length || !providers.data?.data.length}
            onClick={openCreate}
          >
            <Plus size={12} />
            New Pricing Rule
          </Button>
        }
      />
      <SimpleResourceTable<PricingRule>
        title="Pricing Rules"
        loading={rules.isLoading}
        error={rules.error}
        onRetry={() => void rules.refetch()}
        rows={rules.data?.data}
        pagination={rules.data?.pagination}
        getRowKey={(rule) => rule.id}
        columns={[
          { key: "model", header: "Model", render: (rule) => modelName.get(rule.model_id) ?? rule.model_id },
          { key: "provider", header: "Provider", render: (rule) => providerName.get(rule.provider_id) ?? rule.provider_id },
          { key: "input", header: "Input / MTok", render: (rule) => formatMoney(rule.input_price_per_million_tokens, rule.currency) },
          { key: "output", header: "Output / MTok", render: (rule) => formatMoney(rule.output_price_per_million_tokens, rule.currency) },
          { key: "cache", header: "Cache Read / Write", render: (rule) => `${formatMoney(rule.cache_read_price_per_million_tokens, rule.currency)} / ${formatMoney(rule.cache_write_price_per_million_tokens, rule.currency)}` },
        ]}
      />
      <MutationError error={mutations.create.error} />
      <Dialog
        open={dialogOpen}
        onOpenChange={(open) => {
          setDialogOpen(open);
          if (!open) {
            setPendingPricingRule(null);
            setPricingConfirmation(null);
          }
        }}
      >
        <DialogContent>
          <form className="space-y-4" onSubmit={submit}>
            <DialogHeader>
              <DialogTitle>Create Pricing Rule</DialogTitle>
              <DialogDescription>All token prices are sent as decimal strings per million tokens.</DialogDescription>
            </DialogHeader>
            <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
              <div>
                <Label htmlFor="pricing-model">Model</Label>
                <Select id="pricing-model" value={form.modelId} onChange={(event) => setForm((value) => ({ ...value, modelId: event.target.value }))}>
                  <option value="">Select model</option>
                  {models.data?.data.map((model) => <option key={model.id} value={model.id}>{model.canonical_name}</option>)}
                </Select>
              </div>
              <div>
                <Label htmlFor="pricing-provider">Provider</Label>
                <Select id="pricing-provider" value={form.providerId} onChange={(event) => setForm((value) => ({ ...value, providerId: event.target.value }))}>
                  <option value="">Select provider</option>
                  {providers.data?.data.map((provider) => <option key={provider.id} value={provider.id}>{provider.display_name}</option>)}
                </Select>
              </div>
              <div>
                <Label htmlFor="pricing-input">Input / MTok</Label>
                <Input id="pricing-input" inputMode="decimal" value={form.inputPricePerMillionTokens} onChange={(event) => setForm((value) => ({ ...value, inputPricePerMillionTokens: event.target.value }))} />
              </div>
              <div>
                <Label htmlFor="pricing-output">Output / MTok</Label>
                <Input id="pricing-output" inputMode="decimal" value={form.outputPricePerMillionTokens} onChange={(event) => setForm((value) => ({ ...value, outputPricePerMillionTokens: event.target.value }))} />
              </div>
              <div>
                <Label htmlFor="pricing-cache-read">Cache Read / MTok</Label>
                <Input id="pricing-cache-read" inputMode="decimal" value={form.cacheReadPricePerMillionTokens} onChange={(event) => setForm((value) => ({ ...value, cacheReadPricePerMillionTokens: event.target.value }))} />
              </div>
              <div>
                <Label htmlFor="pricing-cache-write">Cache Write / MTok</Label>
                <Input id="pricing-cache-write" inputMode="decimal" value={form.cacheWritePricePerMillionTokens} onChange={(event) => setForm((value) => ({ ...value, cacheWritePricePerMillionTokens: event.target.value }))} />
              </div>
              <div>
                <Label htmlFor="pricing-currency">Currency</Label>
                <Input id="pricing-currency" value={form.currency} onChange={(event) => setForm((value) => ({ ...value, currency: event.target.value }))} />
              </div>
              <div>
                <Label htmlFor="pricing-from">Effective From</Label>
                <Input id="pricing-from" type="datetime-local" value={form.effectiveFromLocal} onChange={(event) => setForm((value) => ({ ...value, effectiveFromLocal: event.target.value }))} />
              </div>
              <div>
                <Label htmlFor="pricing-to">Effective To</Label>
                <Input id="pricing-to" type="datetime-local" value={form.effectiveToLocal} onChange={(event) => setForm((value) => ({ ...value, effectiveToLocal: event.target.value }))} />
              </div>
            </div>
            {formError ? <div className="rounded-xl border border-srapi-error/30 bg-srapi-error/5 p-3 text-xs text-srapi-error">{formError}</div> : null}
            <MutationError error={mutations.create.error} />
            <DialogFooter>
              <Button type="button" variant="ghost" onClick={() => setDialogOpen(false)}>Cancel</Button>
              <Button type="submit" disabled={mutations.create.isPending}>Review Rule</Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
      <Dialog open={Boolean(pricingConfirmation)} onOpenChange={(open) => !open && setPricingConfirmation(null)}>
        <DialogContent>
          <div className="space-y-4">
            <DialogHeader>
              <DialogTitle>Confirm Pricing Rule</DialogTitle>
              <DialogDescription>
                Pricing rules affect billing and cost reports. Type the confirmation phrase before creating this rule.
              </DialogDescription>
            </DialogHeader>
            <div className="grid grid-cols-1 gap-3 text-xs md:grid-cols-2">
              <div className="rounded-xl border border-srapi-border p-3">
                <div className="font-mono font-bold uppercase text-srapi-text-secondary">Model</div>
                <div className="mt-1 text-srapi-text-primary">{pricingConfirmation?.modelLabel}</div>
              </div>
              <div className="rounded-xl border border-srapi-border p-3">
                <div className="font-mono font-bold uppercase text-srapi-text-secondary">Provider</div>
                <div className="mt-1 text-srapi-text-primary">{pricingConfirmation?.providerLabel}</div>
              </div>
            </div>
            <div className="rounded-xl border border-srapi-border bg-srapi-card-muted p-3 font-mono text-xs font-bold text-srapi-text-primary">
              {pricingConfirmation?.phrase}
            </div>
            <Label htmlFor="pricing-create-confirmation">Confirmation</Label>
            <Input
              id="pricing-create-confirmation"
              value={pricingConfirmation?.confirmation ?? ""}
              onChange={(event) => setPricingConfirmation((value) => value ? { ...value, confirmation: event.target.value } : value)}
              placeholder={pricingConfirmation?.phrase}
            />
            <MutationError error={mutations.create.error} />
            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => setPricingConfirmation(null)}>Cancel</Button>
              <Button
                type="button"
                variant="danger"
                disabled={!canConfirmPricingRuleCreate(pricingConfirmation) || mutations.create.isPending}
                onClick={confirmCreatePricingRule}
              >
                Create Rule
              </Button>
            </DialogFooter>
          </div>
        </DialogContent>
      </Dialog>
    </AdminShell>
  );
}

export function AdminChannelsMonitorProductionPage() {
  const [q, setQ] = useState("");
  const [status, setStatus] = useState("all");
  const providers = useAdminProviders({ page: 1, q, status });
  const accounts = useAdminAccounts({ page: 1 });
  const mutations = useAdminProviderMutations();
  const [dialogOpen, setDialogOpen] = useState(false);
  const [editing, setEditing] = useState<Provider | null>(null);
  const [form, setForm] = useState<ProviderFormState>(() => emptyProviderForm());
  const [formError, setFormError] = useState<string | null>(null);
  const [testResult, setTestResult] = useState<AdminTestResult | null>(null);

  const activeProviders = providers.data?.data.filter((provider) => provider.status === "active") ?? [];
  const activeAccounts = accounts.data?.data.filter((account) => account.status === "active") ?? [];

  const openCreate = () => {
    setEditing(null);
    setForm(emptyProviderForm());
    setFormError(null);
    setDialogOpen(true);
  };
  const openEdit = (provider: Provider) => {
    setEditing(provider);
    setForm(providerFormFromProvider(provider));
    setFormError(null);
    setDialogOpen(true);
  };
  const submitProvider = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setFormError(null);
    if (editing) {
      let body: Parameters<typeof mutations.update.mutate>[0]["body"];
      try {
        body = buildUpdateProviderBody(form);
      } catch (error) {
        setFormError(error instanceof Error ? error.message : "Provider is invalid.");
        return;
      }
      mutations.update.mutate(
        { id: editing.id, body },
        {
          onSuccess: () => {
            setDialogOpen(false);
            setEditing(null);
          },
        },
      );
      return;
    }

    let body: Parameters<typeof mutations.create.mutate>[0];
    try {
      body = buildCreateProviderBody(form);
    } catch (error) {
      setFormError(error instanceof Error ? error.message : "Provider is invalid.");
      return;
    }
    mutations.create.mutate(body, {
      onSuccess: () => {
        setDialogOpen(false);
        setForm(emptyProviderForm());
      },
    });
  };
  const testProvider = (provider: Provider) => {
    setTestResult(null);
    mutations.test.mutate(provider, {
      onSuccess: setTestResult,
    });
  };

  return (
    <AdminShell>
      <AdminPageHeader
        title="Channel Monitor"
        description="Provider registry and account status from the production admin API. Channel health is not simulated."
        actions={<Button type="button" size="sm" onClick={openCreate}><Plus size={12} />Create Provider</Button>}
      />
      <div className="grid grid-cols-1 gap-4 md:grid-cols-3">
        <AdminStatCard label="Providers" value={formatInteger(providers.data?.data.length)} icon={<Layers size={16} />} />
        <AdminStatCard label="Active Providers" value={formatInteger(activeProviders.length)} tone="success" />
        <AdminStatCard label="Accounts" value={formatInteger(accounts.data?.data.length)} icon={<Key size={16} />} />
        <AdminStatCard label="Active Accounts" value={formatInteger(activeAccounts.length)} tone="success" />
      </div>
      <AdminSection
        title="Provider Registry"
        description="Create, edit and test provider channels before binding accounts."
        actions={
          <>
            <div className="relative">
              <Search className="pointer-events-none absolute left-3 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-srapi-text-secondary" />
              <Input className="pl-9" value={q} onChange={(event) => setQ(event.target.value)} placeholder="Search provider" />
            </div>
            <Select value={status} onChange={(event) => setStatus(event.target.value)}>
              <option value="all">All status</option>
              {RESOURCE_STATUSES.map((item) => <option key={item} value={item}>{item}</option>)}
            </Select>
          </>
        }
      >
        {providers.isLoading ? <AdminLoadingState label="Loading providers..." /> : null}
        {providers.isError ? <AdminErrorState error={providers.error} onRetry={() => void providers.refetch()} /> : null}
        {providers.data ? (
          <>
            <AdminTable
              empty={<AdminEmptyState title="No providers" description="Create a provider before adding provider accounts." />}
              columns={[
                { key: "name", header: "Name" },
                { key: "adapter", header: "Adapter" },
                { key: "protocol", header: "Protocol" },
                { key: "status", header: "Status" },
                { key: "created", header: "Created" },
                { key: "actions", header: "" },
              ]}
              rows={providers.data.data.map((provider) => ({
                name: (
                  <div>
                    <div className="font-medium text-srapi-text-primary">{provider.display_name}</div>
                    <div className="font-mono text-[11px] text-srapi-text-secondary">{provider.name}</div>
                  </div>
                ),
                adapter: provider.adapter_type,
                protocol: provider.protocol,
                status: <AdminStatusBadge status={provider.status} />,
                created: formatDateTime(provider.created_at),
                actions: (
                  <div className="flex flex-wrap justify-end gap-2">
                    <Button type="button" variant="outline" size="sm" onClick={() => testProvider(provider)} disabled={mutations.test.isPending}>
                      Test
                    </Button>
                    <Button type="button" variant="outline" size="sm" onClick={() => openEdit(provider)}>
                      Edit
                    </Button>
                  </div>
                ),
              }))}
              getRowKey={(_, index) => providers.data?.data[index]?.id ?? String(index)}
            />
            <AdminPaginationSummary pagination={providers.data.pagination} />
          </>
        ) : null}
        <MutationError error={mutations.create.error || mutations.update.error || mutations.test.error} />
        <ProviderTestResultPanel result={testResult} />
      </AdminSection>
      <SimpleResourceTable<ProviderAccount>
        title="Account Status"
        loading={accounts.isLoading}
        error={accounts.error}
        onRetry={() => void accounts.refetch()}
        rows={accounts.data?.data}
        pagination={accounts.data?.pagination}
        getRowKey={(account) => account.id}
        columns={[
          { key: "name", header: "Account", render: (account) => account.name },
          { key: "provider", header: "Provider", render: (account) => account.provider_id },
          { key: "runtime", header: "Runtime", render: (account) => account.runtime_class },
          { key: "status", header: "Status", render: (account) => <AdminStatusBadge status={account.status} /> },
        ]}
      />
      <Dialog
        open={dialogOpen}
        onOpenChange={(open) => {
          setDialogOpen(open);
          if (!open) {
            setEditing(null);
            setFormError(null);
          }
        }}
      >
        <DialogContent>
          <form className="space-y-4" onSubmit={submitProvider}>
            <DialogHeader>
              <DialogTitle>{editing ? "Edit Provider" : "Create Provider"}</DialogTitle>
              <DialogDescription>
                Provider records define adapter/protocol capabilities. Credentials stay on provider accounts, not here.
              </DialogDescription>
            </DialogHeader>
            <ProviderRegistryFormFields form={form} onChange={setForm} editing={Boolean(editing)} />
            {formError ? <div className="rounded-xl border border-srapi-error/30 bg-srapi-error/5 p-3 text-xs text-srapi-error">{formError}</div> : null}
            <MutationError error={mutations.create.error || mutations.update.error} />
            <DialogFooter>
              <Button type="button" variant="ghost" onClick={() => setDialogOpen(false)}>Cancel</Button>
              <Button type="submit" disabled={mutations.create.isPending || mutations.update.isPending}>
                {editing ? "Save Provider" : "Create Provider"}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </AdminShell>
  );
}

export function AdminOrdersDashboardProductionPage() {
  const orders = useAdminPaymentOrders({ page: 1 });
  const providers = useAdminPaymentProviders({ page: 1 });
  const providerMutations = useAdminPaymentProviderMutations();
  const [providerDialogOpen, setProviderDialogOpen] = useState(false);
  const [providerForm, setProviderForm] = useState<PaymentProviderFormState>(() => emptyPaymentProviderForm());
  const [formError, setFormError] = useState<string | null>(null);
  const paid = orders.data?.data.filter((order) => ["paid", "fulfilled", "partially_refunded"].includes(order.status)) ?? [];
  const revenue = sumOrderAmounts(paid);
  const activeProviders = providers.data?.data.filter((provider) => provider.status === "active") ?? [];

  const openProviderDialog = () => {
    setProviderForm(emptyPaymentProviderForm());
    setFormError(null);
    setProviderDialogOpen(true);
  };

  const submitProvider = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setFormError(null);
    let body: Parameters<typeof providerMutations.create.mutate>[0];
    try {
      body = buildCreatePaymentProviderBody(providerForm);
    } catch (error) {
      setFormError(error instanceof Error ? error.message : "Payment provider is invalid.");
      return;
    }
    providerMutations.create.mutate(body, {
      onSuccess: () => {
        setProviderDialogOpen(false);
        setProviderForm(emptyPaymentProviderForm());
      },
    });
  };

  return (
    <AdminShell>
      <AdminPageHeader
        title="Payment Dashboard"
        description="Payment overview from production order and payment-provider endpoints."
        actions={
          <Button type="button" variant="outline" size="sm" onClick={openProviderDialog}>
            <Plus size={12} />
            New Provider
          </Button>
        }
      />
      <div className="grid grid-cols-1 gap-4 md:grid-cols-4">
        <AdminStatCard label="Orders Loaded" value={formatInteger(orders.data?.data.length)} />
        <AdminStatCard label="Paid/Fulfilled" value={formatInteger(paid.length)} tone="success" />
        <AdminStatCard label="Revenue Loaded" value={formatMoney(revenue)} icon={<DollarSign size={16} />} />
        <AdminStatCard label="Active Providers" value={formatInteger(activeProviders.length)} detail={`${formatInteger(providers.data?.data.length)} loaded`} />
      </div>
      <SimpleResourceTable<PaymentProviderInstance>
        title="Payment Providers"
        description="Encrypted provider instances. Secrets are sent only on create and are not read back into this table."
        loading={providers.isLoading}
        error={providers.error}
        onRetry={() => void providers.refetch()}
        rows={providers.data?.data}
        pagination={providers.data?.pagination}
        getRowKey={(provider) => provider.id}
        columns={[
          { key: "name", header: "Name", render: (provider) => provider.name },
          { key: "provider", header: "Provider", render: (provider) => provider.provider },
          { key: "methods", header: "Methods", render: (provider) => provider.supported_methods.join(", ") || "none" },
          { key: "status", header: "Status", render: (provider) => <AdminStatusBadge status={provider.status} /> },
          { key: "version", header: "Config", render: (provider) => `v${provider.config_version}` },
        ]}
      />
      <MutationError error={providerMutations.create.error} />
      <Dialog open={providerDialogOpen} onOpenChange={setProviderDialogOpen}>
        <DialogContent>
          <form className="space-y-4" onSubmit={submitProvider}>
            <DialogHeader>
              <DialogTitle>Create Payment Provider</DialogTitle>
              <DialogDescription>
                Provider config is sent to the admin API for encrypted storage. It is not shown again after save.
              </DialogDescription>
            </DialogHeader>
            <PaymentProviderFormFields form={providerForm} onChange={setProviderForm} />
            {formError ? <div className="rounded-xl border border-srapi-error/30 bg-srapi-error/5 p-3 text-xs text-srapi-error">{formError}</div> : null}
            <MutationError error={providerMutations.create.error} />
            <DialogFooter>
              <Button type="button" variant="ghost" onClick={() => setProviderDialogOpen(false)}>Cancel</Button>
              <Button type="submit" disabled={providerMutations.create.isPending}>Create Provider</Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </AdminShell>
  );
}

export function AdminOrdersProductionPage() {
  const [status, setStatus] = useState<PaymentOrderStatus | "all">("all");
  const orders = useAdminPaymentOrders({ page: 1, status });
  const mutations = useAdminPaymentOrderMutations();
  const [refundForm, setRefundForm] = useState<RefundOrderFormState | null>(null);
  const [formError, setFormError] = useState<string | null>(null);

  const openRefund = (order: PaymentOrder) => {
    setRefundForm(refundFormFromOrder(order));
    setFormError(null);
  };
  const submitRefund = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!refundForm) {
      return;
    }
    setFormError(null);
    let payload: ReturnType<typeof buildRefundPaymentOrderBody>;
    try {
      payload = buildRefundPaymentOrderBody(refundForm);
    } catch (error) {
      setFormError(error instanceof Error ? error.message : "Refund request is invalid.");
      return;
    }
    mutations.refund.mutate(
      { id: payload.id, body: { amount: payload.amount, reason: payload.reason } },
      {
        onSuccess: () => {
          setRefundForm(null);
        },
      },
    );
  };

  return (
    <AdminShell>
      <AdminPageHeader title="Orders" description="Payment orders with explicit refund confirmation routed to the generated SDK." />
      <AdminSection
        title="Orders"
        actions={
          <Select value={status} onChange={(event) => setStatus(event.target.value as PaymentOrderStatus | "all")}>
            <option value="all">All status</option>
            <option value="pending">Pending</option>
            <option value="paid">Paid</option>
            <option value="fulfilled">Fulfilled</option>
            <option value="failed">Failed</option>
            <option value="refunded">Refunded</option>
          </Select>
        }
      >
        {orders.isLoading ? <AdminLoadingState label="Loading orders..." /> : null}
        {orders.isError ? <AdminErrorState error={orders.error} onRetry={() => void orders.refetch()} /> : null}
        {orders.data ? (
          <AdminTable
            empty={<AdminEmptyState title="No orders" />}
            columns={[
              { key: "order", header: "Order" },
              { key: "user", header: "User" },
              { key: "amount", header: "Amount" },
              { key: "product", header: "Product" },
              { key: "status", header: "Status" },
              { key: "created", header: "Created" },
              { key: "actions", header: "" },
            ]}
            rows={orders.data.data.map((order: PaymentOrder) => ({
              order: <span className="select-all">{order.order_no}</span>,
              user: order.user_id,
              amount: formatMoney(order.amount, order.currency),
              product: `${order.product_type}:${order.product_id}`,
              status: <AdminStatusBadge status={order.status} />,
              created: formatDateTime(order.created_at),
              actions: (
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  disabled={!isRefundableOrder(order)}
                  onClick={() => openRefund(order)}
                >
                  Refund
                </Button>
              ),
            }))}
            getRowKey={(_, index) => orders.data?.data[index]?.id ?? String(index)}
          />
        ) : null}
        <MutationError error={mutations.refund.error} />
      </AdminSection>
      <Dialog open={Boolean(refundForm)} onOpenChange={(open) => !open && setRefundForm(null)}>
        <DialogContent>
          <form className="space-y-4" onSubmit={submitRefund}>
            <DialogHeader>
              <DialogTitle>Confirm Refund</DialogTitle>
              <DialogDescription>
                Refunds are financial operations. Leave amount empty only when issuing a full refund.
              </DialogDescription>
            </DialogHeader>
            {refundForm ? (
              <>
                <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
                  <div className="rounded-xl border border-srapi-border p-4">
                    <div className="font-mono text-[11px] font-bold uppercase text-srapi-text-secondary">Order</div>
                    <div className="mt-2 select-all font-mono text-sm text-srapi-text-primary">{refundForm.orderNo}</div>
                  </div>
                  <div className="rounded-xl border border-srapi-border p-4">
                    <div className="font-mono text-[11px] font-bold uppercase text-srapi-text-secondary">Currency</div>
                    <div className="mt-2 font-mono text-sm text-srapi-text-primary">{refundForm.currency}</div>
                  </div>
                  <div>
                    <Label htmlFor="refund-amount">Refund Amount</Label>
                    <Input
                      id="refund-amount"
                      inputMode="decimal"
                      placeholder="Full refund"
                      value={refundForm.amount}
                      onChange={(event) => setRefundForm((value) => value ? { ...value, amount: event.target.value } : value)}
                    />
                  </div>
                  <div>
                    <Label htmlFor="refund-reason">Reason</Label>
                    <Input
                      id="refund-reason"
                      value={refundForm.reason}
                      onChange={(event) => setRefundForm((value) => value ? { ...value, reason: event.target.value } : value)}
                    />
                  </div>
                </div>
              </>
            ) : null}
            {formError ? <div className="rounded-xl border border-srapi-error/30 bg-srapi-error/5 p-3 text-xs text-srapi-error">{formError}</div> : null}
            <MutationError error={mutations.refund.error} />
            <DialogFooter>
              <Button type="button" variant="ghost" onClick={() => setRefundForm(null)}>Cancel</Button>
              <Button type="submit" variant="danger" disabled={mutations.refund.isPending}>Confirm Refund</Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </AdminShell>
  );
}

export function AdminOrdersPlansProductionPage() {
  const plans = useAdminSubscriptionPlans({ page: 1 });
  const rules = useAdminPricingRules({ page: 1 });
  const providers = useAdminPaymentProviders({ page: 1 });
  const planMutations = useAdminSubscriptionPlanMutations();
  const [planDialogOpen, setPlanDialogOpen] = useState(false);
  const [planForm, setPlanForm] = useState<SubscriptionPlanFormState>(() => emptySubscriptionPlanForm());
  const [formError, setFormError] = useState<string | null>(null);

  const activePlans = plans.data?.data.filter((plan) => plan.status === "active") ?? [];
  const salePlans = activePlans.filter((plan) => plan.for_sale);
  const activePaymentProviders = providers.data?.data.filter((provider) => provider.status === "active") ?? [];

  const submitPlan = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setFormError(null);
    let body: Parameters<typeof planMutations.create.mutate>[0];
    try {
      body = buildCreateSubscriptionPlanBody(planForm);
    } catch (error) {
      setFormError(error instanceof Error ? error.message : "Plan is invalid.");
      return;
    }
    planMutations.create.mutate(body, {
      onSuccess: () => {
        setPlanDialogOpen(false);
        setPlanForm(emptySubscriptionPlanForm());
      },
    });
  };

  return (
    <AdminShell>
      <AdminPageHeader
        title="Order Plans"
        description="Sales catalog for subscription-plan products, backed by subscription plans, pricing rules and payment-provider instances."
        actions={
          <Button type="button" variant="outline" size="sm" onClick={() => setPlanDialogOpen(true)}>
            <Plus size={12} />
            New Plan
          </Button>
        }
      />
      <div className="grid grid-cols-1 gap-4 md:grid-cols-4">
        <AdminStatCard label="Sale Plans" value={formatInteger(salePlans.length)} tone="success" />
        <AdminStatCard label="Active Plans" value={formatInteger(activePlans.length)} />
        <AdminStatCard label="Pricing Rules" value={formatInteger(rules.data?.data.length)} />
        <AdminStatCard label="Active Payment Providers" value={formatInteger(activePaymentProviders.length)} />
      </div>
      <SimpleResourceTable<SubscriptionPlan>
        title="Catalog Plans"
        description="Plans sold through orders use product_type=subscription_plan and keep monetary amounts as decimal strings."
        loading={plans.isLoading}
        error={plans.error}
        onRetry={() => void plans.refetch()}
        rows={plans.data?.data}
        pagination={plans.data?.pagination}
        getRowKey={(plan) => plan.id}
        columns={[
          { key: "name", header: "Name", render: (plan) => plan.name },
          { key: "price", header: "Price", render: (plan) => formatMoney(plan.price, plan.currency) },
          { key: "validity", header: "Validity", render: (plan) => `${plan.validity_days} days` },
          { key: "sale", header: "For Sale", render: (plan) => <Badge variant={plan.for_sale ? "success" : "neutral"}>{String(plan.for_sale)}</Badge> },
          { key: "status", header: "Status", render: (plan) => <AdminStatusBadge status={plan.status} /> },
        ]}
      />
      <div className="grid grid-cols-1 gap-4 xl:grid-cols-2">
        <SimpleResourceTable<PricingRule>
          title="Pricing Coverage"
          description="Model/provider pricing used by cost reporting and order margin review."
          loading={rules.isLoading}
          error={rules.error}
          onRetry={() => void rules.refetch()}
          rows={rules.data?.data}
          pagination={rules.data?.pagination}
          getRowKey={(rule) => rule.id}
          columns={[
            { key: "model", header: "Model", render: (rule) => rule.model_id },
            { key: "provider", header: "Provider", render: (rule) => rule.provider_id },
            { key: "input", header: "Input / MTok", render: (rule) => formatMoney(rule.input_price_per_million_tokens, rule.currency) },
            { key: "output", header: "Output / MTok", render: (rule) => formatMoney(rule.output_price_per_million_tokens, rule.currency) },
          ]}
        />
        <SimpleResourceTable<PaymentProviderInstance>
          title="Checkout Providers"
          description="Active providers determine which checkout methods can create orders."
          loading={providers.isLoading}
          error={providers.error}
          onRetry={() => void providers.refetch()}
          rows={providers.data?.data}
          pagination={providers.data?.pagination}
          getRowKey={(provider) => provider.id}
          columns={[
            { key: "name", header: "Name", render: (provider) => provider.name },
            { key: "provider", header: "Provider", render: (provider) => provider.provider },
            { key: "methods", header: "Methods", render: (provider) => provider.supported_methods.join(", ") || "none" },
            { key: "status", header: "Status", render: (provider) => <AdminStatusBadge status={provider.status} /> },
          ]}
        />
      </div>
      <MutationError error={planMutations.create.error} />
      <Dialog
        open={planDialogOpen}
        onOpenChange={(open) => {
          setPlanDialogOpen(open);
          if (!open) {
            setFormError(null);
          }
        }}
      >
        <DialogContent>
          <form className="space-y-4" onSubmit={submitPlan}>
            <DialogHeader>
              <DialogTitle>Create Order Plan</DialogTitle>
              <DialogDescription>Create a subscription-plan product that can be sold through payment orders.</DialogDescription>
            </DialogHeader>
            <SubscriptionPlanFormFields form={planForm} onChange={setPlanForm} />
            {formError ? <div className="rounded-xl border border-srapi-error/30 bg-srapi-error/5 p-3 text-xs text-srapi-error">{formError}</div> : null}
            <MutationError error={planMutations.create.error} />
            <DialogFooter>
              <Button type="button" variant="ghost" onClick={() => setPlanDialogOpen(false)}>Cancel</Button>
              <Button type="submit" disabled={planMutations.create.isPending}>Create Plan</Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </AdminShell>
  );
}

export function AdminRiskControlProductionPage() {
  const risk = useAdminRiskControl({ page: 1 });
  const mutations = useAdminRiskMutations();
  const [dialogOpen, setDialogOpen] = useState(false);
  const [activeTab, setActiveTab] = useState<RiskControlTab>("basic");
  const [draft, setDraft] = useState<RiskControlFormState | null>(null);
  const [pendingConfig, setPendingConfig] = useState<RiskControlConfig | null>(null);
  const [saveConfirmation, setSaveConfirmation] = useState<RiskControlSaveConfirmationState | null>(null);
  const [formError, setFormError] = useState<string | null>(null);

  const openConfig = (config: RiskControlConfig) => {
    setDraft(createRiskControlForm(config));
    setActiveTab("basic");
    setFormError(null);
    setDialogOpen(true);
  };
  const updateDraft = (updater: (form: RiskControlFormState) => RiskControlFormState) => {
    setDraft((current) => (current ? updateRiskControlForm(current, updater) : current));
  };
  const submitConfig = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!draft) {
      return;
    }
    setFormError(null);
    let body: RiskControlConfig;
    try {
      body = buildRiskControlConfig(draft);
    } catch (error) {
      setFormError(error instanceof Error ? error.message : "Risk-control config is invalid.");
      return;
    }
    setPendingConfig(body);
    setSaveConfirmation(createRiskControlSaveConfirmation());
  };
  const confirmRiskConfigSave = () => {
    if (!pendingConfig || !canConfirmRiskControlSave(saveConfirmation)) {
      return;
    }
    mutations.updateConfig.mutate(pendingConfig, {
      onSuccess: () => {
        setDialogOpen(false);
        setDraft(null);
        setPendingConfig(null);
        setSaveConfirmation(null);
      },
    });
  };

  return (
    <AdminShell>
      <AdminPageHeader title="Risk Control" description="Risk-control status, config and records from the production risk endpoints." />
      {risk.config.isLoading || risk.status.isLoading ? <AdminLoadingState label="Loading risk-control state..." /> : null}
      {risk.config.isError ? <AdminErrorState error={risk.config.error} onRetry={() => void risk.config.refetch()} /> : null}
      {risk.status.isError ? <AdminErrorState error={risk.status.error} onRetry={() => void risk.status.refetch()} /> : null}
      {risk.status.data ? (
        <div className="grid grid-cols-1 gap-4 md:grid-cols-4">
          <AdminStatCard label="Enabled" value={String(risk.status.data.enabled)} tone={risk.status.data.enabled ? "success" : "warning"} icon={<Shield size={16} />} />
          <AdminStatCard label="Mode" value={risk.status.data.mode} />
          <AdminStatCard label="Active Blocks" value={formatInteger(risk.status.data.active_blocks)} tone={risk.status.data.active_blocks > 0 ? "warning" : "neutral"} />
          <AdminStatCard label="Recent Events" value={formatInteger(risk.status.data.recent_events)} />
        </div>
      ) : null}
      {risk.config.data ? (
        <AdminSection
          title="Config"
          description="Contract-backed runtime risk settings. Lists accept newline or comma separated values."
          actions={
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={() => {
                if (risk.config.data) {
                  openConfig(risk.config.data);
                }
              }}
            >
              <Save size={12} />
              Edit Config
            </Button>
          }
        >
          <div className="grid grid-cols-1 gap-4 md:grid-cols-3">
            <AdminStatCard label="Failed Requests / Min" value={formatInteger(risk.config.data.max_failed_requests_per_minute)} />
            <AdminStatCard label="Daily Cost Limit" value={formatMoney(risk.config.data.max_cost_per_day)} icon={<DollarSign size={16} />} />
            <AdminStatCard label="Cooldown" value={`${formatInteger(risk.config.data.cooldown_seconds)}s`} />
          </div>
          <div className="mt-4 grid grid-cols-1 gap-4 md:grid-cols-2">
            <div className="rounded-2xl border border-srapi-border bg-srapi-bg p-4">
              <p className="font-mono text-[11px] font-bold uppercase tracking-[0.2em] text-srapi-text-secondary">Blocked Countries</p>
              <p className="mt-2 text-sm text-srapi-text-primary">
                {(risk.config.data.blocked_countries ?? []).length > 0 ? (risk.config.data.blocked_countries ?? []).join(", ") : "None"}
              </p>
            </div>
            <div className="rounded-2xl border border-srapi-border bg-srapi-bg p-4">
              <p className="font-mono text-[11px] font-bold uppercase tracking-[0.2em] text-srapi-text-secondary">Blocked IPs</p>
              <p className="mt-2 text-sm text-srapi-text-primary">
                {(risk.config.data.blocked_ips ?? []).length > 0 ? (risk.config.data.blocked_ips ?? []).join(", ") : "None"}
              </p>
            </div>
          </div>
          <MutationError error={mutations.updateConfig.error} />
        </AdminSection>
      ) : null}
      <SimpleResourceTable
        title="Risk Events"
        loading={risk.logs.isLoading}
        error={risk.logs.error}
        onRetry={() => void risk.logs.refetch()}
        rows={risk.logs.data?.data}
        pagination={risk.logs.data?.pagination}
        getRowKey={(log) => log.id}
        columns={[
          { key: "time", header: "Time", render: (log) => formatDateTime(log.created_at) },
          { key: "level", header: "Level", render: (log) => <AdminStatusBadge status={log.level} /> },
          { key: "action", header: "Action", render: (log) => log.action },
          { key: "reason", header: "Reason", render: (log) => <span className="whitespace-normal">{log.reason}</span> },
          { key: "subject", header: "Subject", render: (log) => log.subject || "-" },
        ]}
      />
      <Dialog
        open={dialogOpen}
        onOpenChange={(open) => {
          setDialogOpen(open);
          if (!open) {
            setPendingConfig(null);
            setSaveConfirmation(null);
          }
        }}
      >
        <DialogContent className="max-w-4xl">
          <form onSubmit={submitConfig}>
            <DialogHeader>
              <DialogTitle>Edit Risk-Control Config</DialogTitle>
              <DialogDescription>
                These fields map directly to the current risk-control OpenAPI contract.
              </DialogDescription>
            </DialogHeader>
            {draft ? (
              <>
                <div className="mt-4 flex flex-wrap gap-2">
                  {RISK_CONTROL_TABS.map((tab) => (
                    <Button
                      key={tab.id}
                      type="button"
                      variant={activeTab === tab.id ? "primary" : "outline"}
                      size="sm"
                      onClick={() => setActiveTab(tab.id)}
                    >
                      {tab.label}
                    </Button>
                  ))}
                </div>
                {activeTab === "basic" ? (
                  <div className="mt-4 grid grid-cols-1 gap-4 md:grid-cols-2">
                    <SettingsToggle
                      label="Risk Control Enabled"
                      checked={draft.enabled}
                      onChange={(checked) => updateDraft((current) => ({ ...current, enabled: checked }))}
                    />
                    <div>
                      <Label htmlFor="risk-mode">Mode</Label>
                      <Select
                        id="risk-mode"
                        value={draft.mode}
                        onChange={(event) => updateDraft((current) => ({ ...current, mode: event.target.value as RiskControlConfig["mode"] }))}
                      >
                        <option value="monitor">Monitor</option>
                        <option value="enforce">Enforce</option>
                      </Select>
                    </div>
                  </div>
                ) : null}
                {activeTab === "limits" ? (
                  <div className="mt-4 grid grid-cols-1 gap-4 md:grid-cols-3">
                    <div>
                      <Label htmlFor="risk-max-failed">Max Failed Requests / Minute</Label>
                      <Input
                        id="risk-max-failed"
                        type="number"
                        min="0"
                        value={draft.maxFailedRequestsPerMinute}
                        onChange={(event) => updateDraft((current) => ({ ...current, maxFailedRequestsPerMinute: event.target.value }))}
                      />
                    </div>
                    <div>
                      <Label htmlFor="risk-cost-limit">Max Cost / Day</Label>
                      <Input
                        id="risk-cost-limit"
                        inputMode="decimal"
                        value={draft.maxCostPerDay}
                        onChange={(event) => updateDraft((current) => ({ ...current, maxCostPerDay: event.target.value }))}
                      />
                    </div>
                    <div>
                      <Label htmlFor="risk-cooldown">Cooldown Seconds</Label>
                      <Input
                        id="risk-cooldown"
                        type="number"
                        min="0"
                        value={draft.cooldownSeconds}
                        onChange={(event) => updateDraft((current) => ({ ...current, cooldownSeconds: event.target.value }))}
                      />
                    </div>
                  </div>
                ) : null}
                {activeTab === "scope" ? (
                  <div className="mt-4 grid grid-cols-1 gap-4 md:grid-cols-2">
                    <div>
                      <Label htmlFor="risk-countries">Blocked Countries</Label>
                      <Textarea
                        id="risk-countries"
                        rows={8}
                        value={draft.blockedCountriesText}
                        onChange={(event) => updateDraft((current) => ({ ...current, blockedCountriesText: event.target.value }))}
                      />
                    </div>
                    <div>
                      <Label htmlFor="risk-ips">Blocked IPs</Label>
                      <Textarea
                        id="risk-ips"
                        rows={8}
                        value={draft.blockedIpsText}
                        onChange={(event) => updateDraft((current) => ({ ...current, blockedIpsText: event.target.value }))}
                      />
                    </div>
                  </div>
                ) : null}
              </>
            ) : null}
            {formError ? (
              <div className="mt-4 rounded-xl border border-srapi-error/30 bg-srapi-error/5 p-3 text-xs text-srapi-error">
                {formError}
              </div>
            ) : null}
            <div className="mt-4">
              <MutationError error={mutations.updateConfig.error} />
            </div>
            <DialogFooter>
              <Button type="button" variant="ghost" onClick={() => setDialogOpen(false)}>
                Cancel
              </Button>
              <Button type="submit" disabled={mutations.updateConfig.isPending}>
                Review Save
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
      <Dialog open={Boolean(saveConfirmation)} onOpenChange={(open) => !open && setSaveConfirmation(null)}>
        <DialogContent>
          <div className="space-y-4">
            <DialogHeader>
              <DialogTitle>Confirm Risk-Control Config</DialogTitle>
              <DialogDescription>
                These settings can block traffic or change enforcement behavior. Type the confirmation phrase before saving.
              </DialogDescription>
            </DialogHeader>
            <div className="rounded-xl border border-srapi-border bg-srapi-card-muted p-3 font-mono text-xs font-bold text-srapi-text-primary">
              {saveConfirmation?.phrase}
            </div>
            <Label htmlFor="risk-save-confirmation">Confirmation</Label>
            <Input
              id="risk-save-confirmation"
              value={saveConfirmation?.confirmation ?? ""}
              onChange={(event) => setSaveConfirmation((value) => value ? { ...value, confirmation: event.target.value } : value)}
              placeholder={saveConfirmation?.phrase}
            />
            <MutationError error={mutations.updateConfig.error} />
            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => setSaveConfirmation(null)}>Cancel</Button>
              <Button
                type="button"
                variant="danger"
                disabled={!canConfirmRiskControlSave(saveConfirmation) || mutations.updateConfig.isPending}
                onClick={confirmRiskConfigSave}
              >
                Save Config
              </Button>
            </DialogFooter>
          </div>
        </DialogContent>
      </Dialog>
    </AdminShell>
  );
}

export function AdminProxiesProductionPage() {
  const page = 1;
  const [status, setStatus] = useState<ProxyDefinitionStatus | "all">("all");
  const [drafts, setDrafts] = useState<Record<string, string>>({});
  const [bindingConfirmation, setBindingConfirmation] = useState<AccountProxyBindingConfirmationState | null>(null);
  const [dialogOpen, setDialogOpen] = useState(false);
  const [editing, setEditing] = useState<ProxyDefinition | null>(null);
  const [form, setForm] = useState<ProxyFormState>(() => emptyProxyForm());
  const [formError, setFormError] = useState<string | null>(null);
  const proxies = useAdminProxies({ page, status });
  const proxyMutations = useAdminProxyMutations();
  const accounts = useAdminAccounts({ page, status: "all" });
  const proxyRows = useMemo(() => proxies.data?.data ?? [], [proxies.data?.data]);
  const accountRows = useMemo(() => accounts.data?.data ?? [], [accounts.data?.data]);
  const accountIds = useMemo(() => accountRows.map((account) => account.id), [accountRows]);
  const quality = useAdminAccountProxyQuality(accountIds);
  const mutations = useAdminAccountProxyMutations();
  const openBindingConfirmation = (account: ProviderAccount, currentProxyId: string, nextProxyId: string | null) => {
    setBindingConfirmation(createAccountProxyBindingConfirmation({
      account,
      currentProxyId,
      nextProxyId,
    }));
  };
  const confirmProxyBinding = () => {
    if (!bindingConfirmation || !canConfirmAccountProxyBinding(bindingConfirmation)) {
      return;
    }
    mutations.bind.mutate(
      { id: bindingConfirmation.accountId, proxyId: bindingConfirmation.nextProxyId },
      {
        onSuccess: () => {
          setDrafts((value) => ({
            ...value,
            [bindingConfirmation.accountId]: bindingConfirmation.nextProxyId ?? "",
          }));
          setBindingConfirmation(null);
        },
      },
    );
  };
  const openCreate = () => {
    setEditing(null);
    setForm(emptyProxyForm());
    setFormError(null);
    setDialogOpen(true);
  };
  const openEdit = (proxy: ProxyDefinition) => {
    setEditing(proxy);
    setForm(proxyFormFromProxy(proxy));
    setFormError(null);
    setDialogOpen(true);
  };
  const submitProxy = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setFormError(null);
    try {
      if (editing) {
        proxyMutations.update.mutate(
          { id: editing.id, body: buildUpdateProxyBody(form) },
          { onSuccess: () => setDialogOpen(false) },
        );
        return;
      }
      proxyMutations.create.mutate(
        buildCreateProxyBody(form),
        { onSuccess: () => setDialogOpen(false) },
      );
    } catch (error) {
      setFormError(error instanceof Error ? error.message : "Proxy form is invalid.");
    }
  };

  return (
    <AdminShell>
      <AdminPageHeader
        title="Proxy Management"
        description="Encrypted egress proxy registry, account bindings, and quality evidence from the production admin contract."
        actions={
          <div className="flex gap-2">
            <Button type="button" variant="outline" size="sm" onClick={() => {
              void proxies.refetch();
              void accounts.refetch();
              void quality.refetch();
            }}>
              <RefreshCw size={12} />
              Refresh
            </Button>
            <Button type="button" size="sm" onClick={openCreate}>
              <Plus size={12} />
              New Proxy
            </Button>
          </div>
        }
      />
      <AdminSection
        title="Proxy Registry"
        description="Proxy URLs are write-only secrets. The API only returns whether an encrypted URL is configured."
        actions={
          <Select value={status} onChange={(event) => setStatus(event.target.value as ProxyDefinitionStatus | "all")}>
            <option value="all">All status</option>
            <option value="active">Active</option>
            <option value="disabled">Disabled</option>
          </Select>
        }
      >
        {proxies.isLoading ? <AdminLoadingState label="Loading proxy registry..." /> : null}
        {proxies.isError ? <AdminErrorState error={proxies.error} onRetry={() => void proxies.refetch()} /> : null}
        {proxyRows.length > 0 ? (
          <>
            <AdminTable
              empty={<AdminEmptyState title="No proxies" />}
              columns={[
                { key: "name", header: "Name" },
                { key: "type", header: "Type" },
                { key: "status", header: "Status" },
                { key: "url", header: "Secret" },
                { key: "metadata", header: "Metadata" },
                { key: "updated", header: "Updated" },
                { key: "action", header: "" },
              ]}
              rows={proxyRows.map((proxy) => ({
                name: (
                  <div>
                    <div className="font-semibold text-srapi-text-primary">{proxy.name}</div>
                    <div className="font-mono text-[10px] text-srapi-text-secondary">{proxy.id}</div>
                  </div>
                ),
                type: <Badge variant="neutral">{proxy.type}</Badge>,
                status: <AdminStatusBadge status={proxy.status} />,
                url: proxy.url_configured ? "Encrypted URL configured" : "Missing URL",
                metadata: <span className="font-mono text-[11px]">{safeJson(proxy.metadata ?? {})}</span>,
                updated: formatDateTime(proxy.updated_at),
                action: (
                  <div className="flex justify-end">
                    <Button type="button" variant="outline" size="sm" onClick={() => openEdit(proxy)}>
                      Edit
                    </Button>
                  </div>
                ),
              }))}
              getRowKey={(_, index) => proxyRows[index]?.id ?? String(index)}
            />
            <AdminPaginationSummary pagination={proxies.data?.pagination} />
          </>
        ) : proxies.isSuccess ? (
          <AdminEmptyState title="No proxies" description="Create a proxy definition before binding accounts." />
        ) : null}
        <MutationError error={proxyMutations.create.error || proxyMutations.update.error} />
      </AdminSection>
      <AdminSection
        title="Provider Account Proxy Bindings"
        description="Bind active registry entries to provider accounts. Existing legacy raw proxy bindings are shown as external values."
      >
        {accounts.isLoading ? <AdminLoadingState label="Loading provider accounts..." /> : null}
        {accounts.isError ? <AdminErrorState error={accounts.error} onRetry={() => void accounts.refetch()} /> : null}
        {quality.isError ? <AdminErrorState error={quality.error} onRetry={() => void quality.refetch()} /> : null}
        {accountRows.length > 0 ? (
          <>
            <AdminTable
              empty={<AdminEmptyState title="No provider accounts" />}
              columns={[
                { key: "account", header: "Account" },
                { key: "status", header: "Status" },
                { key: "proxy", header: "Proxy" },
                { key: "quality", header: "Quality" },
                { key: "action", header: "" },
              ]}
              rows={accountRows.map((account) => {
                const currentQuality = quality.data?.[account.id];
                const currentProxy = currentQuality?.proxy_id ?? "";
                const draft = drafts[account.id] ?? currentProxy;
                const isPending = mutations.bind.isPending;
                const draftExists = draft && proxyRows.some((proxy) => proxy.id === draft);
                return {
                  account: (
                    <div>
                      <div className="font-semibold text-srapi-text-primary">{account.name}</div>
                      <div className="text-[10px] text-srapi-text-secondary">
                        {account.id} / {account.runtime_class}
                      </div>
                    </div>
                  ),
                  status: <AdminStatusBadge status={account.status} />,
                  proxy: (
                    <Select
                      value={draft}
                      onChange={(event) =>
                        setDrafts((value) => ({ ...value, [account.id]: event.target.value }))
                      }
                      className="w-full"
                    >
                      <option value="">No proxy</option>
                      {currentProxy && !draftExists ? (
                        <option value={currentProxy}>Legacy or unavailable: {currentProxy}</option>
                      ) : null}
                      {proxyRows
                        .filter((proxy) => proxy.status === "active")
                        .map((proxy) => (
                          <option key={proxy.id} value={proxy.id}>
                            {proxy.name} ({proxy.type})
                          </option>
                        ))}
                    </Select>
                  ),
                  quality: currentQuality ? (
                    <div className="space-y-1 text-[11px] text-srapi-text-secondary">
                      <div>Success {formatPercent(currentQuality.success_rate)}</div>
                      <div>Error {formatPercent(currentQuality.error_rate)}</div>
                      <div>P95 {formatInteger(currentQuality.latency_p95_ms)}ms / {formatInteger(currentQuality.sample_count)} samples</div>
                    </div>
                  ) : quality.isLoading ? (
                    <span className="text-xs text-srapi-text-secondary">Loading quality...</span>
                  ) : (
                    <span className="text-xs text-srapi-text-secondary">No quality evidence</span>
                  ),
                  action: (
                    <div className="flex justify-end gap-2">
                      <Button
                        type="button"
                        variant="outline"
                        size="sm"
                        disabled={isPending}
                        onClick={() => openBindingConfirmation(account, currentProxy, draft.trim() ? draft.trim() : null)}
                      >
                        Save
                      </Button>
                      <Button
                        type="button"
                        variant="ghost"
                        size="sm"
                        disabled={isPending || !currentProxy}
                        onClick={() => openBindingConfirmation(account, currentProxy, null)}
                      >
                        Clear
                      </Button>
                    </div>
                  ),
                };
              })}
              getRowKey={(_, index) => accountRows[index]?.id ?? String(index)}
            />
            <AdminPaginationSummary pagination={accounts.data?.pagination} />
          </>
        ) : accounts.isSuccess ? (
          <AdminEmptyState title="No provider accounts" description="Create provider accounts before binding proxy ids." />
        ) : null}
        <MutationError error={mutations.bind.error} />
      </AdminSection>
      <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
        <DialogContent className="max-w-2xl">
          <form className="space-y-4" onSubmit={submitProxy}>
            <DialogHeader>
              <DialogTitle>{editing ? "Edit Proxy" : "Create Proxy"}</DialogTitle>
              <DialogDescription>Proxy URL is write-only. Leave it blank while editing to keep the current encrypted URL.</DialogDescription>
            </DialogHeader>
            <Label htmlFor="proxy-name">Name</Label>
            <Input id="proxy-name" required value={form.name} onChange={(event) => setForm((value) => ({ ...value, name: event.target.value }))} />
            <div className="grid grid-cols-1 gap-3 md:grid-cols-2">
              <div className="space-y-2">
                <Label htmlFor="proxy-type">Type</Label>
                <Select id="proxy-type" value={form.type} onChange={(event) => setForm((value) => ({ ...value, type: event.target.value as ProxyDefinition["type"] }))}>
                  {PROXY_TYPES.map((type) => (
                    <option key={type} value={type}>{type.toUpperCase()}</option>
                  ))}
                </Select>
              </div>
              <div className="space-y-2">
                <Label htmlFor="proxy-status">Status</Label>
                <Select id="proxy-status" value={form.status} onChange={(event) => setForm((value) => ({ ...value, status: event.target.value as ProxyDefinition["status"] }))}>
                  {PROXY_STATUSES.map((proxyStatus) => (
                    <option key={proxyStatus} value={proxyStatus}>{proxyStatus}</option>
                  ))}
                </Select>
              </div>
            </div>
            <Label htmlFor="proxy-url">Proxy URL</Label>
            <Input
              id="proxy-url"
              required={!editing}
              value={form.url}
              placeholder={editing ? "Leave blank to keep encrypted URL" : "http://user:pass@proxy.example:8080"}
              onChange={(event) => setForm((value) => ({ ...value, url: event.target.value }))}
            />
            <Label htmlFor="proxy-metadata">Metadata JSON</Label>
            <Textarea id="proxy-metadata" rows={5} value={form.metadata} onChange={(event) => setForm((value) => ({ ...value, metadata: event.target.value }))} />
            {formError ? (
              <div className="rounded-xl border border-srapi-error/30 bg-srapi-error/5 p-3 text-xs text-srapi-error">
                {formError}
              </div>
            ) : null}
            <MutationError error={proxyMutations.create.error || proxyMutations.update.error} />
            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => setDialogOpen(false)}>Cancel</Button>
              <Button type="submit" disabled={proxyMutations.create.isPending || proxyMutations.update.isPending}>
                <Save size={12} />
                Save Proxy
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
      <Dialog open={Boolean(bindingConfirmation)} onOpenChange={(open) => !open && setBindingConfirmation(null)}>
        <DialogContent>
          <div className="space-y-4">
            <DialogHeader>
              <DialogTitle>{bindingConfirmation?.action === "clear" ? "Clear Account Proxy" : "Bind Account Proxy"}</DialogTitle>
              <DialogDescription>
                Proxy binding changes affect egress routing for this provider account. Type the confirmation phrase before applying it.
              </DialogDescription>
            </DialogHeader>
            <div className="grid grid-cols-1 gap-3 text-xs md:grid-cols-2">
              <div className="rounded-xl border border-srapi-border p-3">
                <div className="font-mono font-bold uppercase text-srapi-text-secondary">Account</div>
                <div className="mt-1 text-srapi-text-primary">{bindingConfirmation?.accountName}</div>
              </div>
              <div className="rounded-xl border border-srapi-border p-3">
                <div className="font-mono font-bold uppercase text-srapi-text-secondary">Next Proxy</div>
                <div className="mt-1 font-mono text-srapi-text-primary">{bindingConfirmation?.nextProxyId ?? "No proxy"}</div>
              </div>
            </div>
            <div className="rounded-xl border border-srapi-border bg-srapi-card-muted p-3 font-mono text-xs font-bold text-srapi-text-primary">
              {bindingConfirmation?.phrase}
            </div>
            <Label htmlFor="account-proxy-confirmation">Confirmation</Label>
            <Input
              id="account-proxy-confirmation"
              value={bindingConfirmation?.confirmation ?? ""}
              onChange={(event) => setBindingConfirmation((value) => value ? { ...value, confirmation: event.target.value } : value)}
              placeholder={bindingConfirmation?.phrase}
            />
            <MutationError error={mutations.bind.error} />
            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => setBindingConfirmation(null)}>Cancel</Button>
              <Button
                type="button"
                variant={bindingConfirmation?.action === "clear" ? "danger" : "primary"}
                disabled={!canConfirmAccountProxyBinding(bindingConfirmation) || mutations.bind.isPending}
                onClick={confirmProxyBinding}
              >
                {bindingConfirmation?.action === "clear" ? "Clear Proxy" : "Bind Proxy"}
              </Button>
            </DialogFooter>
          </div>
        </DialogContent>
      </Dialog>
    </AdminShell>
  );
}

export function AdminAffiliateRecordsProductionPage({ kind }: { kind: "invites" | "rebates" | "transfers" }) {
  if (kind === "invites") {
    return <AdminAffiliateInvitesPage />;
  }
  if (kind === "rebates") {
    return <AdminAffiliateRebatesPage />;
  }
  return <AdminAffiliateTransfersPage />;
}

function AdminAffiliateInvitesPage() {
  const invites = useAdminAffiliateInvites({ page: 1 });

  return (
    <AdminShell>
      <AdminPageHeader
        title="Affiliate Invites"
        description="Real invite relationships from /api/v1/admin/affiliates/invites."
      />
      <AdminSection title="Invite Relationships" description="Shows bound inviter/invitee pairs. Invite code secrets are not expanded in the table.">
        {invites.isLoading ? <AdminLoadingState label="Loading affiliate invites..." /> : null}
        {invites.error ? <AdminErrorState error={invites.error} onRetry={() => invites.refetch()} /> : null}
        {invites.data ? (
          <>
            <AdminTable
              empty={<AdminEmptyState title="No affiliate invite records" />}
              columns={[
                { key: "id", header: "ID" },
                { key: "inviter", header: "Inviter" },
                { key: "invitee", header: "Invitee" },
                { key: "code", header: "Invite Code" },
                { key: "status", header: "Status" },
                { key: "firstPaid", header: "First Paid" },
                { key: "created", header: "Created" },
              ]}
              rows={invites.data.data.map((invite: AffiliateInviteRecord) => ({
                id: <span className="font-mono">{invite.id}</span>,
                inviter: <span className="font-mono">{invite.inviter_user_id}</span>,
                invitee: <span className="font-mono">{invite.invitee_user_id}</span>,
                code: <span className="font-mono">{invite.invite_code_id}</span>,
                status: <AdminStatusBadge status={invite.status} />,
                firstPaid: formatDateTime(invite.first_paid_at),
                created: formatDateTime(invite.created_at),
              }))}
              getRowKey={(_, index) => String(invites.data.data[index]?.id ?? index)}
            />
            <AdminPaginationSummary pagination={invites.data.pagination} />
          </>
        ) : null}
      </AdminSection>
    </AdminShell>
  );
}

function AdminAffiliateRebatesPage() {
  const query = useAdminAffiliateRebates({ page: 1 });
  return (
    <AdminAffiliateLedgerTable
      kind="rebates"
      title="Affiliate Rebates"
      description="Rebate accrual and refund compensation ledger entries from the affiliate domain."
      query={query}
    />
  );
}

function AdminAffiliateTransfersPage() {
  const query = useAdminAffiliateTransfers({ page: 1 });
  return (
    <AdminAffiliateLedgerTable
      kind="transfers"
      title="Affiliate Transfers"
      description="Transfer, settlement, and withdrawal ledger entries from the affiliate domain."
      query={query}
    />
  );
}

function AdminAffiliateLedgerTable({
  kind,
  title,
  description,
  query,
}: {
  kind: "rebates" | "transfers";
  title: string;
  description: string;
  query: ReturnType<typeof useAdminAffiliateRebates> | ReturnType<typeof useAdminAffiliateTransfers>;
}) {
  return (
    <AdminShell>
      <AdminPageHeader title={title} description={`Real records from /api/v1/admin/affiliates/${kind}.`} />
      <AdminSection title="Ledger Entries" description={description}>
        {query.isLoading ? <AdminLoadingState label={`Loading ${kind}...`} /> : null}
        {query.error ? <AdminErrorState error={query.error} onRetry={() => query.refetch()} /> : null}
        {query.data ? (
          <>
            <AdminTable
              empty={<AdminEmptyState title={`No affiliate ${kind} records`} />}
              columns={[
                { key: "id", header: "ID" },
                { key: "user", header: "User" },
                { key: "related", header: "Related User" },
                { key: "type", header: "Type" },
                { key: "amount", header: "Amount" },
                { key: "status", header: "Status" },
                { key: "reference", header: "Reference" },
                { key: "created", header: "Created" },
              ]}
              rows={query.data.data.map((entry: AffiliateLedgerEntry) => ({
                id: <span className="font-mono">{entry.id}</span>,
                user: <span className="font-mono">{entry.user_id}</span>,
                related: <span className="font-mono">{entry.related_user_id}</span>,
                type: <span className="font-mono text-xs">{entry.type}</span>,
                amount: formatMoney(entry.amount, entry.currency),
                status: <AdminStatusBadge status={entry.status} />,
                reference: <span className="font-mono text-xs">{entry.reference_id}</span>,
                created: formatDateTime(entry.created_at),
              }))}
              getRowKey={(_, index) => String(query.data.data[index]?.id ?? index)}
            />
            <AdminPaginationSummary pagination={query.data.pagination} />
          </>
        ) : null}
      </AdminSection>
    </AdminShell>
  );
}

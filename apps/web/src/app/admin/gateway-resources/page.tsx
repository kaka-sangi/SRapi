"use client";

import Link from "next/link";
import {
  Activity,
  AlertTriangle,
  Cable,
  CheckCircle2,
  Globe,
  KeyRound,
  Route,
  SearchX,
  Shuffle,
  Tag,
} from "lucide-react";
import { AdminShell } from "@/components/layout/admin-shell";
import { PageHeader } from "@/components/layout/page-header";
import { ListToolbar, FilterSelect, SearchInput } from "@/components/admin/list-toolbar";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { QuietBadge } from "@/components/ui/quiet-badge";
import { Button } from "@/components/ui/button";
import { EmptyState } from "@/components/ui/empty-state";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
  TableScroll,
} from "@/components/ui/table";
import { ADMIN_ROUTES } from "@/lib/routes";
import { useAdminList } from "@/hooks/use-admin-list";
import { useLanguage } from "@/context/LanguageContext";
import {
  useAdminGatewayResources,
  useAdminSettings,
  useAdminPricingRulePresets,
  useInstallPricingRulePresets,
} from "@/hooks/admin-queries";
import { useToast } from "@/context/ToastContext";
import { adminErrorMessage } from "@/lib/admin-api";
import {
  PROTOCOL_CONVERSION_ROUTES,
  type ProtocolConversionRoute,
} from "@/lib/admin-settings-form";
import type {
  GatewayAccountBlockers,
  GatewayEndpointResourceRow,
  GatewayEndpointResourceSummaryRow,
  GatewayProviderResourceReason,
  GatewayProviderResourceStatus,
  GatewayResourceFix,
  GatewayModelResourceRow,
  GatewayPricingCoverage,
  GatewayProviderResourceRow,
  GatewayRouteResourceRow,
  GatewayResourceSummary,
  PricingRulePreset,
} from "@/lib/sdk-types";

type GatewayResourceScope = "endpoints" | "providers" | "models" | "routes";

const GATEWAY_STATUSES: GatewayProviderResourceStatus[] = ["ready", "limited", "blocked"];
const CONVERSATION_ENDPOINT_KEYS = ["chat_completions", "responses", "messages"] as const;
const GATEWAY_REASONS: GatewayProviderResourceReason[] = [
  "provider_disabled",
  "no_active_models",
  "no_model_mappings",
  "no_active_accounts",
  "no_routable_accounts",
  "proxy_attention",
  "no_api_keys",
  "pricing_uncovered",
];
const GATEWAY_SCOPES: GatewayResourceScope[] = ["endpoints", "providers", "models", "routes"];

export default function AdminGatewayResourcesPage() {
  return (
    <AdminShell>
      <GatewayResourcesContent />
    </AdminShell>
  );
}

function GatewayResourcesContent() {
  const { t } = useLanguage();
  const { toast } = useToast();
  const list = useAdminList({ pageSize: 1000 });
  const gatewayResources = useAdminGatewayResources();
  const settings = useAdminSettings();
  const pricingPresets = useAdminPricingRulePresets();
  const installPricingPresets = useInstallPricingRulePresets();
  const loading = gatewayResources.isLoading;
  const error = gatewayResources.isError;
  const summary = gatewayResources.data;
  const filters = gatewayResourceFilters(summary, list.search, list.filters);
  const missingPricingPresetFamilies = gatewayMissingPricingPresetFamilies(
    summary,
    pricingPresets.data ?? [],
  );
  const isFiltered = Boolean(
    list.search || list.filters.status || list.filters.reason || list.filters.scope,
  );

  async function installMissingPricingPresets() {
    if (missingPricingPresetFamilies.length === 0) return;
    try {
      const result = await installPricingPresets.mutateAsync({
        families: missingPricingPresetFamilies,
      });
      toast({
        title: t("adminGatewayResources.pricingPresetsInstalled", { count: result.created }),
        description: t("adminGatewayResources.pricingPresetsInstalledHint", {
          families: missingPricingPresetFamilies.join(", "),
        }),
        tone: result.errors.length > 0 ? "warning" : "success",
      });
      await gatewayResources.refetch();
    } catch (err) {
      toast({ title: t("feedback.failed"), description: adminErrorMessage(err), tone: "error" });
    }
  }

  return (
    <>
      <PageHeader
        eyebrow={t("nav.sectionAdminGateway")}
        title={t("adminGatewayResources.title")}
        description={t("adminGatewayResources.subtitle")}
        actions={
          <div className="flex items-center gap-2">
            <Button asChild variant="outline" size="sm">
              <Link href={ADMIN_ROUTES.quickSetup}>{t("nav.adminQuickSetup")}</Link>
            </Button>
            <Button asChild variant="primary" size="sm">
              <Link href={ADMIN_ROUTES.accounts}>{t("adminAccounts.create")}</Link>
            </Button>
          </div>
        }
      />

      {loading ? <GatewayResourcesSkeleton /> : null}
      {!loading && error ? (
        <EmptyState
          icon={AlertTriangle}
          title={t("common.error")}
          description={t("common.errorBody")}
          action={
            <Button variant="outline" size="sm" onClick={() => window.location.reload()}>
              {t("common.retry")}
            </Button>
          }
        />
      ) : null}
      {!loading && !error ? (
        <div className="space-y-4">
          <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-5">
            <ResourceKpi
              icon={Cable}
              label={t("adminGatewayResources.activeProviders")}
              value={`${summary?.active_providers ?? 0}/${summary?.providers ?? 0}`}
            />
            <ResourceKpi
              icon={Activity}
              label={t("adminGatewayResources.routableAccounts")}
              value={`${summary?.routable_accounts ?? 0}/${summary?.active_accounts ?? 0}`}
            />
            <ResourceKpi
              icon={Route}
              label={t("adminGatewayResources.activeModels")}
              value={String(summary?.active_models ?? 0)}
            />
            <ResourceKpi
              icon={Globe}
              label={t("adminGatewayResources.availableProxies")}
              value={`${summary?.available_proxies ?? 0}/${summary?.active_proxies ?? 0}`}
            />
            <ResourceKpi
              icon={KeyRound}
              label={t("adminGatewayResources.scopedApiKeys")}
              value={`${summary?.scoped_api_keys ?? 0}/${summary?.active_api_keys ?? 0}`}
            />
          </div>

          <GatewayFixQueue
            fixes={summary?.fixes ?? []}
            pricingPresetFamilies={missingPricingPresetFamilies}
            installingPricingPresets={installPricingPresets.isPending}
            onInstallPricingPresets={installMissingPricingPresets}
          />

          <ProtocolConversionRoutePanel
            routes={settings.data?.gateway.protocol_conversion_routes}
          />

          <GatewayResourceToolbar
            search={list.searchInput}
            onSearch={list.setSearchInput}
            status={list.filters.status}
            onStatus={(value) => list.setFilter("status", value)}
            reason={list.filters.reason}
            onReason={(value) => list.setFilter("reason", value)}
            scope={list.filters.scope}
            onScope={(value) => list.setFilter("scope", value)}
            isFiltered={isFiltered}
            onClearFilters={list.clearFilters}
          />

          {isFiltered && filters.total === 0 ? (
            <EmptyState
              icon={SearchX}
              title={t("adminGatewayResources.emptyFilteredTitle")}
              description={t("adminGatewayResources.emptyFilteredBody")}
              action={
                <Button variant="outline" size="sm" onClick={list.clearFilters}>
                  {t("adminCommon.clearFilters")}
                </Button>
              }
            />
          ) : null}

          {filters.endpointRows !== null ? (
            <GatewayEndpointSummary
              rows={filters.endpointRows}
              total={summary?.endpoint_rows.length ?? 0}
            />
          ) : null}

          {filters.providerRows !== null ? (
            <Card>
              <CardHeader>
                <CardTitle>
                  {t("adminGatewayResources.providerMatrix")}{" "}
                  <span className="text-2xs text-srapi-text-tertiary font-mono">
                    {filters.providerRows.length}/{summary?.rows.length ?? 0}
                  </span>
                </CardTitle>
              </CardHeader>
              <CardContent className="p-0">
                <TableScroll minWidth={900}>
                  <Table>
                    <TableHeader>
                      <tr>
                        <TableHead>{t("adminProviders.name")}</TableHead>
                        <TableHead>{t("adminProviders.adapterType")}</TableHead>
                        <TableHead>{t("adminGatewayResources.endpointSwitches")}</TableHead>
                        <TableHead className="text-right">
                          {t("adminGatewayResources.modelMappings")}
                        </TableHead>
                        <TableHead className="text-right">
                          {t("adminGatewayResources.accounts")}
                        </TableHead>
                        <TableHead className="text-right">
                          {t("adminGatewayResources.proxies")}
                        </TableHead>
                        <TableHead className="text-right">
                          {t("adminGatewayResources.apiKeys")}
                        </TableHead>
                        <TableHead>{t("adminCommon.status")}</TableHead>
                        <TableHead>{t("adminGatewayResources.blockers")}</TableHead>
                        <TableHead>{t("adminGatewayResources.fixActions")}</TableHead>
                      </tr>
                    </TableHeader>
                    <TableBody>
                      {filters.providerRows.map((row) => (
                        <ProviderResourceRow key={row.provider.id} row={row} />
                      ))}
                    </TableBody>
                  </Table>
                </TableScroll>
              </CardContent>
            </Card>
          ) : null}

          {filters.modelRows !== null ? (
            <Card>
              <CardHeader>
                <CardTitle>
                  {t("adminGatewayResources.modelMatrix")}{" "}
                  <span className="text-2xs text-srapi-text-tertiary font-mono">
                    {filters.modelRows.length}/{summary?.model_rows.length ?? 0}
                  </span>
                </CardTitle>
              </CardHeader>
              <CardContent className="p-0">
                <TableScroll minWidth={900}>
                  <Table>
                    <TableHeader>
                      <tr>
                        <TableHead>{t("adminGatewayResources.model")}</TableHead>
                        <TableHead className="text-right">
                          {t("adminGatewayResources.providers")}
                        </TableHead>
                        <TableHead className="text-right">
                          {t("adminGatewayResources.modelMappings")}
                        </TableHead>
                        <TableHead className="text-right">
                          {t("adminGatewayResources.routableAccountsShort")}
                        </TableHead>
                        <TableHead>{t("adminGatewayResources.endpoints")}</TableHead>
                        <TableHead>{t("adminGatewayResources.pricing")}</TableHead>
                        <TableHead className="text-right">
                          {t("adminGatewayResources.apiKeys")}
                        </TableHead>
                        <TableHead>{t("adminCommon.status")}</TableHead>
                        <TableHead>{t("adminGatewayResources.blockers")}</TableHead>
                        <TableHead>{t("adminGatewayResources.fixActions")}</TableHead>
                      </tr>
                    </TableHeader>
                    <TableBody>
                      {filters.modelRows.map((row) => (
                        <ModelResourceRow key={row.model.id} row={row} />
                      ))}
                    </TableBody>
                  </Table>
                </TableScroll>
              </CardContent>
            </Card>
          ) : null}

          {filters.routeRows !== null ? (
            <Card>
              <CardHeader>
                <CardTitle>
                  {t("adminGatewayResources.routeMatrix")}{" "}
                  <span className="text-2xs text-srapi-text-tertiary font-mono">
                    {filters.routeRows.length}/{summary?.route_rows.length ?? 0}
                  </span>
                </CardTitle>
              </CardHeader>
              <CardContent className="p-0">
                <TableScroll minWidth={1040}>
                  <Table>
                    <TableHeader>
                      <tr>
                        <TableHead>{t("adminGatewayResources.model")}</TableHead>
                        <TableHead>{t("adminGatewayResources.provider")}</TableHead>
                        <TableHead>{t("adminGatewayResources.upstreamModel")}</TableHead>
                        <TableHead className="text-right">
                          {t("adminGatewayResources.routableAccountsShort")}
                        </TableHead>
                        <TableHead>{t("adminGatewayResources.endpoints")}</TableHead>
                        <TableHead>{t("adminGatewayResources.pricing")}</TableHead>
                        <TableHead className="text-right">
                          {t("adminGatewayResources.apiKeys")}
                        </TableHead>
                        <TableHead>{t("adminCommon.status")}</TableHead>
                        <TableHead>{t("adminGatewayResources.blockers")}</TableHead>
                        <TableHead>{t("adminGatewayResources.fixActions")}</TableHead>
                      </tr>
                    </TableHeader>
                    <TableBody>
                      {filters.routeRows.map((row) => (
                        <RouteResourceRow key={`${row.mapping_id}`} row={row} />
                      ))}
                    </TableBody>
                  </Table>
                </TableScroll>
              </CardContent>
            </Card>
          ) : null}
        </div>
      ) : null}
    </>
  );
}

function GatewayResourceToolbar({
  search,
  onSearch,
  status,
  onStatus,
  reason,
  onReason,
  scope,
  onScope,
  isFiltered,
  onClearFilters,
}: {
  search: string;
  onSearch: (value: string) => void;
  status: string | undefined;
  onStatus: (value: string | undefined) => void;
  reason: string | undefined;
  onReason: (value: string | undefined) => void;
  scope: string | undefined;
  onScope: (value: string | undefined) => void;
  isFiltered: boolean;
  onClearFilters: () => void;
}) {
  const { t } = useLanguage();
  return (
    <Card className="overflow-hidden">
      <ListToolbar>
        <SearchInput
          value={search}
          onChange={onSearch}
          placeholder={t("adminGatewayResources.searchPlaceholder")}
          className="sm:max-w-sm"
        />
        <FilterSelect
          value={validStatusFilter(status)}
          onChange={onStatus}
          options={GATEWAY_STATUSES.map((value) => ({
            value,
            label: t(`adminGatewayResources.${value}`),
          }))}
          allLabel={t("adminGatewayResources.allStatuses")}
        />
        <FilterSelect
          value={validReasonFilter(reason)}
          onChange={onReason}
          options={GATEWAY_REASONS.map((value) => ({
            value,
            label: t(`adminGatewayResources.reason.${value}`),
          }))}
          allLabel={t("adminGatewayResources.allReasons")}
        />
        <FilterSelect
          value={validScopeFilter(scope)}
          onChange={onScope}
          options={GATEWAY_SCOPES.map((value) => ({
            value,
            label: t(`adminGatewayResources.scope.${value}`),
          }))}
          allLabel={t("adminGatewayResources.allScopes")}
        />
        {isFiltered ? (
          <Button variant="ghost" size="sm" onClick={onClearFilters}>
            {t("adminCommon.clearFilters")}
          </Button>
        ) : null}
      </ListToolbar>
    </Card>
  );
}

function ResourceKpi({
  icon: Icon,
  label,
  value,
}: {
  icon: typeof Cable;
  label: string;
  value: string;
}) {
  return (
    <Card className="p-5">
      <div className="flex items-center justify-between gap-3">
        <span className="text-2xs text-srapi-text-tertiary font-mono uppercase">{label}</span>
        <Icon className="text-srapi-text-tertiary size-4" />
      </div>
      <div className="text-srapi-text-primary tabular mt-3 font-serif text-3xl leading-none">
        {value}
      </div>
    </Card>
  );
}

function gatewayResourceFilters(
  summary: GatewayResourceSummary | undefined,
  rawSearch: string,
  filters: Record<string, string>,
): {
  endpointRows: GatewayEndpointResourceSummaryRow[] | null;
  providerRows: GatewayProviderResourceRow[] | null;
  modelRows: GatewayModelResourceRow[] | null;
  routeRows: GatewayRouteResourceRow[] | null;
  total: number;
} {
  const scope = validScopeFilter(filters.scope);
  const status = validStatusFilter(filters.status);
  const reason = validReasonFilter(filters.reason);
  const search = rawSearch.trim().toLowerCase();
  const includeEndpoints = !scope || scope === "endpoints";
  const includeProviders = !scope || scope === "providers";
  const includeModels = !scope || scope === "models";
  const includeRoutes = !scope || scope === "routes";
  const endpointRows = includeEndpoints
    ? (summary?.endpoint_rows ?? []).filter((row) =>
        endpointSummaryRowMatches(row, search, status, reason),
      )
    : null;
  const providerRows = includeProviders
    ? (summary?.rows ?? []).filter((row) => providerResourceRowMatches(row, search, status, reason))
    : null;
  const modelRows = includeModels
    ? (summary?.model_rows ?? []).filter((row) =>
        modelResourceRowMatches(row, search, status, reason),
      )
    : null;
  const routeRows = includeRoutes
    ? (summary?.route_rows ?? []).filter((row) =>
        routeResourceRowMatches(row, search, status, reason),
      )
    : null;
  return {
    endpointRows,
    providerRows,
    modelRows,
    routeRows,
    total:
      (endpointRows?.length ?? 0) +
      (providerRows?.length ?? 0) +
      (modelRows?.length ?? 0) +
      (routeRows?.length ?? 0),
  };
}

function endpointSummaryRowMatches(
  row: GatewayEndpointResourceSummaryRow,
  search: string,
  status: GatewayProviderResourceStatus | undefined,
  reason: GatewayProviderResourceReason | undefined,
) {
  if (status && row.status !== status) return false;
  if (reason) {
    if (reason !== "no_routable_accounts") return false;
    if (
      row.ready_routes === row.routes &&
      row.unsupported_account_routes === 0 &&
      row.unavailable_model_account_routes === 0
    ) {
      return false;
    }
  }
  if (!search) return true;
  return rowText([row.key, row.source_endpoint, row.status]).includes(search);
}

function providerResourceRowMatches(
  row: GatewayProviderResourceRow,
  search: string,
  status: GatewayProviderResourceStatus | undefined,
  reason: GatewayProviderResourceReason | undefined,
) {
  if (status && row.status !== status) return false;
  if (reason && !rowMatchesReason(row.reasons, reason)) return false;
  if (!search) return true;
  return rowText([
    row.provider.name,
    row.provider.display_name,
    row.provider.adapter_type,
    row.status,
    ...row.reasons,
  ]).includes(search);
}

function modelResourceRowMatches(
  row: GatewayModelResourceRow,
  search: string,
  status: GatewayProviderResourceStatus | undefined,
  reason: GatewayProviderResourceReason | undefined,
) {
  if (status && row.status !== status) return false;
  if (
    reason &&
    !rowMatchesReason(row.reasons, reason) &&
    !rowMatchesPricingReason(row.pricing, reason)
  ) {
    return false;
  }
  if (!search) return true;
  return rowText([
    row.model.canonical_name,
    row.model.display_name,
    row.model.family,
    row.status,
    row.pricing.source,
    row.pricing.status,
    row.pricing.currency,
    ...row.reasons,
    ...row.endpoints.map((endpoint) => endpoint.source_endpoint),
    ...row.endpoints.map((endpoint) => endpoint.key),
  ]).includes(search);
}

function routeResourceRowMatches(
  row: GatewayRouteResourceRow,
  search: string,
  status: GatewayProviderResourceStatus | undefined,
  reason: GatewayProviderResourceReason | undefined,
) {
  if (status && row.status !== status) return false;
  if (
    reason &&
    !rowMatchesReason(row.reasons, reason) &&
    !rowMatchesPricingReason(row.pricing, reason)
  ) {
    return false;
  }
  if (!search) return true;
  return rowText([
    row.model.canonical_name,
    row.model.display_name,
    row.provider.name,
    row.provider.display_name,
    row.provider.adapter_type,
    row.upstream_model,
    row.status,
    row.pricing.source,
    row.pricing.status,
    row.pricing.currency,
    ...row.reasons,
    ...row.endpoints.map((endpoint) => endpoint.source_endpoint),
    ...row.endpoints.map((endpoint) => endpoint.key),
  ]).includes(search);
}

function rowMatchesReason(
  reasons: GatewayProviderResourceReason[],
  reason: GatewayProviderResourceReason,
) {
  return reasons.includes(reason);
}

function rowMatchesPricingReason(
  pricing: GatewayPricingCoverage,
  reason: GatewayProviderResourceReason,
) {
  return reason === "pricing_uncovered" && gatewayPricingNeedsAttention(pricing);
}

function gatewayPricingNeedsAttention(pricing: GatewayPricingCoverage) {
  return pricing.status === "error" || pricing.source === "default_zero";
}

function gatewayMissingPricingPresetFamilies(
  summary: GatewayResourceSummary | undefined,
  presets: PricingRulePreset[],
) {
  const presetByFamily = new Map<string, string>();
  for (const preset of presets) {
    const family = normalizePricingFamily(preset.model_family);
    if (family && !presetByFamily.has(family)) {
      presetByFamily.set(family, preset.model_family);
    }
  }
  const families = new Set<string>();
  for (const row of summary?.route_rows ?? []) {
    if (!gatewayPricingNeedsAttention(row.pricing)) continue;
    const family = normalizePricingFamily(row.model.family);
    if (!family) continue;
    const presetFamily = presetByFamily.get(family);
    if (presetFamily) {
      families.add(presetFamily);
    }
  }
  return Array.from(families).sort((left, right) => left.localeCompare(right));
}

function normalizePricingFamily(value: string | null | undefined) {
  return (value ?? "").trim().toLowerCase();
}

function cleanVisibleProtocolConversionRoutes(
  routes: readonly string[] | null | undefined,
): ProtocolConversionRoute[] {
  if (routes === null || routes === undefined) {
    return [...PROTOCOL_CONVERSION_ROUTES];
  }
  const selected = new Set(routes ?? []);
  return PROTOCOL_CONVERSION_ROUTES.filter((route) => selected.has(route));
}

function rowText(values: Array<string | number | null | undefined>) {
  return values
    .filter((value) => value !== null && value !== undefined)
    .join(" ")
    .toLowerCase();
}

function validStatusFilter(value: string | undefined): GatewayProviderResourceStatus | undefined {
  return GATEWAY_STATUSES.includes(value as GatewayProviderResourceStatus)
    ? (value as GatewayProviderResourceStatus)
    : undefined;
}

function validReasonFilter(value: string | undefined): GatewayProviderResourceReason | undefined {
  return GATEWAY_REASONS.includes(value as GatewayProviderResourceReason)
    ? (value as GatewayProviderResourceReason)
    : undefined;
}

function validScopeFilter(value: string | undefined): GatewayResourceScope | undefined {
  return GATEWAY_SCOPES.includes(value as GatewayResourceScope)
    ? (value as GatewayResourceScope)
    : undefined;
}

function GatewayFixQueue({
  fixes,
  pricingPresetFamilies,
  installingPricingPresets,
  onInstallPricingPresets,
}: {
  fixes: GatewayResourceFix[];
  pricingPresetFamilies: string[];
  installingPricingPresets: boolean;
  onInstallPricingPresets: () => void;
}) {
  const { t } = useLanguage();
  if (fixes.length === 0 && pricingPresetFamilies.length === 0) {
    return null;
  }
  const visible = fixes.slice(0, 5);
  return (
    <Card className="p-4">
      <div className="flex flex-wrap items-center gap-2">
        <span className="text-2xs text-srapi-text-tertiary font-mono uppercase">
          {t("adminGatewayResources.fixQueue")}
        </span>
        {visible.map((fix) => (
          <Link
            key={`${fix.area}:${fix.reason}`}
            href={fix.href}
            className="border-srapi-border bg-srapi-card-muted hover:border-srapi-border-strong text-2xs inline-flex min-w-0 items-center gap-1.5 rounded-md border px-2 py-1 font-mono transition-colors"
            title={t(`adminGatewayResources.fixReason.${fix.reason}`, { count: fix.count })}
          >
            <QuietBadge
              status={
                fix.severity === "critical"
                  ? "error"
                  : fix.severity === "warning"
                    ? "limited"
                    : "disabled"
              }
              label={t(`adminGatewayResources.fixSeverity.${fix.severity}`)}
            />
            <span className="text-srapi-text-secondary truncate">
              {t(`adminGatewayResources.fixArea.${fix.area}`)}
            </span>
            <span className="text-srapi-text-tertiary truncate">
              {t(`adminGatewayResources.reason.${fix.reason}`)}
            </span>
            <span className="text-srapi-text-primary tabular">{fix.count}</span>
          </Link>
        ))}
        {fixes.length > visible.length ? (
          <span className="text-2xs text-srapi-text-tertiary font-mono">
            +{fixes.length - visible.length}
          </span>
        ) : null}
        {pricingPresetFamilies.length > 0 ? (
          <Button
            type="button"
            variant="outline"
            size="sm"
            loading={installingPricingPresets}
            onClick={onInstallPricingPresets}
            title={pricingPresetFamilies.join(", ")}
          >
            <Tag className="size-3.5" />
            {t("adminGatewayResources.installPricingPresets", {
              count: pricingPresetFamilies.length,
            })}
          </Button>
        ) : null}
      </div>
    </Card>
  );
}

function ProtocolConversionRoutePanel({
  routes,
}: {
  routes: readonly string[] | null | undefined;
}) {
  const { t } = useLanguage();
  const enabledRoutes = cleanVisibleProtocolConversionRoutes(routes);
  const enabled = new Set(enabledRoutes);
  return (
    <Card className="p-4">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0">
          <div className="flex items-center gap-2">
            <Shuffle className="text-srapi-text-tertiary size-4" />
            <h3 className="text-srapi-text-primary font-medium">
              {t("adminGatewayResources.protocolConversions")}
            </h3>
            <QuietBadge
              status={
                enabledRoutes.length === PROTOCOL_CONVERSION_ROUTES.length ? "active" : "limited"
              }
              label={t("adminGatewayResources.protocolConversionCount", {
                enabled: enabledRoutes.length,
                total: PROTOCOL_CONVERSION_ROUTES.length,
              })}
            />
          </div>
          <p className="text-2xs text-srapi-text-tertiary mt-1">
            {t("adminGatewayResources.protocolConversionsHint")}
          </p>
        </div>
        <Button asChild variant="outline" size="sm">
          <Link href={`${ADMIN_ROUTES.settings}?tab=gateway`}>
            {t("adminGatewayResources.openConversionSettings")}
          </Link>
        </Button>
      </div>
      <div className="mt-3 grid gap-2 md:grid-cols-2 xl:grid-cols-3">
        {PROTOCOL_CONVERSION_ROUTES.map((route) => {
          const on = enabled.has(route);
          return (
            <Link
              key={route}
              href={`${ADMIN_ROUTES.settings}?tab=gateway`}
              className="border-srapi-border bg-srapi-card-muted hover:border-srapi-border-strong flex min-w-0 items-center justify-between gap-3 rounded-md border px-3 py-2 transition-colors"
              title={t("adminGatewayResources.protocolConversionRouteHint", {
                route: t(`adminSettings.protocolConversionRoutes.${route}`),
                mode: on
                  ? t("adminGatewayResources.protocolConversionEnabled")
                  : t("adminGatewayResources.protocolConversionDisabled"),
              })}
            >
              <span className="text-2xs text-srapi-text-secondary truncate">
                {t(`adminSettings.protocolConversionRoutes.${route}`)}
              </span>
              <QuietBadge
                status={on ? "active" : "disabled"}
                label={
                  on
                    ? t("adminGatewayResources.protocolConversionEnabled")
                    : t("adminGatewayResources.protocolConversionDisabled")
                }
              />
            </Link>
          );
        })}
      </div>
    </Card>
  );
}

function GatewayEndpointSummary({
  rows,
  total,
}: {
  rows: GatewayEndpointResourceSummaryRow[];
  total: number;
}) {
  const { t } = useLanguage();
  return (
    <Card>
      <CardHeader>
        <CardTitle>
          {t("adminGatewayResources.endpointSummary")}{" "}
          <span className="text-2xs text-srapi-text-tertiary font-mono">
            {rows.length}/{total}
          </span>
        </CardTitle>
      </CardHeader>
      <CardContent>
        <div className="grid gap-2 md:grid-cols-2 xl:grid-cols-3">
          {rows.map((row) => (
            <GatewayEndpointSummaryItem key={row.key} row={row} />
          ))}
        </div>
      </CardContent>
    </Card>
  );
}

function GatewayEndpointSummaryItem({ row }: { row: GatewayEndpointResourceSummaryRow }) {
  const { t } = useLanguage();
  const status = resourceStatusMeta(row.status, t);
  const StatusIcon = status.icon;
  const modelCoverage = `${row.ready_models}/${row.models}`;
  const routeCoverage = `${row.ready_routes}/${row.routes}`;
  const accountCoverage = `${row.routable_account_routes}/${row.candidate_account_routes}`;
  return (
    <Link
      href={`${ADMIN_ROUTES.gatewayResources}?f_scope=routes&q=${encodeURIComponent(row.source_endpoint)}`}
      className="border-srapi-border bg-srapi-card-muted hover:border-srapi-border-strong grid gap-3 rounded-md border p-3 transition-colors"
      title={endpointSummaryTitle(row, t)}
    >
      <div className="flex min-w-0 items-start justify-between gap-3">
        <div className="min-w-0">
          <div className="text-srapi-text-primary truncate text-sm font-medium">
            {t(`adminGatewayResources.endpoint.${row.key}`)}
          </div>
          <div className="text-2xs text-srapi-text-tertiary truncate font-mono">
            {row.source_endpoint}
          </div>
        </div>
        <span className="inline-flex shrink-0 items-center gap-1.5">
          <StatusIcon className="text-srapi-text-tertiary size-3.5" />
          <QuietBadge status={status.quiet} label={status.label} />
        </span>
      </div>
      <div className="grid grid-cols-3 gap-2">
        <EndpointSummaryMetric
          label={t("adminGatewayResources.endpointModels")}
          value={modelCoverage}
          ready={row.ready_models === row.models && row.models > 0}
        />
        <EndpointSummaryMetric
          label={t("adminGatewayResources.endpointRoutes")}
          value={routeCoverage}
          ready={row.ready_routes === row.routes && row.routes > 0}
        />
        <EndpointSummaryMetric
          label={t("adminGatewayResources.endpointAccounts")}
          value={accountCoverage}
          ready={row.routable_account_routes > 0}
        />
      </div>
      {row.unsupported_account_routes > 0 || row.unavailable_model_account_routes > 0 ? (
        <div className="flex flex-wrap gap-1">
          {row.unsupported_account_routes > 0 ? (
            <span className="border-srapi-border text-srapi-text-tertiary rounded-md border px-1.5 py-0.5 font-mono text-[10px]">
              {t("adminGatewayResources.endpointUnsupported")}:{" "}
              <span className="text-srapi-text-primary tabular">
                {row.unsupported_account_routes}
              </span>
            </span>
          ) : null}
          {row.unavailable_model_account_routes > 0 ? (
            <span className="border-srapi-border text-srapi-text-tertiary rounded-md border px-1.5 py-0.5 font-mono text-[10px]">
              {t("adminGatewayResources.endpointModelBlocked")}:{" "}
              <span className="text-srapi-text-primary tabular">
                {row.unavailable_model_account_routes}
              </span>
            </span>
          ) : null}
        </div>
      ) : null}
    </Link>
  );
}

function EndpointSummaryMetric({
  label,
  value,
  ready,
}: {
  label: string;
  value: string;
  ready: boolean;
}) {
  return (
    <div className="min-w-0">
      <div className="text-srapi-text-tertiary truncate font-mono text-[10px] uppercase">
        {label}
      </div>
      <div
        className={
          ready
            ? "text-srapi-success tabular font-mono text-sm"
            : "text-srapi-text-secondary tabular font-mono text-sm"
        }
      >
        {value}
      </div>
    </div>
  );
}

function endpointSummaryTitle(
  row: GatewayEndpointResourceSummaryRow,
  t: (key: string, params?: Record<string, string | number>) => string,
) {
  return [
    `${t(`adminGatewayResources.endpoint.${row.key}`)} · ${row.source_endpoint}`,
    `${t("adminGatewayResources.endpointModels")}: ${row.ready_models}/${row.models}`,
    `${t("adminGatewayResources.endpointRoutes")}: ${row.ready_routes}/${row.routes}`,
    `${t("adminGatewayResources.endpointAccounts")}: ${row.routable_account_routes}/${row.candidate_account_routes}`,
    `${t("adminGatewayResources.endpointUnsupported")}: ${row.unsupported_account_routes}`,
    `${t("adminGatewayResources.endpointModelBlocked")}: ${row.unavailable_model_account_routes}`,
  ].join("\n");
}

function ModelResourceRow({ row }: { row: GatewayModelResourceRow }) {
  const { t } = useLanguage();
  const status = resourceStatusMeta(row.status, t);
  const StatusIcon = status.icon;
  return (
    <TableRow>
      <TableCell>
        <div className="min-w-0">
          <Link
            href={ADMIN_ROUTES.models}
            className="text-srapi-text-primary hover:text-srapi-accent truncate transition-colors"
          >
            {row.model.display_name || row.model.canonical_name}
          </Link>
          <div className="text-2xs text-srapi-text-tertiary truncate font-mono">
            {row.model.canonical_name}
          </div>
        </div>
      </TableCell>
      <TableCell className="text-2xs tabular text-right font-mono">
        <span className={row.active_providers > 0 ? "text-srapi-text-primary" : "text-srapi-error"}>
          {row.active_providers}
        </span>
      </TableCell>
      <TableCell className="text-2xs tabular text-right font-mono">
        <span
          className={row.active_model_mappings > 0 ? "text-srapi-text-primary" : "text-srapi-error"}
        >
          {row.active_model_mappings}
        </span>
      </TableCell>
      <TableCell className="text-2xs tabular text-right font-mono">
        <span className={row.routable_accounts > 0 ? "text-srapi-success" : "text-srapi-error"}>
          {row.routable_accounts}
        </span>
      </TableCell>
      <TableCell>
        <EndpointMatrix row={row} />
      </TableCell>
      <TableCell>
        <PricingCoverageBadge pricing={row.pricing} />
      </TableCell>
      <TableCell className="text-2xs tabular text-right font-mono">
        <span className={row.api_key_count > 0 ? "text-srapi-text-primary" : "text-srapi-error"}>
          {row.api_key_count}
        </span>
        {row.scoped_key_count > 0 ? (
          <span className="text-srapi-text-tertiary"> · {row.scoped_key_count}</span>
        ) : null}
      </TableCell>
      <TableCell>
        <span className="inline-flex items-center gap-1.5">
          <StatusIcon className="text-srapi-text-tertiary size-3.5" />
          <QuietBadge status={status.quiet} label={status.label} />
        </span>
      </TableCell>
      <TableCell>
        {row.reasons.length > 0 ? (
          <div className="flex max-w-md flex-wrap gap-1">
            {row.reasons.map((reason) => (
              <span
                key={reason}
                className="border-srapi-border bg-srapi-card-muted text-2xs text-srapi-text-tertiary rounded-md border px-1.5 py-0.5 font-mono"
              >
                {t(`adminGatewayResources.reason.${reason}`)}
              </span>
            ))}
          </div>
        ) : (
          <span className="text-2xs text-srapi-text-tertiary">-</span>
        )}
      </TableCell>
      <TableCell>
        <GatewayRowFixActions row={row} />
      </TableCell>
    </TableRow>
  );
}

function RouteResourceRow({ row }: { row: GatewayRouteResourceRow }) {
  const { t } = useLanguage();
  const status = resourceStatusMeta(row.status, t);
  const StatusIcon = status.icon;
  return (
    <TableRow>
      <TableCell>
        <div className="min-w-0">
          <Link
            href={ADMIN_ROUTES.models}
            className="text-srapi-text-primary hover:text-srapi-accent truncate transition-colors"
          >
            {row.model.display_name || row.model.canonical_name}
          </Link>
          <div className="text-2xs text-srapi-text-tertiary truncate font-mono">
            {row.model.canonical_name}
          </div>
        </div>
      </TableCell>
      <TableCell>
        <div className="min-w-0">
          <Link
            href={ADMIN_ROUTES.providers}
            className="text-srapi-text-primary hover:text-srapi-accent truncate transition-colors"
          >
            {row.provider.display_name || row.provider.name}
          </Link>
          <div className="text-2xs text-srapi-text-tertiary truncate font-mono">
            {row.provider.adapter_type}
          </div>
        </div>
      </TableCell>
      <TableCell className="text-2xs text-srapi-text-secondary max-w-[220px] truncate font-mono">
        {row.upstream_model}
      </TableCell>
      <TableCell className="text-2xs tabular text-right font-mono">
        <span className={row.routable_accounts > 0 ? "text-srapi-success" : "text-srapi-error"}>
          {row.routable_accounts}
        </span>
      </TableCell>
      <TableCell>
        <EndpointMatrix row={row} />
      </TableCell>
      <TableCell>
        <PricingCoverageBadge pricing={row.pricing} />
      </TableCell>
      <TableCell className="text-2xs tabular text-right font-mono">
        <span className={row.api_key_count > 0 ? "text-srapi-text-primary" : "text-srapi-error"}>
          {row.api_key_count}
        </span>
        {row.scoped_key_count > 0 ? (
          <span className="text-srapi-text-tertiary"> · {row.scoped_key_count}</span>
        ) : null}
      </TableCell>
      <TableCell>
        <span className="inline-flex items-center gap-1.5">
          <StatusIcon className="text-srapi-text-tertiary size-3.5" />
          <QuietBadge status={status.quiet} label={status.label} />
        </span>
      </TableCell>
      <TableCell>
        {row.reasons.length > 0 ? (
          <div className="flex max-w-md flex-wrap gap-1">
            {row.reasons.map((reason) => (
              <span
                key={reason}
                className="border-srapi-border bg-srapi-card-muted text-2xs text-srapi-text-tertiary rounded-md border px-1.5 py-0.5 font-mono"
              >
                {t(`adminGatewayResources.reason.${reason}`)}
              </span>
            ))}
          </div>
        ) : (
          <span className="text-2xs text-srapi-text-tertiary">-</span>
        )}
      </TableCell>
      <TableCell>
        <GatewayRowFixActions row={row} />
      </TableCell>
    </TableRow>
  );
}

function PricingCoverageBadge({ pricing }: { pricing: GatewayPricingCoverage }) {
  const { t } = useLanguage();
  const status =
    pricing.status === "priced" ? "active" : pricing.status === "error" ? "error" : "limited";
  const routeCount = `${pricing.priced_routes}/${pricing.total_routes}`;
  const billingMode = pricing.billing_mode
    ? t(`adminGatewayResources.billingMode.${pricing.billing_mode}`)
    : "-";
  const titleParts = [
    t(`adminGatewayResources.pricingSource.${pricing.source}`),
    routeCount,
    pricing.currency ?? "",
    billingMode,
    pricing.pricing_rule_id
      ? `${t("adminGatewayResources.pricingRule")} #${pricing.pricing_rule_id}`
      : "",
  ].filter(Boolean);
  return (
    <div className="flex min-w-[120px] flex-col items-start gap-1" title={titleParts.join(" · ")}>
      <QuietBadge
        status={status}
        label={t(`adminGatewayResources.pricingSource.${pricing.source}`)}
      />
      <span className="text-2xs text-srapi-text-tertiary tabular font-mono">
        {routeCount}
        {pricing.currency ? <span> · {pricing.currency}</span> : null}
      </span>
    </div>
  );
}

function EndpointMatrix({
  row,
}: {
  row:
    | Pick<GatewayModelResourceRow, "endpoints" | "model">
    | Pick<GatewayRouteResourceRow, "endpoints" | "model" | "provider">;
}) {
  const { t } = useLanguage();
  return (
    <div className="grid min-w-[260px] grid-cols-2 gap-1 lg:grid-cols-4">
      {row.endpoints.map((endpoint) => {
        const available = endpoint.routable_accounts > 0 && endpoint.status !== "blocked";
        const title = endpointTitle(endpoint, t);
        const href = endpointDiagnosticsHref(row, endpoint);
        const className = available
          ? "border-srapi-success/30 bg-srapi-success/10 text-srapi-success inline-flex items-center justify-between gap-1 rounded-md border px-1.5 py-0.5 font-mono text-[10px]"
          : "border-srapi-border bg-srapi-card-muted text-srapi-text-tertiary hover:border-srapi-border-strong hover:text-srapi-text-primary inline-flex items-center justify-between gap-1 rounded-md border px-1.5 py-0.5 font-mono text-[10px] transition-colors";
        const content = (
          <>
            <span>{t(`adminGatewayResources.endpointShort.${endpoint.key}`)}</span>
            <span className="tabular">
              {endpoint.routable_accounts}/{endpoint.candidate_accounts}
            </span>
          </>
        );
        if (href) {
          return (
            <Link key={endpoint.key} href={href} title={title} className={className}>
              {content}
            </Link>
          );
        }
        return (
          <span key={endpoint.key} title={title} className={className}>
            {content}
          </span>
        );
      })}
    </div>
  );
}

function endpointDiagnosticsHref(
  row:
    | Pick<GatewayModelResourceRow, "model" | "endpoints">
    | Pick<GatewayRouteResourceRow, "model" | "provider" | "endpoints">,
  endpoint: GatewayEndpointResourceRow,
) {
  if (endpoint.status !== "blocked") return undefined;
  if (!("provider" in row)) {
    return `${ADMIN_ROUTES.gatewayResources}?f_scope=routes&q=${encodeURIComponent(endpoint.source_endpoint)}`;
  }
  if (endpoint.unsupported_accounts > 0) {
    return `${ADMIN_ROUTES.providers}?q=${encodeURIComponent(row.provider.name)}`;
  }
  if (endpoint.unavailable_model_accounts > 0) {
    return `${ADMIN_ROUTES.accounts}?f_providerId=${encodeURIComponent(row.provider.id)}`;
  }
  return `${ADMIN_ROUTES.models}?q=${encodeURIComponent(row.model.canonical_name)}`;
}

type GatewayFixActionRow =
  | GatewayProviderResourceRow
  | GatewayModelResourceRow
  | GatewayRouteResourceRow;

interface GatewayFixAction {
  key: string;
  href: string;
  labelKey: string;
}

function GatewayRowFixActions({ row }: { row: GatewayFixActionRow }) {
  const { t } = useLanguage();
  const actions = gatewayRowFixActions(row);
  if (actions.length === 0) {
    return <span className="text-2xs text-srapi-text-tertiary">-</span>;
  }
  return (
    <div className="flex min-w-[130px] flex-wrap gap-1">
      {actions.slice(0, 3).map((action) => (
        <Link
          key={action.key}
          href={action.href}
          className="border-srapi-border bg-srapi-card-muted hover:border-srapi-border-strong text-2xs text-srapi-text-secondary rounded-md border px-2 py-1 font-mono transition-colors"
        >
          {t(`adminGatewayResources.fixAction.${action.labelKey}`)}
        </Link>
      ))}
      {actions.length > 3 ? (
        <span className="text-2xs text-srapi-text-tertiary font-mono">+{actions.length - 3}</span>
      ) : null}
    </div>
  );
}

function gatewayRowFixActions(row: GatewayFixActionRow): GatewayFixAction[] {
  const actions: GatewayFixAction[] = [];
  const add = (action: GatewayFixAction) => {
    if (!actions.some((existing) => existing.key === action.key)) {
      actions.push(action);
    }
  };

  if (hasProvider(row)) {
    if (hasAccountBlockers(row)) {
      if (
        row.account_blockers.inactive > 0 ||
        row.account_blockers.health > 0 ||
        row.account_blockers.quota > 0
      ) {
        add({
          key: "accounts",
          href: accountsHref(row.provider.id),
          labelKey: "accounts",
        });
      }
      if (row.account_blockers.proxy > 0) {
        add({
          key: "proxies",
          href: ADMIN_ROUTES.proxies,
          labelKey: "proxies",
        });
      }
    }
    for (const reason of row.reasons) {
      if (reason === "provider_disabled") {
        add({
          key: "provider",
          href: providerHref(row.provider.name),
          labelKey: "provider",
        });
      }
      if (reason === "no_active_accounts" || reason === "no_routable_accounts") {
        add({
          key: "accounts",
          href: accountsHref(row.provider.id),
          labelKey: "accounts",
        });
      }
      if (reason === "proxy_attention") {
        add({
          key: "proxies",
          href: ADMIN_ROUTES.proxies,
          labelKey: "proxies",
        });
      }
      if (reason === "no_api_keys") {
        add({
          key: "apiKeys",
          href: ADMIN_ROUTES.apiKeys,
          labelKey: "apiKeys",
        });
      }
      if (reason === "no_model_mappings") {
        add({
          key: "mappings",
          href: hasModel(row) ? modelsHref(row.model.canonical_name) : ADMIN_ROUTES.models,
          labelKey: "modelMappings",
        });
      }
    }
  }

  if (hasModel(row)) {
    for (const reason of row.reasons) {
      if (reason === "no_active_models") {
        add({ key: "models", href: modelsHref(row.model.canonical_name), labelKey: "models" });
      }
      if (reason === "pricing_uncovered") {
        add({
          key: "pricing",
          href: pricingHref(
            row.model.id,
            hasProvider(row) ? row.provider.id : undefined,
            row.model.canonical_name,
          ),
          labelKey: "pricing",
        });
      }
    }
    if (gatewayPricingNeedsAttention(row.pricing)) {
      add({
        key: "pricing",
        href: pricingHref(
          row.model.id,
          hasProvider(row) ? row.provider.id : undefined,
          row.model.canonical_name,
        ),
        labelKey: "pricing",
      });
    }
    for (const endpoint of row.endpoints) {
      if (endpoint.status !== "blocked") continue;
      if (hasProvider(row) && endpoint.unsupported_accounts > 0) {
        add({
          key: "endpointSwitches",
          href: providerHref(row.provider.name),
          labelKey: "endpointSwitches",
        });
      }
      if (hasProvider(row) && endpoint.unavailable_model_accounts > 0) {
        add({
          key: "accountModels",
          href: accountsHref(row.provider.id),
          labelKey: "accountModels",
        });
      }
    }
  }

  if (!hasProvider(row)) {
    for (const reason of row.reasons) {
      if (reason === "no_model_mappings") {
        add({
          key: "mappings",
          href: modelsHref(hasModel(row) ? row.model.canonical_name : ""),
          labelKey: "modelMappings",
        });
      }
      if (reason === "no_api_keys") {
        add({ key: "apiKeys", href: ADMIN_ROUTES.apiKeys, labelKey: "apiKeys" });
      }
    }
  }

  return actions;
}

function hasProvider(row: GatewayFixActionRow): row is GatewayProviderResourceRow | GatewayRouteResourceRow {
  return "provider" in row;
}

function hasModel(row: GatewayFixActionRow): row is GatewayModelResourceRow | GatewayRouteResourceRow {
  return "model" in row;
}

function hasAccountBlockers(row: GatewayFixActionRow): row is GatewayProviderResourceRow {
  return "account_blockers" in row;
}

function providerHref(providerName: string) {
  return `${ADMIN_ROUTES.providers}?q=${encodeURIComponent(providerName)}`;
}

function modelsHref(modelName: string) {
  return modelName
    ? `${ADMIN_ROUTES.models}?q=${encodeURIComponent(modelName)}`
    : ADMIN_ROUTES.models;
}

function accountsHref(providerID: string | number) {
  return `${ADMIN_ROUTES.accounts}?f_providerId=${encodeURIComponent(String(providerID))}`;
}

function pricingHref(
  modelID: string | number,
  providerID: string | number | undefined,
  fallbackSearch: string,
) {
  const params = new URLSearchParams();
  params.set("f_modelId", String(modelID));
  if (providerID !== undefined) {
    params.set("f_providerId", String(providerID));
  } else if (fallbackSearch) {
    params.set("q", fallbackSearch);
  }
  const qs = params.toString();
  return qs ? `${ADMIN_ROUTES.channelsPricing}?${qs}` : ADMIN_ROUTES.channelsPricing;
}

function endpointTitle(
  endpoint: GatewayEndpointResourceRow,
  t: (key: string, params?: Record<string, string | number>) => string,
) {
  return [
    `${t(`adminGatewayResources.endpoint.${endpoint.key}`)} · ${endpoint.source_endpoint}`,
    `${t("adminGatewayResources.endpointDiagnostics.routable")}: ${endpoint.routable_accounts}`,
    `${t("adminGatewayResources.endpointDiagnostics.candidate")}: ${endpoint.candidate_accounts}`,
    `${t("adminGatewayResources.endpointDiagnostics.unsupported")}: ${endpoint.unsupported_accounts}`,
    `${t("adminGatewayResources.endpointDiagnostics.unavailableModel")}: ${endpoint.unavailable_model_accounts}`,
  ].join("\n");
}

function resourceStatusMeta(
  status: GatewayProviderResourceStatus,
  t: (key: string, params?: Record<string, string | number>) => string,
) {
  if (status === "ready") {
    return {
      quiet: "active" as const,
      label: t("adminGatewayResources.ready"),
      icon: CheckCircle2,
    };
  }
  if (status === "limited") {
    return {
      quiet: "limited" as const,
      label: t("adminGatewayResources.limited"),
      icon: AlertTriangle,
    };
  }
  return {
    quiet: "error" as const,
    label: t("adminGatewayResources.blocked"),
    icon: AlertTriangle,
  };
}

function ProviderResourceRow({ row }: { row: GatewayProviderResourceRow }) {
  const { t } = useLanguage();
  const status = resourceStatusMeta(row.status, t);
  const StatusIcon = status.icon;
  return (
    <TableRow>
      <TableCell>
        <div className="min-w-0">
          <Link
            href={ADMIN_ROUTES.providers}
            className="text-srapi-text-primary hover:text-srapi-accent truncate transition-colors"
          >
            {row.provider.display_name || row.provider.name}
          </Link>
          <div className="text-2xs text-srapi-text-tertiary truncate font-mono">
            {row.provider.name}
          </div>
        </div>
      </TableCell>
      <TableCell className="text-2xs text-srapi-text-secondary font-mono">
        {row.provider.adapter_type}
      </TableCell>
      <TableCell>
        <EndpointCapabilitySwitchStrip provider={row.provider} />
      </TableCell>
      <TableCell className="text-2xs tabular text-right font-mono">
        <span
          className={row.active_model_mappings > 0 ? "text-srapi-text-primary" : "text-srapi-error"}
        >
          {row.active_model_mappings}
        </span>
      </TableCell>
      <TableCell className="text-2xs tabular text-right font-mono">
        <div className="flex min-w-[120px] flex-col items-end gap-1">
          <div>
            <span className="text-srapi-success">{row.routable_accounts}</span>
            <span className="text-srapi-text-tertiary"> / {row.total_accounts}</span>
          </div>
          <AccountBlockersStrip blockers={row.account_blockers} />
        </div>
      </TableCell>
      <TableCell className="text-2xs tabular text-right font-mono">
        <span
          className={
            row.proxy_attention_accounts > 0 ? "text-srapi-error" : "text-srapi-text-primary"
          }
        >
          {row.proxied_accounts}
        </span>
        {row.proxy_attention_accounts > 0 ? (
          <span className="text-srapi-text-tertiary"> · {row.proxy_attention_accounts}</span>
        ) : null}
      </TableCell>
      <TableCell className="text-2xs tabular text-right font-mono">
        <span className="text-srapi-text-primary">{row.api_key_count}</span>
        {row.scoped_key_count > 0 ? (
          <span className="text-srapi-text-tertiary"> · {row.scoped_key_count}</span>
        ) : null}
      </TableCell>
      <TableCell>
        <span className="inline-flex items-center gap-1.5">
          <StatusIcon className="text-srapi-text-tertiary size-3.5" />
          <QuietBadge status={status.quiet} label={status.label} />
        </span>
      </TableCell>
      <TableCell>
        {row.reasons.length > 0 ? (
          <div className="flex max-w-md flex-wrap gap-1">
            {row.reasons.map((reason) => (
              <span
                key={reason}
                className="border-srapi-border bg-srapi-card-muted text-2xs text-srapi-text-tertiary rounded-md border px-1.5 py-0.5 font-mono"
              >
                {t(`adminGatewayResources.reason.${reason}`)}
              </span>
            ))}
          </div>
        ) : (
          <span className="text-2xs text-srapi-text-tertiary">—</span>
        )}
      </TableCell>
      <TableCell>
        <GatewayRowFixActions row={row} />
      </TableCell>
    </TableRow>
  );
}

function EndpointCapabilitySwitchStrip({
  provider,
}: {
  provider: GatewayProviderResourceRow["provider"];
}) {
  const { t } = useLanguage();
  return (
    <div className="flex min-w-[190px] flex-wrap gap-1">
      {CONVERSATION_ENDPOINT_KEYS.map((key) => {
        const mode = providerEndpointCapabilityMode(provider.capabilities, key);
        const label = `${t(`adminGatewayResources.endpointShort.${key}`)}:${t(
          `adminGatewayResources.capabilityModeShort.${mode}`,
        )}`;
        return (
          <Link
            key={key}
            href={`${ADMIN_ROUTES.providers}?q=${encodeURIComponent(provider.name)}`}
            className="border-srapi-border bg-srapi-card-muted hover:border-srapi-border-strong inline-flex items-center gap-1 rounded-md border px-1.5 py-0.5 font-mono text-[10px] transition-colors"
            title={t("adminGatewayResources.endpointSwitchHint", {
              endpoint: t(`adminGatewayResources.endpoint.${key}`),
              mode: t(`adminGatewayResources.capabilityMode.${mode}`),
            })}
          >
            <span className={endpointCapabilityModeTone(mode)}>■</span>
            <span className="text-srapi-text-tertiary">{label}</span>
          </Link>
        );
      })}
    </div>
  );
}

function providerEndpointCapabilityMode(
  capabilities: Record<string, unknown> | undefined,
  key: (typeof CONVERSATION_ENDPOINT_KEYS)[number],
) {
  const value = capabilities?.[key];
  if (value === true) return "on";
  if (value === false) return "off";
  return "auto";
}

function endpointCapabilityModeTone(mode: "auto" | "on" | "off") {
  if (mode === "on") return "text-srapi-success";
  if (mode === "off") return "text-srapi-error";
  return "text-srapi-text-tertiary";
}

function AccountBlockersStrip({ blockers }: { blockers: GatewayAccountBlockers }) {
  const { t } = useLanguage();
  const items = [
    { key: "inactive", value: blockers.inactive },
    { key: "health", value: blockers.health },
    { key: "quota", value: blockers.quota },
    { key: "proxy", value: blockers.proxy },
  ].filter((item) => item.value > 0);
  if (items.length === 0) {
    return null;
  }
  return (
    <div
      className="flex flex-wrap justify-end gap-1"
      title={items
        .map((item) => `${t(`adminGatewayResources.accountBlockers.${item.key}`)}: ${item.value}`)
        .join("\n")}
    >
      {items.map((item) => (
        <span
          key={item.key}
          className="border-srapi-error/20 bg-srapi-error/10 text-srapi-error rounded px-1 py-0.5 text-[10px] leading-none"
        >
          {t(`adminGatewayResources.accountBlockersShort.${item.key}`)}
          <span className="tabular ml-0.5">{item.value}</span>
        </span>
      ))}
    </div>
  );
}

function GatewayResourcesSkeleton() {
  return (
    <div className="space-y-4">
      <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-5">
        {Array.from({ length: 5 }).map((_, index) => (
          <Card key={index} className="p-5">
            <Skeleton className="h-3 w-28" />
            <Skeleton className="mt-4 h-8 w-20" />
          </Card>
        ))}
      </div>
      <Card className="p-5">
        <Skeleton className="h-5 w-44" />
        <Skeleton className="mt-4 h-48 w-full" />
      </Card>
    </div>
  );
}

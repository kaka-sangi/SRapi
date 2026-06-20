"use client";

import Link from "next/link";
import { Activity, AlertTriangle, Cable, CheckCircle2, Globe, KeyRound, Route } from "lucide-react";
import { AdminShell } from "@/components/layout/admin-shell";
import { PageHeader } from "@/components/layout/page-header";
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
import { useLanguage } from "@/context/LanguageContext";
import { useAdminGatewayResources } from "@/hooks/admin-queries";
import type { GatewayModelResourceRow, GatewayProviderResourceRow } from "@/lib/sdk-types";

export default function AdminGatewayResourcesPage() {
  return (
    <AdminShell>
      <GatewayResourcesContent />
    </AdminShell>
  );
}

function GatewayResourcesContent() {
  const { t } = useLanguage();
  const gatewayResources = useAdminGatewayResources();
  const loading = gatewayResources.isLoading;
  const error = gatewayResources.isError;
  const summary = gatewayResources.data;

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

          <Card>
            <CardHeader>
              <CardTitle>{t("adminGatewayResources.providerMatrix")}</CardTitle>
            </CardHeader>
            <CardContent className="p-0">
              <TableScroll minWidth={760}>
                <Table>
                  <TableHeader>
                    <tr>
                      <TableHead>{t("adminProviders.name")}</TableHead>
                      <TableHead>{t("adminProviders.adapterType")}</TableHead>
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
                    </tr>
                  </TableHeader>
                  <TableBody>
                    {(summary?.rows ?? []).map((row) => (
                      <ProviderResourceRow key={row.provider.id} row={row} />
                    ))}
                  </TableBody>
                </Table>
              </TableScroll>
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle>{t("adminGatewayResources.modelMatrix")}</CardTitle>
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
                    </tr>
                  </TableHeader>
                  <TableBody>
                    {(summary?.model_rows ?? []).map((row) => (
                      <ModelResourceRow key={row.model.id} row={row} />
                    ))}
                  </TableBody>
                </Table>
              </TableScroll>
            </CardContent>
          </Card>
        </div>
      ) : null}
    </>
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

function ModelResourceRow({ row }: { row: GatewayModelResourceRow }) {
  const { t } = useLanguage();
  const status =
    row.status === "ready"
      ? { quiet: "active" as const, label: t("adminGatewayResources.ready"), icon: CheckCircle2 }
      : row.status === "limited"
        ? {
            quiet: "limited" as const,
            label: t("adminGatewayResources.limited"),
            icon: AlertTriangle,
          }
        : {
            quiet: "error" as const,
            label: t("adminGatewayResources.blocked"),
            icon: AlertTriangle,
          };
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
    </TableRow>
  );
}

function PricingCoverageBadge({ pricing }: { pricing: GatewayModelResourceRow["pricing"] }) {
  const { t } = useLanguage();
  const status =
    pricing.status === "priced"
      ? "active"
      : pricing.status === "error"
        ? "error"
        : "limited";
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
      <span className="text-2xs text-srapi-text-tertiary font-mono tabular">
        {routeCount}
        {pricing.currency ? <span> · {pricing.currency}</span> : null}
      </span>
    </div>
  );
}

function EndpointMatrix({ row }: { row: GatewayModelResourceRow }) {
  const { t } = useLanguage();
  return (
    <div className="grid min-w-[260px] grid-cols-2 gap-1 lg:grid-cols-4">
      {row.endpoints.map((endpoint) => {
        const available = endpoint.routable_accounts > 0 && endpoint.status !== "blocked";
        return (
          <span
            key={endpoint.key}
            title={`${t(`adminGatewayResources.endpoint.${endpoint.key}`)}: ${endpoint.routable_accounts}`}
            className={
              available
                ? "border-srapi-success/30 bg-srapi-success/10 text-srapi-success inline-flex items-center justify-between gap-1 rounded-md border px-1.5 py-0.5 font-mono text-[10px]"
                : "border-srapi-border bg-srapi-card-muted text-srapi-text-tertiary inline-flex items-center justify-between gap-1 rounded-md border px-1.5 py-0.5 font-mono text-[10px]"
            }
          >
            <span>{t(`adminGatewayResources.endpointShort.${endpoint.key}`)}</span>
            <span className="tabular">{endpoint.routable_accounts}</span>
          </span>
        );
      })}
    </div>
  );
}

function ProviderResourceRow({ row }: { row: GatewayProviderResourceRow }) {
  const { t } = useLanguage();
  const status =
    row.status === "ready"
      ? { quiet: "active" as const, label: t("adminGatewayResources.ready"), icon: CheckCircle2 }
      : row.status === "limited"
        ? {
            quiet: "limited" as const,
            label: t("adminGatewayResources.limited"),
            icon: AlertTriangle,
          }
        : {
            quiet: "error" as const,
            label: t("adminGatewayResources.blocked"),
            icon: AlertTriangle,
          };
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
      <TableCell className="text-2xs tabular text-right font-mono">
        <span
          className={row.active_model_mappings > 0 ? "text-srapi-text-primary" : "text-srapi-error"}
        >
          {row.active_model_mappings}
        </span>
      </TableCell>
      <TableCell className="text-2xs tabular text-right font-mono">
        <span className="text-srapi-success">{row.routable_accounts}</span>
        <span className="text-srapi-text-tertiary"> / {row.total_accounts}</span>
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
    </TableRow>
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

"use client";

import Link from "next/link";
import { useState } from "react";
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
import {
  useAccountsHealthSummary,
  useAdminAccounts,
  useAdminApiKeys,
  useAdminModelMappings,
  useAdminModels,
  useAdminProviders,
  useAdminProxies,
} from "@/hooks/admin-queries";
import {
  buildGatewayResourceSummary,
  type GatewayProviderResourceRow,
} from "@/lib/admin-gateway-resources";

const OVERVIEW_LIMIT = 500;

export default function AdminGatewayResourcesPage() {
  return (
    <AdminShell>
      <GatewayResourcesContent />
    </AdminShell>
  );
}

function GatewayResourcesContent() {
  const { t } = useLanguage();
  const providers = useAdminProviders({ page: 1, page_size: OVERVIEW_LIMIT });
  const accounts = useAdminAccounts({ page: 1, page_size: OVERVIEW_LIMIT });
  const apiKeys = useAdminApiKeys({ page: 1, page_size: OVERVIEW_LIMIT });
  const models = useAdminModels({ page: 1, page_size: OVERVIEW_LIMIT });
  const modelMappings = useAdminModelMappings({ page: 1, page_size: OVERVIEW_LIMIT, status: "active" });
  const proxies = useAdminProxies({ page: 1, page_size: OVERVIEW_LIMIT });
  const health = useAccountsHealthSummary();
  const [nowMs] = useState(() => Date.now());

  const loading =
    providers.isLoading ||
    accounts.isLoading ||
    apiKeys.isLoading ||
    models.isLoading ||
    modelMappings.isLoading ||
    proxies.isLoading ||
    health.isLoading;
  const error =
    providers.isError ||
    accounts.isError ||
    apiKeys.isError ||
    models.isError ||
    modelMappings.isError ||
    proxies.isError ||
    health.isError;
  const summary = buildGatewayResourceSummary({
    providers: providers.data?.data ?? [],
    accounts: accounts.data?.data ?? [],
    apiKeys: apiKeys.data?.data ?? [],
    models: models.data?.data ?? [],
    modelMappings: modelMappings.data?.data ?? [],
    proxies: proxies.data?.data ?? [],
    health: health.data ?? [],
    nowMs,
  });

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
              value={`${summary.activeProviders}/${summary.providers}`}
            />
            <ResourceKpi
              icon={Activity}
              label={t("adminGatewayResources.routableAccounts")}
              value={`${summary.routableAccounts}/${summary.activeAccounts}`}
            />
            <ResourceKpi
              icon={Route}
              label={t("adminGatewayResources.activeModels")}
              value={String(summary.activeModels)}
            />
            <ResourceKpi
              icon={Globe}
              label={t("adminGatewayResources.availableProxies")}
              value={`${summary.availableProxies}/${summary.activeProxies}`}
            />
            <ResourceKpi
              icon={KeyRound}
              label={t("adminGatewayResources.scopedApiKeys")}
              value={`${summary.scopedApiKeys}/${summary.activeApiKeys}`}
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
                      <TableHead className="text-right">{t("adminGatewayResources.modelMappings")}</TableHead>
                      <TableHead className="text-right">{t("adminGatewayResources.accounts")}</TableHead>
                      <TableHead className="text-right">{t("adminGatewayResources.proxies")}</TableHead>
                      <TableHead className="text-right">{t("adminGatewayResources.apiKeys")}</TableHead>
                      <TableHead>{t("adminCommon.status")}</TableHead>
                      <TableHead>{t("adminGatewayResources.blockers")}</TableHead>
                    </tr>
                  </TableHeader>
                  <TableBody>
                    {summary.rows.map((row) => (
                      <ProviderResourceRow key={row.provider.id} row={row} />
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
        <span className="font-mono text-2xs uppercase text-srapi-text-tertiary">{label}</span>
        <Icon className="size-4 text-srapi-text-tertiary" />
      </div>
      <div className="mt-3 font-serif text-3xl leading-none text-srapi-text-primary tabular">
        {value}
      </div>
    </Card>
  );
}

function ProviderResourceRow({ row }: { row: GatewayProviderResourceRow }) {
  const { t } = useLanguage();
  const status =
    row.status === "ready"
      ? { quiet: "active" as const, label: t("adminGatewayResources.ready"), icon: CheckCircle2 }
      : row.status === "limited"
        ? { quiet: "limited" as const, label: t("adminGatewayResources.limited"), icon: AlertTriangle }
        : { quiet: "error" as const, label: t("adminGatewayResources.blocked"), icon: AlertTriangle };
  const StatusIcon = status.icon;
  return (
    <TableRow>
      <TableCell>
        <div className="min-w-0">
          <Link
            href={ADMIN_ROUTES.providers}
            className="truncate text-srapi-text-primary transition-colors hover:text-srapi-accent"
          >
            {row.provider.display_name || row.provider.name}
          </Link>
          <div className="truncate font-mono text-2xs text-srapi-text-tertiary">{row.provider.name}</div>
        </div>
      </TableCell>
      <TableCell className="font-mono text-2xs text-srapi-text-secondary">
        {row.provider.adapter_type}
      </TableCell>
      <TableCell className="text-right font-mono text-2xs tabular">
        <span className={row.activeModelMappings > 0 ? "text-srapi-text-primary" : "text-srapi-error"}>
          {row.activeModelMappings}
        </span>
      </TableCell>
      <TableCell className="text-right font-mono text-2xs tabular">
        <span className="text-srapi-success">{row.routableAccounts}</span>
        <span className="text-srapi-text-tertiary"> / {row.totalAccounts}</span>
      </TableCell>
      <TableCell className="text-right font-mono text-2xs tabular">
        <span className={row.proxyAttentionAccounts > 0 ? "text-srapi-error" : "text-srapi-text-primary"}>
          {row.proxiedAccounts}
        </span>
        {row.proxyAttentionAccounts > 0 ? (
          <span className="text-srapi-text-tertiary"> · {row.proxyAttentionAccounts}</span>
        ) : null}
      </TableCell>
      <TableCell className="text-right font-mono text-2xs tabular">
        <span className="text-srapi-text-primary">{row.apiKeyCount}</span>
        {row.scopedKeyCount > 0 ? (
          <span className="text-srapi-text-tertiary"> · {row.scopedKeyCount}</span>
        ) : null}
      </TableCell>
      <TableCell>
        <span className="inline-flex items-center gap-1.5">
          <StatusIcon className="size-3.5 text-srapi-text-tertiary" />
          <QuietBadge status={status.quiet} label={status.label} />
        </span>
      </TableCell>
      <TableCell>
        {row.reasons.length > 0 ? (
          <div className="flex max-w-md flex-wrap gap-1">
            {row.reasons.map((reason) => (
              <span
                key={reason}
                className="rounded-md border border-srapi-border bg-srapi-card-muted px-1.5 py-0.5 font-mono text-2xs text-srapi-text-tertiary"
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

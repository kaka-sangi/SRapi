"use client";

import type { ReactNode } from "react";
import { useMemo } from "react";
import Link from "next/link";
import { useParams } from "next/navigation";
import { useQueries } from "@tanstack/react-query";
import {
  ArrowUpRight,
  Fingerprint,
  GitBranch,
  Layers,
  Tag,
  type LucideIcon,
} from "lucide-react";
import { AdminShell } from "@/components/layout/admin-shell";
import { PageHeader } from "@/components/layout/page-header";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { QuietBadge } from "@/components/ui/quiet-badge";
import {
  useAdminAccounts,
  useAdminModels,
  useAdminPricingRules,
  useAdminProviders,
  useErrorPassthroughRules,
  useModelRateLimits,
  usePayloadRules,
  useTlsProfiles,
} from "@/hooks/admin-queries";
import { useLanguage } from "@/context/LanguageContext";
import { adminApi } from "@/lib/admin-api";
import { formatDateTime, formatMoney } from "@/lib/admin-format";
import { ADMIN_ROUTES } from "@/lib/routes";
import { quietStatusFor, statusLabel } from "@/lib/status-badge";
import type {
  ErrorPassthroughRule,
  Model,
  ModelProviderMapping,
  ModelRateLimit,
  PayloadRule,
  PricingRule,
  Provider,
  TlsProfile,
} from "@/lib/sdk-types";

type MappingWithModel = ModelProviderMapping & { model: Model };
type LimitWithModel = ModelRateLimit & { model: Model };

export default function AdminChannelDetailPage() {
  return (
    <AdminShell>
      <ChannelDetailContent />
    </AdminShell>
  );
}

function useChannelParam(): string {
  const params = useParams<{ id?: string | string[] }>();
  const raw = params.id;
  return decodeURIComponent(Array.isArray(raw) ? raw[0] ?? "" : raw ?? "");
}

function idEquals(left: string | number | null | undefined, right: string | number): boolean {
  return String(left ?? "") === String(right);
}

function modelLabel(model: Model | undefined, fallback: string | number): string {
  if (!model) return String(fallback);
  return model.display_name || model.canonical_name || String(model.id);
}

function protocolMatches(rule: PayloadRule, provider: Provider): boolean {
  const protocol = (rule.match_protocol || "").trim().toLowerCase();
  if (!protocol || protocol === "*") return true;
  return protocol === provider.protocol.toLowerCase() || protocol === provider.name.toLowerCase();
}

function sortedPayloadRules(rules: PayloadRule[], provider: Provider): PayloadRule[] {
  return rules
    .filter((rule) => protocolMatches(rule, provider))
    .sort((a, b) => a.priority - b.priority || a.name.localeCompare(b.name));
}

function sortedErrorRules(rules: ErrorPassthroughRule[]): ErrorPassthroughRule[] {
  return [...rules].sort((a, b) => a.priority - b.priority || a.name.localeCompare(b.name));
}

function rateSummary(limit: ModelRateLimit, offLabel: string): string {
  if (!limit.enabled) return offLabel;
  return `${limit.rpm_limit} RPM · ${limit.tpm_limit} TPM · ${limit.max_concurrency} CC`;
}

function ChannelDetailContent() {
  const { t } = useLanguage();
  const requestedId = useChannelParam();
  const providers = useAdminProviders({ page: 1, page_size: 200 });
  const channel = useMemo(
    () =>
      (providers.data?.data ?? []).find(
        (provider) => provider.id === requestedId || provider.name === requestedId,
      ) ?? null,
    [providers.data, requestedId],
  );
  const providerId = channel?.id ?? "0";

  const accounts = useAdminAccounts({ page: 1, page_size: 200, provider_id: providerId });
  const models = useAdminModels({ page: 1, page_size: 200 });
  const pricing = useAdminPricingRules({ page: 1, page_size: 500 });
  const limits = useModelRateLimits();
  const payloadRules = usePayloadRules();
  const errorRules = useErrorPassthroughRules();
  const tlsProfiles = useTlsProfiles();

  const modelRows = useMemo(() => models.data?.data ?? [], [models.data]);
  const modelMap = useMemo(
    () => new Map(modelRows.map((model) => [String(model.id), model] as const)),
    [modelRows],
  );
  const mappingQueries = useQueries({
    queries: modelRows.map((model) => ({
      queryKey: ["admin", "models", model.id, "mappings", "channel-detail"],
      queryFn: () => adminApi.listModelMappings(model.id),
      enabled: Boolean(channel),
      staleTime: 30_000,
    })),
  });

  const mappings = useMemo<MappingWithModel[]>(() => {
    if (!channel) return [];
    return mappingQueries.flatMap((query, index) => {
      const model = modelRows[index];
      if (!model) return [];
      return (query.data?.data ?? [])
        .filter((mapping) => idEquals(mapping.provider_id, channel.id))
        .map((mapping) => ({ ...mapping, model }));
    });
  }, [channel, mappingQueries, modelRows]);

  const mappedModelIds = useMemo(
    () => new Set(mappings.map((mapping) => String(mapping.model_id))),
    [mappings],
  );
  const channelLimits = useMemo<LimitWithModel[]>(
    () =>
      (limits.data?.data ?? [])
        .filter((limit) => mappedModelIds.has(String(limit.model_id)))
        .map((limit) => ({ ...limit, model: modelMap.get(String(limit.model_id)) }))
        .filter((limit): limit is LimitWithModel => Boolean(limit.model)),
    [limits.data, mappedModelIds, modelMap],
  );
  const channelPricing = useMemo(
    () =>
      (pricing.data?.data ?? []).filter(
        (rule) => idEquals(rule.provider_id, providerId) || idEquals(rule.provider_id, "0"),
      ),
    [pricing.data, providerId],
  );
  const channelPayloadRules = channel
    ? sortedPayloadRules(payloadRules.data?.data ?? [], channel)
    : [];
  const sortedErrors = sortedErrorRules(errorRules.data?.data ?? []);
  const activeAccounts = (accounts.data?.data ?? []).filter((account) => account.status === "active");
  const activeTlsProfiles = (tlsProfiles.data?.data ?? []).filter((profile) => profile.enabled);
  const mappingLoading = mappingQueries.some((query) => query.isLoading);

  if (providers.isLoading) {
    return <ChannelSkeleton />;
  }

  if (!channel) {
    return (
      <Card>
        <CardContent>
          <p className="text-sm text-srapi-text-secondary">
            {t("adminChannelsDetail.notFound", { id: requestedId })}
          </p>
        </CardContent>
      </Card>
    );
  }

  return (
    <>
      <PageHeader
        eyebrow={t("nav.sectionAdminGateway")}
        title={t("adminChannelsDetail.title", { name: channel.display_name || channel.name })}
        description={t("adminChannelsDetail.subtitle", {
          protocol: channel.protocol,
          adapter: channel.adapter_type,
        })}
        actions={
          <Button asChild variant="outline" size="sm">
            <Link href={ADMIN_ROUTES.providers}>
              {t("adminChannelsDetail.manageProvider")}
              <ArrowUpRight className="size-3.5" />
            </Link>
          </Button>
        }
      />

      <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
        <SummaryCard
          icon={Layers}
          label={t("adminChannelsDetail.accounts")}
          value={`${activeAccounts.length}/${accounts.data?.data.length ?? 0}`}
          loading={accounts.isLoading}
        />
        <SummaryCard
          icon={GitBranch}
          label={t("adminChannelsDetail.modelMappings")}
          value={mappings.length}
          loading={mappingLoading}
        />
        <SummaryCard
          icon={Tag}
          label={t("adminChannelsDetail.pricingRules")}
          value={channelPricing.length}
          loading={pricing.isLoading}
        />
        <SummaryCard
          icon={Fingerprint}
          label={t("adminChannelsDetail.tlsFingerprints")}
          value={activeTlsProfiles.length}
          loading={tlsProfiles.isLoading}
        />
      </div>

      <Tabs defaultValue="pricing">
        <TabsList className="flex flex-wrap">
          <TabsTrigger value="pricing">{t("adminChannelsDetail.tabPricing")}</TabsTrigger>
          <TabsTrigger value="mappings">{t("adminChannelsDetail.tabMappings")}</TabsTrigger>
          <TabsTrigger value="limits">{t("adminChannelsDetail.tabLimits")}</TabsTrigger>
          <TabsTrigger value="payload">{t("adminChannelsDetail.tabPayload")}</TabsTrigger>
          <TabsTrigger value="errors">{t("adminChannelsDetail.tabErrors")}</TabsTrigger>
          <TabsTrigger value="tls">{t("adminChannelsDetail.tabTls")}</TabsTrigger>
        </TabsList>

        <TabsContent value="pricing">
          <PricingPanel
            rules={channelPricing}
            loading={pricing.isLoading || models.isLoading}
            modelMap={modelMap}
          />
        </TabsContent>
        <TabsContent value="mappings">
          <MappingsPanel mappings={mappings} loading={mappingLoading} />
        </TabsContent>
        <TabsContent value="limits">
          <LimitsPanel limits={channelLimits} loading={limits.isLoading || mappingLoading} />
        </TabsContent>
        <TabsContent value="payload">
          <PayloadPanel
            rules={channelPayloadRules}
            loading={payloadRules.isLoading}
            protocol={channel.protocol}
          />
        </TabsContent>
        <TabsContent value="errors">
          <ErrorPanel rules={sortedErrors} loading={errorRules.isLoading} />
        </TabsContent>
        <TabsContent value="tls">
          <TlsPanel profiles={tlsProfiles.data?.data ?? []} loading={tlsProfiles.isLoading} />
        </TabsContent>
      </Tabs>
    </>
  );
}

function SummaryCard({
  icon: Icon,
  label,
  value,
  loading,
}: {
  icon: LucideIcon;
  label: string;
  value: string | number;
  loading?: boolean;
}) {
  return (
    <Card>
      <CardContent>
        <div className="flex items-center gap-2 font-mono text-2xs uppercase text-srapi-text-tertiary">
          <Icon className="size-3.5" />
          {label}
        </div>
        {loading ? (
          <Skeleton className="mt-3 h-8 w-24" />
        ) : (
          <div className="mt-2 font-serif text-3xl text-srapi-text-primary tabular">{value}</div>
        )}
      </CardContent>
    </Card>
  );
}

function Panel({
  title,
  actionHref,
  actionLabel,
  loading,
  empty,
  children,
}: {
  title: string;
  actionHref: string;
  actionLabel: string;
  loading?: boolean;
  empty: boolean;
  children: ReactNode;
}) {
  return (
    <Card>
      <CardHeader>
        <CardTitle>{title}</CardTitle>
        <Button asChild variant="ghost" size="sm">
          <Link href={actionHref}>
            {actionLabel}
            <ArrowUpRight className="size-3.5" />
          </Link>
        </Button>
      </CardHeader>
      <CardContent>
        {loading ? <PanelSkeleton /> : empty ? <EmptyPanel /> : children}
      </CardContent>
    </Card>
  );
}

function EmptyPanel() {
  return <p className="text-sm text-srapi-text-tertiary">—</p>;
}

function PanelSkeleton() {
  return (
    <div className="space-y-3">
      {Array.from({ length: 3 }).map((_, index) => (
        <Skeleton key={index} className="h-12 w-full" />
      ))}
    </div>
  );
}

function Row({
  primary,
  secondary,
  meta,
  badge,
}: {
  primary: ReactNode;
  secondary?: ReactNode;
  meta?: ReactNode;
  badge?: ReactNode;
}) {
  return (
    <div className="flex items-center gap-4 py-3">
      <div className="min-w-0 flex-1">
        <div className="truncate text-sm text-srapi-text-primary">{primary}</div>
        {secondary ? (
          <div className="mt-1 truncate font-mono text-2xs text-srapi-text-tertiary">
            {secondary}
          </div>
        ) : null}
      </div>
      {meta ? <div className="shrink-0 font-mono text-2xs text-srapi-text-secondary">{meta}</div> : null}
      {badge}
    </div>
  );
}

function PricingPanel({
  rules,
  loading,
  modelMap,
}: {
  rules: PricingRule[];
  loading?: boolean;
  modelMap: Map<string, Model>;
}) {
  const { t } = useLanguage();
  return (
    <Panel
      title={t("adminChannelsDetail.tabPricing")}
      actionHref={ADMIN_ROUTES.channelsPricing}
      actionLabel={t("common.edit")}
      loading={loading}
      empty={rules.length === 0}
    >
      <div className="divide-y divide-srapi-border">
        {rules.map((rule) => (
          <Row
            key={rule.id}
            primary={modelLabel(modelMap.get(String(rule.model_id)), rule.model_id)}
            secondary={`${t("adminChannelsDetail.inputOutput")} ${formatMoney(
              rule.input_price_per_million_tokens,
              rule.currency,
            )} / ${formatMoney(rule.output_price_per_million_tokens, rule.currency)}`}
            meta={formatDateTime(rule.effective_from)}
            badge={
              <QuietBadge
                status={idEquals(rule.provider_id, "0") ? "limited" : "active"}
                label={idEquals(rule.provider_id, "0") ? t("adminChannelsDetail.global") : t("common.active")}
              />
            }
          />
        ))}
      </div>
    </Panel>
  );
}

function MappingsPanel({ mappings, loading }: { mappings: MappingWithModel[]; loading?: boolean }) {
  const { t } = useLanguage();
  return (
    <Panel
      title={t("adminChannelsDetail.tabMappings")}
      actionHref={ADMIN_ROUTES.models}
      actionLabel={t("common.edit")}
      loading={loading}
      empty={mappings.length === 0}
    >
      <div className="divide-y divide-srapi-border">
        {mappings.map((mapping) => (
          <Row
            key={mapping.id}
            primary={modelLabel(mapping.model, mapping.model_id)}
            secondary={mapping.upstream_model_name}
            meta={formatDateTime(mapping.created_at)}
            badge={
              <QuietBadge
                status={quietStatusFor(mapping.status)}
                label={statusLabel(t, mapping.status)}
              />
            }
          />
        ))}
      </div>
    </Panel>
  );
}

function LimitsPanel({ limits, loading }: { limits: LimitWithModel[]; loading?: boolean }) {
  const { t } = useLanguage();
  return (
    <Panel
      title={t("adminChannelsDetail.tabLimits")}
      actionHref={ADMIN_ROUTES.models}
      actionLabel={t("common.edit")}
      loading={loading}
      empty={limits.length === 0}
    >
      <div className="divide-y divide-srapi-border">
        {limits.map((limit) => (
          <Row
            key={limit.model_id}
            primary={modelLabel(limit.model, limit.model_id)}
            secondary={rateSummary(limit, t("common.off"))}
            badge={
              <QuietBadge
                status={limit.enabled ? "active" : "disabled"}
                label={limit.enabled ? t("common.active") : t("common.off")}
              />
            }
          />
        ))}
      </div>
    </Panel>
  );
}

function PayloadPanel({
  rules,
  loading,
  protocol,
}: {
  rules: PayloadRule[];
  loading?: boolean;
  protocol: string;
}) {
  const { t } = useLanguage();
  return (
    <Panel
      title={t("adminChannelsDetail.tabPayload")}
      actionHref={ADMIN_ROUTES.payloadRules}
      actionLabel={t("common.edit")}
      loading={loading}
      empty={rules.length === 0}
    >
      <div className="divide-y divide-srapi-border">
        {rules.map((rule) => (
          <Row
            key={rule.id}
            primary={rule.name}
            secondary={`${rule.match_model || "*"} · ${rule.match_protocol || protocol}`}
            meta={`${rule.priority}`}
            badge={
              <QuietBadge
                status={rule.enabled ? "active" : "disabled"}
                label={rule.action}
              />
            }
          />
        ))}
      </div>
    </Panel>
  );
}

function ErrorPanel({ rules, loading }: { rules: ErrorPassthroughRule[]; loading?: boolean }) {
  const { t } = useLanguage();
  return (
    <Panel
      title={t("adminChannelsDetail.tabErrors")}
      actionHref={ADMIN_ROUTES.errorPassthrough}
      actionLabel={t("common.edit")}
      loading={loading}
      empty={rules.length === 0}
    >
      <div className="divide-y divide-srapi-border">
        {rules.map((rule) => (
          <Row
            key={rule.id}
            primary={rule.name}
            secondary={[...rule.classes, ...rule.keywords, ...rule.status_codes.map(String)].join(" · ") || "—"}
            meta={`${rule.priority}`}
            badge={
              <QuietBadge
                status={rule.enabled ? "active" : "disabled"}
                label={rule.action}
              />
            }
          />
        ))}
      </div>
    </Panel>
  );
}

function TlsPanel({ profiles, loading }: { profiles: TlsProfile[]; loading?: boolean }) {
  const { t } = useLanguage();
  return (
    <Panel
      title={t("adminChannelsDetail.tabTls")}
      actionHref={ADMIN_ROUTES.tlsProfiles}
      actionLabel={t("common.edit")}
      loading={loading}
      empty={profiles.length === 0}
    >
      <div className="divide-y divide-srapi-border">
        {profiles.map((profile) => (
          <Row
            key={profile.id}
            primary={profile.name}
            secondary={`${profile.tls_template || "default"} · ${profile.http_version_policy}`}
            meta={profile.user_agent || "—"}
            badge={
              <QuietBadge
                status={profile.enabled ? "active" : "disabled"}
                label={profile.enabled ? t("common.active") : t("common.disabled")}
              />
            }
          />
        ))}
      </div>
    </Panel>
  );
}

function ChannelSkeleton() {
  return (
    <>
      <Skeleton className="h-20 w-full" />
      <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
        {Array.from({ length: 4 }).map((_, index) => (
          <Skeleton key={index} className="h-28 w-full" />
        ))}
      </div>
      <Skeleton className="h-96 w-full" />
    </>
  );
}

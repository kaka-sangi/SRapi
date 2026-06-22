"use client";

import { useState } from "react";
import Link from "next/link";
import { useSearchParams, useRouter } from "next/navigation";
import {
  BellRing,
  Maximize2,
  Minimize2,
  Pencil,
  Trash2,
} from "lucide-react";
import { AdminShell } from "@/components/layout/admin-shell";
import { SectionHero } from "@/components/visual/section-hero";
import { PageQueryState } from "@/components/layout/page-query-state";
import { SloFormDialog } from "@/components/admin/slo-form-dialog";
import { AlertRuleFormDialog } from "@/components/admin/alert-rule-form-dialog";
import { AlertSilenceFormDialog } from "@/components/admin/alert-silence-form-dialog";
import { OpsNotificationChannelFormDialog } from "@/components/admin/ops-notification-channel-form-dialog";
import { ConfirmDialog } from "@/components/admin/confirm-dialog";
import { OpsLogCleanupDialog } from "@/components/admin/ops-log-cleanup-dialog";
import { ResourceFormDialog, type FieldConfig } from "@/components/admin/resource-form-dialog";
import {
  useOpsSlos,
  useOpsAlerts,
  useAcknowledgeAlert,
  useOpsRealtimeTraffic,
  useOpsThroughput,
  useOpsErrorTrend,
  useUpdateOpsSettings,
  useOpsAlertRules,
  useDeleteOpsAlertRule,
  useDeleteOpsSlo,
  useOpsAlertSilences,
  useDeleteOpsAlertSilence,
  useOpsNotificationChannels,
  useOpsNotificationDeliveries,
  useDeleteOpsNotificationChannel,
  useOpsSystemLogHealth,
} from "@/hooks/admin-queries";
import { useOpsLatencyHistogram, useOpsErrorDistribution } from "@/hooks/admin-queries/ops-charts";
import { OpsLatencyHistogramChart } from "@/components/admin/ops-latency-histogram-chart";
import { OpsErrorDistributionChart } from "@/components/admin/ops-error-distribution-chart";
import { OpsAlertRunbookSteps } from "@/components/admin/ops-alert-runbook-steps";
import {
  defaultOpsSettingsForm,
  buildOpsSettingsBody,
  type OpsSettingsFormState,
} from "@/lib/admin-ops-settings-form";
import type {
  OpsSloDefinition,
  OpsAlertRule,
  OpsAlertRuleBaselinePosture,
  OpsAlertRuleBaselinePostureItem,
  OpsAlertSilence,
  OpsNotificationChannel,
  OpsRealtimeTraffic,
} from "@/lib/sdk-types";
import { useLanguage } from "@/context/LanguageContext";
import { useToast } from "@/context/ToastContext";
import { adminErrorMessage } from "@/lib/admin-api";
import { Card, CardHeader, CardTitle, CardContent } from "@/components/ui/card";
import { Sparkline } from "@/components/charts/sparkline";
import { QuotaNotchRail } from "@/components/ui/quota-notch-rail";
import { QuietBadge } from "@/components/ui/quiet-badge";
import { IllustratedEmptyState } from "@/components/ui/illustrated-empty-state";
import { SloCardSkeleton } from "@/components/charts/chart-skeleton";
import { Button } from "@/components/ui/button";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { DataPill } from "@/components/ui/data-pill";
import { IconBubble } from "@/components/ui/icon-bubble";
import { quietStatusFor } from "@/lib/status-badge";
import { formatDateTime, formatInteger, formatPercent } from "@/lib/admin-format";
import { MonitorContent } from "@/components/admin/ops-channel-monitor";
import { ScheduledTestsContent } from "@/components/admin/ops-scheduled-tests";
import { StrategyContent } from "@/components/admin/ops-strategy";
import { SchedulerDecisionsPanel } from "@/components/features/scheduler-decisions-panel";
import { adminErrorInvestigationHref } from "@/lib/admin-log-links";
import {
  buildOpsAlertEvidenceLinks,
  buildOpsAlertRunbookSteps,
} from "@/lib/admin-ops-alert-evidence";
import { OpsEvidenceChainHealth } from "./evidence-chain-health";

export default function AdminOpsPage() {
  return (
    <AdminShell>
      <OpsWrapper />
    </AdminShell>
  );
}

function OpsWrapper() {
  const searchParams = useSearchParams();
  const fullscreen = searchParams.get("fullscreen") === "1";

  if (fullscreen) {
    return (
      <div className="bg-srapi-bg fixed inset-0 z-50 overflow-y-auto p-6">
        <div className="mx-auto max-w-7xl space-y-6">
          <OpsContent />
        </div>
      </div>
    );
  }

  return <OpsContent />;
}

function OpsContent() {
  const searchParams = useSearchParams();
  const tab = searchParams.get("tab") ?? "overview";

  if (tab !== "overview") {
    return (
      <>
        <OpsTabs value={tab} />
        {tab === "channel-monitor" ? <MonitorContent /> : null}
        {tab === "scheduled-tests" ? <ScheduledTestsContent /> : null}
        {tab === "strategy" ? <StrategyContent /> : null}
        {tab === "scheduler-decisions" ? <SchedulerDecisionsPanel /> : null}
      </>
    );
  }

  return (
    <>
      <OpsTabs value="overview" />
      <OpsOverviewContent />
    </>
  );
}

function alertRuleScopeLabel(rule: OpsAlertRule, fallback: string): string {
  const parts = [
    rule.scope.source_endpoint,
    rule.scope.model,
    rule.scope.error_class,
    rule.scope.provider_id ? `provider:${rule.scope.provider_id}` : "",
  ].filter(Boolean);
  return parts.length > 0 ? parts.join(" · ") : fallback;
}

function alertSilenceMatcherLabel(silence: OpsAlertSilence, fallback: string): string {
  const matcher = silence.matcher;
  const parts = [
    matcher.rule_id,
    matcher.severity,
    matcher.source_endpoint,
    matcher.model,
    matcher.error_class,
    matcher.provider_id ? `provider:${matcher.provider_id}` : "",
  ].filter(Boolean);
  return parts.length > 0 ? parts.join(" · ") : fallback;
}

function OpsOverviewContent() {
  const { t } = useLanguage();
  // The shared message catalog has no keys for the new ops charts yet; fall back
  // to a readable English string so they never render as a raw dotted key.
  const tWithFallback = (key: string, fallback: string) => {
    const value = t(key);
    return value === key ? fallback : value;
  };
  const { toast } = useToast();
  const router = useRouter();
  const searchParams = useSearchParams();
  const fullscreen = searchParams.get("fullscreen") === "1";
  const slos = useOpsSlos();
  const alerts = useOpsAlerts();
  const systemLogHealth = useOpsSystemLogHealth();
  const ackMut = useAcknowledgeAlert();
  const settingsMut = useUpdateOpsSettings();
  const [showCleanup, setShowCleanup] = useState(false);
  const [showSettings, setShowSettings] = useState(false);

  function toggleFullscreen() {
    const params = new URLSearchParams(searchParams.toString());
    if (fullscreen) {
      params.delete("fullscreen");
    } else {
      params.set("fullscreen", "1");
    }
    const qs = params.toString();
    router.replace(`/admin/ops${qs ? `?${qs}` : ""}`, { scroll: false });
  }

  const settingsFields: FieldConfig<OpsSettingsFormState>[] = [
    { name: "autoRefreshEnabled", label: t("adminOpsSettings.autoRefresh"), type: "switch" },
    {
      name: "refreshIntervalSeconds",
      label: t("adminOpsSettings.refreshInterval"),
      type: "number",
    },
  ];

  async function ackAlert(id: string) {
    try {
      await ackMut.mutateAsync(id);
      toast({ title: t("feedback.acknowledged"), tone: "success" });
    } catch (err) {
      toast({ title: t("feedback.failed"), description: adminErrorMessage(err), tone: "error" });
    }
  }

  const throughput = useOpsThroughput();
  const realtimeTraffic = useOpsRealtimeTraffic({ window: "5m" });
  const errorTrend = useOpsErrorTrend();
  const latencyHistogram = useOpsLatencyHistogram();
  const errorDistribution = useOpsErrorDistribution();
  const alertRules = useOpsAlertRules();
  const alertSilences = useOpsAlertSilences();
  const notificationChannels = useOpsNotificationChannels();
  const notificationDeliveries = useOpsNotificationDeliveries({ page_size: 6 });
  const deleteRuleMut = useDeleteOpsAlertRule();
  const deleteSilenceMut = useDeleteOpsAlertSilence();
  const deleteNotificationChannelMut = useDeleteOpsNotificationChannel();
  const deleteSloMut = useDeleteOpsSlo();
  const [sloToDelete, setSloToDelete] = useState<OpsSloDefinition | null>(null);
  const [sloTarget, setSloTarget] = useState<OpsSloDefinition | "new" | null>(null);
  const [ruleTarget, setRuleTarget] = useState<OpsAlertRule | "new" | null>(null);
  const [notificationChannelTarget, setNotificationChannelTarget] = useState<
    OpsNotificationChannel | "new" | null
  >(null);
  const [showSilenceForm, setShowSilenceForm] = useState(false);
  const [ruleToDelete, setRuleToDelete] = useState<OpsAlertRule | null>(null);
  const [silenceToDelete, setSilenceToDelete] = useState<OpsAlertSilence | null>(null);
  const [notificationChannelToDelete, setNotificationChannelToDelete] =
    useState<OpsNotificationChannel | null>(null);
  // Only firing alerts are "active". A resolved/suppressed alert also has no
  // acknowledged_at, so the old `!acknowledged_at` check wrongly kept them here.
  const activeAlerts = (alerts.data?.data ?? []).filter((a) => a.status === "firing");

  const throughputValues = (throughput.data?.points ?? []).map((p) => p.request_count);
  const errorValues = (errorTrend.data?.points ?? []).map((p) => p.error_rate * 100);

  return (
    <>
      <SectionHero
        eyebrow="Ops · Overview"
        title={t("adminOps.title")}
        description={t("adminOps.subtitle")}
        metrics={[
          {
            label: "在线 channel",
            value: formatInteger(
              (notificationChannels.data?.data ?? []).filter((c) => c.status === "active").length,
            ),
          },
          { label: "活跃告警", value: formatInteger(activeAlerts.length), tone: activeAlerts.length > 0 ? "warning" : "default" },
        ]}
        actions={
          <div className="flex items-center gap-2">
            <Button variant="outline" size="sm" onClick={toggleFullscreen}>
              {fullscreen ? <Minimize2 className="size-3.5" /> : <Maximize2 className="size-3.5" />}
            </Button>
            <Button variant="outline" size="sm" onClick={() => setShowCleanup(true)}>
              {t("adminOpsCleanup.action")}
            </Button>
            <Button variant="outline" size="sm" onClick={() => setShowSettings(true)}>
              {t("adminOpsSettings.action")}
            </Button>
            <Button variant="primary" size="sm" onClick={() => setSloTarget("new")}>
              ＋ {t("adminOps.createSlo")}
            </Button>
          </div>
        }
      />

      <OpsEvidenceChainHealth health={systemLogHealth.data} loading={systemLogHealth.isLoading} />

      <RealtimeTrafficPanel traffic={realtimeTraffic.data} loading={realtimeTraffic.isLoading} />

      {throughputValues.length > 0 || errorValues.length > 0 ? (
        <div className="grid gap-4 md:grid-cols-2">
          <Card>
            <CardContent className="space-y-3">
              <span className="text-xs font-semibold uppercase tracking-[0.12em] text-srapi-text-tertiary">
                {t("adminOps.throughput")}
              </span>
              <Sparkline values={throughputValues} ariaLabel={t("adminOps.throughput")} />
            </CardContent>
          </Card>
          <Card>
            <CardContent className="space-y-3">
              <span className="text-xs font-semibold uppercase tracking-[0.12em] text-srapi-text-tertiary">
                {t("adminOps.errorRate")}
              </span>
              <Sparkline values={errorValues} ariaLabel={t("adminOps.errorRate")} />
            </CardContent>
          </Card>
        </div>
      ) : null}

      <div className="grid gap-4 lg:grid-cols-2">
        <OpsLatencyHistogramChart
          buckets={latencyHistogram.data?.buckets ?? []}
          loading={latencyHistogram.isLoading}
          title={tWithFallback("adminOps.latencyHistogram", "Latency histogram")}
          emptyLabel={tWithFallback("adminOps.latencyHistogramEmpty", "No latency samples yet")}
          requestsLabel={t("adminOps.throughput")}
        />
        <OpsErrorDistributionChart
          items={errorDistribution.data?.items ?? []}
          loading={errorDistribution.isLoading}
          title={tWithFallback("adminOps.errorDistribution", "Error distribution")}
          emptyLabel={tWithFallback("adminOps.errorDistributionEmpty", "No errors in window")}
          totalLabel={tWithFallback("adminOps.errorsTotal", "errors")}
          ownerLabels={{
            provider: tWithFallback("adminOps.owner.provider", "Provider"),
            client: tWithFallback("adminOps.owner.client", "Client"),
            platform: tWithFallback("adminOps.owner.platform", "Platform"),
            other: tWithFallback("adminOps.owner.other", "Other"),
          }}
          investigationHref={(item) =>
            adminErrorInvestigationHref({ error_class: item.error_class })
          }
        />
      </div>

      <PageQueryState
        query={slos}
        isEmpty={(d) => d.data.length === 0}
        skeleton={
          <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-3">
            {Array.from({ length: 3 }).map((_, i) => (
              <SloCardSkeleton key={i} />
            ))}
          </div>
        }
      >
        {(result) =>
          result.data.length === 0 ? (
            <Card>
              <IllustratedEmptyState
                illust="chart"
                title={t("adminOps.emptySlo")}
                description={t("adminOps.emptySloBody")}
              />
            </Card>
          ) : (
            <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-3">
              {result.data.map((slo, i) => {
                const def = slo.definition;
                const objective = def?.objective ?? slo.evaluation?.objective ?? 0;
                const errorRate = slo.evaluation?.error_rate ?? 0;
                const availability = Math.max(0, (1 - errorRate) * 100);
                const objectivePct = objective > 1 ? objective : objective * 100;
                // Derive reliability health from the evaluation, not the SLO's
                // enable-state (`def.status` is "active"/"disabled", not health).
                const enabled = def?.status !== "disabled";
                const health = !enabled
                  ? "disabled"
                  : availability >= objectivePct
                    ? "healthy"
                    : "breached";
                const healthLabel = enabled ? t(`adminOps.${health}`) : t("common.disabled");
                return (
                  <Card key={def?.id ?? i}>
                    <CardHeader>
                      <CardTitle>
                        {def?.name ?? t("adminOps.slo")}
                      </CardTitle>
                      <div className="flex items-center gap-2">
                        <QuietBadge
                          status={!enabled ? "disabled" : health === "healthy" ? "active" : "error"}
                          label={healthLabel}
                        />
                        {def ? (
                          <>
                            <button
                              type="button"
                              onClick={() => setSloTarget(def)}
                              aria-label={t("adminOps.editSlo")}
                              className="text-srapi-text-tertiary hover:text-srapi-text-primary transition-colors"
                            >
                              <Pencil className="size-3.5" />
                            </button>
                            <button
                              type="button"
                              onClick={() => setSloToDelete(def)}
                              aria-label={t("adminOps.deleteSlo")}
                              className="text-srapi-text-tertiary hover:text-srapi-error transition-colors"
                            >
                              <Trash2 className="size-3.5" />
                            </button>
                          </>
                        ) : null}
                      </div>
                    </CardHeader>
                    <CardContent className="space-y-4">
                      <div className="flex items-baseline justify-between">
                        <span className="text-xs text-srapi-text-secondary">
                          {t("adminOps.availability")}
                        </span>
                        <span className="text-2xl font-semibold tracking-tight tabular text-srapi-text-primary">
                          {availability.toFixed(2)}%
                        </span>
                      </div>
                      <QuotaNotchRail value={availability} />
                      <div className="flex items-center justify-between text-[11px] text-srapi-text-tertiary">
                        <span>
                          {t("adminOps.objective")} {objectivePct.toFixed(1)}%
                        </span>
                        <span>
                          {t("adminOps.window")} {def?.window_days ?? 30}d
                        </span>
                      </div>
                    </CardContent>
                  </Card>
                );
              })}
            </div>
          )
        }
      </PageQueryState>

      {activeAlerts.length > 0 ? (
        <Card>
          <CardHeader>
            <CardTitle>{t("adminOps.alerts")}</CardTitle>
            <DataPill tone="error">{activeAlerts.length}</DataPill>
          </CardHeader>
          <CardContent>
            <div className="grid gap-3 md:grid-cols-2">
              {activeAlerts.map((alert) => {
                const evidenceLinks = buildOpsAlertEvidenceLinks(alert.details);
                const runbookSteps = buildOpsAlertRunbookSteps(alert.details);
                const sev = alert.severity;
                const tone =
                  sev === "critical" ? "error" : sev === "warning" ? "warning" : "neutral";
                return (
                  <div
                    key={alert.id}
                    className="flex flex-col gap-3 rounded-2xl border border-srapi-border bg-srapi-card-muted/40 p-4"
                  >
                    <div className="flex items-start gap-3">
                      <IconBubble tone={tone} size="md">
                        <BellRing aria-hidden />
                      </IconBubble>
                      <div className="min-w-0 flex-1">
                        <div className="truncate text-sm font-semibold tracking-tight text-srapi-text-primary">
                          {alert.summary}
                        </div>
                        <div className="text-[12px] tabular text-srapi-text-tertiary">
                          {formatDateTime(alert.started_at ?? alert.created_at)}
                        </div>
                      </div>
                      <QuietBadge status={quietStatusFor(alert.severity)} label={alert.severity} />
                    </div>
                    <OpsAlertRunbookSteps steps={runbookSteps} compact />
                    <div className="flex flex-wrap items-center justify-between gap-2">
                      <AlertEvidenceActions links={evidenceLinks} />
                      <Button
                        variant="outline"
                        size="sm"
                        loading={ackMut.isPending && ackMut.variables === alert.id}
                        onClick={() => ackAlert(alert.id)}
                      >
                        {t("adminOps.acknowledge")}
                      </Button>
                    </div>
                  </div>
                );
              })}
            </div>
          </CardContent>
        </Card>
      ) : null}

      <Card>
        <CardHeader className="flex-row items-center justify-between">
          <div>
            <CardTitle>{t("adminOps.alertRules.title")}</CardTitle>
            <p className="mt-1 text-xs text-srapi-text-secondary">
              {t("adminOps.alertRules.subtitle")}
            </p>
          </div>
          <Button variant="outline" size="sm" onClick={() => setRuleTarget("new")}>
            ＋ {t("adminOps.alertRules.create")}
          </Button>
        </CardHeader>
        <CardContent className="space-y-3">
          <AlertRuleBaselinePostureSummary posture={alertRules.data?.baseline_posture} />
          {(alertRules.data?.data ?? []).length === 0 ? (
            <IllustratedEmptyState
              illust="bell"
              title={t("adminOps.alertRules.empty")}
              description={t("adminOps.alertRules.emptyBody")}
            />
          ) : (
            <div className="divide-y divide-srapi-border/70">
              {(alertRules.data?.data ?? []).map((rule) => (
                <div
                  key={rule.id}
                  className="flex items-center justify-between gap-4 py-3"
                >
                  <div className="min-w-0">
                    <div className="flex items-center gap-2">
                      <span className="truncate text-sm font-medium text-srapi-text-primary">{rule.name}</span>
                      <QuietBadge status={quietStatusFor(rule.severity)} label={rule.severity} />
                      {rule.builtin_baseline ? (
                        <QuietBadge status="active" label={t("adminOps.alertRules.builtinBaseline")} />
                      ) : null}
                    </div>
                    <div className="mt-1 text-[12px] tabular text-srapi-text-tertiary">
                      {t(`adminOps.alertRules.metricType.${rule.metric_type}`)}{" "}
                      {t(`adminOps.alertRules.operators.${rule.operator}`)} {rule.threshold}
                      {" · "}
                      {alertRuleScopeLabel(rule, t("adminOps.alertRules.globalScope"))}
                      {rule.builtin_baseline && rule.baseline_key ? (
                        <>
                          {" · "}
                          {rule.baseline_key}
                        </>
                      ) : null}
                    </div>
                  </div>
                  <div className="flex shrink-0 items-center gap-3">
                    <QuietBadge
                      status={rule.enabled ? "active" : "disabled"}
                      label={rule.enabled ? t("adminOps.alertRules.enabled") : t("common.disabled")}
                    />
                    <button
                      type="button"
                      onClick={() => setRuleTarget(rule)}
                      aria-label={t("adminOps.alertRules.edit")}
                      className="text-srapi-text-tertiary hover:text-srapi-text-primary transition-colors"
                    >
                      <Pencil className="size-3.5" />
                    </button>
                    <button
                      type="button"
                      onClick={() => setRuleToDelete(rule)}
                      aria-label={t("common.delete")}
                      className="text-srapi-text-tertiary hover:text-srapi-error transition-colors"
                    >
                      <Trash2 className="size-3.5" />
                    </button>
                  </div>
                </div>
              ))}
            </div>
          )}
        </CardContent>
      </Card>

      <div className="grid gap-4 lg:grid-cols-[minmax(0,1fr)_minmax(22rem,0.75fr)]">
        <Card>
          <CardHeader className="flex-row items-center justify-between">
            <div>
              <CardTitle>{t("adminOps.notificationChannels.title")}</CardTitle>
              <p className="mt-1 text-xs text-srapi-text-secondary">
                {t("adminOps.notificationChannels.subtitle")}
              </p>
            </div>
            <Button variant="outline" size="sm" onClick={() => setNotificationChannelTarget("new")}>
              ＋ {t("adminOps.notificationChannels.create")}
            </Button>
          </CardHeader>
          <CardContent className="space-y-3">
            {(notificationChannels.data?.data ?? []).length === 0 ? (
              <IllustratedEmptyState
                illust="inbox"
                title={t("adminOps.notificationChannels.empty")}
                description={t("adminOps.notificationChannels.emptyBody")}
              />
            ) : (
              <div className="divide-y divide-srapi-border/70">
                {(notificationChannels.data?.data ?? []).map((channel) => (
                  <div
                    key={channel.id}
                    className="flex items-center justify-between gap-4 py-3"
                  >
                    <div className="min-w-0">
                      <div className="flex items-center gap-2">
                        <span className="truncate text-sm font-medium text-srapi-text-primary">
                          {channel.name}
                        </span>
                        <QuietBadge
                          status={channel.status === "active" ? "active" : "disabled"}
                          label={
                            channel.status === "active"
                              ? t("adminOps.notificationChannels.active")
                              : t("common.disabled")
                          }
                        />
                        <QuietBadge
                          status={quietStatusFor(channel.min_severity)}
                          label={channel.min_severity}
                        />
                      </div>
                      <div className="mt-1 truncate text-[12px] text-srapi-text-tertiary">
                        {channel.email_recipients.join(", ")}
                      </div>
                    </div>
                    <div className="flex shrink-0 items-center gap-3">
                      <span className="text-[11px] text-srapi-text-tertiary">
                        {channel.send_resolved
                          ? t("adminOps.notificationChannels.sendsResolved")
                          : t("adminOps.notificationChannels.firingOnly")}
                      </span>
                      <button
                        type="button"
                        onClick={() => setNotificationChannelTarget(channel)}
                        aria-label={t("adminOps.notificationChannels.edit")}
                        className="text-srapi-text-tertiary hover:text-srapi-text-primary transition-colors"
                      >
                        <Pencil className="size-3.5" />
                      </button>
                      <button
                        type="button"
                        onClick={() => setNotificationChannelToDelete(channel)}
                        aria-label={t("common.delete")}
                        className="text-srapi-text-tertiary hover:text-srapi-error transition-colors"
                      >
                        <Trash2 className="size-3.5" />
                      </button>
                    </div>
                  </div>
                ))}
              </div>
            )}
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>{t("adminOps.notificationDeliveries.title")}</CardTitle>
            <p className="mt-1 text-xs text-srapi-text-secondary">
              {t("adminOps.notificationDeliveries.subtitle")}
            </p>
          </CardHeader>
          <CardContent className="space-y-3">
            {(notificationDeliveries.data?.data ?? []).length === 0 ? (
              <IllustratedEmptyState
                illust="bell"
                title={t("adminOps.notificationDeliveries.empty")}
                description={t("adminOps.notificationDeliveries.emptyBody")}
              />
            ) : (
              <div className="divide-y divide-srapi-border/70">
                {(notificationDeliveries.data?.data ?? []).map((delivery) => (
                  <div
                    key={delivery.id}
                    className="py-3"
                  >
                    <div className="flex items-center justify-between gap-3">
                      <span className="min-w-0 truncate text-sm text-srapi-text-primary">
                        {delivery.alert_summary ?? delivery.target}
                      </span>
                      <QuietBadge
                        status={
                          delivery.status === "delivered"
                            ? "active"
                            : delivery.status === "failed"
                              ? "error"
                              : "limited"
                        }
                        label={t(`adminOps.notificationDeliveries.status.${delivery.status}`)}
                      />
                    </div>
                    <div className="mt-1 flex flex-wrap items-center gap-x-3 gap-y-1 text-[11px] text-srapi-text-tertiary">
                      <span>{delivery.target}</span>
                      <span>{delivery.channel_name ?? delivery.channel_id}</span>
                      <span>
                        {t("adminOps.notificationDeliveries.attempts")} {delivery.attempt_count}
                      </span>
                      <span className="tabular">{formatDateTime(delivery.updated_at)}</span>
                    </div>
                    {delivery.last_error ? (
                      <div className="mt-1 truncate text-[11px] text-srapi-error">
                        {delivery.last_error}
                      </div>
                    ) : null}
                  </div>
                ))}
              </div>
            )}
          </CardContent>
        </Card>
      </div>

      <Card>
        <CardHeader className="flex-row items-center justify-between">
          <div>
            <CardTitle>{t("adminOps.silences.title")}</CardTitle>
            <p className="mt-1 text-xs text-srapi-text-secondary">
              {t("adminOps.silences.subtitle")}
            </p>
          </div>
          <Button variant="outline" size="sm" onClick={() => setShowSilenceForm(true)}>
            ＋ {t("adminOps.silences.create")}
          </Button>
        </CardHeader>
        <CardContent className="space-y-3">
          {(alertSilences.data?.data ?? []).length === 0 ? (
            <IllustratedEmptyState
              illust="bell"
              title={t("adminOps.silences.empty")}
              description={t("adminOps.silences.emptyBody")}
            />
          ) : (
            <div className="divide-y divide-srapi-border/70">
              {(alertSilences.data?.data ?? []).map((silence) => {
                const matcherText = alertSilenceMatcherLabel(
                  silence,
                  t("adminOps.silences.anyMatcher"),
                );
                return (
                  <div
                    key={silence.id}
                    className="flex items-center justify-between gap-4 py-3"
                  >
                    <div className="min-w-0">
                      <div className="truncate text-sm text-srapi-text-primary">
                        {silence.comment || matcherText}
                      </div>
                      <div className="mt-1 text-[12px] tabular text-srapi-text-tertiary">
                        {silence.comment ? `${matcherText} · ` : ""}
                        {formatDateTime(silence.starts_at)} → {formatDateTime(silence.ends_at)}
                      </div>
                    </div>
                    <button
                      type="button"
                      onClick={() => setSilenceToDelete(silence)}
                      aria-label={t("common.delete")}
                      className="shrink-0 text-srapi-text-tertiary transition-colors hover:text-srapi-error"
                    >
                      <Trash2 className="size-3.5" />
                    </button>
                  </div>
                );
              })}
            </div>
          )}
        </CardContent>
      </Card>

      {sloTarget !== null ? (
        <SloFormDialog
          key={sloTarget === "new" ? "new" : sloTarget.id}
          open
          target={sloTarget === "new" ? null : sloTarget}
          onOpenChange={(open) => {
            if (!open) setSloTarget(null);
          }}
        />
      ) : null}

      {ruleTarget !== null ? (
        <AlertRuleFormDialog
          key={ruleTarget === "new" ? "new" : ruleTarget.id}
          open
          target={ruleTarget === "new" ? null : ruleTarget}
          onOpenChange={(open) => {
            if (!open) setRuleTarget(null);
          }}
        />
      ) : null}

      {showSilenceForm ? <AlertSilenceFormDialog open onOpenChange={setShowSilenceForm} /> : null}

      {notificationChannelTarget !== null ? (
        <OpsNotificationChannelFormDialog
          key={notificationChannelTarget === "new" ? "new" : notificationChannelTarget.id}
          open
          target={notificationChannelTarget === "new" ? null : notificationChannelTarget}
          onOpenChange={(open) => {
            if (!open) setNotificationChannelTarget(null);
          }}
        />
      ) : null}

      {ruleToDelete ? (
        <ConfirmDialog
          open
          onOpenChange={(open) => {
            if (!open) setRuleToDelete(null);
          }}
          title={t("adminOps.alertRules.deleteConfirm")}
          body={ruleToDelete.name}
          confirmLabel={t("common.delete")}
          successMessage={t("feedback.deleted")}
          isPending={deleteRuleMut.isPending}
          onConfirm={() => deleteRuleMut.mutateAsync(ruleToDelete.id)}
        />
      ) : null}

      {sloToDelete ? (
        <ConfirmDialog
          open
          onOpenChange={(open) => {
            if (!open) setSloToDelete(null);
          }}
          title={t("adminOps.deleteSloTitle")}
          body={sloToDelete.name}
          confirmLabel={t("common.delete")}
          successMessage={t("feedback.deleted")}
          isPending={deleteSloMut.isPending}
          onConfirm={() => deleteSloMut.mutateAsync(sloToDelete.id)}
        />
      ) : null}

      {silenceToDelete ? (
        <ConfirmDialog
          open
          onOpenChange={(open) => {
            if (!open) setSilenceToDelete(null);
          }}
          title={t("adminOps.silences.deleteConfirm")}
          confirmLabel={t("common.delete")}
          successMessage={t("feedback.deleted")}
          isPending={deleteSilenceMut.isPending}
          onConfirm={() => deleteSilenceMut.mutateAsync(silenceToDelete.id)}
        />
      ) : null}

      {notificationChannelToDelete ? (
        <ConfirmDialog
          open
          onOpenChange={(open) => {
            if (!open) setNotificationChannelToDelete(null);
          }}
          title={t("adminOps.notificationChannels.deleteConfirm")}
          body={notificationChannelToDelete.name}
          confirmLabel={t("common.delete")}
          successMessage={t("feedback.deleted")}
          isPending={deleteNotificationChannelMut.isPending}
          onConfirm={() => deleteNotificationChannelMut.mutateAsync(notificationChannelToDelete.id)}
        />
      ) : null}

      <OpsLogCleanupDialog open={showCleanup} onOpenChange={setShowCleanup} />

      {showSettings ? (
        <ResourceFormDialog
          open
          onOpenChange={setShowSettings}
          title={t("adminOpsSettings.title")}
          description={t("adminOpsSettings.note")}
          fields={settingsFields}
          initial={defaultOpsSettingsForm()}
          buildBody={buildOpsSettingsBody}
          submit={(body) => settingsMut.mutateAsync(body)}
          successMessage={t("feedback.saved")}
          isPending={settingsMut.isPending}
        />
      ) : null}
    </>
  );
}

function RealtimeTrafficPanel({
  traffic,
  loading,
}: {
  traffic?: OpsRealtimeTraffic;
  loading?: boolean;
}) {
  const { t } = useLanguage();
  const request = traffic?.requests_per_min;
  const token = traffic?.tokens_per_min;
  return (
    <Card>
      <CardContent>
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div>
            <span className="text-xs font-semibold uppercase tracking-[0.12em] text-srapi-text-tertiary">
              {t("adminOps.realtimeTraffic")}
            </span>
            <div className="mt-1 text-[12px] text-srapi-text-secondary">
              {traffic
                ? t("adminOps.realtimeTrafficWindow", {
                    start: formatDateTime(traffic.window.start),
                    end: formatDateTime(traffic.window.end),
                  })
                : loading
                  ? t("common.loading")
                  : t("adminOps.realtimeTrafficEmpty")}
            </div>
          </div>
          {traffic ? (
            <QuietBadge
              status={traffic.error_rate > 0 ? "limited" : "active"}
              label={t("adminOps.realtimeTrafficErrors", {
                count: formatInteger(traffic.error_count),
                rate: formatPercent(traffic.error_rate),
              })}
            />
          ) : null}
        </div>
        <div className="mt-4 grid gap-3 md:grid-cols-2 xl:grid-cols-4">
          <RealtimeMetric
            label="RPM"
            current={request?.current}
            peak={request?.peak}
            average={request?.average}
          />
          <RealtimeMetric
            label="TPM"
            current={token?.current}
            peak={token?.peak}
            average={token?.average}
          />
          <CompactMetric
            label={t("adminOps.realtimeTrafficRequests")}
            value={formatInteger(traffic?.total_requests)}
            hint={t("adminOps.realtimeTrafficEvidence", {
              usage: formatInteger(traffic?.usage_log_count),
              errors: formatInteger(traffic?.ops_error_log_count),
            })}
          />
          <CompactMetric
            label={t("adminOps.errorRate")}
            value={traffic ? formatPercent(traffic.error_rate) : "-"}
            hint={t("adminOps.realtimeTrafficCurrentHint")}
          />
        </div>
      </CardContent>
    </Card>
  );
}

function RealtimeMetric({
  label,
  current,
  peak,
  average,
}: {
  label: string;
  current?: number;
  peak?: number;
  average?: number;
}) {
  const { t } = useLanguage();
  return (
    <CompactMetric
      label={label}
      value={formatInteger(current)}
      hint={t("adminOps.realtimeTrafficRateHint", {
        peak: formatInteger(peak),
        average: formatInteger(average),
      })}
    />
  );
}

function CompactMetric({ label, value, hint }: { label: string; value: string; hint: string }) {
  return (
    <div className="rounded-xl border border-srapi-border/80 bg-srapi-card-muted/50 px-3 py-2.5">
      <div className="text-[10px] font-semibold uppercase tracking-[0.12em] text-srapi-text-tertiary">{label}</div>
      <div className="mt-1 text-lg font-semibold tracking-tight tabular text-srapi-text-primary">{value}</div>
      <div className="mt-1 truncate text-[11px] text-srapi-text-tertiary">{hint}</div>
    </div>
  );
}

function AlertRuleBaselinePostureSummary({
  posture,
}: {
  posture: OpsAlertRuleBaselinePosture | undefined;
}) {
  const { t } = useLanguage();
  if (!posture) return null;
  const attention = posture.items.filter((item) => item.status !== "covered");
  const status =
    posture.missing_count > 0 || posture.disabled_count > 0
      ? "error"
      : posture.modified_count > 0
        ? "limited"
        : "active";

  return (
    <div className="space-y-3 rounded-xl border border-srapi-border bg-srapi-card-muted/50 p-3">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div className="min-w-0">
          <div className="text-sm font-medium text-srapi-text-primary">
            {t("adminOps.alertRules.baselinePosture")}
          </div>
          <div className="text-[11px] text-srapi-text-tertiary">
            {t("adminOps.alertRules.baselinePostureSummary", {
              enabled: posture.enabled_count,
              total: posture.total_count,
              disabled: posture.disabled_count,
              modified: posture.modified_count,
              missing: posture.missing_count,
            })}
          </div>
        </div>
        <QuietBadge
          status={status}
          label={
            status === "active"
              ? t("adminOps.alertRules.baselineHealthy")
              : t("adminOps.alertRules.baselineNeedsAttention")
          }
        />
      </div>
      {attention.length > 0 ? (
        <div className="grid gap-2 md:grid-cols-2 xl:grid-cols-3">
          {attention.slice(0, 6).map((item) => (
            <div key={item.baseline_key} className="min-w-0 rounded-lg border border-srapi-border/80 px-2.5 py-2">
              <div className="flex items-center gap-2">
                <span className="truncate text-[11px] font-medium text-srapi-text-primary">
                  {item.baseline_key}
                </span>
                <QuietBadge status={baselinePostureTone(item)} label={baselinePostureLabel(t, item)} />
              </div>
              {item.differences.length > 0 ? (
                <div className="mt-1 truncate text-[11px] text-srapi-text-tertiary">
                  {item.differences.join(", ")}
                </div>
              ) : null}
            </div>
          ))}
        </div>
      ) : null}
    </div>
  );
}

function baselinePostureTone(item: OpsAlertRuleBaselinePostureItem) {
  if (item.status === "covered") return "active";
  if (item.status === "modified") return "limited";
  return "error";
}

function baselinePostureLabel(
  t: (key: string, vars?: Record<string, string | number>) => string,
  item: OpsAlertRuleBaselinePostureItem,
) {
  return t(`adminOps.alertRules.baselineStatus.${item.status}`);
}

function AlertEvidenceActions({ links }: { links: ReturnType<typeof buildOpsAlertEvidenceLinks> }) {
  const { t } = useLanguage();
  const actions = [
    { href: links.errorLogs, label: t("adminOps.evidence.errorLogs") },
    { href: links.requestEvidence, label: t("adminOps.evidence.requestEvidence") },
    { href: links.schedulerDecision, label: t("adminOps.evidence.schedulerDecision") },
    { href: links.accountHealth, label: t("adminOps.evidence.accountHealth") },
  ].filter((item): item is { href: string; label: string } => Boolean(item.href));
  if (actions.length === 0) return null;
  return (
    <div className="flex flex-wrap justify-end gap-1">
      {actions.map((action) => (
        <Button key={`${action.label}:${action.href}`} asChild variant="ghost" size="sm">
          <Link href={action.href}>{action.label}</Link>
        </Button>
      ))}
    </div>
  );
}

function OpsTabs({ value }: { value: string }) {
  const { t } = useLanguage();
  const router = useRouter();

  function setTab(next: string) {
    const params = new URLSearchParams();
    if (next !== "overview") {
      params.set("tab", next);
    }
    const qs = params.toString();
    router.replace(`/admin/ops${qs ? `?${qs}` : ""}`, { scroll: false });
  }

  return (
    <Tabs value={value} onValueChange={setTab}>
      <TabsList className="flex flex-wrap">
        <TabsTrigger value="overview">{t("adminOps.tabs.overview")}</TabsTrigger>
        <TabsTrigger value="channel-monitor">{t("adminOps.tabs.channelMonitor")}</TabsTrigger>
        <TabsTrigger value="scheduled-tests">{t("adminOps.tabs.scheduledTests")}</TabsTrigger>
        <TabsTrigger value="strategy">{t("adminOps.tabs.strategy")}</TabsTrigger>
        <TabsTrigger value="scheduler-decisions">
          {t("adminOps.tabs.schedulerDecisions")}
        </TabsTrigger>
      </TabsList>
    </Tabs>
  );
}

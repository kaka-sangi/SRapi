"use client";

import {
  acknowledgeAdminOpsAlert,
  cleanupAdminOpsSystemLogs,
  createAdminOpsSlo,
  deleteAdminOpsSlo,
  listAdminOpsAlertRules,
  createAdminOpsAlertRule,
  updateAdminOpsAlertRule,
  deleteAdminOpsAlertRule,
  listAdminOpsAlertSilences,
  createAdminOpsAlertSilence,
  deleteAdminOpsAlertSilence,
  getAdminDashboardSnapshot,
  getAdminOpsConcurrency,
  getAdminOpsErrorDistribution,
  getAdminOpsErrorTrend,
  getAdminOpsLatencyHistogram,
  getAdminOpsOverview,
  listAdminOpsRequestEvidence,
  getAdminOpsSystemLogHealth,
  getAdminOpsThroughputTrend,
  listAdminOpsAlertEvents,
  listAdminOpsAlerts,
  listAdminOpsRealtimeSlots,
  listAdminOpsSlos,
  listAdminOpsSystemLogs,
  updateAdminOpsSettings,
  updateAdminOpsSlo,
} from "../../../../../packages/sdk/typescript/src/index";
import type {
  AdminDashboardSnapshot,
  ListAdminOpsAlertEventsData,
  ListAdminOpsAlertsData,
  ListAdminOpsRequestEvidenceData,
  ListAdminOpsSystemLogsData,
  OpsAlertEvent,
  OpsAlertRule,
  OpsAlertSilence,
  CreateAdminOpsAlertRuleData,
  UpdateAdminOpsAlertRuleData,
  CreateAdminOpsAlertSilenceData,
  OpsConcurrency,
  OpsErrorDistribution,
  OpsErrorTrend,
  OpsLatencyHistogram,
  RequestEvidenceRow,
  OpsOverview,
  OpsSlo,
  OpsSettings,
  OpsSloDefinition,
  UpdateAdminOpsSloData,
  OpsSystemLog,
  OpsSystemLogHealth,
  OpsSystemLogCleanupRequest,
  OpsSystemLogCleanupResult,
  OpsThroughputTrend,
  RealtimeActiveSlot,
  Id,
} from "../../../../../packages/sdk/typescript/src/types.gen";
import { configureAdminClient, unwrapData, unwrapList } from "./_shared";
import type { AdminListResult, AdminTimeRange } from "./types";

export const opsApi = {
  getDashboardSnapshot(query?: AdminTimeRange): Promise<AdminDashboardSnapshot> {
    return unwrapData(() => getAdminDashboardSnapshot({ query, throwOnError: true }));
  },

  getOpsOverview(query?: AdminTimeRange): Promise<OpsOverview> {
    return unwrapData(() => getAdminOpsOverview({ query, throwOnError: true }));
  },

  getOpsThroughputTrend(
    query?: AdminTimeRange & { bucket?: "hour" | "day" },
  ): Promise<OpsThroughputTrend> {
    return unwrapData(() => getAdminOpsThroughputTrend({ query, throwOnError: true }));
  },

  getOpsErrorTrend(query?: AdminTimeRange & { bucket?: "hour" | "day" }): Promise<OpsErrorTrend> {
    return unwrapData(() => getAdminOpsErrorTrend({ query, throwOnError: true }));
  },

  getOpsErrorDistribution(query?: AdminTimeRange): Promise<OpsErrorDistribution> {
    return unwrapData(() => getAdminOpsErrorDistribution({ query, throwOnError: true }));
  },

  getOpsLatencyHistogram(query?: AdminTimeRange): Promise<OpsLatencyHistogram> {
    return unwrapData(() => getAdminOpsLatencyHistogram({ query, throwOnError: true }));
  },

  getOpsConcurrency(): Promise<OpsConcurrency> {
    return unwrapData(() => getAdminOpsConcurrency({ throwOnError: true }));
  },

  listOpsSystemLogs(
    query?: ListAdminOpsSystemLogsData["query"],
  ): Promise<AdminListResult<OpsSystemLog>> {
    return unwrapList(() => listAdminOpsSystemLogs({ query, throwOnError: true }));
  },

  listOpsRequestEvidence(
    query?: ListAdminOpsRequestEvidenceData["query"],
  ): Promise<AdminListResult<RequestEvidenceRow>> {
    return unwrapList(() => listAdminOpsRequestEvidence({ query, throwOnError: true }));
  },

  getOpsSystemLogHealth(): Promise<OpsSystemLogHealth> {
    return unwrapData(() => getAdminOpsSystemLogHealth({ throwOnError: true }));
  },

  // Bounded deletion of sanitized system logs (requires ≥1 filter; dry_run
  // previews without deleting). Returns matched/deleted counts.
  cleanupOpsSystemLogs(body: OpsSystemLogCleanupRequest): Promise<OpsSystemLogCleanupResult> {
    return unwrapData(() => cleanupAdminOpsSystemLogs({ body, throwOnError: true }));
  },
  // Replace the operational monitoring settings (auto-refresh, alert thresholds,
  // retention). The backend exposes no read endpoint, so this is write-only.
  updateOpsSettings(body: OpsSettings): Promise<OpsSettings> {
    return unwrapData(() => updateAdminOpsSettings({ body, throwOnError: true }));
  },

  listOpsAlerts(query?: ListAdminOpsAlertsData["query"]): Promise<AdminListResult<OpsAlertEvent>> {
    return unwrapList(() => listAdminOpsAlerts({ query, throwOnError: true }));
  },

  listOpsAlertEvents(
    query?: ListAdminOpsAlertEventsData["query"],
  ): Promise<AdminListResult<OpsAlertEvent>> {
    return unwrapList(() => listAdminOpsAlertEvents({ query, throwOnError: true }));
  },

  listOpsRealtimeSlots(): Promise<AdminListResult<RealtimeActiveSlot>> {
    return unwrapList(() => listAdminOpsRealtimeSlots({ throwOnError: true }));
  },

  listOpsSlos(): Promise<AdminListResult<OpsSlo>> {
    return unwrapList(() => listAdminOpsSlos({ throwOnError: true }));
  },

  acknowledgeAlert(id: Id): Promise<OpsAlertEvent> {
    return unwrapData(() => acknowledgeAdminOpsAlert({ path: { id }, throwOnError: true }));
  },

  createOpsSlo(body: Parameters<typeof createAdminOpsSlo>[0]["body"]): Promise<OpsSloDefinition> {
    return unwrapData(() => createAdminOpsSlo({ body, throwOnError: true }));
  },

  updateOpsSlo(id: Id, body: UpdateAdminOpsSloData["body"]): Promise<OpsSloDefinition> {
    return unwrapData(() => updateAdminOpsSlo({ path: { id }, body, throwOnError: true }));
  },

  deleteOpsSlo(id: Id): Promise<{ deleted: boolean }> {
    return unwrapData(() => deleteAdminOpsSlo({ path: { id }, throwOnError: true }));
  },

  listOpsAlertRules(): Promise<AdminListResult<OpsAlertRule>> {
    return unwrapList(() => listAdminOpsAlertRules({ throwOnError: true }));
  },

  createOpsAlertRule(body: CreateAdminOpsAlertRuleData["body"]): Promise<OpsAlertRule> {
    return unwrapData(() => createAdminOpsAlertRule({ body, throwOnError: true }));
  },

  updateOpsAlertRule(id: Id, body: UpdateAdminOpsAlertRuleData["body"]): Promise<OpsAlertRule> {
    return unwrapData(() => updateAdminOpsAlertRule({ path: { id }, body, throwOnError: true }));
  },

  async deleteOpsAlertRule(id: Id): Promise<void> {
    configureAdminClient();
    await deleteAdminOpsAlertRule({ path: { id }, throwOnError: true });
  },

  listOpsAlertSilences(): Promise<AdminListResult<OpsAlertSilence>> {
    return unwrapList(() => listAdminOpsAlertSilences({ throwOnError: true }));
  },

  createOpsAlertSilence(body: CreateAdminOpsAlertSilenceData["body"]): Promise<OpsAlertSilence> {
    return unwrapData(() => createAdminOpsAlertSilence({ body, throwOnError: true }));
  },

  async deleteOpsAlertSilence(id: Id): Promise<void> {
    configureAdminClient();
    await deleteAdminOpsAlertSilence({ path: { id }, throwOnError: true });
  },
};

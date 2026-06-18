"use client";

import { useQuery } from "@tanstack/react-query";
import { adminApi } from "@/lib/admin-api";
import { queryKeys } from "@/lib/query-keys";
import { type B, type P, useAdminMutation } from "./_shared";

// ---- Ops ----
export function useOpsSlos() {
  return useQuery({
    queryKey: queryKeys.admin.opsSlos(),
    queryFn: () => adminApi.listOpsSlos(),
    refetchInterval: 30_000,
  });
}
export function useCleanupOpsSystemLogs() {
  return useAdminMutation(
    (body: P<typeof adminApi.cleanupOpsSystemLogs>) => adminApi.cleanupOpsSystemLogs(body),
    ["admin", "ops", "system-logs"],
    queryKeys.admin.opsSystemLogHealth(),
  );
}
// Operator on-demand usage-record cleanup. Invalidates the usage-* queries so
// the admin usage view refreshes once records are actually deleted.
export function useCleanupAdminUsage() {
  return useAdminMutation(
    (body: P<typeof adminApi.cleanupUsage>) => adminApi.cleanupUsage(body),
    ["admin", "usage-logs"],
  );
}
export function useUpdateOpsSettings() {
  return useAdminMutation(
    (body: P<typeof adminApi.updateOpsSettings>) => adminApi.updateOpsSettings(body),
    ["admin", "ops"],
  );
}

export function useOpsSystemLogs(params?: P<typeof adminApi.listOpsSystemLogs>) {
  return useQuery({
    queryKey: queryKeys.admin.opsSystemLogs(params),
    queryFn: () => adminApi.listOpsSystemLogs(params),
  });
}

export function useOpsSystemLogHealth() {
  return useQuery({
    queryKey: queryKeys.admin.opsSystemLogHealth(),
    queryFn: () => adminApi.getOpsSystemLogHealth(),
    refetchInterval: 30_000,
  });
}

export function useOpsAlerts(params?: P<typeof adminApi.listOpsAlerts>) {
  return useQuery({
    queryKey: queryKeys.admin.opsAlerts(params),
    queryFn: () => adminApi.listOpsAlerts(params),
  });
}

export function useOpsThroughput(params?: P<typeof adminApi.getOpsThroughputTrend>) {
  return useQuery({
    queryKey: queryKeys.admin.opsThroughput(params),
    queryFn: () => adminApi.getOpsThroughputTrend(params),
    refetchInterval: 30_000,
  });
}

export function useOpsErrorTrend(params?: P<typeof adminApi.getOpsErrorTrend>) {
  return useQuery({
    queryKey: queryKeys.admin.opsErrorTrend(params),
    queryFn: () => adminApi.getOpsErrorTrend(params),
    refetchInterval: 30_000,
  });
}

export function useAdminUsageDaily(params?: P<typeof adminApi.listUsageDaily>) {
  return useQuery({
    queryKey: queryKeys.admin.usageDaily(params),
    queryFn: () => adminApi.listUsageDaily(params),
  });
}

export function useAdminUsageAggregates(
  dimension: P<typeof adminApi.listUsageAggregates>,
  range?: B<typeof adminApi.listUsageAggregates>,
) {
  return useQuery({
    queryKey: queryKeys.admin.usageAggregates(dimension, range),
    queryFn: () => adminApi.listUsageAggregates(dimension, range),
  });
}

// ---- Risk control ----
export function useRiskStatus() {
  return useQuery({
    queryKey: queryKeys.admin.riskStatus(),
    queryFn: () => adminApi.getRiskStatus(),
    refetchInterval: 30_000,
  });
}

export function useRiskLogs(params?: P<typeof adminApi.listRiskLogs>) {
  return useQuery({
    queryKey: queryKeys.admin.riskLogs(params),
    queryFn: () => adminApi.listRiskLogs(params),
  });
}

// ---- Audit & billing ----
export function useAuditLogs(params?: P<typeof adminApi.listAuditLogs>) {
  return useQuery({
    queryKey: queryKeys.admin.auditLogs(params),
    queryFn: () => adminApi.listAuditLogs(params),
  });
}
export function useOutboxEvents(params?: P<typeof adminApi.listOutboxEvents>) {
  return useQuery({
    queryKey: queryKeys.admin.outboxEvents(params),
    queryFn: () => adminApi.listOutboxEvents(params),
  });
}

export function useBillingLedger(params?: P<typeof adminApi.listBillingLedger>) {
  return useQuery({
    queryKey: queryKeys.admin.billingLedger(params),
    queryFn: () => adminApi.listBillingLedger(params),
  });
}

// ---- Admin usage ----
export function useAdminUsageLogs(params?: P<typeof adminApi.listUsageLogs>) {
  return useQuery({
    queryKey: queryKeys.admin.usageLogs(params),
    queryFn: () => adminApi.listUsageLogs(params),
  });
}

// Risk control
export function useUpdateRiskConfig() {
  return useAdminMutation(
    (body: P<typeof adminApi.updateRiskConfig>) => adminApi.updateRiskConfig(body),
    ["admin", "risk-config"],
  );
}

// Ops alerts
export function useAcknowledgeAlert() {
  return useAdminMutation(
    (id: string) => adminApi.acknowledgeAlert(id),
    ["admin", "ops", "alerts"],
  );
}

// Ops SLO definitions
export function useCreateOpsSlo() {
  return useAdminMutation(
    (body: P<typeof adminApi.createOpsSlo>) => adminApi.createOpsSlo(body),
    ["admin", "ops", "slos"],
  );
}
export function useUpdateOpsSlo() {
  return useAdminMutation(
    (vars: { id: string; body: B<typeof adminApi.updateOpsSlo> }) =>
      adminApi.updateOpsSlo(vars.id, vars.body),
    ["admin", "ops", "slos"],
  );
}
export function useDeleteOpsSlo() {
  return useAdminMutation((id: string) => adminApi.deleteOpsSlo(id), ["admin", "ops", "slos"]);
}

// Ops alert rules + silences (configurable generic metric alerting)
export function useOpsAlertRules() {
  return useQuery({
    queryKey: queryKeys.admin.opsAlertRules(),
    queryFn: () => adminApi.listOpsAlertRules(),
  });
}
export function useCreateOpsAlertRule() {
  return useAdminMutation(
    (body: P<typeof adminApi.createOpsAlertRule>) => adminApi.createOpsAlertRule(body),
    ["admin", "ops"],
  );
}
export function useUpdateOpsAlertRule() {
  return useAdminMutation(
    (vars: { id: string; body: B<typeof adminApi.updateOpsAlertRule> }) =>
      adminApi.updateOpsAlertRule(vars.id, vars.body),
    ["admin", "ops"],
  );
}
export function useDeleteOpsAlertRule() {
  return useAdminMutation(
    (id: string) => adminApi.deleteOpsAlertRule(id),
    ["admin", "ops"],
  );
}
export function useOpsAlertSilences() {
  return useQuery({
    queryKey: queryKeys.admin.opsAlertSilences(),
    queryFn: () => adminApi.listOpsAlertSilences(),
  });
}
export function useCreateOpsAlertSilence() {
  return useAdminMutation(
    (body: P<typeof adminApi.createOpsAlertSilence>) => adminApi.createOpsAlertSilence(body),
    ["admin", "ops"],
  );
}
export function useDeleteOpsAlertSilence() {
  return useAdminMutation(
    (id: string) => adminApi.deleteOpsAlertSilence(id),
    ["admin", "ops"],
  );
}

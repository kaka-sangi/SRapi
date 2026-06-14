"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { adminApi } from "@/lib/admin-api";
import { queryKeys } from "@/lib/query-keys";
import { type B, type P, useAdminMutation } from "./_shared";

// ---- Scheduled account health-test plans ----
export function useScheduledTestPlans() {
  return useQuery({
    queryKey: queryKeys.admin.scheduledTestPlans(),
    queryFn: () => adminApi.listScheduledTestPlans(),
  });
}

export function useScheduledTestPlanRuns(planId: string | null) {
  return useQuery({
    queryKey: queryKeys.admin.scheduledTestPlanRuns(planId ?? ""),
    queryFn: () => adminApi.listScheduledTestPlanRuns(planId ?? ""),
    enabled: Boolean(planId),
  });
}

// ---- Channel monitors (synthetic probes) ----
export function useChannelMonitors() {
  return useQuery({
    queryKey: queryKeys.admin.channelMonitors(),
    queryFn: () => adminApi.listChannelMonitors(),
  });
}

export function useChannelMonitorRuns(monitorId: string | null) {
  return useQuery({
    queryKey: queryKeys.admin.channelMonitorRuns(monitorId ?? ""),
    queryFn: () => adminApi.listChannelMonitorRuns(monitorId as string),
    enabled: Boolean(monitorId),
  });
}

export function useChannelMonitorTemplates() {
  return useQuery({
    queryKey: queryKeys.admin.channelMonitorTemplates(),
    queryFn: () => adminApi.listChannelMonitorTemplates(),
  });
}

// Scheduled account health-test plans
export function useCreateScheduledTestPlan() {
  return useAdminMutation(
    (body: P<typeof adminApi.createScheduledTestPlan>) =>
      adminApi.createScheduledTestPlan(body),
    ["admin", "scheduled-test-plans"],
  );
}
export function useUpdateScheduledTestPlan() {
  return useAdminMutation(
    (vars: { id: string; body: B<typeof adminApi.updateScheduledTestPlan> }) =>
      adminApi.updateScheduledTestPlan(vars.id, vars.body),
    ["admin", "scheduled-test-plans"],
  );
}
export function useDeleteScheduledTestPlan() {
  return useAdminMutation(
    (id: string) => adminApi.deleteScheduledTestPlan(id),
    ["admin", "scheduled-test-plans"],
  );
}
export function useRunScheduledTestPlan() {
  return useAdminMutation(
    (id: string) => adminApi.runScheduledTestPlan(id),
    ["admin", "scheduled-test-plans"],
  );
}

// Channel monitors (synthetic probes)
export function useCreateChannelMonitor() {
  return useAdminMutation(
    (body: P<typeof adminApi.createChannelMonitor>) => adminApi.createChannelMonitor(body),
    ["admin", "channel-monitors"],
  );
}
export function useUpdateChannelMonitor() {
  return useAdminMutation(
    (vars: { id: string; body: B<typeof adminApi.updateChannelMonitor> }) =>
      adminApi.updateChannelMonitor(vars.id, vars.body),
    ["admin", "channel-monitors"],
  );
}
export function useDeleteChannelMonitor() {
  return useAdminMutation(
    (id: string) => adminApi.deleteChannelMonitor(id),
    ["admin", "channel-monitors"],
  );
}
export function useRunChannelMonitor() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => adminApi.runChannelMonitor(id),
    onSuccess: (_data, id) => {
      qc.invalidateQueries({ queryKey: queryKeys.admin.channelMonitorRuns(id) });
    },
  });
}
export function useCreateChannelMonitorTemplate() {
  return useAdminMutation(
    (body: P<typeof adminApi.createChannelMonitorTemplate>) =>
      adminApi.createChannelMonitorTemplate(body),
    ["admin", "channel-monitor-templates"],
  );
}
export function useUpdateChannelMonitorTemplate() {
  return useAdminMutation(
    (vars: { id: string; body: B<typeof adminApi.updateChannelMonitorTemplate> }) =>
      adminApi.updateChannelMonitorTemplate(vars.id, vars.body),
    ["admin", "channel-monitor-templates"],
  );
}
export function useDeleteChannelMonitorTemplate() {
  return useAdminMutation(
    (id: string) => adminApi.deleteChannelMonitorTemplate(id),
    ["admin", "channel-monitor-templates"],
  );
}
export function useApplyChannelMonitorTemplate() {
  return useAdminMutation(
    (vars: { id: string; monitorIds: number[] }) =>
      adminApi.applyChannelMonitorTemplate(vars.id, vars.monitorIds),
    ["admin", "channel-monitors"],
  );
}

// Scheduler strategy replay
export function useSchedulerOverview() {
  return useQuery({
    queryKey: queryKeys.admin.schedulerOverview(),
    queryFn: () => adminApi.schedulerOverview(),
  });
}

export function useSchedulerStrategies() {
  return useQuery({
    queryKey: queryKeys.admin.schedulerStrategies(),
    queryFn: () => adminApi.listSchedulerStrategies(),
  });
}

export function useCreateSchedulerStrategy() {
  return useAdminMutation(
    (body: P<typeof adminApi.createSchedulerStrategy>) => adminApi.createSchedulerStrategy(body),
    ["admin", "scheduler"],
  );
}

export function useUpdateSchedulerStrategy() {
  return useAdminMutation(
    (vars: { id: string; body: B<typeof adminApi.updateSchedulerStrategy> }) =>
      adminApi.updateSchedulerStrategy(vars.id, vars.body),
    ["admin", "scheduler"],
  );
}

export function useDeprecateSchedulerStrategy() {
  return useAdminMutation(
    (id: string) => adminApi.deprecateSchedulerStrategy(id),
    ["admin", "scheduler"],
  );
}

export function useActivateSchedulerStrategy() {
  return useAdminMutation(
    (id: string) => adminApi.activateSchedulerStrategy(id),
    ["admin", "scheduler"],
  );
}

export function useSimulateSchedulerStrategy() {
  return useMutation({
    mutationFn: (body: P<typeof adminApi.simulateSchedulerStrategy>) =>
      adminApi.simulateSchedulerStrategy(body),
  });
}

export function useReplaySchedulerStrategy() {
  return useMutation({
    mutationFn: (body: P<typeof adminApi.replaySchedulerStrategy>) =>
      adminApi.replaySchedulerStrategy(body),
  });
}

// ---- Diagnostics ----

export function useAdminCircuitBreakers() {
  return useQuery({
    queryKey: ["admin", "diagnostics", "circuit-breakers"],
    queryFn: () => adminApi.getCircuitBreakers(),
    refetchInterval: 10_000,
  });
}

export function useResetCircuitBreaker() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (accountId: number) => adminApi.resetCircuitBreaker(accountId),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["admin", "diagnostics", "circuit-breakers"] });
    },
  });
}

export function useAdminCacheStats() {
  return useQuery({
    queryKey: ["admin", "diagnostics", "cache-stats"],
    queryFn: () => adminApi.getCacheStats(),
    refetchInterval: 15_000,
  });
}

export function useClearCache() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => adminApi.clearCache(),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["admin", "diagnostics", "cache-stats"] }),
  });
}

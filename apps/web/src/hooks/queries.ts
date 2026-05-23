"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { apiService, type ApiRuntimeStatus } from "@/lib/api";
import { queryKeys } from "@/lib/query-keys";
import type {
  MockApiKey,
  MockProviderAccount,
  MockSchedulerDecision,
  MockSlo,
  MockUsageLog,
  SmokeChecklist,
} from "@/lib/mockData";

/**
 * SRapi v0.1.0 query hooks.
 *
 * Each hook wraps `apiService.*` so pages stay declarative. The underlying
 * `apiService` already falls back to demo mocks when the live API is offline,
 * so callers always render something deterministic.
 */

export function useRuntimeStatus() {
  return useQuery<ApiRuntimeStatus>({
    queryKey: queryKeys.runtimeStatus(),
    queryFn: () => apiService.getRuntimeStatus(),
    staleTime: 10_000,
  });
}

export function useSmokeStatus(model?: string) {
  return useQuery<SmokeChecklist>({
    queryKey: queryKeys.smokeStatus(model),
    queryFn: () => apiService.getSmokeStatus(model),
  });
}

export function useApiKeys() {
  return useQuery<MockApiKey[]>({
    queryKey: queryKeys.apiKeys(),
    queryFn: () => apiService.listApiKeys(),
  });
}

export function useUsageLogs() {
  return useQuery<MockUsageLog[]>({
    queryKey: queryKeys.usageLogs(),
    queryFn: () => apiService.listUsageLogs(),
  });
}

export function useProviderAccounts() {
  return useQuery<MockProviderAccount[]>({
    queryKey: queryKeys.providerAccounts(),
    queryFn: () => apiService.listProviderAccounts(),
  });
}

export function useSchedulerDecisions() {
  return useQuery<MockSchedulerDecision[]>({
    queryKey: queryKeys.schedulerDecisions(),
    queryFn: () => apiService.listSchedulerDecisions(),
  });
}

export function useOverviewStats() {
  return useQuery({
    queryKey: queryKeys.overviewStats(),
    queryFn: () => apiService.getOverviewStats(),
  });
}

export function useSlos() {
  return useQuery<MockSlo[]>({
    queryKey: queryKeys.slos(),
    queryFn: () => apiService.listSlos(),
  });
}

export interface CreateApiKeyInput {
  name: string;
  allowedModels: string[];
  groupIds: string[];
}

export function useCreateApiKey() {
  const qc = useQueryClient();
  return useMutation<MockApiKey, Error, CreateApiKeyInput>({
    mutationFn: ({ name, allowedModels, groupIds }) =>
      apiService.createApiKey(name, allowedModels, groupIds),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: queryKeys.apiKeys() });
    },
  });
}

export interface ToggleApiKeyInput {
  id: string;
  currentStatus: "active" | "disabled";
}

export function useToggleApiKey() {
  const qc = useQueryClient();
  return useMutation<MockApiKey | null, Error, ToggleApiKeyInput>({
    mutationFn: ({ id, currentStatus }) => apiService.toggleApiKeyStatus(id, currentStatus),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: queryKeys.apiKeys() });
    },
  });
}

"use client";

import { useMutation, useQuery, useQueryClient, type UseQueryOptions } from "@tanstack/react-query";
import { apiService, type ApiRuntimeStatus } from "@/lib/api";
import { queryKeys } from "@/lib/query-keys";
import type {
  ApiKeySummary,
  ProviderAccountSummary,
  SchedulerDecisionSummary,
  SloSummary,
  UsageLogSummary,
  SmokeChecklist,
} from "@/lib/srapi-types";

/**
 * SRapi v0.1.0 query hooks.
 *
 * Each hook wraps `apiService.*` so pages stay declarative. Production pages
 * render explicit loading, empty, and error states; the service layer does not
 * synthesize fallback business data.
 */

export function useRuntimeStatus() {
  return useQuery<ApiRuntimeStatus>({
    queryKey: queryKeys.runtimeStatus(),
    queryFn: () => apiService.getRuntimeStatus(),
    staleTime: 10_000,
  });
}

export function useSmokeStatus(
  model?: string,
  options?: Pick<UseQueryOptions<SmokeChecklist, Error>, "enabled">,
) {
  return useQuery<SmokeChecklist>({
    queryKey: queryKeys.smokeStatus(model),
    queryFn: () => apiService.getSmokeStatus(model),
    ...options,
  });
}

export function useLiveCurrentUser() {
  return useQuery({
    queryKey: queryKeys.currentUser(),
    queryFn: () => apiService.getLiveCurrentUser(),
  });
}

export function useApiKeys() {
  return useQuery<ApiKeySummary[]>({
    queryKey: queryKeys.apiKeys(),
    queryFn: () => apiService.listApiKeys(),
  });
}

export function useUsageLogs() {
  return useQuery<UsageLogSummary[]>({
    queryKey: queryKeys.usageLogs(),
    queryFn: () => apiService.listUsageLogs(),
  });
}

export function useProviderAccounts() {
  return useQuery<ProviderAccountSummary[]>({
    queryKey: queryKeys.providerAccounts(),
    queryFn: () => apiService.listProviderAccounts(),
  });
}

export function useSchedulerDecisions() {
  return useQuery<SchedulerDecisionSummary[]>({
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
  return useQuery<SloSummary[]>({
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
  return useMutation<ApiKeySummary, Error, CreateApiKeyInput>({
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
  return useMutation<ApiKeySummary | null, Error, ToggleApiKeyInput>({
    mutationFn: ({ id, currentStatus }) => apiService.toggleApiKeyStatus(id, currentStatus),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: queryKeys.apiKeys() });
    },
  });
}

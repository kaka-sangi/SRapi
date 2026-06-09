"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { apiService } from "@/lib/api";
import { meApi } from "@/lib/me-api";
import { queryKeys } from "@/lib/query-keys";

/**
 * User-facing data hooks. Pages consume ONLY these — never useEffect+fetch.
 * Everything routes through `apiService` (lib/api.ts) → generated SDK.
 */

export function useRuntimeStatus() {
  return useQuery({
    queryKey: queryKeys.runtimeStatus(),
    queryFn: () => apiService.getRuntimeStatus(),
    staleTime: 15_000,
    refetchInterval: 30_000,
  });
}

export function useApiKeys() {
  return useQuery({
    queryKey: queryKeys.apiKeys(),
    queryFn: () => apiService.listApiKeys(),
  });
}

export function useCreateApiKey() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({
      name,
      allowedModels,
      groupIds,
      allowedIps,
      deniedIps,
      requestLimit5h,
      requestLimit1d,
      requestLimit7d,
      costQuota,
      costLimit5h,
      costLimit1d,
      costLimit7d,
      rpmLimit,
      tpmLimit,
      concurrencyLimit,
      expiresAt,
    }: {
      name: string;
      allowedModels: string[];
      groupIds: string[];
      allowedIps?: string[];
      deniedIps?: string[];
      requestLimit5h?: number;
      requestLimit1d?: number;
      requestLimit7d?: number;
      costQuota?: string;
      costLimit5h?: string;
      costLimit1d?: string;
      costLimit7d?: string;
      rpmLimit?: number;
      tpmLimit?: number;
      concurrencyLimit?: number;
      expiresAt?: string;
    }) =>
      apiService.createApiKey(name, allowedModels, groupIds, {
        allowedIps,
        deniedIps,
        requestLimit5h,
        requestLimit1d,
        requestLimit7d,
        costQuota,
        costLimit5h,
        costLimit1d,
        costLimit7d,
        rpmLimit,
        tpmLimit,
        concurrencyLimit,
        expiresAt,
      }),
    onSuccess: () => qc.invalidateQueries({ queryKey: queryKeys.apiKeys() }),
  });
}

type ApiKeyList = Awaited<ReturnType<typeof apiService.listApiKeys>>;

export function useToggleApiKey() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, status }: { id: string; status: "active" | "disabled" }) =>
      apiService.toggleApiKeyStatus(id, status),
    // 联动: flip the row instantly. onError rolls back; onSettled refetches so
    // the optimistic guess reconciles with server truth.
    onMutate: async ({ id, status }) => {
      await qc.cancelQueries({ queryKey: queryKeys.apiKeys() });
      const prev = qc.getQueryData<ApiKeyList>(queryKeys.apiKeys());
      const next = status === "active" ? "disabled" : "active";
      qc.setQueryData<ApiKeyList>(queryKeys.apiKeys(), (old) =>
        old?.map((k) => (k.id === id ? { ...k, status: next } : k)),
      );
      return { prev };
    },
    onError: (_err, _vars, ctx) => {
      if (ctx?.prev) qc.setQueryData(queryKeys.apiKeys(), ctx.prev);
    },
    onSettled: () => qc.invalidateQueries({ queryKey: queryKeys.apiKeys() }),
  });
}

export function useUpdateApiKey() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({
      id,
      ...policy
    }: {
      id: string;
    } & Parameters<typeof apiService.updateApiKey>[1]) => apiService.updateApiKey(id, policy),
    onSuccess: () => qc.invalidateQueries({ queryKey: queryKeys.apiKeys() }),
  });
}

export function useDeleteApiKey() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => apiService.deleteApiKey(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: queryKeys.apiKeys() }),
  });
}

export function useApiKeyUsage(id: string | null, days: number, enabled: boolean) {
  return useQuery({
    queryKey: queryKeys.apiKeyUsage(id ?? "", days),
    queryFn: () => apiService.getApiKeyUsage(id as string, days),
    enabled: enabled && Boolean(id),
  });
}

export function useUsageLogs() {
  return useQuery({
    queryKey: queryKeys.usageLogs(),
    queryFn: () => apiService.listUsageLogs(),
  });
}

export function useAvailableModels() {
  return useQuery({
    queryKey: queryKeys.me.availableModels(),
    queryFn: () => apiService.listAvailableModels(),
  });
}

export function useProviderAccounts() {
  return useQuery({
    queryKey: queryKeys.providerAccounts(),
    queryFn: () => apiService.listProviderAccounts(),
  });
}

export function useTestProviderAccount() {
  return useMutation({
    mutationFn: (id: string) => apiService.testProviderAccount(id),
  });
}

export function useSchedulerDecisions() {
  return useQuery({
    queryKey: queryKeys.schedulerDecisions(),
    queryFn: () => apiService.listSchedulerDecisions(),
  });
}

export function useSlos() {
  return useQuery({
    queryKey: queryKeys.slos(),
    queryFn: () => apiService.listSlos(),
  });
}

export function useSmokeStatus(model = "gpt-4o-mini") {
  return useQuery({
    queryKey: queryKeys.smokeStatus(model),
    queryFn: () => apiService.getSmokeStatus(model),
  });
}

// ============================================================
// Self-service (/me) hooks. Route through `meApi` (lib/me-api.ts).
// ============================================================

type MeP<F extends (...a: never[]) => unknown> = Parameters<F>[0];

// ---- Profile & security ----
export function useProfile() {
  return useQuery({ queryKey: queryKeys.me.profile(), queryFn: () => meApi.getProfile() });
}
export function useUpdateProfile() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: MeP<typeof meApi.updateProfile>) => meApi.updateProfile(body),
    onSuccess: () => qc.invalidateQueries({ queryKey: queryKeys.me.profile() }),
  });
}
export function useChangePassword() {
  return useMutation({
    mutationFn: (body: MeP<typeof meApi.changePassword>) => meApi.changePassword(body),
  });
}
export function useRevokeAllSessions() {
  return useMutation({ mutationFn: () => meApi.revokeAllSessions() });
}
export function useTotpStatus() {
  return useQuery({ queryKey: queryKeys.me.totpStatus(), queryFn: () => meApi.getTotpStatus() });
}
export function useSetupTotp() {
  return useMutation({ mutationFn: () => meApi.setupTotp() });
}
export function useEnableTotp() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: MeP<typeof meApi.enableTotp>) => meApi.enableTotp(body),
    onSuccess: () => qc.invalidateQueries({ queryKey: queryKeys.me.totpStatus() }),
  });
}
export function useDisableTotp() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: MeP<typeof meApi.disableTotp>) => meApi.disableTotp(body),
    onSuccess: () => qc.invalidateQueries({ queryKey: queryKeys.me.totpStatus() }),
  });
}

// ---- Avatar ----
export function useUploadAvatar() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (file: File) => meApi.uploadAvatar(file),
    onSuccess: () => qc.invalidateQueries({ queryKey: queryKeys.me.profile() }),
  });
}
export function useDeleteAvatar() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => meApi.deleteAvatar(),
    onSuccess: () => qc.invalidateQueries({ queryKey: queryKeys.me.profile() }),
  });
}

// ---- Notification preferences ----
export function useNotificationPreferences() {
  return useQuery({
    queryKey: queryKeys.me.notificationPreferences(),
    queryFn: () => meApi.listNotificationPreferences(),
  });
}
export function useUpdateNotificationPreferences() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: MeP<typeof meApi.updateNotificationPreferences>) =>
      meApi.updateNotificationPreferences(body),
    onSuccess: () =>
      qc.invalidateQueries({ queryKey: queryKeys.me.notificationPreferences() }),
  });
}

// ---- Notification contacts ----
export function useNotificationContacts() {
  return useQuery({
    queryKey: queryKeys.me.notificationContacts(),
    queryFn: () => meApi.listNotificationContacts(),
  });
}
export function useRequestNotificationContact() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: MeP<typeof meApi.requestNotificationContactVerification>) =>
      meApi.requestNotificationContactVerification(body),
    onSuccess: () => qc.invalidateQueries({ queryKey: queryKeys.me.notificationContacts() }),
  });
}
export function useConfirmNotificationContact() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: MeP<typeof meApi.confirmNotificationContact>) =>
      meApi.confirmNotificationContact(body),
    onSuccess: () => qc.invalidateQueries({ queryKey: queryKeys.me.notificationContacts() }),
  });
}
export function useUpdateNotificationContact() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, disabled }: { id: string; disabled: boolean }) =>
      meApi.updateNotificationContact(id, { disabled }),
    onSuccess: () => qc.invalidateQueries({ queryKey: queryKeys.me.notificationContacts() }),
  });
}
export function useDeleteNotificationContact() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => meApi.deleteNotificationContact(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: queryKeys.me.notificationContacts() }),
  });
}

// ---- Linked sign-in identities ----
export function useAuthIdentities() {
  return useQuery({
    queryKey: queryKeys.me.authIdentities(),
    queryFn: () => meApi.listAuthIdentities(),
  });
}
export function useUnbindAuthIdentity() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => meApi.unbindAuthIdentity(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: queryKeys.me.authIdentities() }),
  });
}

// ---- Billing ----
export function useBalance() {
  return useQuery({ queryKey: queryKeys.me.balance(), queryFn: () => meApi.getBalance() });
}
export function usePlatformQuotas() {
  return useQuery({
    queryKey: queryKeys.me.platformQuotas(),
    queryFn: () => meApi.listPlatformQuotas(),
  });
}

// ---- Playground (交界地) ----
export function usePlaygroundModels() {
  return useQuery({
    queryKey: queryKeys.me.playgroundModels(),
    queryFn: () => meApi.getPlaygroundModels(),
  });
}
export function usePaymentMethods() {
  return useQuery({
    queryKey: queryKeys.me.paymentMethods(),
    queryFn: () => meApi.listPaymentMethods(),
  });
}
export function useMyOrders() {
  return useQuery({ queryKey: queryKeys.me.orders(), queryFn: () => meApi.listOrders() });
}
export function useCreateOrder() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: MeP<typeof meApi.createOrder>) => meApi.createOrder(body),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["me", "orders"] }),
  });
}
export function useCancelOrder() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => meApi.cancelOrder(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["me", "orders"] }),
  });
}
export function useMySubscriptions() {
  return useQuery({
    queryKey: queryKeys.me.subscriptions(),
    queryFn: () => meApi.getSubscriptions(),
  });
}

// ---- Redeem ----
export function useRedeemCode() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: MeP<typeof meApi.redeemCode>) => meApi.redeemCode(body),
    onSuccess: () => qc.invalidateQueries({ queryKey: queryKeys.me.balance() }),
  });
}

// ---- Affiliate ----
export function useAffiliate() {
  return useQuery({ queryKey: queryKeys.me.affiliate(), queryFn: () => meApi.getAffiliate() });
}
export function useAffiliateLedger() {
  return useQuery({
    queryKey: queryKeys.me.affiliateLedger(),
    queryFn: () => meApi.listAffiliateLedger(),
  });
}
export function useTransferToBalance() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: MeP<typeof meApi.transferToBalance>) => meApi.transferToBalance(body),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.me.affiliate() });
      qc.invalidateQueries({ queryKey: queryKeys.me.balance() });
    },
  });
}

// ---- Announcements ----
// Polls in the background so the unread badge stays fresh without a reload.
export function useMyAnnouncements() {
  return useQuery({
    queryKey: queryKeys.me.announcements(),
    queryFn: () => meApi.listAnnouncements(),
    refetchInterval: 60_000,
    staleTime: 30_000,
  });
}
export function useMarkAnnouncementRead() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => meApi.markAnnouncementRead(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: queryKeys.me.announcements() }),
  });
}

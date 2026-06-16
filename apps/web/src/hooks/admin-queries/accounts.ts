"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { adminApi } from "@/lib/admin-api";
import { queryKeys } from "@/lib/query-keys";
import type {
  ProviderAccountStatus,
  AdminAccountTestRequest,
} from "../../../../../packages/sdk/typescript/src/types.gen";
import { type B, type P, useAdminMutation } from "./_shared";

// ---- Provider accounts ----
export function useAdminAccounts(params?: P<typeof adminApi.listAccounts>) {
  return useQuery({
    queryKey: queryKeys.admin.accounts(params),
    queryFn: () => adminApi.listAccounts(params),
  });
}

type AccountList = Awaited<ReturnType<typeof adminApi.listAccounts>>;
const ACCOUNT_LIST_KEY = ["admin", "accounts"] as const;

function isAccountListQuery(query: { queryKey: readonly unknown[] }) {
  const key = query.queryKey;
  return key.length === 3 && key[0] === "admin" && key[1] === "accounts" && isRecord(key[2]);
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

export function useSetAccountStatus() {
  const qc = useQueryClient();
  const accountListFilter = { queryKey: ACCOUNT_LIST_KEY, predicate: isAccountListQuery };
  return useMutation({
    mutationFn: ({ id, status }: { id: string; status: ProviderAccountStatus }) =>
      adminApi.setAccountStatus(id, status),
    // 联动: flip the account row instantly; rollback on error, reconcile on settle.
    onMutate: async ({ id, status }) => {
      await qc.cancelQueries(accountListFilter);
      const prev = qc.getQueriesData<AccountList>(accountListFilter);
      qc.setQueriesData<AccountList>(accountListFilter, (old) =>
        old ? { ...old, data: old.data.map((a) => (a.id === id ? { ...a, status } : a)) } : old,
      );
      return { prev };
    },
    onError: (_e, _v, ctx) => ctx?.prev?.forEach(([key, data]) => qc.setQueryData(key, data)),
    onSettled: () => {
      qc.invalidateQueries(accountListFilter);
      qc.invalidateQueries({ queryKey: queryKeys.admin.accountsHealthSummary() });
    },
  });
}

export function useTestAccount() {
  const qc = useQueryClient();
  return useMutation({
    // A connectivity test can flip the account status (e.g. needs_reauth/dead on
    // auth failure), so refetch the list to reflect the new state.
    mutationFn: ({ id, body }: { id: string; body?: AdminAccountTestRequest }) =>
      adminApi.testAccount(id, body),
    onSuccess: (_data, { id }) => {
      qc.invalidateQueries({ queryKey: ["admin", "accounts"] });
      qc.invalidateQueries({ queryKey: queryKeys.admin.accountHealth(id) });
      qc.invalidateQueries({ queryKey: queryKeys.admin.accountsHealthSummary() });
    },
  });
}

export function useAccountsHealthSummary() {
  return useQuery({
    queryKey: queryKeys.admin.accountsHealthSummary(),
    queryFn: () => adminApi.getAccountsHealthSummary(),
    staleTime: 30_000,
    refetchInterval: 60_000,
  });
}

// Per-account diagnostics — fetched on demand (when a detail view opens) so the
// list stays cheap; `enabled` gates each query behind a selected account id.
export function useAccountHealth(id: string | null) {
  return useQuery({
    queryKey: queryKeys.admin.accountHealth(id ?? ""),
    queryFn: () => adminApi.getAccountHealth(id as string),
    enabled: Boolean(id),
  });
}
export function useAccountQuota(id: string | null) {
  return useQuery({
    queryKey: queryKeys.admin.accountQuota(id ?? ""),
    queryFn: () => adminApi.getAccountQuota(id as string),
    enabled: Boolean(id),
  });
}
export function useFetchAccountQuota() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => adminApi.fetchAccountQuota(id),
    // A live pull refreshes the stored snapshots, so re-read them for this account.
    onSuccess: (_data, id) => {
      qc.invalidateQueries({ queryKey: ["admin", "accounts"] });
      qc.invalidateQueries({ queryKey: queryKeys.admin.accountQuota(id) });
      qc.invalidateQueries({ queryKey: queryKeys.admin.accountsHealthSummary() });
    },
  });
}
export function useAccountRpmStatus(id: string | null) {
  return useQuery({
    queryKey: queryKeys.admin.accountRpm(id ?? ""),
    queryFn: () => adminApi.getAccountRpmStatus(id as string),
    enabled: Boolean(id),
  });
}
export function useAccountProxyQuality(id: string | null) {
  return useQuery({
    queryKey: queryKeys.admin.accountProxyQuality(id ?? ""),
    queryFn: () => adminApi.getAccountProxyQuality(id as string),
    enabled: Boolean(id),
  });
}

// ---- Account groups ----
export function useAdminGroups() {
  return useQuery({
    queryKey: queryKeys.admin.accountGroups(),
    queryFn: () => adminApi.listAccountGroups(),
  });
}

// Members of one group — fetched on demand (when the manage-members dialog opens).
// Keyed under the account-groups prefix so add/remove member mutations refetch it.
export function useGroupMembers(groupId: string | null) {
  return useQuery({
    queryKey: queryKeys.admin.accountGroupMembers(groupId ?? ""),
    queryFn: () => adminApi.listAccountGroupMembers(groupId as string),
    enabled: Boolean(groupId),
  });
}

// ---- Proxies ----
export function useAdminProxies(params?: P<typeof adminApi.listProxies>) {
  return useQuery({
    queryKey: queryKeys.admin.proxies(params),
    queryFn: () => adminApi.listProxies(params),
  });
}

// ---- Account availability (monitoring) ----
export function useAccountsAvailability(days?: number) {
  return useQuery({
    queryKey: queryKeys.admin.accountsAvailability(days),
    queryFn: () => adminApi.listAccountsAvailability(days),
  });
}

// Provider accounts
export function useCreateAccount() {
  return useAdminMutation(
    (body: P<typeof adminApi.createAccount>) => adminApi.createAccount(body),
    ["admin", "accounts"],
  );
}
export function useUpdateAccount() {
  return useAdminMutation(
    (vars: { id: string; body: B<typeof adminApi.updateAccount> }) =>
      adminApi.updateAccount(vars.id, vars.body),
    ["admin", "accounts"],
  );
}
export function useDeleteAccount() {
  return useAdminMutation((id: string) => adminApi.deleteAccount(id), ["admin", "accounts"]);
}
export function useBindAccountProxy() {
  return useAdminMutation(
    (vars: { id: string; proxyId: string | null }) =>
      adminApi.bindAccountProxy(vars.id, vars.proxyId),
    ["admin", "accounts"],
  );
}
export function useClearAccountError() {
  return useAdminMutation((id: string) => adminApi.clearAccountError(id), ["admin", "accounts"]);
}
export function useRecoverAccount() {
  return useAdminMutation((id: string) => adminApi.recoverAccount(id), ["admin", "accounts"]);
}
export function useResetAccountQuota() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => adminApi.resetAccountQuota(id),
    onSuccess: (_data, id) => {
      qc.invalidateQueries({ queryKey: ["admin", "accounts"] });
      qc.invalidateQueries({ queryKey: queryKeys.admin.accountQuota(id) });
      qc.invalidateQueries({ queryKey: queryKeys.admin.accountsHealthSummary() });
    },
  });
}
export function useDiscoverAccountModels() {
  return useAdminMutation(
    (vars: { id: string; body?: B<typeof adminApi.discoverAccountModels> }) =>
      adminApi.discoverAccountModels(vars.id, vars.body),
    ["admin", "accounts"],
  );
}
export function useImportAccounts() {
  return useAdminMutation(
    (body: P<typeof adminApi.importAccounts>) => adminApi.importAccounts(body),
    ["admin", "accounts"],
  );
}
/** Import Codex/ChatGPT desktop session blobs as upstream codex_cli accounts. */
export function useImportCodexSession() {
  return useAdminMutation(
    (body: P<typeof adminApi.importCodexSession>) => adminApi.importCodexSession(body),
    ["admin", "accounts"],
  );
}
export function useBatchActionAccounts() {
  return useAdminMutation(
    (body: P<typeof adminApi.batchActionAccounts>) => adminApi.batchActionAccounts(body),
    ["admin", "accounts"],
  );
}
// PATCH /admin/accounts/batch — atomic multi-id status change.
// Replaces the old Promise.allSettled-over-single-item-endpoint pattern that
// fired N requests per bulk action (and couldn't roll back on partial fail).
export function useBatchUpdateAccounts() {
  return useAdminMutation(
    (body: P<typeof adminApi.batchUpdateAccounts>) => adminApi.batchUpdateAccounts(body),
    ["admin", "accounts"],
  );
}
/** Export is read-only; expose as a mutation so pages can trigger it on click. */
export function useExportAccounts() {
  return useMutation({ mutationFn: () => adminApi.exportAccounts() });
}

// Proxies
export function useCreateProxy() {
  return useAdminMutation(
    (body: P<typeof adminApi.createProxy>) => adminApi.createProxy(body),
    ["admin", "proxies"],
  );
}
export function useUpdateProxy() {
  return useAdminMutation(
    (vars: { id: string; body: B<typeof adminApi.updateProxy> }) =>
      adminApi.updateProxy(vars.id, vars.body),
    ["admin", "proxies"],
  );
}
export function useDeleteProxy() {
  return useAdminMutation((id: string) => adminApi.deleteProxy(id), ["admin", "proxies"]);
}
// One-shot probe through the proxy. No cache invalidation — the test doesn't
// mutate proxy state, just returns the probe outcome the page surfaces in a
// toast.
export function useTestProxy() {
  return useMutation({
    mutationFn: (vars: { id: string; targetUrl?: string }) =>
      adminApi.testProxy(vars.id, vars.targetUrl),
  });
}
// Bulk-import — dedupes by name + returns per-row outcome. Cache invalidation
// is identical to single-row create, so existing list views refresh.
export function useBatchCreateProxies() {
  return useAdminMutation(
    (proxies: P<typeof adminApi.batchCreateProxies>) => adminApi.batchCreateProxies(proxies),
    ["admin", "proxies"],
  );
}
// Bulk soft-delete with per-id outcome. Same semantics as single-id delete:
// accounts routed through the deleted proxy fall back to direct connection.
export function useBatchDeleteProxies() {
  return useAdminMutation(
    (proxyIds: P<typeof adminApi.batchDeleteProxies>) => adminApi.batchDeleteProxies(proxyIds),
    ["admin", "proxies"],
  );
}

// Account groups
export function useCreateGroup() {
  return useAdminMutation(
    (body: P<typeof adminApi.createAccountGroup>) => adminApi.createAccountGroup(body),
    ["admin", "account-groups"],
  );
}
export function useUpdateGroup() {
  return useAdminMutation(
    (vars: { id: string; body: B<typeof adminApi.updateAccountGroup> }) =>
      adminApi.updateAccountGroup(vars.id, vars.body),
    ["admin", "account-groups"],
  );
}
export function useDeleteGroup() {
  return useAdminMutation((id: string) => adminApi.deleteAccountGroup(id), ["admin", "account-groups"]);
}
export function useAddGroupMember() {
  return useAdminMutation(
    (vars: { accountId: string; groupId: string }) =>
      adminApi.addAccountToGroup(vars.accountId, vars.groupId),
    ["admin", "account-groups"],
  );
}
export function useRemoveGroupMember() {
  return useAdminMutation(
    (vars: { accountId: string; groupId: string }) =>
      adminApi.removeAccountFromGroup(vars.accountId, vars.groupId),
    ["admin", "account-groups"],
  );
}

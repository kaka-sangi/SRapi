"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { adminApi } from "@/lib/admin-api";
import { queryKeys } from "@/lib/query-keys";
import type { User } from "../../../../../packages/sdk/typescript/src/types.gen";
import { type B, type P, useAdminMutation } from "./_shared";

// ---- Dashboard ----
export function useAdminDashboard(range?: P<typeof adminApi.getDashboardSnapshot>) {
  return useQuery({
    queryKey: queryKeys.admin.dashboardSnapshot(range),
    queryFn: () => adminApi.getDashboardSnapshot(range),
    // Polling is driven by the page's AutoRefreshControl (single source of truth).
  });
}

// ---- Users ----
export function useAdminUsers(params?: P<typeof adminApi.listUsers>) {
  return useQuery({
    queryKey: queryKeys.admin.users(params),
    queryFn: () => adminApi.listUsers(params),
  });
}

type UserList = Awaited<ReturnType<typeof adminApi.listUsers>>;
const USERS_KEY = ["admin", "users"] as const;

/** Optimistically patch every cached users page (any filter/sort/page), and
 *  return the prior snapshots so onError can roll them all back. */
function optimisticUsers(qc: ReturnType<typeof useQueryClient>, apply: (u: User) => User) {
  const prev = qc.getQueriesData<UserList>({ queryKey: USERS_KEY });
  qc.setQueriesData<UserList>({ queryKey: USERS_KEY }, (old) =>
    old ? { ...old, data: old.data.map(apply) } : old,
  );
  return prev;
}

export function useSetUserEnabled() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (user: User) => adminApi.setUserEnabled(user),
    // 联动: the row's status badge + dim flips the instant you click, well
    // before the server responds. onError restores; onSettled reconciles.
    onMutate: async (user) => {
      await qc.cancelQueries({ queryKey: USERS_KEY });
      const next = user.status === "disabled" ? "active" : "disabled";
      const prev = optimisticUsers(qc, (u) => (u.id === user.id ? { ...u, status: next } : u));
      return { prev };
    },
    onError: (_e, _v, ctx) => ctx?.prev?.forEach(([key, data]) => qc.setQueryData(key, data)),
    onSettled: () => qc.invalidateQueries({ queryKey: USERS_KEY }),
  });
}

export function useBulkSetUsersEnabled() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ ids, enabled }: { ids: string[]; enabled: boolean }) =>
      Promise.all(ids.map((id) => adminApi.setUserEnabledById(id, enabled))),
    onMutate: async ({ ids, enabled }) => {
      await qc.cancelQueries({ queryKey: USERS_KEY });
      const next = enabled ? "active" : "disabled";
      const set = new Set(ids);
      const prev = optimisticUsers(qc, (u) => (set.has(u.id) ? { ...u, status: next } : u));
      return { prev };
    },
    onError: (_e, _v, ctx) => ctx?.prev?.forEach(([key, data]) => qc.setQueryData(key, data)),
    onSettled: () => qc.invalidateQueries({ queryKey: USERS_KEY }),
  });
}

// ---- Per-user platform spend quotas ----
export function useUserPlatformQuotas(userId: string, enabled = true) {
  return useQuery({
    queryKey: queryKeys.admin.userPlatformQuotas(userId),
    queryFn: () => adminApi.listUserPlatformQuotas(userId),
    enabled: enabled && Boolean(userId),
  });
}
export function useUpsertUserPlatformQuota() {
  return useAdminMutation(
    (vars: { userId: string; body: B<typeof adminApi.upsertUserPlatformQuota> }) =>
      adminApi.upsertUserPlatformQuota(vars.userId, vars.body),
    ["admin", "user-platform-quotas"],
  );
}
export function useDeleteUserPlatformQuota() {
  return useAdminMutation(
    (vars: { userId: string; platform: string }) =>
      adminApi.deleteUserPlatformQuota(vars.userId, vars.platform),
    ["admin", "user-platform-quotas"],
  );
}

// Users
export function useCreateAdminUser() {
  return useAdminMutation(
    (body: P<typeof adminApi.createUser>) => adminApi.createUser(body),
    ["admin", "users"],
  );
}
export function useUpdateAdminUser() {
  return useAdminMutation(
    (vars: { id: string; body: B<typeof adminApi.updateUser> }) =>
      adminApi.updateUser(vars.id, vars.body),
    ["admin", "users"],
  );
}
export function useDeleteAdminUser() {
  return useAdminMutation(
    (id: string) => adminApi.deleteUser(id),
    ["admin", "users"],
  );
}
export function useUpdateUserBalance() {
  return useAdminMutation(
    (vars: { id: string; body: B<typeof adminApi.updateUserBalance> }) =>
      adminApi.updateUserBalance(vars.id, vars.body),
    ["admin", "users"],
  );
}

// ---- Custom user attribute definitions ----
export function useUserAttributeDefinitions() {
  return useQuery({
    queryKey: queryKeys.admin.userAttributes(),
    queryFn: () => adminApi.listUserAttributeDefinitions(),
  });
}

// Custom user attribute definitions
export function useCreateUserAttributeDefinition() {
  return useAdminMutation(
    (body: P<typeof adminApi.createUserAttributeDefinition>) =>
      adminApi.createUserAttributeDefinition(body),
    ["admin", "user-attributes"],
  );
}
export function useUpdateUserAttributeDefinition() {
  return useAdminMutation(
    (vars: { id: string; body: B<typeof adminApi.updateUserAttributeDefinition> }) =>
      adminApi.updateUserAttributeDefinition(vars.id, vars.body),
    ["admin", "user-attributes"],
  );
}
export function useDeleteUserAttributeDefinition() {
  return useAdminMutation(
    (id: string) => adminApi.deleteUserAttributeDefinition(id),
    ["admin", "user-attributes"],
  );
}

// ---- Per-user attribute values ----
export function useUserAttributeValues(userId: string | null) {
  return useQuery({
    queryKey: queryKeys.admin.userAttributeValues(userId ?? ""),
    queryFn: () => adminApi.listUserAttributeValues(userId as string),
    enabled: Boolean(userId),
  });
}

// Batched read for the users list "Attributes" column. Sorts the ids so the
// cache key is stable across renders that ask for the same set in a
// different order.
export function useUserAttributeValuesBatch(userIds: string[]) {
  const sorted = [...userIds].sort();
  return useQuery({
    queryKey: queryKeys.admin.userAttributeValuesBatch(sorted),
    queryFn: () => adminApi.batchListUserAttributeValues(sorted),
    enabled: sorted.length > 0,
    staleTime: 30_000,
  });
}

// Batched read for the users list "Today" column — same sorted-ids cache-key
// pattern as the attribute-values batch above.
export function useUsersSpendingTodayBatch(userIds: string[]) {
  const sorted = [...userIds].sort();
  return useQuery({
    queryKey: queryKeys.admin.usersSpendingTodayBatch(sorted),
    queryFn: () => adminApi.batchGetUsersSpendingToday(sorted),
    enabled: sorted.length > 0,
    staleTime: 30_000,
  });
}

export function useSetUserAttributeValue(userId: string) {
  return useAdminMutation(
    (vars: { definitionId: string; body: Parameters<typeof adminApi.setUserAttributeValue>[2] }) =>
      adminApi.setUserAttributeValue(userId, vars.definitionId, vars.body),
    ["admin", "user-attribute-values", userId],
  );
}

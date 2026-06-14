"use client";

import { useQuery } from "@tanstack/react-query";
import { adminApi } from "@/lib/admin-api";
import { queryKeys } from "@/lib/query-keys";
import { type B, type P, useAdminMutation } from "./_shared";

export function useAdminModels(params?: P<typeof adminApi.listModels>) {
  return useQuery({
    queryKey: queryKeys.admin.models(params),
    queryFn: () => adminApi.listModels(params),
  });
}

// ---- Rate limits (per-model & per-account-group TPM/RPM/concurrency) ----
// The API has no per-id GET, so these list-all and the UI joins by id.
export function useModelRateLimits() {
  return useQuery({
    queryKey: ["admin", "model-rate-limits"],
    queryFn: () => adminApi.listModelRateLimits(),
  });
}
export function useUpsertModelRateLimit() {
  return useAdminMutation(
    (body: P<typeof adminApi.upsertModelRateLimit>) => adminApi.upsertModelRateLimit(body),
    ["admin", "model-rate-limits"],
  );
}
export function useDeleteModelRateLimit() {
  return useAdminMutation(
    (modelId: string) => adminApi.deleteModelRateLimit(modelId),
    ["admin", "model-rate-limits"],
  );
}
export function useGroupRateLimits() {
  return useQuery({
    queryKey: ["admin", "group-rate-limits"],
    queryFn: () => adminApi.listGroupRateLimits(),
  });
}
export function useUpsertGroupRateLimit() {
  return useAdminMutation(
    (body: P<typeof adminApi.upsertGroupRateLimit>) => adminApi.upsertGroupRateLimit(body),
    ["admin", "group-rate-limits"],
  );
}
export function useDeleteGroupRateLimit() {
  return useAdminMutation(
    (groupId: string) => adminApi.deleteGroupRateLimit(groupId),
    ["admin", "group-rate-limits"],
  );
}

// Models (model registry)
export function useCreateModel() {
  return useAdminMutation(
    (body: P<typeof adminApi.createModel>) => adminApi.createModel(body),
    ["admin", "models"],
  );
}
export function useUpdateModel() {
  return useAdminMutation(
    (vars: { id: string; body: B<typeof adminApi.updateModel> }) =>
      adminApi.updateModel(vars.id, vars.body),
    ["admin", "models"],
  );
}
export function useDeleteModel() {
  return useAdminMutation((id: string) => adminApi.deleteModel(id), ["admin", "models"]);
}
export function useCreateModelAlias() {
  return useAdminMutation(
    (vars: { id: string; body: B<typeof adminApi.createModelAlias> }) =>
      adminApi.createModelAlias(vars.id, vars.body),
    ["admin", "models"],
  );
}
export function useCreateModelMapping() {
  return useAdminMutation(
    (vars: { id: string; body: B<typeof adminApi.createModelMapping> }) =>
      adminApi.createModelMapping(vars.id, vars.body),
    ["admin", "models"],
  );
}
// Aliases/mappings of one model — fetched on demand (manage dialog). Keyed under
// the models prefix so create/delete mutations (which invalidate ["admin","models"])
// refetch them.
export function useModelAliases(modelId: string | null) {
  return useQuery({
    queryKey: ["admin", "models", modelId ?? "", "aliases"],
    queryFn: () => adminApi.listModelAliases(modelId as string),
    enabled: Boolean(modelId),
  });
}
export function useModelMappings(modelId: string | null) {
  return useQuery({
    queryKey: ["admin", "models", modelId ?? "", "mappings"],
    queryFn: () => adminApi.listModelMappings(modelId as string),
    enabled: Boolean(modelId),
  });
}
export function useDeleteModelAlias() {
  return useAdminMutation(
    (vars: { id: string; aliasId: string }) => adminApi.deleteModelAlias(vars.id, vars.aliasId),
    ["admin", "models"],
  );
}
export function useDeleteModelMapping() {
  return useAdminMutation(
    (vars: { id: string; mappingId: string }) => adminApi.deleteModelMapping(vars.id, vars.mappingId),
    ["admin", "models"],
  );
}

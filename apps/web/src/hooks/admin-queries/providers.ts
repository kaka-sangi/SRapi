"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { adminApi } from "@/lib/admin-api";
import { queryKeys } from "@/lib/query-keys";
import { type B, type P, useAdminMutation } from "./_shared";

// ---- Providers & models (reference data) ----
export function useAdminProviders(params?: P<typeof adminApi.listProviders>) {
  return useQuery({
    queryKey: queryKeys.admin.providers(params),
    queryFn: () => adminApi.listProviders(params),
  });
}

// Providers (upstream platform registry)
export function useCreateProvider() {
  return useAdminMutation(
    (body: P<typeof adminApi.createProvider>) => adminApi.createProvider(body),
    ["admin", "providers"],
  );
}
export function useUpdateProvider() {
  return useAdminMutation(
    (vars: { id: string; body: B<typeof adminApi.updateProvider> }) =>
      adminApi.updateProvider(vars.id, vars.body),
    ["admin", "providers"],
  );
}
export function useDeleteProvider() {
  return useAdminMutation((id: string) => adminApi.deleteProvider(id), ["admin", "providers"]);
}
export function useTestProvider() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => adminApi.testProvider(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["admin", "providers"] }),
  });
}
export function useInstallProviderPresets() {
  return useAdminMutation<void, Awaited<ReturnType<typeof adminApi.installProviderPresets>>>(
    () => adminApi.installProviderPresets(),
    ["admin", "providers"],
  );
}

export function useRunQuickSetup() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: P<typeof adminApi.runQuickSetup>) => adminApi.runQuickSetup(body),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["admin", "providers"] });
      qc.invalidateQueries({ queryKey: ["admin", "accounts"] });
      qc.invalidateQueries({ queryKey: ["admin", "models"] });
      qc.invalidateQueries({ queryKey: queryKeys.admin.accountsHealthSummary() });
    },
  });
}

// Bulk-create canonical models + provider mappings for a provider. Used by the
// quick-setup custom-platform path to actually map the operator's models
// (instead of silently creating none).
export function useQuickMapModels() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: P<typeof adminApi.quickMapModels>) => adminApi.quickMapModels(body),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["admin", "models"] });
    },
  });
}

// ---- TLS fingerprint profiles ----
export function useTlsProfiles() {
  return useQuery({
    queryKey: queryKeys.admin.tlsProfiles(),
    queryFn: () => adminApi.listTlsProfiles(),
  });
}

// TLS fingerprint profiles
export function useCreateTlsProfile() {
  return useAdminMutation(
    (body: P<typeof adminApi.createTlsProfile>) => adminApi.createTlsProfile(body),
    ["admin", "tls-profiles"],
  );
}
export function useUpdateTlsProfile() {
  return useAdminMutation(
    (vars: { id: string; body: B<typeof adminApi.updateTlsProfile> }) =>
      adminApi.updateTlsProfile(vars.id, vars.body),
    ["admin", "tls-profiles"],
  );
}
export function useDeleteTlsProfile() {
  return useAdminMutation(
    (id: string) => adminApi.deleteTlsProfile(id),
    ["admin", "tls-profiles"],
  );
}

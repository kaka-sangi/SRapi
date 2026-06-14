"use client";

import {
  createAdminProvider,
  deleteAdminProvider,
  getAdminProviderOAuthConfig,
  installAdminProviderPresets,
  listAdminProviders,
  runAdminQuickSetup,
  testAdminProvider,
  updateAdminProvider,
} from "../../../../../packages/sdk/typescript/src/index";
import type {
  AdminQuickSetupResult,
  AdminTestResult,
  ProviderOAuthConfig,
  Id,
  ListAdminProvidersData,
  Provider,
  BatchOperationResult,
  RunAdminQuickSetupData,
  UpdateAdminProviderData,
} from "../../../../../packages/sdk/typescript/src/types.gen";
import { unwrapData, unwrapList } from "./_shared";
import type { AdminListResult } from "./types";

export const providersApi = {
  listProviders(query?: ListAdminProvidersData["query"]): Promise<AdminListResult<Provider>> {
    return unwrapList(() => listAdminProviders({ query, throwOnError: true }));
  },

  createProvider(body: Parameters<typeof createAdminProvider>[0]["body"]): Promise<Provider> {
    return unwrapData(() => createAdminProvider({ body, throwOnError: true }));
  },

  updateProvider(id: Id, body: UpdateAdminProviderData["body"]): Promise<Provider> {
    return unwrapData(() =>
      updateAdminProvider({ path: { id }, body, throwOnError: true }),
    );
  },

  deleteProvider(id: Id): Promise<{ deleted: boolean }> {
    return unwrapData(() => deleteAdminProvider({ path: { id }, throwOnError: true }));
  },

  testProvider(id: Id): Promise<AdminTestResult> {
    return unwrapData(() => testAdminProvider({ path: { id }, throwOnError: true }));
  },
  // Bulk-create any missing built-in provider presets (existing ones are skipped,
  // new ones land disabled). Returns how many were requested/created/failed.
  installProviderPresets(): Promise<BatchOperationResult> {
    return unwrapData(() => installAdminProviderPresets({ throwOnError: true }));
  },

  runQuickSetup(body: RunAdminQuickSetupData["body"]): Promise<AdminQuickSetupResult> {
    return unwrapData(() => runAdminQuickSetup({ body, throwOnError: true }));
  },

  getProviderOAuthConfig(id: Id): Promise<ProviderOAuthConfig> {
    return unwrapData(() => getAdminProviderOAuthConfig({ path: { id }, throwOnError: true }));
  },
};

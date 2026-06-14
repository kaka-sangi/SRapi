"use client";

import {
  listAdminModelRateLimits,
  upsertAdminModelRateLimit,
  deleteAdminModelRateLimit,
  listAdminGroupRateLimits,
  upsertAdminGroupRateLimit,
  deleteAdminGroupRateLimit,
  getAdminConfigSnapshot,
  importAdminConfigSnapshot,
  getAdminSettings,
  getAdminCopilotConfig,
  sendAdminTestEmail,
  updateAdminSettings,
} from "../../../../../packages/sdk/typescript/src/index";
import type {
  AdminCopilotConfig,
  AdminSendTestEmailRequest,
  AdminSettings,
  AdminTestResult,
  ConfigImportRequest,
  ConfigImportResponse,
  ConfigSnapshotResponse,
  Id,
  ModelRateLimit,
  AccountGroupRateLimit,
  UpsertModelRateLimitRequest,
  UpsertGroupRateLimitRequest,
} from "../../../../../packages/sdk/typescript/src/types.gen";
import { configureAdminClient, unwrapData, unwrapList } from "./_shared";
import type { AdminListResult } from "./types";

export const settingsApi = {
  getSettings(): Promise<AdminSettings> {
    return unwrapData(() => getAdminSettings({ throwOnError: true }));
  },

  updateSettings(body: AdminSettings): Promise<AdminSettings> {
    return unwrapData(() => updateAdminSettings({ body, throwOnError: true }));
  },

  // Deliver a probe email through the effective SMTP config. The write-only SMTP
  // password makes this the only way to confirm the credentials actually work.
  sendTestEmail(body?: AdminSendTestEmailRequest): Promise<AdminTestResult> {
    return unwrapData(() => sendAdminTestEmail({ body: body ?? {}, throwOnError: true }));
  },

  getCopilotConfig(): Promise<AdminCopilotConfig> {
    return unwrapData(() => getAdminCopilotConfig({ throwOnError: true }));
  },

  getConfigSnapshot(): Promise<ConfigSnapshotResponse["data"]> {
    return unwrapData(() => getAdminConfigSnapshot({ throwOnError: true }));
  },

  importConfigSnapshot(
    body: ConfigImportRequest,
    dryRun = false,
  ): Promise<ConfigImportResponse["data"]> {
    return unwrapData(() =>
      importAdminConfigSnapshot({ body, query: { dry_run: dryRun }, throwOnError: true }),
    );
  },

  // Rate limits (per-model & per-account-group TPM/RPM/concurrency). The API keys
  // them by id with no per-id GET, so reads list all and the UI joins by id.
  listModelRateLimits(): Promise<AdminListResult<ModelRateLimit>> {
    return unwrapList(() => listAdminModelRateLimits({ throwOnError: true }));
  },
  upsertModelRateLimit(body: UpsertModelRateLimitRequest): Promise<ModelRateLimit> {
    return unwrapData(() => upsertAdminModelRateLimit({ body, throwOnError: true }));
  },
  async deleteModelRateLimit(modelId: Id): Promise<void> {
    configureAdminClient();
    await deleteAdminModelRateLimit({ path: { modelId }, throwOnError: true });
  },
  listGroupRateLimits(): Promise<AdminListResult<AccountGroupRateLimit>> {
    return unwrapList(() => listAdminGroupRateLimits({ throwOnError: true }));
  },
  upsertGroupRateLimit(body: UpsertGroupRateLimitRequest): Promise<AccountGroupRateLimit> {
    return unwrapData(() => upsertAdminGroupRateLimit({ body, throwOnError: true }));
  },
  async deleteGroupRateLimit(groupId: Id): Promise<void> {
    configureAdminClient();
    await deleteAdminGroupRateLimit({ path: { groupId }, throwOnError: true });
  },
};

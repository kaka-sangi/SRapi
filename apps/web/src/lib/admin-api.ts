"use client";

import type {
  AnnouncementStatus,
  UserStatus,
} from "../../../../packages/sdk/typescript/src/types.gen";
import { accountsApi } from "./admin-api/accounts";
import { affiliateApi } from "./admin-api/affiliate";
import { diagnosticsApi } from "./admin-api/diagnostics";
import { modelsApi } from "./admin-api/models";
import { monitoringApi } from "./admin-api/monitoring";
import { notificationsApi } from "./admin-api/notifications";
import { opsApi } from "./admin-api/ops";
import { paymentsApi } from "./admin-api/payments";
import { providersApi } from "./admin-api/providers";
import { proxiesApi } from "./admin-api/proxies";
import { rbacApi } from "./admin-api/rbac";
import { riskApi } from "./admin-api/risk";
import { schedulerApi } from "./admin-api/scheduler";
import { settingsApi } from "./admin-api/settings";
import { usageApi } from "./admin-api/usage";
import { usersApi } from "./admin-api/users";

// Public type surface — re-exported so every type stays importable from
// "@/lib/admin-api" exactly as before the decomposition.
export type {
  AdminListResult,
  AdminTimeRange,
  CircuitBreakerEntry,
  CRSPreviewRequest,
  CRSPreviewAccount,
  CRSPreviewResult,
  CRSSyncRequest,
  CRSSyncResult,
  CacheStatsEntry,
  AdminUnsupportedSurface,
} from "./admin-api/types";

export function adminErrorMessage(error: unknown): string {
  if (error instanceof Error && error.message) {
    return error.message;
  }

  if (typeof error === "object" && error !== null) {
    const maybe = error as {
      error?: { message?: string };
      message?: string;
      response?: { data?: { error?: { message?: string } } };
    };
    return (
      maybe.response?.data?.error?.message ||
      maybe.error?.message ||
      maybe.message ||
      "Admin API request failed."
    );
  }

  return "Admin API request failed.";
}

// The single public `adminApi` object, composed from per-domain method groups.
// Spread preserves every method name and signature 1:1 (no `this`/shared
// closure state is involved), so the public surface is byte-identical to the
// previous monolithic object literal.
export const adminApi = {
  ...opsApi,
  ...schedulerApi,
  ...usersApi,
  ...providersApi,
  ...modelsApi,
  ...accountsApi,
  ...proxiesApi,
  ...usageApi,
  ...affiliateApi,
  ...paymentsApi,
  ...monitoringApi,
  ...rbacApi,
  ...notificationsApi,
  ...riskApi,
  ...settingsApi,
  ...diagnosticsApi,
};

export function statusQuery(status: string): { status?: UserStatus | AnnouncementStatus | string } {
  return status === "all" ? {} : { status };
}

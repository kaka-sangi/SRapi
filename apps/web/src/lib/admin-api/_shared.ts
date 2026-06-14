"use client";

import {
  configureSdkClient,
  configuredApiBaseUrl,
  getStoredCSRFToken,
} from "../sdk-client";
import type { Pagination } from "../../../../../packages/sdk/typescript/src/types.gen";
import type { AdminListResult } from "./types";

// SDK-client setup (base URL, cookie credentials, CSRF auth) is shared across
// the functional clients; see ../sdk-client. Kept under the original local name
// so the many call sites below stay untouched.
export const configureAdminClient = configureSdkClient;

configureAdminClient();

export async function unwrapData<T>(
  request: () => Promise<{ data?: { data?: T } }>,
): Promise<T> {
  configureAdminClient();
  const response = await request();
  if (!response.data || !("data" in response.data)) {
    throw new Error("Admin API returned an empty response.");
  }
  return response.data.data as T;
}

export async function unwrapList<T>(
  request: () => Promise<{ data?: { data?: T[]; pagination?: Pagination } }>,
): Promise<AdminListResult<T>> {
  configureAdminClient();
  const response = await request();
  if (!response.data || !Array.isArray(response.data.data)) {
    throw new Error("Admin API returned an empty list response.");
  }
  return {
    data: response.data.data,
    pagination: response.data.pagination,
  };
}

// Re-exported for the raw-fetch diagnostics/CRS-sync client, which talks to
// endpoints not yet present in the generated SDK.
export { configuredApiBaseUrl, getStoredCSRFToken };

"use client";

import { configuredApiBaseUrl, getStoredCSRFToken } from "./_shared";
import type {
  CircuitBreakerEntry,
  CacheStatsEntry,
  CRSPreviewRequest,
  CRSPreviewResult,
  CRSSyncRequest,
  CRSSyncResult,
} from "./types";

// TODO(sdk-gap): the diagnostics (circuit-breaker / cache) and CRS-sync
// endpoints below are not in the generated SDK, so they stay raw fetches
// (auth/base-url via the shared sdk-client helpers). Swap to SDK functions
// once those endpoints are generated.
export const diagnosticsApi = {
  async getCircuitBreakers(): Promise<CircuitBreakerEntry[]> {
    const base = configuredApiBaseUrl();
    const res = await fetch(`${base}/api/v1/admin/diagnostics/circuit-breakers`, {
      credentials: "include",
      headers: { "X-CSRF-Token": getStoredCSRFToken() ?? "" },
    });
    if (!res.ok) throw new Error("Failed to fetch circuit breaker status");
    const json = await res.json();
    return json.data ?? [];
  },

  async resetCircuitBreaker(accountId: number): Promise<void> {
    const base = configuredApiBaseUrl();
    const res = await fetch(`${base}/api/v1/admin/diagnostics/circuit-breakers/${accountId}/reset`, {
      method: "POST",
      credentials: "include",
      headers: { "X-CSRF-Token": getStoredCSRFToken() ?? "" },
    });
    if (!res.ok) throw new Error("Failed to reset circuit breaker");
  },

  async getCacheStats(): Promise<CacheStatsEntry[]> {
    const base = configuredApiBaseUrl();
    const res = await fetch(`${base}/api/v1/admin/diagnostics/cache-stats`, {
      credentials: "include",
      headers: { "X-CSRF-Token": getStoredCSRFToken() ?? "" },
    });
    if (!res.ok) throw new Error("Failed to fetch cache stats");
    const json = await res.json();
    return json.data ?? [];
  },

  async clearCache(): Promise<{ cleared: number }> {
    const base = configuredApiBaseUrl();
    const res = await fetch(`${base}/api/v1/admin/diagnostics/cache/clear`, {
      method: "POST",
      credentials: "include",
      headers: { "X-CSRF-Token": getStoredCSRFToken() ?? "" },
    });
    if (!res.ok) throw new Error("Failed to clear cache");
    const json = await res.json();
    return json.data;
  },

  async crsPreview(body: CRSPreviewRequest): Promise<CRSPreviewResult> {
    const base = configuredApiBaseUrl();
    const res = await fetch(`${base}/api/v1/admin/accounts/sync/crs/preview`, {
      method: "POST",
      credentials: "include",
      headers: {
        "Content-Type": "application/json",
        "X-CSRF-Token": getStoredCSRFToken() ?? "",
      },
      body: JSON.stringify(body),
    });
    if (!res.ok) {
      const err = await res.json().catch(() => ({}));
      throw new Error(err?.error?.message || "CRS preview failed");
    }
    const json = await res.json();
    return json.data;
  },

  async crsSync(body: CRSSyncRequest): Promise<CRSSyncResult> {
    const base = configuredApiBaseUrl();
    const res = await fetch(`${base}/api/v1/admin/accounts/sync/crs`, {
      method: "POST",
      credentials: "include",
      headers: {
        "Content-Type": "application/json",
        "X-CSRF-Token": getStoredCSRFToken() ?? "",
      },
      body: JSON.stringify(body),
    });
    if (!res.ok) {
      const err = await res.json().catch(() => ({}));
      throw new Error(err?.error?.message || "CRS sync failed");
    }
    const json = await res.json();
    return json.data;
  },
};

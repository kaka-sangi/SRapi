"use client";

/**
 * Shared SDK-client configuration.
 *
 * The generated SDK exposes a single module-level `client` singleton. Every
 * functional client wrapper (admin-api, me-api, the auth/session layer in
 * api.ts) needs the same setup before each call: point the client at the
 * configured base URL, send cookies (`credentials: "include"`), and resolve the
 * X-CSRF-Token header from localStorage. This used to be copy-pasted into each
 * module; it now lives here so there is exactly one source of truth.
 *
 * `configureSdkClient()` is intentionally cheap and idempotent — call it at the
 * top of every request path so the singleton is always pointed at the right
 * place even after another module reconfigured it.
 */

import type { Auth } from "../../../../packages/sdk/typescript/src/core/auth.gen";
import { client } from "../../../../packages/sdk/typescript/src/client.gen";

export const CSRF_STORAGE_KEY = "srapi_csrf_token";

export function configuredApiBaseUrl(): string {
  return (process.env.NEXT_PUBLIC_SRAPI_BASE_URL || "").replace(/\/+$/, "");
}

export function getStoredCSRFToken(): string | undefined {
  if (typeof window === "undefined") {
    return undefined;
  }
  return localStorage.getItem(CSRF_STORAGE_KEY) || undefined;
}

function resolveAuthToken(auth: Auth): string | undefined {
  if (auth.name === "X-CSRF-Token") {
    return getStoredCSRFToken();
  }
  // Browser cookies are sent by fetch credentials. Do not synthesize Cookie headers.
  return undefined;
}

export function configureSdkClient(): void {
  client.setConfig({
    baseUrl: configuredApiBaseUrl(),
    credentials: "include",
    auth: resolveAuthToken,
  });
}

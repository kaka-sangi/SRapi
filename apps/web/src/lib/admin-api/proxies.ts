"use client";

import {
  batchCreateAdminProxies,
  batchDeleteAdminProxies,
  batchTestAdminProxies,
  createAdminProxy,
  deleteAdminProxy,
  listAdminProxies,
  testAdminProxy,
  updateAdminProxy,
} from "../../../../../packages/sdk/typescript/src/index";
import type {
  BatchCreateProxiesResult,
  BatchDeleteProxiesResult,
  CreateAdminProxyData,
  CreateProxyDefinitionRequestWritable,
  Id,
  ListAdminProxiesData,
  ProxyDefinition,
  ProxyBatchTestRow,
  ProxyTestResult,
  UpdateAdminProxyData,
} from "../../../../../packages/sdk/typescript/src/types.gen";
import { unwrapData, unwrapList } from "./_shared";
import type { AdminListResult } from "./types";

export const proxiesApi = {
  listProxies(query?: ListAdminProxiesData["query"]): Promise<AdminListResult<ProxyDefinition>> {
    return unwrapList(() => listAdminProxies({ query, throwOnError: true }));
  },

  createProxy(body: CreateAdminProxyData["body"]): Promise<ProxyDefinition> {
    return unwrapData(() => createAdminProxy({ body, throwOnError: true }));
  },

  // Bulk-create with name dedupe + per-row outcome (created/skipped/errors).
  // Use when importing a CSV; the single-row createProxy is fine for one-offs.
  // The 'Writable' variant carries the URL field that's read-only on responses.
  batchCreateProxies(
    proxies: CreateProxyDefinitionRequestWritable[],
  ): Promise<BatchCreateProxiesResult> {
    return unwrapData(() =>
      batchCreateAdminProxies({ body: { proxies }, throwOnError: true }),
    );
  },

  updateProxy(id: Id, body: UpdateAdminProxyData["body"]): Promise<ProxyDefinition> {
    return unwrapData(() => updateAdminProxy({ path: { id }, body, throwOnError: true }));
  },

  deleteProxy(id: Id): Promise<{ deleted: boolean }> {
    return unwrapData(() => deleteAdminProxy({ path: { id }, throwOnError: true }));
  },

  // Bulk soft-delete with per-id outcome — accounts routed through deleted
  // proxies fall back to a direct connection (matches single-id DELETE).
  batchDeleteProxies(proxyIds: Id[]): Promise<BatchDeleteProxiesResult> {
    return unwrapData(() =>
      batchDeleteAdminProxies({ body: { proxy_ids: proxyIds }, throwOnError: true }),
    );
  },

  // One-shot probe through the proxy. Always resolves to a ProxyTestResult
  // (ok=false categorizes the failure mode in `error_class`) unless the proxy
  // itself was deleted, in which case the SDK throws on the 404.
  testProxy(id: Id, targetUrl?: string): Promise<ProxyTestResult> {
    const body = targetUrl ? { target_url: targetUrl } : undefined;
    return unwrapData(() => testAdminProxy({ path: { id }, body, throwOnError: true }));
  },

  // Bulk probe — server runs the probes in parallel with its own concurrency
  // cap so the client doesn't have to (and one HTTP round-trip beats N).
  // Each input id gets one row in the result; missing ids surface as
  // error_class="not_found" rather than failing the whole call.
  batchTestProxies(proxyIds: Id[]): Promise<ProxyBatchTestRow[]> {
    return unwrapData(() =>
      batchTestAdminProxies({ body: { proxy_ids: proxyIds }, throwOnError: true }),
    );
  },
};

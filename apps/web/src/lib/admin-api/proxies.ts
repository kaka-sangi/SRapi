"use client";

import {
  batchCreateAdminProxies,
  batchDeleteAdminProxies,
  createAdminProxy,
  deleteAdminProxy,
  listAdminProxies,
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
};

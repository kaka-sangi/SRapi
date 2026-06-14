"use client";

import {
  createAdminProxy,
  deleteAdminProxy,
  listAdminProxies,
  updateAdminProxy,
} from "../../../../../packages/sdk/typescript/src/index";
import type {
  CreateAdminProxyData,
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

  updateProxy(id: Id, body: UpdateAdminProxyData["body"]): Promise<ProxyDefinition> {
    return unwrapData(() => updateAdminProxy({ path: { id }, body, throwOnError: true }));
  },

  deleteProxy(id: Id): Promise<{ deleted: boolean }> {
    return unwrapData(() => deleteAdminProxy({ path: { id }, throwOnError: true }));
  },
};

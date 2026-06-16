"use client";

import {
  getAdminPermissionCatalog,
  createAdminRole,
  deleteAdminRole,
  listAdminRoles,
  updateAdminApiKey,
  updateAdminRole,
  listAdminApiKeys as listAdminApiKeysFn,
  getAdminApiKeyUsage as getAdminApiKeyUsageFn,
  resetAdminApiKeyUsage,
} from "../../../../../packages/sdk/typescript/src/index";
import type {
  AdminUpdateApiKeyRequest,
  ApiKey,
  GatewayUsageResponse,
  PermissionDefinition,
  Role,
  CreateRoleRequest,
  UpdateRoleRequest,
  Id,
  ListAdminApiKeysData,
} from "../../../../../packages/sdk/typescript/src/types.gen";
import { configureAdminClient, unwrapData, unwrapList } from "./_shared";
import type { AdminListResult } from "./types";

export const rbacApi = {
  listRoles(): Promise<AdminListResult<Role>> {
    return unwrapList(() => listAdminRoles({ throwOnError: true }));
  },

  listPermissionCatalog(): Promise<PermissionDefinition[]> {
    return unwrapData(() => getAdminPermissionCatalog({ throwOnError: true }));
  },

  createRole(body: CreateRoleRequest): Promise<Role> {
    return unwrapData(() => createAdminRole({ body, throwOnError: true }));
  },

  updateRole(id: Id, body: UpdateRoleRequest): Promise<Role> {
    return unwrapData(() => updateAdminRole({ path: { id }, body, throwOnError: true }));
  },

  deleteRole(id: Id): Promise<{ deleted: boolean }> {
    return unwrapData(() => deleteAdminRole({ path: { id }, throwOnError: true }));
  },

  listAdminApiKeys(query?: ListAdminApiKeysData["query"]): Promise<AdminListResult<ApiKey>> {
    return unwrapList(() => listAdminApiKeysFn({ query, throwOnError: true }));
  },

  updateAdminApiKey(id: Id, body: AdminUpdateApiKeyRequest): Promise<ApiKey> {
    return unwrapData(() => updateAdminApiKey({ path: { id }, body, throwOnError: true }));
  },

  // Admin recovery: zero the key's rolling cost-used counters so it can serve
  // again after tripping its quota. Single-UPDATE on the backend so it can't
  // race with concurrent ApplyCostUsage.
  resetAdminApiKeyUsage(id: Id): Promise<ApiKey> {
    return unwrapData(() => resetAdminApiKeyUsage({ path: { id }, throwOnError: true }));
  },

  // The usage envelope is returned bare (no { data } wrapper), so call directly.
  async getAdminApiKeyUsage(id: Id, days: number): Promise<GatewayUsageResponse> {
    configureAdminClient();
    const response = await getAdminApiKeyUsageFn({ path: { id }, query: { days }, throwOnError: true });
    if (!response.data) {
      throw new Error("API key usage returned an empty response.");
    }
    return response.data;
  },
};

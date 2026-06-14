import {
  listApiKeys as sdkListApiKeys,
  createApiKey as sdkCreateApiKey,
  updateApiKey as sdkUpdateApiKey,
  deleteApiKey as sdkDeleteApiKey,
  getApiKeyUsage as sdkGetApiKeyUsage,
} from '../../../../../packages/sdk/typescript/src/index';
import type { GatewayUsageResponse } from '../../../../../packages/sdk/typescript/src/types.gen';
import type { ApiKeySummary } from '../srapi-types';
import { configureSDKClient } from './_shared';
import type { LiveApiKey } from './types';

export const keysApi = {
  async listApiKeys(): Promise<ApiKeySummary[]> {
    const response = await sdkListApiKeys({ throwOnError: true });
    if (response.data) {
      const liveKeys = (response.data.data || []) as LiveApiKey[];
      return liveKeys.map((key) => ({
        id: key.id,
        name: key.name,
        prefix: key.prefix,
        status: (key.status === 'active' ? 'active' : 'disabled') as 'active' | 'disabled',
        created_at: key.created_at,
        allowed_models: key.allowed_models || [],
        group_ids: key.group_ids || [],
        allowed_ips: key.allowed_ips || [],
        denied_ips: key.denied_ips || [],
        request_limit_5h: key.request_limit_5h ?? null,
        request_limit_1d: key.request_limit_1d ?? null,
        request_limit_7d: key.request_limit_7d ?? null,
        cost_quota: key.cost_quota ?? null,
        cost_used: key.cost_used ?? null,
        cost_limit_5h: key.cost_limit_5h ?? null,
        cost_used_5h: key.cost_used_5h ?? null,
        cost_limit_1d: key.cost_limit_1d ?? null,
        cost_used_1d: key.cost_used_1d ?? null,
        cost_limit_7d: key.cost_limit_7d ?? null,
        cost_used_7d: key.cost_used_7d ?? null,
        rpm_limit: key.rpm_limit ?? null,
        tpm_limit: key.tpm_limit ?? null,
        concurrency_limit: key.concurrency_limit ?? null,
        expires_at: key.expires_at ?? null
      }));
    }
    return [];
  },

  async createApiKey(
    name: string,
    allowedModels: string[],
    groupIds: string[],
    options?: {
      allowedIps?: string[];
      deniedIps?: string[];
      requestLimit5h?: number;
      requestLimit1d?: number;
      requestLimit7d?: number;
      costQuota?: string;
      costLimit5h?: string;
      costLimit1d?: string;
      costLimit7d?: string;
      rpmLimit?: number;
      tpmLimit?: number;
      concurrencyLimit?: number;
      expiresAt?: string;
    }
  ): Promise<ApiKeySummary> {
    const response = await sdkCreateApiKey({
      body: {
        name,
        allowed_models: allowedModels,
        group_ids: groupIds,
        scopes: ['gateway:invoke'],
        ...(options?.allowedIps?.length ? { allowed_ips: options.allowedIps } : {}),
        ...(options?.deniedIps?.length ? { denied_ips: options.deniedIps } : {}),
        ...(options?.requestLimit5h != null ? { request_limit_5h: options.requestLimit5h } : {}),
        ...(options?.requestLimit1d != null ? { request_limit_1d: options.requestLimit1d } : {}),
        ...(options?.requestLimit7d != null ? { request_limit_7d: options.requestLimit7d } : {}),
        ...(options?.costQuota ? { cost_quota: options.costQuota } : {}),
        ...(options?.costLimit5h ? { cost_limit_5h: options.costLimit5h } : {}),
        ...(options?.costLimit1d ? { cost_limit_1d: options.costLimit1d } : {}),
        ...(options?.costLimit7d ? { cost_limit_7d: options.costLimit7d } : {}),
        ...(options?.rpmLimit != null ? { rpm_limit: options.rpmLimit } : {}),
        ...(options?.tpmLimit != null ? { tpm_limit: options.tpmLimit } : {}),
        ...(options?.concurrencyLimit != null ? { concurrency_limit: options.concurrencyLimit } : {}),
        ...(options?.expiresAt ? { expires_at: options.expiresAt } : {})
      },
      throwOnError: true
    });
    if (response.data?.data) {
      const key = response.data.data.api_key;
      const plaintext = response.data.data.plaintext_key;
      return {
        id: key.id,
        name: key.name,
        prefix: key.prefix,
        plaintextKey: plaintext,
        status: key.status === 'active' ? 'active' : 'disabled',
        created_at: key.created_at || new Date().toISOString(),
        allowed_models: key.allowed_models || allowedModels,
        group_ids: key.group_ids || groupIds,
        allowed_ips: key.allowed_ips || [],
        denied_ips: key.denied_ips || [],
        request_limit_5h: key.request_limit_5h ?? null,
        request_limit_1d: key.request_limit_1d ?? null,
        request_limit_7d: key.request_limit_7d ?? null,
        cost_quota: key.cost_quota ?? null,
        cost_used: key.cost_used ?? null,
        cost_limit_5h: key.cost_limit_5h ?? null,
        cost_used_5h: key.cost_used_5h ?? null,
        cost_limit_1d: key.cost_limit_1d ?? null,
        cost_used_1d: key.cost_used_1d ?? null,
        cost_limit_7d: key.cost_limit_7d ?? null,
        cost_used_7d: key.cost_used_7d ?? null,
        rpm_limit: key.rpm_limit ?? null,
        tpm_limit: key.tpm_limit ?? null,
        concurrency_limit: key.concurrency_limit ?? null,
        expires_at: key.expires_at ?? null
      };
    }
    throw new Error('API key creation returned an empty response.');
  },

  async toggleApiKeyStatus(id: string, currentStatus: 'active' | 'disabled'): Promise<ApiKeySummary | null> {
    const nextStatus = currentStatus === 'active' ? 'disabled' : 'active';

    const response = await sdkUpdateApiKey({
      path: { id },
      body: { status: nextStatus },
      throwOnError: true
    });
    if (response.data?.data) {
      const key = response.data.data;
      return {
        id: key.id,
        name: key.name,
        prefix: key.prefix,
        status: (key.status === 'active' ? 'active' : 'disabled') as 'active' | 'disabled',
        created_at: key.created_at,
        allowed_models: key.allowed_models || [],
        group_ids: key.group_ids || [],
        allowed_ips: key.allowed_ips || [],
        denied_ips: key.denied_ips || [],
        request_limit_5h: key.request_limit_5h ?? null,
        request_limit_1d: key.request_limit_1d ?? null,
        request_limit_7d: key.request_limit_7d ?? null,
        cost_quota: key.cost_quota ?? null,
        cost_used: key.cost_used ?? null,
        cost_limit_5h: key.cost_limit_5h ?? null,
        cost_used_5h: key.cost_used_5h ?? null,
        cost_limit_1d: key.cost_limit_1d ?? null,
        cost_used_1d: key.cost_used_1d ?? null,
        cost_limit_7d: key.cost_limit_7d ?? null,
        cost_used_7d: key.cost_used_7d ?? null,
        rpm_limit: key.rpm_limit ?? null,
        tpm_limit: key.tpm_limit ?? null,
        concurrency_limit: key.concurrency_limit ?? null,
        expires_at: key.expires_at ?? null
      };
    }
    throw new Error('API key update returned an empty response.');
  },

  // Edit the full policy of an existing key. Status stays under the row
  // enable/disable control; everything else the create dialog collects is
  // editable here. `expiresAt` is omitted when blank to leave expiry unchanged.
  async updateApiKey(
    id: string,
    policy: {
      name: string;
      allowedModels: string[];
      groupIds: string[];
      allowedIps?: string[];
      deniedIps?: string[];
      requestLimit5h?: number;
      requestLimit1d?: number;
      requestLimit7d?: number;
      costQuota?: string;
      costLimit5h?: string;
      costLimit1d?: string;
      costLimit7d?: string;
      rpmLimit?: number;
      tpmLimit?: number;
      concurrencyLimit?: number;
      expiresAt?: string;
    }
  ): Promise<void> {
    await sdkUpdateApiKey({
      path: { id },
      body: {
        name: policy.name,
        allowed_models: policy.allowedModels,
        group_ids: policy.groupIds,
        // Arrays are always sent so emptying a list clears it server-side.
        allowed_ips: policy.allowedIps ?? [],
        denied_ips: policy.deniedIps ?? [],
        // 0 is the backend's canonical "unlimited" (enforcement keeps only
        // limit > 0), so a blank field clears the limit rather than no-op.
        rpm_limit: policy.rpmLimit ?? 0,
        tpm_limit: policy.tpmLimit ?? 0,
        concurrency_limit: policy.concurrencyLimit ?? 0,
        request_limit_5h: policy.requestLimit5h ?? 0,
        request_limit_1d: policy.requestLimit1d ?? 0,
        request_limit_7d: policy.requestLimit7d ?? 0,
        cost_quota: policy.costQuota ?? null,
        cost_limit_5h: policy.costLimit5h ?? null,
        cost_limit_1d: policy.costLimit1d ?? null,
        cost_limit_7d: policy.costLimit7d ?? null,
        ...(policy.expiresAt ? { expires_at: policy.expiresAt } : {})
      },
      throwOnError: true
    });
  },

  async deleteApiKey(id: string): Promise<void> {
    configureSDKClient();
    await sdkDeleteApiKey({ path: { id }, throwOnError: true });
  },

  async getApiKeyUsage(id: string, days: number): Promise<GatewayUsageResponse> {
    const response = await sdkGetApiKeyUsage({ path: { id }, query: { days }, throwOnError: true });
    if (!response.data) {
      throw new Error('API key usage returned an empty response.');
    }
    return response.data;
  },
};

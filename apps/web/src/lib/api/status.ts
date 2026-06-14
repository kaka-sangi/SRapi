import {
  listAdminUsageLogs as sdkListAdminUsageLogs,
  listAdminSchedulerDecisions as sdkListAdminSchedulerDecisions,
} from '../../../../../packages/sdk/typescript/src/index';
import {
  offlineSmokeStatus,
  type SmokeChecklist,
} from '../srapi-types';
import {
  apiBaseUrlLabel,
  fetchApiJSON,
  isBackendConnected as isBackendConnectedFn,
  isPublicHTTPSURL,
  shouldUseLiveAPI as shouldUseLiveAPIFn,
} from './_shared';
import type {
  ApiRuntimeStatus,
  LiveModel,
  LiveProviderAccount,
  LiveSchedulerDecision,
  LiveUsageLog,
  SiteConfig,
} from './types';

export const statusApi = {
  async isBackendConnected(): Promise<boolean> {
    return isBackendConnectedFn();
  },

  async getRuntimeStatus(): Promise<ApiRuntimeStatus> {
    const connected = await this.isBackendConnected();

    return {
      mode: connected ? 'live' : 'offline',
      connected,
      apiBaseUrl: apiBaseUrlLabel(),
      checkedAt: new Date().toISOString()
    };
  },

  async getSiteConfig(): Promise<SiteConfig> {
    const response = await fetchApiJSON<{ data: SiteConfig }>('/api/v1/site-config');
    return response.data;
  },

  async shouldUseLiveAPI(): Promise<boolean> {
    return shouldUseLiveAPIFn();
  },

  async getSmokeStatus(model: string = 'gpt-4o-mini'): Promise<SmokeChecklist> {
    if (await this.shouldUseLiveAPI()) {
      try {
        const [modelsRes, accountsRes, usageRes, decisionsRes] = await Promise.all([
          fetchApiJSON<{ data?: LiveModel[] }>(`/api/v1/admin/models?q=${encodeURIComponent(model)}`),
          fetchApiJSON<{ data?: LiveProviderAccount[] }>('/api/v1/admin/accounts?status=active'),
          sdkListAdminUsageLogs(),
          sdkListAdminSchedulerDecisions()
        ]);

        const coreEndpoints = ['/v1/chat/completions', '/v1/responses', '/v1/messages'];
        const activeAccounts = accountsRes.data || [];
        const realUpstreamAccounts = activeAccounts.filter((account) => {
          const baseUrl = typeof account.metadata?.base_url === 'string' ? account.metadata.base_url : undefined;
          return isPublicHTTPSURL(baseUrl);
        });
        const realUpstreamAccountIDs = new Set(realUpstreamAccounts.map((account) => String(account.id)));

        const usageLogs = (usageRes.data?.data || []) as LiveUsageLog[];
        const usageEndpoints = new Set(usageLogs
          .filter((row) => row.model === model && row.success === true)
          .map((row) => row.source_endpoint)
        );

        const decisionLogs = (decisionsRes.data?.data || []) as LiveSchedulerDecision[];
        const realDecisionEndpoints = new Set(decisionLogs
          .filter((row) => row.model === model && realUpstreamAccountIDs.has(String(row.selected_account_id || '')))
          .map((row) => row.source_endpoint)
        );

        const missingUsage = coreEndpoints.filter(endpoint => !usageEndpoints.has(endpoint));
        const missingRealDecisions = coreEndpoints.filter(endpoint => !realDecisionEndpoints.has(endpoint));
        const models = modelsRes.data || [];
        const modelExists = models.some((item) => item.canonical_name === model);
        const gatewaySmokeComplete = modelExists
          && activeAccounts.length > 0
          && missingUsage.length === 0;

        return {
          base_url: apiBaseUrlLabel(),
          model,
          model_exists: modelExists,
          active_account_count: activeAccounts.length,
          public_https_upstream_account_count: realUpstreamAccounts.length,
          usage_endpoints: Array.from(usageEndpoints) as string[],
          real_upstream_scheduler_decision_endpoints: Array.from(realDecisionEndpoints) as string[],
          missing_usage_endpoints: missingUsage,
          missing_real_upstream_scheduler_decision_endpoints: missingRealDecisions,
          v0_1_smoke_evidence_complete: gatewaySmokeComplete
        };
      } catch {
        // The self-check drawer already renders an explicit incomplete/offline
        // state. Avoid noisy console warnings when a route transition aborts
        // these background requests.
      }
    }

    return {
      ...offlineSmokeStatus,
      base_url: apiBaseUrlLabel()
    };
  }
};

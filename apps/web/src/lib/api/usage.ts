import {
  getCurrentUserUsage as sdkGetCurrentUserUsage,
  listCurrentUserAvailableModels as sdkListCurrentUserAvailableModels,
  listAdminSchedulerDecisions as sdkListAdminSchedulerDecisions,
  listAdminOpsSlos as sdkListAdminOpsSlos,
} from '../../../../../packages/sdk/typescript/src/index';
import type {
  AvailableModelSummary,
  SchedulerDecisionSummary,
  SloSummary,
  UsageLogSummary,
} from '../srapi-types';
import { configureSDKClient, parseMoneyValue } from './_shared';
import type { LiveSchedulerDecision, LiveSlo, LiveUsageLog } from './types';

export interface SchedulerDecisionListParams {
  request_id?: string;
  model?: string;
}

export const usageApi = {
  async listUsageLogs(): Promise<UsageLogSummary[]> {
    const response = await sdkGetCurrentUserUsage({ throwOnError: true });
    if (response.data) {
      return ((response.data.data || []) as LiveUsageLog[]).map((log) => ({
        created_at: log.created_at,
        request_id: log.request_id,
        model: log.model,
        source_endpoint: log.source_endpoint,
        success: log.success,
        total_tokens: log.total_tokens || 0,
        cost: typeof log.cost === 'number' ? log.cost : parseFloat(log.cost || '0'),
        input_cost: parseMoneyValue(log.input_cost),
        output_cost: parseMoneyValue(log.output_cost),
        cache_read_cost: parseMoneyValue(log.cache_read_cost),
        cache_write_cost: parseMoneyValue(log.cache_write_cost),
        requested_model: log.requested_model,
        upstream_model: log.upstream_model,
        billing_mode: log.billing_mode,
        currency: log.currency || 'USD'
      }));
    }
    return [];
  },

  async listAvailableModels(): Promise<AvailableModelSummary[]> {
    configureSDKClient();
    const response = await sdkListCurrentUserAvailableModels({ throwOnError: true });
    return (response.data?.data ?? []).map((model) => ({
      id: model.id,
      name: model.name,
      family: model.family ?? null,
      status: model.status,
      context_window: model.context_window ?? null,
      max_output_tokens: model.max_output_tokens ?? null,
      channels: (model.channels ?? []).map((channel) => ({
        provider_id: channel.provider_id,
        provider_name: channel.provider_name,
        provider_display_name: channel.provider_display_name,
        adapter_type: channel.adapter_type,
        protocol: channel.protocol,
        upstream_model: channel.upstream_model,
        status: channel.status,
        active_account_count: channel.active_account_count,
        total_account_count: channel.total_account_count,
        pricing: {
          billing_mode: channel.pricing.billing_mode,
          currency: channel.pricing.currency,
          input_price_per_million_tokens: channel.pricing.input_price_per_million_tokens,
          output_price_per_million_tokens: channel.pricing.output_price_per_million_tokens,
          cache_read_price_per_million_tokens: channel.pricing.cache_read_price_per_million_tokens,
          cache_write_price_per_million_tokens: channel.pricing.cache_write_price_per_million_tokens,
          per_request_price: channel.pricing.per_request_price,
          source: channel.pricing.source,
        },
      })),
    }));
  },

  async listSchedulerDecisions(params: SchedulerDecisionListParams = {}): Promise<SchedulerDecisionSummary[]> {
    const response = await sdkListAdminSchedulerDecisions({
      query: {
        request_id: params.request_id || undefined,
        model: params.model || undefined,
      },
      throwOnError: true,
    });
    if (response.data) {
      return ((response.data.data || []) as LiveSchedulerDecision[]).map((decision) => ({
        created_at: decision.created_at,
        request_id: decision.request_id,
        model: decision.model,
        source_endpoint: decision.source_endpoint,
        candidate_count: decision.candidate_count || 1,
        selected_account_id: decision.selected_account_id || '',
        selected_account_name: decision.selected_account?.name || 'Upstream Account',
        rejected_count: decision.rejected_count || 0,
        rejected_reasons: Array.isArray(decision.rejected_reasons) ? decision.rejected_reasons : [],
        scores: Array.isArray(decision.scores) ? decision.scores : [],
        warnings: decision.warnings || [],
        logs: decision.logs || []
      }));
    }
    return [];
  },

  async listSlos(): Promise<SloSummary[]> {
    const response = await sdkListAdminOpsSlos({ throwOnError: true });
    if (response.data) {
      return ((response.data.data || []) as LiveSlo[]).map((slo) => {
        const definition = slo.definition;
        const objective = slo.objective ?? definition?.objective ?? slo.evaluation?.objective ?? 0.99;
        const errorRate = slo.evaluation?.error_rate ?? 0;

        return {
          id: slo.id || definition?.id || 'slo',
          name: slo.name || definition?.name || 'Gateway SLO',
          sli_type: slo.sli_type || definition?.sli_type || 'availability',
          objective: objective > 1 ? objective : objective * 100,
          window: slo.window || (definition?.window_days ? `${definition.window_days}-day` : '30-day'),
          availability: slo.availability ?? Math.max(0, (1 - errorRate) * 100),
          status: (slo.status || definition?.status || 'healthy') as 'healthy' | 'burning' | 'breached'
        };
      });
    }
    return [];
  },
};

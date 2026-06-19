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
  account_id?: string;
  provider_id?: string;
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
        account_id: params.account_id || undefined,
        provider_id: params.provider_id || undefined,
        model: params.model || undefined,
      },
      throwOnError: true,
    });
    if (response.data) {
      return ((response.data.data || []) as LiveSchedulerDecision[]).map(mapSchedulerDecision);
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

export function mapSchedulerDecision(decision: LiveSchedulerDecision): SchedulerDecisionSummary {
  const selectedAccountID = cleanString(decision.selected_account_id);
  return {
    id: cleanString(decision.id) || `${decision.request_id}:${decision.attempt_no ?? 1}`,
    created_at: decision.created_at,
    request_id: decision.request_id,
    attempt_no: positiveInt(decision.attempt_no, 1),
    model: decision.model,
    source_protocol: cleanString(decision.source_protocol) || 'openai-compatible',
    source_endpoint: decision.source_endpoint,
    target_protocol: cleanString(decision.target_protocol),
    strategy: cleanString(decision.strategy) || 'balanced',
    strategy_version: cleanString(decision.strategy_version),
    fallback_from_decision_id: cleanString(decision.fallback_from_decision_id) || null,
    candidate_count: Math.max(0, positiveInt(decision.candidate_count, 0)),
    selected_provider_id: cleanString(decision.selected_provider_id) || null,
    selected_account_id: selectedAccountID || null,
    selected_account_name: decision.selected_account?.name || (selectedAccountID ? `#${selectedAccountID}` : 'None'),
    rejected_count: Math.max(0, positiveInt(decision.rejected_count, 0)),
    rejected_reasons: normalizeRejectReasons(decision.reject_reasons ?? decision.rejected_reasons),
    scores: normalizeScores(decision.scores),
    selection_rationale: cleanString(decision.selection_rationale),
    sticky_hit: Boolean(decision.sticky_hit),
    cache_affinity_hit: Boolean(decision.cache_affinity_hit),
    estimated_cost: cleanString(decision.estimated_cost) || '0.00000000',
    currency: cleanString(decision.currency) || 'USD',
    warnings: nonEmptyStrings(decision.compatibility_warnings ?? decision.warnings),
    logs: nonEmptyStrings(decision.logs),
  };
}

function normalizeRejectReasons(raw: unknown): SchedulerDecisionSummary['rejected_reasons'] {
  if (Array.isArray(raw)) {
    return raw.flatMap((item) => {
      const record = asRecord(item);
      if (!record) return [];
      const reason = cleanString(record.reason);
      if (!reason) return [];
      const accountID = cleanString(record.account_id);
      return [{
        account_id: accountID || undefined,
        account: cleanString(record.account) || accountLabel(accountID),
        reason,
      }];
    });
  }

  const record = asRecord(raw);
  if (!record) return [];
  return Object.entries(record).flatMap(([key, value]) => {
    const reason = cleanString(value);
    if (!reason) return [];
    const accountID = accountIDFromKey(key);
    return [{
      account_id: accountID || undefined,
      account: accountID ? accountLabel(accountID) : key,
      reason,
    }];
  });
}

function normalizeScores(raw: unknown): SchedulerDecisionSummary['scores'] {
  if (Array.isArray(raw)) {
    return raw
      .flatMap((item) => scoreFromRecord(asRecord(item), undefined, new Set<string>()))
      .sort(compareScores);
  }

  const record = asRecord(raw);
  if (!record) return [];
  const frontier = paretoFrontierIDs(record.pareto);
  return Object.entries(record)
    .flatMap(([key, value]) => {
      if (key === 'pareto' || key === 'routing_hints' || key === 'pricing') return [];
      return scoreFromRecord(asRecord(value), accountIDFromKey(key), frontier);
    })
    .sort(compareScores);
}

function scoreFromRecord(
  record: Record<string, unknown> | null,
  fallbackAccountID: string | undefined,
  frontier: Set<string>,
): SchedulerDecisionSummary['scores'] {
  if (!record) return [];
  const accountID = cleanString(record.account_id) || fallbackAccountID;
  const account = cleanString(record.account) || accountLabel(accountID);
  const score = numberField(record.final_score, record.score);
  if (!account && score === 0) return [];
  return [{
    account_id: accountID || undefined,
    account,
    score,
    health: numberField(record.health_score, record.health),
    latency: numberField(record.latency_score, record.latency),
    cost: numberField(record.cost_score, record.cost),
    quota: numberField(record.quota_score, record.quota),
    quality: numberField(record.quality_score, record.quality),
    sticky: numberField(record.sticky_score, record.sticky),
    cache: numberField(record.cache_score, record.cache),
    fairness: numberField(record.fairness_score, record.fairness),
    risk_penalty: numberField(record.risk_penalty),
    saturation_penalty: numberField(record.saturation_penalty),
    quality_tier: cleanString(record.quality_tier) || undefined,
    pareto_frontier: accountID ? frontier.has(accountID) : false,
  }];
}

function paretoFrontierIDs(raw: unknown): Set<string> {
  const record = asRecord(raw);
  const rawIDs = record?.frontier_account_ids;
  if (!Array.isArray(rawIDs)) return new Set();
  return new Set(rawIDs.map(cleanString).filter(Boolean));
}

function compareScores(a: SchedulerDecisionSummary['scores'][number], b: SchedulerDecisionSummary['scores'][number]): number {
  if (a.score !== b.score) return b.score - a.score;
  return a.account.localeCompare(b.account);
}

function asRecord(value: unknown): Record<string, unknown> | null {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
    ? value as Record<string, unknown>
    : null;
}

function accountIDFromKey(key: string): string | undefined {
  const match = /^account_(.+)$/.exec(key.trim());
  if (match?.[1]) return match[1];
  return /^\d+$/.test(key.trim()) ? key.trim() : undefined;
}

function accountLabel(accountID?: string): string {
  return accountID ? `#${accountID}` : '';
}

function numberField(...values: unknown[]): number {
  for (const value of values) {
    const num = typeof value === 'number' ? value : typeof value === 'string' ? Number(value) : NaN;
    if (Number.isFinite(num)) return num;
  }
  return 0;
}

function positiveInt(value: unknown, fallback: number): number {
  const num = numberField(value);
  return num > 0 ? Math.floor(num) : fallback;
}

function nonEmptyStrings(values: unknown): string[] {
  if (!Array.isArray(values)) return [];
  return values.map(cleanString).filter(Boolean);
}

function cleanString(value: unknown): string {
  if (value === null || value === undefined) return '';
  return String(value).trim();
}

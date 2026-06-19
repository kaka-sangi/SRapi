export interface CurrentUser {
  id?: string;
  email: string;
  name: string;
  role: "admin" | "user";
  balance: string;
  currency: string;
  rpm_limit?: number | null;
  last_login_at?: string | null;
  created_at?: string;
}

export interface ApiKeySummary {
  id: string;
  name: string;
  prefix: string;
  plaintextKey?: string;
  status: "active" | "disabled";
  created_at: string;
  allowed_models: string[];
  group_ids: string[];
  // Full policy, carried so the edit dialog can prefill without a second fetch.
  allowed_ips?: string[];
  denied_ips?: string[];
  request_limit_5h?: number | null;
  request_limit_1d?: number | null;
  request_limit_7d?: number | null;
  cost_quota?: string | null;
  cost_used?: string | null;
  cost_limit_5h?: string | null;
  cost_used_5h?: string | null;
  cost_limit_1d?: string | null;
  cost_used_1d?: string | null;
  cost_limit_7d?: string | null;
  cost_used_7d?: string | null;
  rpm_limit?: number | null;
  tpm_limit?: number | null;
  concurrency_limit?: number | null;
  expires_at?: string | null;
}

export interface ProviderAccountSummary {
  id: string;
  name: string;
  provider_id: string;
  provider_name: string;
  runtime_class: "api_key" | "cli_client_token" | "oauth_refresh";
  status: "active" | "limited" | "disabled";
  base_url: string;
  supported_models: string[];
  latency: number;
  quota_percentage: number;
}

export interface UsageLogSummary {
  created_at: string;
  request_id: string;
  model: string;
  source_endpoint: string;
  success: boolean;
  total_tokens: number;
  cost: number;
  input_cost: number;
  output_cost: number;
  cache_read_cost: number;
  cache_write_cost: number;
  requested_model?: string;
  upstream_model?: string;
  billing_mode?: "token" | "per_request" | "image";
  currency: string;
}

export interface AvailableModelPricingSummary {
  billing_mode: "token" | "per_request" | "image";
  currency: string;
  input_price_per_million_tokens: string;
  output_price_per_million_tokens: string;
  cache_read_price_per_million_tokens: string;
  cache_write_price_per_million_tokens: string;
  per_request_price: string;
  source: "pricing_rule" | "mapping_override" | "default_zero";
}

export interface AvailableModelChannelSummary {
  provider_id: string;
  provider_name: string;
  provider_display_name: string;
  adapter_type: string;
  protocol: string;
  upstream_model: string;
  status: "available" | "limited" | "unavailable";
  active_account_count: number;
  total_account_count: number;
  pricing: AvailableModelPricingSummary;
}

export interface AvailableModelSummary {
  id: string;
  name: string;
  family?: string | null;
  status: "available" | "limited" | "unavailable";
  context_window?: number | null;
  max_output_tokens?: number | null;
  channels: AvailableModelChannelSummary[];
}

export interface SchedulerDecisionSummary {
  id: string;
  created_at: string;
  request_id: string;
  attempt_no: number;
  model: string;
  source_protocol: string;
  source_endpoint: string;
  target_protocol: string;
  strategy: string;
  strategy_version: string;
  fallback_from_decision_id?: string | null;
  candidate_count: number;
  selected_provider_id?: string | null;
  selected_account_id?: string | null;
  selected_account_name: string;
  rejected_count: number;
  rejected_reasons: SchedulerRejectReasonSummary[];
  scores: SchedulerScoreSummary[];
  selection_rationale: string;
  sticky_hit: boolean;
  cache_affinity_hit: boolean;
  estimated_cost: string;
  currency: string;
  warnings: string[];
  logs: string[];
}

export interface SchedulerRejectReasonSummary {
  account_id?: string;
  account: string;
  reason: string;
}

export interface SchedulerScoreSummary {
  account_id?: string;
  account: string;
  score: number;
  health: number;
  latency: number;
  cost: number;
  quota: number;
  quality: number;
  sticky: number;
  cache: number;
  fairness: number;
  risk_penalty: number;
  saturation_penalty: number;
  quality_tier?: string;
  pareto_frontier: boolean;
}

export interface SloSummary {
  id: string;
  name: string;
  sli_type: string;
  objective: number;
  window: string;
  availability: number;
  status: "healthy" | "burning" | "breached";
}

export interface SmokeChecklist {
  base_url: string;
  model: string;
  model_exists: boolean;
  active_account_count: number;
  public_https_upstream_account_count: number;
  usage_endpoints: string[];
  real_upstream_scheduler_decision_endpoints: string[];
  missing_usage_endpoints: string[];
  missing_real_upstream_scheduler_decision_endpoints: string[];
  v0_1_smoke_evidence_complete: boolean;
}

export const offlineSmokeStatus: SmokeChecklist = {
  base_url: "http://127.0.0.1:8080",
  model: "gpt-4o-mini",
  model_exists: false,
  active_account_count: 0,
  public_https_upstream_account_count: 0,
  usage_endpoints: [],
  real_upstream_scheduler_decision_endpoints: [],
  missing_usage_endpoints: ["/v1/chat/completions", "/v1/responses", "/v1/messages"],
  missing_real_upstream_scheduler_decision_endpoints: [
    "/v1/chat/completions",
    "/v1/responses",
    "/v1/messages",
  ],
  v0_1_smoke_evidence_complete: false,
};

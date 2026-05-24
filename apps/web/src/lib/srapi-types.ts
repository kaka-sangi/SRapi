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
  currency: string;
}

export interface SchedulerDecisionSummary {
  created_at: string;
  request_id: string;
  model: string;
  source_endpoint: string;
  candidate_count: number;
  selected_account_id: string;
  selected_account_name: string;
  rejected_count: number;
  rejected_reasons: { account: string; reason: string }[];
  scores: { account: string; score: number; latency: number; cost: number; quota: number }[];
  warnings: string[];
  logs: string[];
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

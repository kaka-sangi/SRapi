export interface MockUser {
  email: string;
  name: string;
  role: 'admin' | 'user';
  balance: string;
  currency: string;
  authMode?: 'live' | 'demo';
}

export interface MockApiKey {
  id: string;
  name: string;
  prefix: string;
  plaintextKey?: string;
  status: 'active' | 'disabled';
  created_at: string;
  allowed_models: string[];
  group_ids: string[];
}

export interface MockProviderAccount {
  id: string;
  name: string;
  provider_id: string;
  provider_name: string;
  runtime_class: 'api_key' | 'cli_client_token' | 'oauth_refresh';
  status: 'active' | 'limited' | 'disabled';
  base_url: string;
  supported_models: string[];
  latency: number;
  quota_percentage: number;
}

export interface MockUsageLog {
  created_at: string;
  request_id: string;
  model: string;
  source_endpoint: string;
  success: boolean;
  total_tokens: number;
  cost: number;
  currency: string;
}

export interface MockSchedulerDecision {
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

export interface MockSlo {
  id: string;
  name: string;
  sli_type: string;
  objective: number;
  window: string;
  availability: number;
  status: 'healthy' | 'burning' | 'breached';
}

export const mockUsers: Record<string, MockUser> = {
  admin: {
    email: 'admin@srapi.local',
    name: 'Academic Operator',
    role: 'admin',
    balance: '9999.00',
    currency: 'USD',
  },
  user: {
    email: 'developer@srapi.local',
    name: 'Jane Dev',
    role: 'user',
    balance: '42.50',
    currency: 'USD',
  }
};

export const initialApiKeys: MockApiKey[] = [
  {
    id: "key-01",
    name: "Production Gateway",
    prefix: "sk-srapi-prod...",
    status: "active",
    created_at: "2026-05-15T08:00:00Z",
    allowed_models: ["gpt-4o-mini", "claude-3-5-sonnet"],
    group_ids: ["group-01"]
  },
  {
    id: "key-02",
    name: "Local Testing Key",
    prefix: "sk-srapi-test...",
    status: "active",
    created_at: "2026-05-20T14:32:00Z",
    allowed_models: ["gpt-4o-mini"],
    group_ids: ["group-02"]
  },
  {
    id: "key-03",
    name: "Deprecated Integration",
    prefix: "sk-srapi-depr...",
    status: "disabled",
    created_at: "2026-04-10T10:15:00Z",
    allowed_models: ["gpt-4o-mini", "claude-3-5-sonnet", "gemini-1.5-pro"],
    group_ids: ["group-01"]
  }
];

export const mockProviderAccounts: MockProviderAccount[] = [
  {
    id: "acc-01",
    name: "OpenAI Main Pool #1",
    provider_id: "prov-openai",
    provider_name: "OpenAI",
    runtime_class: "api_key",
    status: "active",
    base_url: "https://api.openai.com/v1",
    supported_models: ["gpt-4o-mini", "gpt-4o"],
    latency: 184,
    quota_percentage: 82
  },
  {
    id: "acc-02",
    name: "Anthropic Corporate Pool",
    provider_id: "prov-anthropic",
    provider_name: "Anthropic",
    runtime_class: "api_key",
    status: "active",
    base_url: "https://api.anthropic.com/v1",
    supported_models: ["claude-3-5-sonnet", "claude-3-5-haiku"],
    latency: 240,
    quota_percentage: 95
  },
  {
    id: "acc-03",
    name: "Gemini Public High-Perf",
    provider_id: "prov-google",
    provider_name: "Google Gemini",
    runtime_class: "api_key",
    status: "limited",
    base_url: "https://generativelanguage.googleapis.com/v1beta",
    supported_models: ["gemini-1.5-flash", "gemini-1.5-pro"],
    latency: 310,
    quota_percentage: 24
  },
  {
    id: "acc-04",
    name: "Mock Local Dev Endpoint",
    provider_id: "prov-mock",
    provider_name: "Local Mock",
    runtime_class: "cli_client_token",
    status: "active",
    base_url: "http://localhost:8090/v1",
    supported_models: ["gpt-4o-mini"],
    latency: 45,
    quota_percentage: 100
  },
  {
    id: "acc-05",
    name: "Expired Sandbox Key",
    provider_id: "prov-openai",
    provider_name: "OpenAI",
    runtime_class: "oauth_refresh",
    status: "disabled",
    base_url: "https://api.openai.com/v1",
    supported_models: ["gpt-4o-mini"],
    latency: 999,
    quota_percentage: 0
  }
];

export const mockUsageLogs: MockUsageLog[] = [
  {
    created_at: "2026-05-23T14:15:20Z",
    request_id: "req_a1b2c3d4e5f601",
    model: "gpt-4o-mini",
    source_endpoint: "/v1/chat/completions",
    success: true,
    total_tokens: 1250,
    cost: 0.000375,
    currency: "USD"
  },
  {
    created_at: "2026-05-23T14:12:45Z",
    request_id: "req_f6e5d4c3b2a102",
    model: "claude-3-5-sonnet",
    source_endpoint: "/v1/messages",
    success: true,
    total_tokens: 4200,
    cost: 0.021000,
    currency: "USD"
  },
  {
    created_at: "2026-05-23T14:10:02Z",
    request_id: "req_f6e5d4c3b2a103",
    model: "gpt-4o-mini",
    source_endpoint: "/v1/responses",
    success: true,
    total_tokens: 890,
    cost: 0.000267,
    currency: "USD"
  },
  {
    created_at: "2026-05-23T14:05:12Z",
    request_id: "req_99887766554404",
    model: "gemini-1.5-flash",
    source_endpoint: "/v1/chat/completions",
    success: false,
    total_tokens: 0,
    cost: 0.0,
    currency: "USD"
  },
  {
    created_at: "2026-05-23T13:58:30Z",
    request_id: "req_11223344556605",
    model: "gpt-4o-mini",
    source_endpoint: "/v1/chat/completions",
    success: true,
    total_tokens: 1840,
    cost: 0.000552,
    currency: "USD"
  }
];

export const mockSchedulerDecisions: MockSchedulerDecision[] = [
  {
    created_at: "2026-05-23T14:15:20Z",
    request_id: "req_a1b2c3d4e5f601",
    model: "gpt-4o-mini",
    source_endpoint: "/v1/chat/completions",
    candidate_count: 3,
    selected_account_id: "acc-01",
    selected_account_name: "OpenAI Main Pool #1",
    rejected_count: 2,
    rejected_reasons: [
      { account: "acc-03 (Gemini Public)", reason: "Model capability mapping mismatch" },
      { account: "acc-05 (Expired Sandbox)", reason: "Account is disabled by operator" }
    ],
    scores: [
      { account: "acc-01 (OpenAI Pool #1)", score: 0.94, latency: 0.95, cost: 0.90, quota: 0.97 },
      { account: "acc-04 (Mock Local Dev)", score: 0.88, latency: 0.99, cost: 0.70, quota: 0.95 }
    ],
    warnings: [],
    logs: [
      "[Scheduler] Resolving capability requirements: [streaming, chat_completions]",
      "[Scheduler] Discovered 3 active candidate accounts matching model 'gpt-4o-mini'",
      "[Scheduler] Applying policy filter: CostLimitPolicy, HealthyStatePolicy",
      "[Scheduler] Evaluating scores via 'balanced' strategy...",
      "[Scheduler] Selected account acc-01 (Score: 0.94) with lease reservation [Lease ID: L-98231]"
    ]
  },
  {
    created_at: "2026-05-23T14:12:45Z",
    request_id: "req_f6e5d4c3b2a102",
    model: "claude-3-5-sonnet",
    source_endpoint: "/v1/messages",
    candidate_count: 2,
    selected_account_id: "acc-02",
    selected_account_name: "Anthropic Corporate Pool",
    rejected_count: 1,
    rejected_reasons: [
      { account: "acc-01 (OpenAI Pool #1)", reason: "Model mismatch: requires Claude 3.5 Sonnet" }
    ],
    scores: [
      { account: "acc-02 (Anthropic Corp)", score: 0.96, latency: 0.92, cost: 0.96, quota: 0.99 }
    ],
    warnings: ["Sticky session affinity fallback: target conversation thread had low latency lease"],
    logs: [
      "[Scheduler] Resolving capability requirements: [streaming, messages]",
      "[Scheduler] Discovered 1 active candidate account matching model 'claude-3-5-sonnet'",
      "[Scheduler] Applying sticky affinity to group 'session-user-9912'",
      "[Scheduler] Selected account acc-02 (Score: 0.96)"
    ]
  },
  {
    created_at: "2026-05-23T14:10:02Z",
    request_id: "req_f6e5d4c3b2a103",
    model: "gpt-4o-mini",
    source_endpoint: "/v1/responses",
    candidate_count: 2,
    selected_account_id: "acc-04",
    selected_account_name: "Mock Local Dev Endpoint",
    rejected_count: 1,
    rejected_reasons: [
      { account: "acc-03 (Gemini Public)", reason: "Latency exceeded 300ms SLA target" }
    ],
    scores: [
      { account: "acc-04 (Mock Local)", score: 0.97, latency: 0.99, cost: 0.95, quota: 0.97 },
      { account: "acc-01 (OpenAI Pool #1)", score: 0.84, latency: 0.85, cost: 0.80, quota: 0.88 }
    ],
    warnings: [],
    logs: [
      "[Scheduler] Resolving capability requirements: [responses]",
      "[Scheduler] Selected account acc-04 (Score: 0.97) under balanced heuristic"
    ]
  }
];

export const mockSlos: MockSlo[] = [
  {
    id: "slo-1",
    name: "API Gateway Availability",
    sli_type: "availability",
    objective: 99.9,
    window: "30-day",
    availability: 99.94,
    status: "healthy"
  },
  {
    id: "slo-2",
    name: "Upstream Latency Target < 500ms",
    sli_type: "latency",
    objective: 95.0,
    window: "7-day",
    availability: 92.40,
    status: "burning"
  },
  {
    id: "slo-3",
    name: "Error Rate Baseline",
    sli_type: "error_rate",
    objective: 99.95,
    window: "30-day",
    availability: 99.98,
    status: "healthy"
  }
];

// v0.1 Smoke Evidence check parameters matching srapi-admin.mjs
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

// Initial state representing a NOT COMPLETE smoke test (which is crucial to report correctly!)
export const mockSmokeStatus: SmokeChecklist = {
  base_url: "http://127.0.0.1:8080",
  model: "gpt-4o-mini",
  model_exists: true,
  active_account_count: 2,
  public_https_upstream_account_count: 0, // 0 public HTTPS accounts
  usage_endpoints: ["/v1/chat/completions"],
  real_upstream_scheduler_decision_endpoints: [],
  missing_usage_endpoints: ["/v1/responses", "/v1/messages"],
  missing_real_upstream_scheduler_decision_endpoints: ["/v1/chat/completions", "/v1/responses", "/v1/messages"],
  v0_1_smoke_evidence_complete: false, // NOT COMPLETE
};

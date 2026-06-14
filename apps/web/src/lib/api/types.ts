import type { JsonObject } from '../../../../../packages/sdk/typescript/src/types.gen';
import type {
  CurrentUser,
  ProviderAccountSummary,
  SloSummary,
} from '../srapi-types';

export interface ApiRuntimeStatus {
  mode: 'live' | 'offline';
  connected: boolean;
  apiBaseUrl: string;
  checkedAt: string;
}

/**
 * Result of a password sign-in: either the user is authenticated, or the
 * account has TOTP enabled and a second factor is required to finish. The form
 * branches on `kind`.
 */
export type LoginResult =
  | { kind: 'user'; user: CurrentUser }
  | { kind: 'twoFactor'; challengeId: string; expiresAt: string };

export type LiveUser = {
  id?: string;
  email?: string;
  name?: string;
  roles?: string[];
  balance?: string;
  currency?: string;
  rpm_limit?: number | null;
  last_login_at?: string | null;
  created_at?: string;
};

export type LiveApiKey = {
  id: string;
  name: string;
  prefix: string;
  status?: string;
  created_at: string;
  allowed_models?: string[];
  group_ids?: string[];
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
};

export type LiveProviderAccount = {
  id: string;
  name: string;
  provider_id: string;
  provider?: {
    display_name?: string;
  };
  runtime_class: ProviderAccountSummary['runtime_class'];
  status: string;
  metadata?: JsonObject;
  supported_models?: string[];
  health_snapshot?: {
    latency_ms?: number;
  };
  quota_snapshot?: {
    remaining_percentage?: number;
  };
};

export type LiveUsageLog = {
  created_at: string;
  request_id: string;
  model: string;
  source_endpoint: string;
  success: boolean;
  total_tokens?: number;
  cost?: string | number;
  input_cost?: string | number;
  output_cost?: string | number;
  cache_read_cost?: string | number;
  cache_write_cost?: string | number;
  requested_model?: string;
  upstream_model?: string;
  billing_mode?: 'token' | 'per_request' | 'image';
  currency?: string;
};

export type LiveSchedulerDecision = {
  created_at: string;
  request_id: string;
  model: string;
  source_endpoint: string;
  candidate_count?: number;
  selected_account_id?: string | null;
  selected_account?: {
    name?: string;
  };
  rejected_count?: number;
  rejected_reasons?: unknown;
  scores?: unknown;
  warnings?: string[];
  logs?: string[];
};

export type LiveSlo = Partial<SloSummary> & {
  definition?: {
    id?: string;
    name?: string;
    sli_type?: string;
    objective?: number;
    window_days?: number;
    status?: string;
  };
  evaluation?: {
    objective?: number;
    error_rate?: number;
  };
};

export type LiveModel = {
  canonical_name?: string;
};

export type SiteConfig = {
  site_name: string;
  logo_url: string;
  version_label: string;
  custom_menus: JsonObject[];
  user_agreement: string;
  privacy_policy: string;
};

export type CurrentUserAttribute = {
  definition_id: number;
  key: string;
  name: string;
  data_type: 'string' | 'number' | 'boolean' | 'select';
  options?: string[];
  required?: boolean;
  value?: string;
  updated_at?: string;
};

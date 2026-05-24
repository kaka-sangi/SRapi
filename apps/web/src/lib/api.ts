import type { Auth } from '../../../../packages/sdk/typescript/src/core/auth.gen';
import { client } from '../../../../packages/sdk/typescript/src/client.gen';
import {
  login as sdkLogin,
  logout as sdkLogout,
  getCurrentUser as sdkGetCurrentUser,
  getCurrentUserUsage as sdkGetCurrentUserUsage,
  listApiKeys as sdkListApiKeys,
  createApiKey as sdkCreateApiKey,
  updateApiKey as sdkUpdateApiKey,
  listAdminAccounts as sdkListAdminAccounts,
  testAdminAccount as sdkTestAdminAccount,
  getAdminOverview as sdkGetAdminOverview,
  listAdminUsageLogs as sdkListAdminUsageLogs,
  listAdminSchedulerDecisions as sdkListAdminSchedulerDecisions,
  listAdminOpsSlos as sdkListAdminOpsSlos
} from '../../../../packages/sdk/typescript/src/index';
import type { JsonObject } from '../../../../packages/sdk/typescript/src/types.gen';
import {
  offlineSmokeStatus,
  type ApiKeySummary,
  type CurrentUser,
  type ProviderAccountSummary,
  type SchedulerDecisionSummary,
  type SloSummary,
  type SmokeChecklist,
  type UsageLogSummary,
} from './srapi-types';
import type { AdminTestResult } from '../../../../packages/sdk/typescript/src/types.gen';
import { setSessionPresenceCookie, clearSessionPresenceCookie } from './session-cookie';

export interface ApiRuntimeStatus {
  mode: 'live' | 'offline';
  connected: boolean;
  apiBaseUrl: string;
  checkedAt: string;
}

const DEFAULT_PROXY_TARGET = 'http://127.0.0.1:8080';
const HEALTH_TIMEOUT_MS = 2500;
const USER_STORAGE_KEY = 'srapi_user';
const CSRF_STORAGE_KEY = 'srapi_csrf_token';

type LiveUser = {
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

type LiveApiKey = {
  id: string;
  name: string;
  prefix: string;
  status?: string;
  created_at: string;
  allowed_models?: string[];
  group_ids?: string[];
};

type LiveProviderAccount = {
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

type LiveUsageLog = {
  created_at: string;
  request_id: string;
  model: string;
  source_endpoint: string;
  success: boolean;
  total_tokens?: number;
  cost?: string | number;
  currency?: string;
};

type LiveSchedulerDecision = {
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

type LiveSlo = Partial<SloSummary> & {
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

type LiveModel = {
  canonical_name?: string;
};

function configuredApiBaseUrl(): string {
  return (process.env.NEXT_PUBLIC_SRAPI_BASE_URL || '').replace(/\/+$/, '');
}

function buildApiUrl(path: string): string {
  const normalized = path.startsWith('/') ? path : `/${path}`;
  const configured = configuredApiBaseUrl();
  return configured ? `${configured}${normalized}` : normalized;
}

function apiBaseUrlLabel(): string {
  return configuredApiBaseUrl() || `same-origin /api proxy -> ${DEFAULT_PROXY_TARGET}`;
}

function getStoredCSRFToken(): string | undefined {
  if (typeof window === 'undefined') {
    return undefined;
  }
  return localStorage.getItem(CSRF_STORAGE_KEY) || undefined;
}

function resolveAuthToken(auth: Auth): string | undefined {
  if (auth.name === 'X-CSRF-Token') {
    return getStoredCSRFToken();
  }

  // Browser cookies are sent by fetch credentials. Do not synthesize Cookie headers.
  return undefined;
}

function configureSDKClient() {
  client.setConfig({
    baseUrl: configuredApiBaseUrl(),
    credentials: 'include',
    auth: resolveAuthToken
  });
}

configureSDKClient();

async function fetchWithTimeout(url: string, init: RequestInit = {}, timeoutMs = HEALTH_TIMEOUT_MS): Promise<Response> {
  const controller = new AbortController();
  const timeout = globalThis.setTimeout(() => controller.abort(), timeoutMs);
  try {
    return await fetch(url, {
      ...init,
      signal: controller.signal
    });
  } finally {
    globalThis.clearTimeout(timeout);
  }
}

async function fetchApiJSON<T>(path: string, init: RequestInit = {}): Promise<T> {
  const headers = new Headers(init.headers);
  const method = (init.method || 'GET').toUpperCase();

  if (init.body && !headers.has('Content-Type')) {
    headers.set('Content-Type', 'application/json');
  }

  if (!['GET', 'HEAD', 'OPTIONS'].includes(method)) {
    const csrfToken = getStoredCSRFToken();
    if (csrfToken) {
      headers.set('X-CSRF-Token', csrfToken);
    }
  }

  const response = await fetchWithTimeout(buildApiUrl(path), {
    ...init,
    method,
    headers,
    credentials: 'include'
  });

  if (!response.ok) {
    const message = await response.text();
    throw new Error(message || `Request failed with HTTP ${response.status}`);
  }

  return response.json() as Promise<T>;
}

function mapLiveUser(user: LiveUser, fallbackEmail: string): CurrentUser {
  const roles = Array.isArray(user?.roles) ? user.roles : [];
  const hasAdminRole = roles.includes('admin') || roles.includes('owner') || roles.includes('operator');

  return {
    id: user?.id,
    email: user?.email || fallbackEmail,
    name: user?.name || 'User',
    role: hasAdminRole ? 'admin' : 'user',
    balance: user?.balance || '0.00000000',
    currency: user?.currency || 'USD',
    rpm_limit: user?.rpm_limit ?? null,
    last_login_at: user?.last_login_at ?? null,
    created_at: user?.created_at
  };
}

function storeSession(user: CurrentUser, csrfToken?: string) {
  if (typeof window === 'undefined') {
    return;
  }

  localStorage.setItem(USER_STORAGE_KEY, JSON.stringify(user));
  if (csrfToken) {
    localStorage.setItem(CSRF_STORAGE_KEY, csrfToken);
  } else {
    localStorage.removeItem(CSRF_STORAGE_KEY);
  }
  // Mirror a presence flag into a cookie so `middleware.ts` can do
  // server-side redirects without a flash of unauthenticated content. The
  // cookie carries no credentials.
  setSessionPresenceCookie(user.role);
}

function clearSession() {
  if (typeof window === 'undefined') {
    return;
  }

  localStorage.removeItem(USER_STORAGE_KEY);
  localStorage.removeItem(CSRF_STORAGE_KEY);
  clearSessionPresenceCookie();
}

function isPublicHTTPSURL(url?: string) {
  if (!url) {
    return false;
  }

  try {
    const parsed = new URL(url);
    const host = parsed.hostname.toLowerCase();
    if (parsed.protocol !== 'https:') {
      return false;
    }
    const octets = host.split('.').map(Number);
    const isPrivate172 = octets.length === 4 && octets[0] === 172 && octets[1] >= 16 && octets[1] <= 31;
    const isPrivate = host === 'localhost'
      || host === '127.0.0.1'
      || host === '::1'
      || host.startsWith('10.')
      || host.startsWith('192.168.')
      || isPrivate172;

    if (isPrivate) {
      return false;
    }
    return true;
  } catch {
    return false;
  }
}

export const apiService = {
  async isBackendConnected(): Promise<boolean> {
    if (typeof window === 'undefined') {
      return false;
    }

    configureSDKClient();
    try {
      const healthURL = configuredApiBaseUrl() ? buildApiUrl('/api/v1/health') : '/srapi-health';
      const response = await fetchWithTimeout(healthURL, {
        method: 'GET',
        credentials: 'include',
        cache: 'no-store'
      });
      return response.ok;
    } catch {
      return false;
    }
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

  async shouldUseLiveAPI(): Promise<boolean> {
    return this.isBackendConnected();
  },

  async login(email: string, password: string): Promise<CurrentUser> {
    configureSDKClient();

    const connected = await this.isBackendConnected();
    if (!connected) {
      throw new Error('SRapi API is offline. Start the backend and try again.');
    }

    const response = await sdkLogin({
      body: { email, password },
      throwOnError: true
    });
    const session = response.data?.data;
    if (!session?.user) {
      throw new Error('Authentication rejected. Verify email and password.');
    }

    const mappedUser = mapLiveUser(session.user, email);
    storeSession(mappedUser, session.csrf_token);
    return mappedUser;
  },

  async logout(): Promise<void> {
    configureSDKClient();

    const currentUser = this.getCurrentUser();
    if (currentUser && await this.isBackendConnected()) {
      try {
        await sdkLogout({ throwOnError: true });
      } catch (err) {
        console.warn('Real API logout failed', err);
      }
    }

    clearSession();
  },

  getCurrentUser(): CurrentUser | null {
    if (typeof window === 'undefined') {
      return null;
    }

    const stored = localStorage.getItem(USER_STORAGE_KEY);
    if (!stored) {
      return null;
    }

    try {
      return JSON.parse(stored) as CurrentUser;
    } catch {
      return null;
    }
  },

  async getLiveCurrentUser(): Promise<CurrentUser> {
    configureSDKClient();
    const response = await sdkGetCurrentUser({ throwOnError: true });
    const user = response.data?.data;
    if (!user) {
      throw new Error('Current user response was empty.');
    }

    const mappedUser = mapLiveUser(user, user.email);
    storeSession(mappedUser, getStoredCSRFToken());
    return mappedUser;
  },

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
        group_ids: key.group_ids || []
      }));
    }
    return [];
  },

  async createApiKey(name: string, allowedModels: string[], groupIds: string[]): Promise<ApiKeySummary> {
    const response = await sdkCreateApiKey({
      body: {
        name,
        allowed_models: allowedModels,
        group_ids: groupIds,
        scopes: ['gateway:invoke']
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
        group_ids: key.group_ids || groupIds
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
        group_ids: key.group_ids || []
      };
    }
    throw new Error('API key update returned an empty response.');
  },

  async listProviderAccounts(): Promise<ProviderAccountSummary[]> {
    const response = await sdkListAdminAccounts({ throwOnError: true });
    if (response.data) {
      return ((response.data.data || []) as LiveProviderAccount[]).map((account) => ({
        id: account.id,
        name: account.name,
        provider_id: account.provider_id,
        provider_name: account.provider?.display_name || account.provider_id,
        runtime_class: account.runtime_class,
        status: account.status as 'active' | 'limited' | 'disabled',
        base_url: typeof account.metadata?.base_url === 'string' ? account.metadata.base_url : 'not configured',
        supported_models: account.supported_models || [],
        latency: account.health_snapshot?.latency_ms || 0,
        quota_percentage: account.quota_snapshot?.remaining_percentage || 0
      }));
    }
    return [];
  },

  async testProviderAccount(id: string): Promise<AdminTestResult> {
    const response = await sdkTestAdminAccount({
      path: { id },
      throwOnError: true,
    });
    if (response.data?.data) {
      return response.data.data;
    }
    throw new Error('Provider account test returned an empty response.');
  },

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
        currency: log.currency || 'USD'
      }));
    }
    return [];
  },

  async listSchedulerDecisions(): Promise<SchedulerDecisionSummary[]> {
    const response = await sdkListAdminSchedulerDecisions({ throwOnError: true });
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

  async getOverviewStats(): Promise<{ providers: number; models: number; accounts: number; usage_logs: number; decisions: number }> {
    const response = await sdkGetAdminOverview({ throwOnError: true });
    if (response.data) {
      const data = response.data.data;
      return {
        providers: data.provider_count || 0,
        models: data.model_count || 0,
        accounts: data.account_count || 0,
        usage_logs: data.usage_log_count || 0,
        decisions: data.scheduler_decision_count || 0
      };
    }
    throw new Error('Admin overview returned an empty response.');
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

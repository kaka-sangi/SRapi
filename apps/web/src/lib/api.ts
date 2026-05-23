import type { Auth } from '../../../../packages/sdk/typescript/src/core/auth.gen';
import { client } from '../../../../packages/sdk/typescript/src/client.gen';
import {
  login as sdkLogin,
  logout as sdkLogout,
  listApiKeys as sdkListApiKeys,
  createApiKey as sdkCreateApiKey,
  updateApiKey as sdkUpdateApiKey,
  listAdminAccounts as sdkListAdminAccounts,
  getAdminOverview as sdkGetAdminOverview,
  listAdminUsageLogs as sdkListAdminUsageLogs,
  listAdminSchedulerDecisions as sdkListAdminSchedulerDecisions,
  listAdminOpsSlos as sdkListAdminOpsSlos
} from '../../../../packages/sdk/typescript/src/index';
import type { JsonObject } from '../../../../packages/sdk/typescript/src/types.gen';
import {
  mockUsers,
  initialApiKeys,
  mockProviderAccounts,
  mockUsageLogs,
  mockSchedulerDecisions,
  mockSlos,
  mockSmokeStatus,
  MockUser,
  MockApiKey,
  MockProviderAccount,
  MockUsageLog,
  MockSchedulerDecision,
  MockSlo,
  SmokeChecklist
} from './mockData';

export interface ApiRuntimeStatus {
  mode: 'live' | 'demo';
  connected: boolean;
  apiBaseUrl: string;
  demoModeForced: boolean;
  checkedAt: string;
}

const DEFAULT_PROXY_TARGET = 'http://127.0.0.1:8080';
const HEALTH_TIMEOUT_MS = 2500;
const USER_STORAGE_KEY = 'srapi_user';
const CSRF_STORAGE_KEY = 'srapi_csrf_token';

const sessionCreatedKeys: MockApiKey[] = [];

type LiveUser = {
  email?: string;
  name?: string;
  roles?: string[];
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
  runtime_class: MockProviderAccount['runtime_class'];
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

type LiveSlo = Partial<MockSlo> & {
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

function demoModeForced(): boolean {
  return process.env.NEXT_PUBLIC_SRAPI_DEMO_MODE === 'true';
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

function mapLiveUser(user: LiveUser, fallbackEmail: string, authMode: MockUser['authMode']): MockUser {
  const roles = Array.isArray(user?.roles) ? user.roles : [];
  const hasAdminRole = roles.includes('admin') || roles.includes('owner') || roles.includes('operator');

  return {
    email: user?.email || fallbackEmail,
    name: user?.name || 'User',
    role: hasAdminRole ? 'admin' : 'user',
    balance: '100.00',
    currency: 'USD',
    authMode
  };
}

function mockLogin(email: string): MockUser {
  const role = email.includes('admin') ? 'admin' : 'user';
  return {
    ...mockUsers[role],
    authMode: 'demo'
  };
}

function storeSession(user: MockUser, csrfToken?: string) {
  if (typeof window === 'undefined') {
    return;
  }

  localStorage.setItem(USER_STORAGE_KEY, JSON.stringify(user));
  if (csrfToken) {
    localStorage.setItem(CSRF_STORAGE_KEY, csrfToken);
  } else {
    localStorage.removeItem(CSRF_STORAGE_KEY);
  }
}

function clearSession() {
  if (typeof window === 'undefined') {
    return;
  }

  localStorage.removeItem(USER_STORAGE_KEY);
  localStorage.removeItem(CSRF_STORAGE_KEY);
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
    const currentUser = this.getCurrentUser();
    const mode = connected && currentUser?.authMode !== 'demo' && !demoModeForced() ? 'live' : 'demo';

    return {
      mode,
      connected,
      apiBaseUrl: apiBaseUrlLabel(),
      demoModeForced: demoModeForced(),
      checkedAt: new Date().toISOString()
    };
  },

  async shouldUseLiveAPI(): Promise<boolean> {
    if (demoModeForced()) {
      return false;
    }

    const currentUser = this.getCurrentUser();
    if (currentUser?.authMode === 'demo') {
      return false;
    }

    return this.isBackendConnected();
  },

  async login(email: string, password: string): Promise<MockUser> {
    configureSDKClient();

    const connected = await this.isBackendConnected();
    if (connected && !demoModeForced()) {
      const response = await sdkLogin({
        body: { email, password },
        throwOnError: true
      });
      const session = response.data?.data;
      if (!session?.user) {
        throw new Error('Authentication rejected. Verify email and password.');
      }

      const mappedUser = mapLiveUser(session.user, email, 'live');
      storeSession(mappedUser, session.csrf_token);
      return mappedUser;
    }

    const user = mockLogin(email);
    storeSession(user);
    return user;
  },

  async logout(): Promise<void> {
    configureSDKClient();

    const currentUser = this.getCurrentUser();
    if (currentUser?.authMode === 'live' && await this.isBackendConnected()) {
      try {
        await sdkLogout({ throwOnError: true });
      } catch (err) {
        console.warn('Real API logout failed', err);
      }
    }

    clearSession();
  },

  getCurrentUser(): MockUser | null {
    if (typeof window === 'undefined') {
      return null;
    }

    const stored = localStorage.getItem(USER_STORAGE_KEY);
    if (!stored) {
      return null;
    }

    try {
      const user = JSON.parse(stored) as MockUser;
      return {
        ...user,
        authMode: user.authMode || 'demo'
      };
    } catch {
      return null;
    }
  },

  async listApiKeys(): Promise<MockApiKey[]> {
    if (await this.shouldUseLiveAPI()) {
      try {
        const response = await sdkListApiKeys();
        if (response.data) {
          const liveKeys = (response.data.data || []) as LiveApiKey[];
          const mapped: MockApiKey[] = liveKeys.map((key) => ({
            id: key.id,
            name: key.name,
            prefix: key.prefix,
            status: (key.status === 'active' ? 'active' : 'disabled') as 'active' | 'disabled',
            created_at: key.created_at,
            allowed_models: key.allowed_models || [],
            group_ids: key.group_ids || []
          }));
          return [...sessionCreatedKeys.filter(sk => !mapped.some(m => m.id === sk.id)), ...mapped];
        }
      } catch (err) {
        console.warn('Failed to fetch real API keys, using demo data', err);
      }
    }

    return [...sessionCreatedKeys, ...initialApiKeys];
  },

  async createApiKey(name: string, allowedModels: string[], groupIds: string[]): Promise<MockApiKey> {
    if (await this.shouldUseLiveAPI()) {
      try {
        const response = await sdkCreateApiKey({
          body: {
            name,
            allowed_models: allowedModels,
            group_ids: groupIds,
            scopes: ['gateway:invoke']
          }
        });
        if (response.data?.data) {
          const key = response.data.data.api_key;
          const plaintext = response.data.data.plaintext_key;
          const newKey: MockApiKey = {
            id: key.id,
            name: key.name,
            prefix: key.prefix,
            plaintextKey: plaintext,
            status: key.status === 'active' ? 'active' : 'disabled',
            created_at: key.created_at || new Date().toISOString(),
            allowed_models: key.allowed_models || allowedModels,
            group_ids: key.group_ids || groupIds
          };
          sessionCreatedKeys.unshift(newKey);
          return newKey;
        }
      } catch (err) {
        console.warn('Failed to create real API key, using demo creation', err);
      }
    }

    const mockId = `key-${Math.floor(Math.random() * 1000)}`;
    const randomHex = Array.from({ length: 32 }, () => Math.floor(Math.random() * 16).toString(16)).join('');
    const newMockKey: MockApiKey = {
      id: mockId,
      name,
      prefix: `sk-srapi-${mockId}...`,
      plaintextKey: `sk-srapi-${mockId}-${randomHex}`,
      status: 'active',
      created_at: new Date().toISOString(),
      allowed_models: allowedModels.length > 0 ? allowedModels : ['gpt-4o-mini'],
      group_ids: groupIds.length > 0 ? groupIds : ['group-01']
    };
    sessionCreatedKeys.unshift(newMockKey);
    return newMockKey;
  },

  async toggleApiKeyStatus(id: string, currentStatus: 'active' | 'disabled'): Promise<MockApiKey | null> {
    const nextStatus = currentStatus === 'active' ? 'disabled' : 'active';

    if (await this.shouldUseLiveAPI()) {
      try {
        const response = await sdkUpdateApiKey({
          path: { id },
          body: { status: nextStatus }
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
      } catch (err) {
        console.warn('Failed to toggle real key status, updating demo data', err);
      }
    }

    const sessionKey = sessionCreatedKeys.find(k => k.id === id);
    if (sessionKey) {
      sessionKey.status = nextStatus;
      return sessionKey;
    }

    const initialKey = initialApiKeys.find(k => k.id === id);
    if (initialKey) {
      initialKey.status = nextStatus;
      return initialKey;
    }

    return null;
  },

  async listProviderAccounts(): Promise<MockProviderAccount[]> {
    if (await this.shouldUseLiveAPI()) {
      try {
        const response = await sdkListAdminAccounts();
        if (response.data) {
          return ((response.data.data || []) as LiveProviderAccount[]).map((account) => ({
            id: account.id,
            name: account.name,
            provider_id: account.provider_id,
            provider_name: account.provider?.display_name || account.provider_id,
            runtime_class: account.runtime_class,
            status: account.status as 'active' | 'limited' | 'disabled',
            base_url: typeof account.metadata?.base_url === 'string' ? account.metadata.base_url : 'https://api.openai.com/v1',
            supported_models: account.supported_models || [],
            latency: account.health_snapshot?.latency_ms || 150,
            quota_percentage: account.quota_snapshot?.remaining_percentage || 80
          }));
        }
      } catch (err) {
        console.warn('Failed to fetch real accounts, using demo data', err);
      }
    }

    return mockProviderAccounts;
  },

  async listUsageLogs(): Promise<MockUsageLog[]> {
    if (await this.shouldUseLiveAPI()) {
      try {
        const response = await sdkListAdminUsageLogs();
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
      } catch (err) {
        console.warn('Failed to fetch real usage logs, using demo data', err);
      }
    }

    return mockUsageLogs;
  },

  async listSchedulerDecisions(): Promise<MockSchedulerDecision[]> {
    if (await this.shouldUseLiveAPI()) {
      try {
        const response = await sdkListAdminSchedulerDecisions();
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
      } catch (err) {
        console.warn('Failed to fetch real scheduler decisions, using demo data', err);
      }
    }

    return mockSchedulerDecisions;
  },

  async listSlos(): Promise<MockSlo[]> {
    if (await this.shouldUseLiveAPI()) {
      try {
        const response = await sdkListAdminOpsSlos();
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
      } catch (err) {
        console.warn('Failed to fetch real SLOs, using demo data', err);
      }
    }

    return mockSlos;
  },

  async getOverviewStats(): Promise<{ providers: number; models: number; accounts: number; usage_logs: number; decisions: number }> {
    if (await this.shouldUseLiveAPI()) {
      try {
        const response = await sdkGetAdminOverview();
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
      } catch (err) {
        console.warn('Failed to get real overview, using demo data', err);
      }
    }

    return {
      providers: 4,
      models: 8,
      accounts: mockProviderAccounts.length,
      usage_logs: 182,
      decisions: 154
    };
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
          v0_1_smoke_evidence_complete: modelExists
            && realUpstreamAccounts.length > 0
            && missingUsage.length === 0
            && missingRealDecisions.length === 0
        };
      } catch (err) {
        console.warn('Failed to compile live smoke status, returning demo status', err);
      }
    }

    return {
      ...mockSmokeStatus,
      base_url: apiBaseUrlLabel()
    };
  }
};

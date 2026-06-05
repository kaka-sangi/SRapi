import type { Auth } from '../../../../packages/sdk/typescript/src/core/auth.gen';
import { client } from '../../../../packages/sdk/typescript/src/client.gen';
import {
  login as sdkLogin,
  loginTwoFactor as sdkLoginTwoFactor,
  listEnabledOAuthProviders as sdkListOAuthProviders,
  getPendingOAuthSession as sdkGetOAuthPending,
  bindPendingOAuthLogin as sdkBindOAuthPendingLogin,
  completePendingOAuthBindLoginTwoFactor as sdkBindOAuthLoginTwoFactor,
  createPendingOAuthAccount as sdkCreateOAuthAccount,
  sendPendingOAuthEmailCompletion as sdkSendOAuthEmailCode,
  confirmPendingOAuthEmailCompletion as sdkConfirmOAuthEmail,
  logout as sdkLogout,
  getSetupStatus as sdkGetSetupStatus,
  completeSetup as sdkCompleteSetup,
  requestPasswordReset as sdkRequestPasswordReset,
  confirmPasswordReset as sdkConfirmPasswordReset,
  register as sdkRegister,
  getAuthCaptchaConfig as sdkGetAuthCaptchaConfig,
  getCurrentUser as sdkGetCurrentUser,
  getCurrentUserUsage as sdkGetCurrentUserUsage,
  listApiKeys as sdkListApiKeys,
  createApiKey as sdkCreateApiKey,
  updateApiKey as sdkUpdateApiKey,
  getApiKeyUsage as sdkGetApiKeyUsage,
  listAdminAccounts as sdkListAdminAccounts,
  testAdminAccount as sdkTestAdminAccount,
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
import type { AdminTestResult, EnabledOAuthProvider, OAuthPendingSession, GatewayUsageResponse } from '../../../../packages/sdk/typescript/src/types.gen';
import { setSessionPresenceCookie, clearSessionPresenceCookie } from './session-cookie';

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
  allowed_ips?: string[];
  denied_ips?: string[];
  request_limit_5h?: number | null;
  request_limit_1d?: number | null;
  request_limit_7d?: number | null;
  rpm_limit?: number | null;
  tpm_limit?: number | null;
  concurrency_limit?: number | null;
  expires_at?: string | null;
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

  async getSetupStatus(): Promise<boolean> {
    configureSDKClient();
    const response = await sdkGetSetupStatus({ throwOnError: true });
    return response.data?.data?.needs_setup ?? false;
  },

  async completeSetup(input: { email: string; name: string; password: string }): Promise<void> {
    configureSDKClient();
    await sdkCompleteSetup({ body: input, throwOnError: true });
  },

  // Public password-reset flow. The request endpoint reports success regardless
  // of whether the email is registered (no account enumeration); the reset
  // token is delivered by email and confirmed on /auth/reset?token=…
  async requestPasswordReset(email: string): Promise<void> {
    configureSDKClient();
    await sdkRequestPasswordReset({ body: { email }, throwOnError: true });
  },

  async confirmPasswordReset(token: string, newPassword: string): Promise<void> {
    configureSDKClient();
    await sdkConfirmPasswordReset({ body: { token, new_password: newPassword }, throwOnError: true });
  },

  // Public captcha config (provider + non-secret site key) for the auth-page
  // widget. Returns undefined on failure → the form treats captcha as off.
  async getCaptchaConfig() {
    configureSDKClient();
    const response = await sdkGetAuthCaptchaConfig({ throwOnError: true });
    return response.data?.data;
  },

  // Public self-service sign-up. Gated server-side by Security.RegistrationEnabled
  // (403 "registration disabled" when off). On success the backend returns a
  // session, mirroring login, so the new account is signed in immediately.
  async register(email: string, name: string, password: string, captchaToken?: string) {
    configureSDKClient();
    const response = await sdkRegister({
      body: { email, name, password },
      headers: captchaToken ? { 'X-Captcha-Token': captchaToken } : undefined,
      throwOnError: true,
    });
    const session = response.data?.data;
    if (!session?.user) {
      throw new Error('Registration did not return a session.');
    }
    const mappedUser = mapLiveUser(session.user, email);
    storeSession(mappedUser, session.csrf_token);
    return mappedUser;
  },

  async login(email: string, password: string, captchaToken?: string): Promise<LoginResult> {
    configureSDKClient();

    const connected = await this.isBackendConnected();
    if (!connected) {
      throw new Error('SRapi API is offline. Start the backend and try again.');
    }

    const response = await sdkLogin({
      body: { email, password },
      headers: captchaToken ? { 'X-Captcha-Token': captchaToken } : undefined,
      throwOnError: true
    });
    const session = response.data?.data;
    if (session && 'required' in session) {
      // 202: password accepted, TOTP required. Hand the challenge back so the
      // form can collect a code and call verifyLoginTwoFactor.
      return { kind: 'twoFactor', challengeId: session.challenge_id, expiresAt: session.expires_at };
    }
    if (!session?.user) {
      throw new Error('Authentication rejected. Verify email and password.');
    }

    const mappedUser = mapLiveUser(session.user, email);
    storeSession(mappedUser, session.csrf_token);
    return { kind: 'user', user: mappedUser };
  },

  // Completes a sign-in that returned a two-factor challenge.
  async verifyLoginTwoFactor(challengeId: string, code: string): Promise<CurrentUser> {
    configureSDKClient();
    const response = await sdkLoginTwoFactor({
      body: { challenge_id: challengeId, code },
      throwOnError: true
    });
    const session = response.data?.data;
    if (!session?.user) {
      throw new Error('Two-factor verification failed.');
    }
    const mappedUser = mapLiveUser(session.user, session.user.email);
    storeSession(mappedUser, session.csrf_token);
    return mappedUser;
  },

  // Public: which OAuth/OIDC providers the sign-in page should offer. Degrades
  // to an empty list (no buttons) if the endpoint is unreachable.
  async listOAuthProviders(): Promise<EnabledOAuthProvider[]> {
    configureSDKClient();
    try {
      const response = await sdkListOAuthProviders({ throwOnError: true });
      return response.data?.data ?? [];
    } catch {
      return [];
    }
  },

  // ---- OAuth pending-session flow (post-callback) ----
  // Inspects the short-lived pending session the callback created. Throws if it
  // is missing/expired so the page can send the user back to sign in.
  async getOAuthPending(): Promise<OAuthPendingSession> {
    configureSDKClient();
    const response = await sdkGetOAuthPending({ throwOnError: true });
    const data = response.data?.data;
    if (!data) throw new Error('No pending OAuth session.');
    return data;
  },

  // Authenticates an existing local account by email+password and binds the
  // pending provider identity to it, then logs in (or asks for a TOTP code).
  async bindOAuthPendingLogin(
    email: string,
    password: string,
    adoptDisplayName?: boolean,
  ): Promise<LoginResult> {
    configureSDKClient();
    const response = await sdkBindOAuthPendingLogin({
      body: { email, password, adopt_display_name: adoptDisplayName },
      throwOnError: true
    });
    const session = response.data?.data;
    if (session && 'required' in session) {
      return { kind: 'twoFactor', challengeId: session.challenge_id, expiresAt: session.expires_at };
    }
    if (!session?.user) throw new Error('OAuth sign-in failed.');
    const mappedUser = mapLiveUser(session.user, session.user.email);
    storeSession(mappedUser, session.csrf_token);
    return { kind: 'user', user: mappedUser };
  },

  async verifyOAuthBindLoginTwoFactor(challengeId: string, code: string): Promise<CurrentUser> {
    configureSDKClient();
    const response = await sdkBindOAuthLoginTwoFactor({
      body: { challenge_id: challengeId, code },
      throwOnError: true
    });
    const session = response.data?.data;
    if (!session?.user) throw new Error('Two-factor verification failed.');
    const mappedUser = mapLiveUser(session.user, session.user.email);
    storeSession(mappedUser, session.csrf_token);
    return mappedUser;
  },

  // Creates a new local account from the provider identity and logs in. The
  // action token comes from the pending session's create_account_action.
  async createOAuthPendingAccount(
    email: string,
    password: string,
    actionToken: string,
    name?: string,
  ): Promise<CurrentUser> {
    configureSDKClient();
    const response = await sdkCreateOAuthAccount({
      body: { email, password, action_token: actionToken, name },
      throwOnError: true
    });
    const session = response.data?.data;
    if (!session?.user) throw new Error('Account creation failed.');
    const mappedUser = mapLiveUser(session.user, email);
    storeSession(mappedUser, session.csrf_token);
    return mappedUser;
  },

  // Sends an email-verification token when the provider did not supply an email.
  async sendOAuthEmailCode(email: string): Promise<void> {
    configureSDKClient();
    await sdkSendOAuthEmailCode({ body: { email }, throwOnError: true });
  },

  // Confirms the email-verification token; returns the refreshed pending session.
  async confirmOAuthEmailCompletion(token: string): Promise<OAuthPendingSession> {
    configureSDKClient();
    const response = await sdkConfirmOAuthEmail({ body: { token }, throwOnError: true });
    const data = response.data?.data;
    if (!data) throw new Error('Email verification failed.');
    return data;
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
        group_ids: key.group_ids || [],
        allowed_ips: key.allowed_ips || [],
        denied_ips: key.denied_ips || [],
        request_limit_5h: key.request_limit_5h ?? null,
        request_limit_1d: key.request_limit_1d ?? null,
        request_limit_7d: key.request_limit_7d ?? null,
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
        ...(policy.expiresAt ? { expires_at: policy.expiresAt } : {})
      },
      throwOnError: true
    });
  },

  async getApiKeyUsage(id: string, days: number): Promise<GatewayUsageResponse> {
    const response = await sdkGetApiKeyUsage({ path: { id }, query: { days }, throwOnError: true });
    if (!response.data) {
      throw new Error('API key usage returned an empty response.');
    }
    return response.data;
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

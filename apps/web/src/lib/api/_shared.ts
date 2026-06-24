import {
  CSRF_STORAGE_KEY,
  configureSdkClient,
  configuredApiBaseUrl,
  getStoredCSRFToken,
} from '../sdk-client';
import type { CurrentUser } from '../srapi-types';
import { setSessionPresenceCookie, clearSessionPresenceCookie } from '../session-cookie';
import type { LiveUser } from './types';

const DEFAULT_PROXY_TARGET = 'http://127.0.0.1:8080';
const HEALTH_TIMEOUT_MS = 2500;
const USER_STORAGE_KEY = 'srapi_user';

// Pick up a CSRF token from the URL (set by OAuth redirect-based login)
// and store it in localStorage so subsequent API calls include it.
if (typeof window !== "undefined") {
  const params = new URLSearchParams(window.location.search);
  const oauthCSRF = params.get("_csrf");
  if (oauthCSRF) {
    localStorage.setItem(CSRF_STORAGE_KEY, oauthCSRF);
    params.delete("_csrf");
    const clean = params.toString();
    window.history.replaceState({}, "", window.location.pathname + (clean ? "?" + clean : ""));
  }
}

export function parseMoneyValue(value: string | number | undefined): number {
  if (typeof value === 'number') return value;
  return parseFloat(value || '0');
}

function buildApiUrl(path: string): string {
  const normalized = path.startsWith('/') ? path : `/${path}`;
  const configured = configuredApiBaseUrl();
  return configured ? `${configured}${normalized}` : normalized;
}

export function apiBaseUrlLabel(): string {
  return configuredApiBaseUrl() || `same-origin /api proxy -> ${DEFAULT_PROXY_TARGET}`;
}

// SDK-client setup (base URL, cookie credentials, CSRF auth) is shared across
// the functional clients; see ../sdk-client. Kept under the original local name
// so the many call sites below stay untouched.
export const configureSDKClient = configureSdkClient;

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

export async function fetchApiJSON<T>(path: string, init: RequestInit = {}): Promise<T> {
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
    headers.set('X-CSRF-Rotation', 'supported');
  }

  const response = await fetchWithTimeout(buildApiUrl(path), {
    ...init,
    method,
    headers,
    credentials: 'include'
  });

  const rotatedCSRF = response.headers.get('X-CSRF-Token-Rotated');
  if (rotatedCSRF) {
    localStorage.setItem(CSRF_STORAGE_KEY, rotatedCSRF);
  }

  if (!response.ok) {
    const message = await response.text();
    throw new Error(message || `Request failed with HTTP ${response.status}`);
  }

  return response.json() as Promise<T>;
}

export function mapLiveUser(user: LiveUser, fallbackEmail: string): CurrentUser {
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

export function storeSession(user: CurrentUser, csrfToken?: string) {
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

export function clearSession() {
  if (typeof window === 'undefined') {
    return;
  }

  localStorage.removeItem(USER_STORAGE_KEY);
  localStorage.removeItem(CSRF_STORAGE_KEY);
  clearSessionPresenceCookie();
}

// Core backend-connectivity / session reads. Several methods on the composed
// `apiService` call each other via `this` (e.g. login -> isBackendConnected,
// getSmokeStatus -> shouldUseLiveAPI). To keep those cross-cutting calls
// behavior-identical across the domain split, the implementations live here and
// the object methods delegate to them.
export async function isBackendConnected(): Promise<boolean> {
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
}

export async function shouldUseLiveAPI(): Promise<boolean> {
  return isBackendConnected();
}

export function getCurrentUser(): CurrentUser | null {
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
}

export function isPublicHTTPSURL(url?: string) {
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

// Re-exported so domain modules share the CSRF accessor without re-importing
// from ../sdk-client (auth.ts uses it in getLiveCurrentUser).
export { getStoredCSRFToken };

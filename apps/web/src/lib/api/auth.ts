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
  requestPasswordlessLogin as sdkRequestPasswordlessLogin,
  completePasswordlessLogin as sdkCompletePasswordlessLogin,
  getAuthCaptchaConfig as sdkGetAuthCaptchaConfig,
  getCurrentUser as sdkGetCurrentUser,
} from '../../../../../packages/sdk/typescript/src/index';
import type { EnabledOAuthProvider, OAuthPendingSession } from '../../../../../packages/sdk/typescript/src/types.gen';
import type { CurrentUser } from '../srapi-types';
import {
  clearSession,
  configureSDKClient,
  fetchApiJSON,
  getCurrentUser as getCurrentUserFn,
  getStoredCSRFToken,
  isBackendConnected as isBackendConnectedFn,
  mapLiveUser,
  storeSession,
} from './_shared';
import type { LiveUser, LoginResult } from './types';

export const authApi = {
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
  async register(
    email: string,
    name: string,
    password: string,
    captchaToken?: string,
    attributes?: Array<{ definition_id: number; value: string }>,
  ) {
    const response = await fetchApiJSON<{
      data?: { user?: LiveUser; csrf_token?: string; expires_at?: string };
    }>('/api/v1/auth/register', {
      method: 'POST',
      headers: captchaToken ? { 'X-Captcha-Token': captchaToken } : undefined,
      body: JSON.stringify({ email, name, password, attributes: attributes || [] }),
    });
    const session = response.data;
    if (!session?.user) {
      throw new Error('Registration did not return a session.');
    }
    const mappedUser = mapLiveUser(session.user, email);
    storeSession(mappedUser, session.csrf_token);
    return mappedUser;
  },

  async requestPasswordlessCode(
    email: string,
    name?: string,
    attributes?: Array<{ definition_id: number; value: string }>,
    captchaToken?: string,
  ): Promise<void> {
    configureSDKClient();
    await sdkRequestPasswordlessLogin({
      body: { email, ...(name ? { name } : {}), attributes: attributes || [] },
      headers: captchaToken ? { 'X-Captcha-Token': captchaToken } : undefined,
      throwOnError: true,
    });
  },

  async passwordlessLogin(token: string): Promise<CurrentUser> {
    configureSDKClient();
    const response = await sdkCompletePasswordlessLogin({ body: { token }, throwOnError: true });
    const session = response.data?.data;
    if (!session?.user) {
      throw new Error('Passwordless sign-in failed.');
    }
    const mappedUser = mapLiveUser(session.user, session.user.email || '');
    storeSession(mappedUser, session.csrf_token);
    return mappedUser;
  },

  async login(email: string, password: string, captchaToken?: string): Promise<LoginResult> {
    configureSDKClient();

    const connected = await isBackendConnectedFn();
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

    const currentUser = getCurrentUserFn();
    if (currentUser && await isBackendConnectedFn()) {
      try {
        await sdkLogout({ throwOnError: true });
      } catch (err) {
        console.warn('Real API logout failed', err);
      }
    }

    clearSession();
  },

  getCurrentUser(): CurrentUser | null {
    return getCurrentUserFn();
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
};

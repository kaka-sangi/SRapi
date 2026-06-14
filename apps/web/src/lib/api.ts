// Public entry for the legacy session/auth + summary-mapping service.
//
// This file was decomposed from a single ~1000-line module into per-domain
// modules under ./api/. The public surface is unchanged: the same `apiService`
// object (same method names/signatures) and the same exported types are still
// importable from "@/lib/api".
//
// IMPORTANT — module-level side effects:
// `./api/_shared` performs the eager `configureSDKClient()` call (base URL,
// cookie credentials, CSRF auth) and owns the SDK-gap raw-fetch helpers plus
// the session-cookie side effects (storeSession / clearSession). Importing it
// first here preserves the original top-level side-effect ordering: the SDK
// client is configured exactly once, before the `apiService` object is built.
import './api/_shared';

import { accountApi } from './api/account';
import { authApi } from './api/auth';
import { keysApi } from './api/keys';
import { statusApi } from './api/status';
import { usageApi } from './api/usage';

// Public type surface — re-exported so every type stays importable from
// "@/lib/api" exactly as before the decomposition.
export type {
  ApiRuntimeStatus,
  LoginResult,
  SiteConfig,
  CurrentUserAttribute,
} from './api/types';

/**
 * Legacy session/auth + summary-mapping service.
 *
 * Domain reads/writes for the signed-in user (`/me`) and admin live in the
 * functional SDK clients (`meApi` in ./me-api, `adminApi` in ./admin-api).
 * What remains here is the work those SDK wrappers deliberately do NOT do:
 *  - session lifecycle: login / 2FA / OAuth-pending / setup / register /
 *    logout, with localStorage + presence-cookie side effects (storeSession /
 *    clearSession / mapLiveUser);
 *  - response mapping into the page-facing summary types in ./srapi-types
 *    (api keys, usage logs, scheduler decisions, SLOs, available models);
 *  - composite/diagnostic reads (runtime status, smoke checklist);
 *  - SDK-gap raw fetches (deleteApiKey, passwordless, site-config and
 *    registration/me attributes) marked with `// TODO(sdk-gap)`.
 *
 * None of these has a 1:1 SDK function already exposed by meApi/adminApi, so
 * there is nothing safe to migrate without changing behavior or adding new SDK
 * surface. Prefer meApi/adminApi for any genuinely 1:1 call going forward.
 *
 * Composed from per-domain method groups under ./api. Every method name and
 * signature is preserved 1:1; methods that call siblings via `this`
 * (login/logout -> isBackendConnected/getCurrentUser, getSmokeStatus ->
 * shouldUseLiveAPI) resolve against this single composed object exactly as
 * before, so the public surface is byte-identical to the previous monolith.
 */
export const apiService = {
  ...statusApi,
  ...accountApi,
  ...authApi,
  ...keysApi,
  ...usageApi,
};

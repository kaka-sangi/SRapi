/**
 * SRapi v0.1.0 client-side session cookie shim.
 *
 * The control panel keeps a minimal user shell in `localStorage`, but Next.js
 * `proxy.ts` runs on the edge and only sees cookies. This module mirrors a
 * tiny presence flag into a non-HTTP-only cookie so the proxy can do
 * server-side redirects without a flash of unauthenticated content.
 *
 * The cookie carries no credentials. Real auth still depends on the backend's
 * `srapi_session` cookie; this shim only signals "the browser
 * believes it has a session" so the server can route accordingly.
 *
 * Setting / clearing also dispatches a `srapi:user-change` event so
 * same-tab `useSyncExternalStore` subscribers in `auth-gate.tsx` invalidate
 * their cache and pick up the new user without a full reload.
 */
export const SESSION_PRESENT_COOKIE = "srapi_session_present";
export const SESSION_ROLE_COOKIE = "srapi_session_role";
const COOKIE_MAX_AGE_SECONDS = 60 * 60 * 24 * 30; // 30 days
const USER_EVENT = "srapi:user-change";

function dispatchUserChange(): void {
  if (typeof window === "undefined") return;
  window.dispatchEvent(new Event(USER_EVENT));
}

export function setSessionPresenceCookie(role: "admin" | "user"): void {
  if (typeof document === "undefined") return;
  const attrs = `; Path=/; Max-Age=${COOKIE_MAX_AGE_SECONDS}; SameSite=Lax`;
  document.cookie = `${SESSION_PRESENT_COOKIE}=1${attrs}`;
  document.cookie = `${SESSION_ROLE_COOKIE}=${role}${attrs}`;
  dispatchUserChange();
}

export function clearSessionPresenceCookie(): void {
  if (typeof document === "undefined") return;
  const attrs = `; Path=/; Max-Age=0; SameSite=Lax`;
  document.cookie = `${SESSION_PRESENT_COOKIE}=${attrs}`;
  document.cookie = `${SESSION_ROLE_COOKIE}=${attrs}`;
  dispatchUserChange();
}

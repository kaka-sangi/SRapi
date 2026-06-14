"use client";

import { createContext, useContext, useEffect, useSyncExternalStore } from "react";
import { useRouter } from "next/navigation";
import { apiService } from "@/lib/api";
import { clearSession } from "@/lib/api/_shared";
import type { CurrentUser } from "@/lib/srapi-types";
import { SIGN_IN_ROUTE, USER_HOME_ROUTE } from "@/lib/routes";
import { Spinner } from "@/components/ui/spinner";

const USER_STORAGE_KEY = "srapi_user";
const USER_EVENT = "srapi:user-change";

function subscribe(callback: () => void): () => void {
  if (typeof window === "undefined") return () => {};
  window.addEventListener(USER_EVENT, callback);
  window.addEventListener("storage", callback);
  return () => {
    window.removeEventListener(USER_EVENT, callback);
    window.removeEventListener("storage", callback);
  };
}

// Return the raw JSON string so useSyncExternalStore gets a stable primitive
// snapshot (avoids React error #185 from a fresh object each render).
function getSnapshot(): string | null {
  if (typeof window === "undefined") return null;
  return window.localStorage.getItem(USER_STORAGE_KEY);
}

function getServerSnapshot(): string | null {
  return null;
}

export function useCurrentUserShell(): CurrentUser | null {
  const raw = useSyncExternalStore(subscribe, getSnapshot, getServerSnapshot);
  if (!raw) return null;
  try {
    return JSON.parse(raw) as CurrentUser;
  } catch {
    return null;
  }
}

/** Authoritative localStorage read — used in the redirect effect so a transient
 *  null snapshot during hydration never bounces an authenticated user. */
function readStoredUser(): CurrentUser | null {
  if (typeof window === "undefined") return null;
  const raw = window.localStorage.getItem(USER_STORAGE_KEY);
  if (!raw) return null;
  try {
    return JSON.parse(raw) as CurrentUser;
  } catch {
    return null;
  }
}

/** True when a live-session error is the backend explicitly rejecting the
 *  session (expired / insufficient role) rather than a transient network or 5xx
 *  failure. Only the former should log the user out — an outage must not. */
function isSessionRejection(err: unknown): boolean {
  const code = (err as { error?: { code?: string } } | null)?.error?.code;
  return code === "UNAUTHORIZED" || code === "FORBIDDEN";
}

const CurrentUserContext = createContext<CurrentUser | null>(null);

export function useAuthUser(): CurrentUser {
  const user = useContext(CurrentUserContext);
  if (!user) throw new Error("useAuthUser must be used within an authenticated AuthGate");
  return user;
}

/**
 * Client-side auth guard. Mirrors `proxy.ts` (edge) but also catches the case
 * where the presence cookie and localStorage drift apart in the same tab.
 */
export function AuthGate({
  allowedRole,
  children,
}: {
  allowedRole?: "admin" | "user";
  children: React.ReactNode;
}) {
  const router = useRouter();
  const user = useCurrentUserShell();

  useEffect(() => {
    // Read localStorage authoritatively rather than the synced snapshot: on a
    // hard navigation/refresh the server snapshot is null on the first client
    // commit even when a session exists, so trusting `user` here would bounce an
    // authenticated user to sign-in. `user` stays in the deps so a real logout
    // (localStorage cleared + event) re-runs this and redirects.
    const current = readStoredUser();
    if (!current) {
      router.replace(SIGN_IN_ROUTE);
      return;
    }
    if (allowedRole === "admin" && current.role !== "admin") {
      router.replace(USER_HOME_ROUTE);
    }
  }, [user, allowedRole, router]);

  useEffect(() => {
    // Validate the session against the backend instead of trusting localStorage
    // + the non-credential presence cookie alone. Those persist for 30 days and
    // are only cleared on an explicit logout, so after the real (HttpOnly)
    // session expires the shell would otherwise render fully "authenticated"
    // (name/email/nav) while every API call 401/403s. On an explicit auth
    // rejection, clear the stale shell and bounce to sign-in; ignore transient
    // network / 5xx errors so an outage never logs everyone out.
    if (typeof window === "undefined") return;
    let cancelled = false;
    async function validate() {
      try {
        await apiService.getLiveCurrentUser();
        if (!cancelled) window.dispatchEvent(new Event(USER_EVENT));
      } catch (err) {
        if (cancelled || !isSessionRejection(err)) return;
        clearSession();
        window.dispatchEvent(new Event(USER_EVENT));
        router.replace(SIGN_IN_ROUTE);
      }
    }
    void validate();
    window.addEventListener("focus", validate);
    return () => {
      cancelled = true;
      window.removeEventListener("focus", validate);
    };
  }, [router]);

  if (!user || (allowedRole === "admin" && user.role !== "admin")) {
    return (
      <div className="flex min-h-dvh items-center justify-center">
        <Spinner className="size-5" />
      </div>
    );
  }

  return <CurrentUserContext.Provider value={user}>{children}</CurrentUserContext.Provider>;
}

export { apiService };

"use client";

import * as React from "react";
import { useRouter } from "next/navigation";
import { apiService, type ApiRuntimeStatus } from "@/lib/api";
import { useRuntimeStatus } from "@/hooks/queries";
import { Spinner } from "@/components/ui";

type Role = "admin" | "user";
type CurrentUser = ReturnType<typeof apiService.getCurrentUser>;

interface AuthGateProps {
  allowedRole?: Role;
  children: (ctx: AuthedContext) => React.ReactNode;
  loadingLabel?: string;
}

export interface AuthedContext {
  user: NonNullable<CurrentUser>;
  runtimeStatus: ApiRuntimeStatus | null;
  refreshRuntimeStatus: () => Promise<void>;
}

const USER_STORAGE_KEY = "srapi_user";
const USER_EVENT = "srapi:user-change";

// `useSyncExternalStore` compares snapshots with Object.is, so we must
// return a stable reference when storage has not changed. `apiService.getCurrentUser`
// always returns a fresh object literal — caching by raw JSON keeps the
// reference identity steady and avoids React error #185 (max update depth).
let cachedRaw: string | null = null;
let cachedUser: CurrentUser = null;

function readCurrentUserStable(): CurrentUser {
  if (typeof window === "undefined") return null;
  const raw = window.localStorage.getItem(USER_STORAGE_KEY);
  if (raw === cachedRaw) return cachedUser;
  cachedRaw = raw;
  cachedUser = apiService.getCurrentUser();
  return cachedUser;
}

function invalidateUserCache(): void {
  cachedRaw = null;
  cachedUser = null;
}

const subscribeToUser = (notify: () => void) => {
  if (typeof window === "undefined") return () => {};
  const listener = (event: StorageEvent | Event) => {
    if (event instanceof StorageEvent && event.key && event.key !== USER_STORAGE_KEY) return;
    invalidateUserCache();
    notify();
  };
  window.addEventListener("storage", listener);
  window.addEventListener(USER_EVENT, listener);
  return () => {
    window.removeEventListener("storage", listener);
    window.removeEventListener(USER_EVENT, listener);
  };
};

const getUserSnapshot = (): CurrentUser => readCurrentUserStable();
const getServerUserSnapshot = (): CurrentUser => null;

// Stable "is hydrated" signal. `useSyncExternalStore` is the canonical way
// to get a server-vs-client diverging value without setState-in-effect.
const subscribeToNothing = () => () => {};
const getServerMounted = () => false;
const getClientMounted = () => true;

/**
 * SRapi v0.1.0 client-side auth gate.
 *
 * Auth state derives from `localStorage` via `useSyncExternalStore` so the
 * first client paint matches the actual session. Redirects are issued from
 * a `useEffect` to avoid the "Maximum update depth" loop that happens when
 * navigation is triggered during render.
 *
 * Runtime status flows through TanStack Query for caching, retry, and
 * deduplication. `proxy.ts` (edge) handles the same redirect server-side
 * using the presence cookie, so this client fallback usually only renders
 * during the brief window between cookie write and SPA navigation.
 */
export function AuthGate({ allowedRole, children, loadingLabel = "Loading..." }: AuthGateProps) {
  const router = useRouter();
  const user = React.useSyncExternalStore(
    subscribeToUser,
    getUserSnapshot,
    getServerUserSnapshot,
  );
  const mounted = React.useSyncExternalStore(
    subscribeToNothing,
    getClientMounted,
    getServerMounted,
  );

  const runtimeQuery = useRuntimeStatus();
  const refreshRuntimeStatus = React.useCallback(async () => {
    await runtimeQuery.refetch();
  }, [runtimeQuery]);

  const wrongRole = !!(user && allowedRole && user.role !== allowedRole);
  const missingUser = mounted && !user;

  React.useEffect(() => {
    // Wait until after hydration so we never redirect on the server snapshot.
    if (!mounted) return;
    if (missingUser) {
      router.replace("/");
      return;
    }
    if (wrongRole && user) {
      router.replace(user.role === "admin" ? "/admin" : "/dashboard");
    }
  }, [mounted, missingUser, wrongRole, user, router]);

  if (!mounted || missingUser || wrongRole) {
    return (
      <div className="flex min-h-[60vh] items-center justify-center p-8">
        <Spinner size={28} label={loadingLabel} />
      </div>
    );
  }

  return (
    <>{children({ user: user!, runtimeStatus: runtimeQuery.data ?? null, refreshRuntimeStatus })}</>
  );
}

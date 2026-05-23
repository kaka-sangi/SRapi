"use client";

import * as React from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

/**
 * Single source of truth for the SRapi React Query client.
 *
 * Defaults pinned for a self-hosted control panel:
 *   - staleTime 30s: avoids hammering the gateway when navigating between pages
 *   - retry 1: control plane endpoints fail fast and surface real errors
 *   - refetchOnWindowFocus false: keeps control panels stable in long sessions
 *   - refetchOnReconnect true: catch up after the laptop wakes from sleep
 */
function makeQueryClient(): QueryClient {
  return new QueryClient({
    defaultOptions: {
      queries: {
        staleTime: 30_000,
        gcTime: 5 * 60_000,
        retry: 1,
        refetchOnWindowFocus: false,
        refetchOnReconnect: true,
      },
      mutations: {
        retry: 0,
      },
    },
  });
}

let browserClient: QueryClient | null = null;

function getQueryClient(): QueryClient {
  if (typeof window === "undefined") {
    return makeQueryClient();
  }
  browserClient ??= makeQueryClient();
  return browserClient;
}

export function QueryProvider({ children }: { children: React.ReactNode }) {
  const client = React.useMemo(() => getQueryClient(), []);
  return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
}

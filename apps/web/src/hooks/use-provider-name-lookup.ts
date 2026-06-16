"use client";

import { useMemo } from "react";
import { useAdminProviders } from "@/hooks/admin-queries";

// Best-effort id → display_name (or slug) resolution for providers. Mirrors
// useUserEmailLookup / useAccountNameLookup / useApiKeyNameLookup. Providers
// are few (one row per upstream brand) so page-size-200 is comfortable.
const PROVIDER_LOOKUP_PAGE_SIZE = 200;

export interface ProviderNameLookup {
  get(id: string | number | null | undefined): string;
  map: Map<string, string>;
  query: ReturnType<typeof useAdminProviders>;
}

export function useProviderNameLookup(): ProviderNameLookup {
  const query = useAdminProviders({ page: 1, page_size: PROVIDER_LOOKUP_PAGE_SIZE });
  const map = useMemo(() => {
    const m = new Map<string, string>();
    for (const p of query.data?.data ?? []) {
      m.set(String(p.id), p.display_name || p.name);
    }
    return m;
  }, [query.data]);
  return {
    map,
    query,
    get: (id) => {
      if (id === null || id === undefined || id === "") return "—";
      const key = String(id);
      return map.get(key) ?? `#${key}`;
    },
  };
}

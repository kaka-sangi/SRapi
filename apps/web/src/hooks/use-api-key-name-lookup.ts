"use client";

import { useMemo } from "react";
import { useAdminApiKeys } from "@/hooks/admin-queries";

// Best-effort id → "name (prefix)" resolution for API keys. Mirrors
// useUserEmailLookup / useAccountNameLookup. Page-size 200 covers most
// installs; ids past the window fall back to the raw id rendered as "#<id>".
const API_KEY_LOOKUP_PAGE_SIZE = 200;

export interface ApiKeyNameLookup {
  get(id: string | number | null | undefined): string;
  map: Map<string, string>;
  query: ReturnType<typeof useAdminApiKeys>;
}

export function useApiKeyNameLookup(): ApiKeyNameLookup {
  const query = useAdminApiKeys({ page: 1, page_size: API_KEY_LOOKUP_PAGE_SIZE });
  const map = useMemo(() => {
    const m = new Map<string, string>();
    for (const k of query.data?.data ?? []) {
      // Surface the public prefix alongside the name — it's the same hint the
      // /admin/api-keys table uses, and gives the operator enough to spot
      // which key is which when several share a name (legacy migrations).
      m.set(String(k.id), k.prefix ? `${k.name} (${k.prefix})` : k.name);
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

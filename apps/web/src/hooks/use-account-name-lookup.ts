"use client";

import { useMemo } from "react";
import { useAdminAccounts } from "@/hooks/admin-queries";

// Best-effort id → name resolution for provider accounts. Mirrors
// useUserEmailLookup: a single 200-row read of /admin/accounts builds a map
// callers query via lookup.get(id). Larger installs whose ids land past row
// 200 fall back to the raw id rendered as "#<id>".
//
// Used by surfaces that expose `account_id` as a numeric value (usage logs,
// error logs, diagnostics).
const ACCOUNT_LOOKUP_PAGE_SIZE = 200;

export interface AccountNameLookup {
  get(id: string | number | null | undefined): string;
  map: Map<string, string>;
  query: ReturnType<typeof useAdminAccounts>;
}

export function useAccountNameLookup(): AccountNameLookup {
  const query = useAdminAccounts({ page: 1, page_size: ACCOUNT_LOOKUP_PAGE_SIZE });
  const map = useMemo(() => {
    const m = new Map<string, string>();
    for (const a of query.data?.data ?? []) {
      m.set(String(a.id), a.name);
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

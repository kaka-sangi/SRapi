"use client";

import { useMemo } from "react";
import { useAdminUsers } from "@/hooks/admin-queries";

// Shared best-effort lookup: resolve a numeric user_id to the user's email via
// the first 200 rows of /admin/users. Operators with ids past the lookup
// window get the raw id back; that's intentional — without a server-side
// join, there's no efficient way to resolve arbitrary stale ids cheaply.
//
// Used by panels that surface user_ids in tables (affiliate invites/ledger,
// audit logs, billing ledger, manual adjustments, payment dashboard).
const USER_LOOKUP_PAGE_SIZE = 200;

export interface UserEmailLookup {
  // Resolve id → email, falling back to the stringified id when not found.
  get(id: string | number | null | undefined): string;
  // Raw map keyed by stringified id, for callers that want to thread it
  // into options arrays (e.g. FilterSelect dropdowns).
  map: Map<string, string>;
  // The underlying query — surfaces isLoading/isError so callers can render
  // a skeleton while the lookup warms.
  query: ReturnType<typeof useAdminUsers>;
}

export function useUserEmailLookup(): UserEmailLookup {
  const query = useAdminUsers({ page: 1, page_size: USER_LOOKUP_PAGE_SIZE });
  const map = useMemo(() => {
    const m = new Map<string, string>();
    for (const u of query.data?.data ?? []) {
      m.set(String(u.id), u.email);
    }
    return m;
  }, [query.data]);
  return {
    map,
    query,
    get: (id) => {
      if (id === null || id === undefined || id === "") return "—";
      const key = String(id);
      return map.get(key) ?? key;
    },
  };
}

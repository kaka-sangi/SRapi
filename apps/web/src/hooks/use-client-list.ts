"use client";

import { useMemo } from "react";
import type { UseQueryResult } from "@tanstack/react-query";
import type { AdminListResult } from "@/lib/admin-api";

interface ClientListControls {
  page: number;
  pageSize: number;
  search: string;
  filters: Record<string, string>;
}

interface ClientListOptions<T> {
  /** Return false to drop a row. `search` is already trimmed + lower-cased. */
  match?: (row: T, search: string, filters: Record<string, string>) => boolean;
  /** Order the full result set before it is sliced into pages. */
  compare?: (a: T, b: T) => number;
}

/**
 * Client-side search + sort + pagination for admin endpoints that return the
 * whole result set in a single page. Several control-plane handlers (audit
 * logs, billing ledger, …) emit `pagination(len(data))` server-side: they
 * ignore the `page`/`page_size` query and hand back every row. Feeding that
 * straight into `AdminListView` would render a toolbar + pagination footer that
 * do nothing. This hook filters, sorts and slices in the browser and returns a
 * derived query (the current page's rows) plus the true filtered `total`, so the
 * controls actually do what they appear to do.
 *
 * When the backend grows real server-side pagination for one of these
 * endpoints, swap the page over to the plain `useAdminList` + pass-through
 * pattern used by e.g. the announcements page.
 */
export function useClientPagedList<T>(
  query: UseQueryResult<AdminListResult<T>>,
  controls: ClientListControls,
  options: ClientListOptions<T> = {},
): { query: UseQueryResult<AdminListResult<T>>; total: number } {
  const { page, pageSize, search, filters } = controls;
  const { match, compare } = options;

  return useMemo(() => {
    if (!query.data) {
      return { query, total: 0 };
    }
    let rows = query.data.data;
    if (match) {
      const term = search.trim().toLowerCase();
      rows = rows.filter((row) => match(row, term, filters));
    }
    if (compare) {
      rows = [...rows].sort(compare);
    }
    const total = rows.length;
    const start = (page - 1) * pageSize;
    const pageRows = rows.slice(start, start + pageSize);
    return {
      query: { ...query, data: { ...query.data, data: pageRows } },
      total,
    };
  }, [query, page, pageSize, search, filters, match, compare]);
}

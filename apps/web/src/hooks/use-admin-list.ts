"use client";

import { useCallback, useMemo, useState } from "react";
import { useDebouncedValue } from "@/hooks/use-debounced-value";

export type SortDir = "asc" | "desc";
export interface SortState {
  key: string;
  dir: SortDir;
}

/**
 * Centralized state for an admin list page: debounced search, filter values,
 * pagination, client-side sort, and row selection for bulk actions. Pages map
 * `search`/`filters`/`page` into their SDK query shape; `AdminListView`
 * consumes the same controls to render the toolbar, footer and checkboxes.
 */
export function useAdminList(opts?: { pageSize?: number; debounceMs?: number }) {
  const pageSize = opts?.pageSize ?? 20;

  const [page, setPage] = useState(1);
  const [searchInput, setSearchInputState] = useState("");
  const search = useDebouncedValue(searchInput.trim(), opts?.debounceMs ?? 300);
  const [filters, setFilters] = useState<Record<string, string>>({});
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [sort, setSort] = useState<SortState | undefined>(undefined);

  // Narrowing the result set (typing or changing a filter) sends the user back
  // to page 1. Done in the event handlers so there is no setState-in-effect.
  const setSearchInput = useCallback((value: string) => {
    setSearchInputState(value);
    setPage(1);
  }, []);

  const setFilter = useCallback((key: string, value: string | undefined) => {
    setFilters((prev) => {
      const next = { ...prev };
      if (!value) delete next[key];
      else next[key] = value;
      return next;
    });
    setPage(1);
  }, []);

  const toggle = useCallback((id: string) => {
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }, []);

  const togglePage = useCallback((ids: string[], checked: boolean) => {
    setSelected((prev) => {
      const next = new Set(prev);
      for (const id of ids) {
        if (checked) next.add(id);
        else next.delete(id);
      }
      return next;
    });
  }, []);

  const clearSelection = useCallback(() => setSelected(new Set()), []);

  // Reset search + every filter back to the unfiltered view (and to page 1) — the
  // recovery path offered from a "no results" empty state.
  const clearFilters = useCallback(() => {
    setSearchInputState("");
    setFilters({});
    setPage(1);
  }, []);

  const toggleSort = useCallback((key: string) => {
    setSort((prev) => {
      if (prev?.key !== key) return { key, dir: "asc" };
      if (prev.dir === "asc") return { key, dir: "desc" };
      return undefined;
    });
  }, []);

  return useMemo(
    () => ({
      page,
      setPage,
      pageSize,
      searchInput,
      setSearchInput,
      search,
      filters,
      setFilter,
      selected,
      toggle,
      togglePage,
      clearSelection,
      clearFilters,
      sort,
      toggleSort,
    }),
    [page, pageSize, searchInput, setSearchInput, search, filters, setFilter, selected, toggle, togglePage, clearSelection, clearFilters, sort, toggleSort],
  );
}

export type AdminListControls = ReturnType<typeof useAdminList>;

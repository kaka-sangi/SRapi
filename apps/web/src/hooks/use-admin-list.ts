"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { useDebouncedValue } from "@/hooks/use-debounced-value";

type SortDir = "asc" | "desc";
export interface SortState {
  key: string;
  dir: SortDir;
}

interface InitialListState {
  page: number;
  search: string;
  filters: Record<string, string>;
  sort: SortState | undefined;
}

// Admin list pages only ever mount client-side (they render inside the
// AuthGate, which shows a spinner until the user resolves in the browser), so
// reading `window.location` in the state initializer is safe and restores a
// shared/refreshed view. Filters are carried as `f_<key>` params.
function readInitialState(): InitialListState {
  if (typeof window === "undefined") {
    return { page: 1, search: "", filters: {}, sort: undefined };
  }
  const sp = new URLSearchParams(window.location.search);
  const pageRaw = Number(sp.get("page"));
  const page = Number.isFinite(pageRaw) && pageRaw > 0 ? Math.floor(pageRaw) : 1;
  const search = sp.get("q") ?? "";
  const filters: Record<string, string> = {};
  sp.forEach((value, key) => {
    if (key.startsWith("f_") && value) filters[key.slice(2)] = value;
  });
  let sort: SortState | undefined;
  const sortRaw = sp.get("sort");
  if (sortRaw) {
    const [key, dir] = sortRaw.split(":");
    if (key) sort = { key, dir: dir === "desc" ? "desc" : "asc" };
  }
  return { page, search, filters, sort };
}

/**
 * Centralized state for an admin list page: debounced search, filter values,
 * pagination, client-side sort, and row selection for bulk actions. Pages map
 * `search`/`filters`/`page` into their SDK query shape; `AdminListView`
 * consumes the same controls to render the toolbar, footer and checkboxes.
 *
 * The active view (search/filters/sort/page — not selection) is mirrored into
 * the URL via `history.replaceState`, so a refresh or a shared link restores it.
 */
export function useAdminList(opts?: { pageSize?: number; debounceMs?: number }) {
  const pageSize = opts?.pageSize ?? 20;

  // Lazy initializers (evaluated once on mount) read the URL directly — no ref,
  // so this stays clear of the "no ref access during render" lint rule.
  const [page, setPage] = useState<number>(() => readInitialState().page);
  const [searchInput, setSearchInputState] = useState<string>(() => readInitialState().search);
  const search = useDebouncedValue(searchInput.trim(), opts?.debounceMs ?? 300);
  const [filters, setFilters] = useState<Record<string, string>>(() => readInitialState().filters);
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [sort, setSort] = useState<SortState | undefined>(() => readInitialState().sort);

  // Mirror the active view into the URL. Uses replaceState (not the Next router)
  // so it never triggers a navigation/re-render loop or a missing-Suspense
  // bailout — it just keeps the address bar in sync for refresh + share.
  useEffect(() => {
    if (typeof window === "undefined") return;
    const sp = new URLSearchParams(window.location.search);
    for (const key of [...sp.keys()]) {
      if (key === "q" || key === "page" || key === "sort" || key.startsWith("f_")) sp.delete(key);
    }
    if (search) sp.set("q", search);
    if (page > 1) sp.set("page", String(page));
    if (sort) sp.set("sort", `${sort.key}:${sort.dir}`);
    for (const [key, value] of Object.entries(filters)) {
      if (value) sp.set(`f_${key}`, value);
    }
    const qs = sp.toString();
    const next = qs ? `${window.location.pathname}?${qs}` : window.location.pathname;
    window.history.replaceState(window.history.state, "", next);
  }, [search, page, sort, filters]);

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

type AdminListControls = ReturnType<typeof useAdminList>;

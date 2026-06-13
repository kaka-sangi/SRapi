"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { useDebouncedValue } from "@/hooks/use-debounced-value";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export interface TableQueryParams<F = Record<string, unknown>> {
  page: number;
  pageSize: number;
  search: string;
  sort: { field: string; order: "asc" | "desc" } | null;
  filters: F;
}

export interface UseTableLoaderOptions<T, F = Record<string, unknown>> {
  queryKey: unknown[];
  queryFn: (params: TableQueryParams<F>) => Promise<T[]>;
  defaultPageSize?: number;
  defaultSort?: { field: string; order: "asc" | "desc" };
  searchFields?: string[];
  filterDefaults?: Partial<F>;
  staleTime?: number;
}

export interface UseTableLoaderReturn<T, F = Record<string, unknown>> {
  data: T[];
  isLoading: boolean;
  error: Error | null;
  page: number;
  pageSize: number;
  setPage: (page: number) => void;
  setPageSize: (size: number) => void;
  search: string;
  setSearch: (search: string) => void;
  debouncedSearch: string;
  sort: { field: string; order: "asc" | "desc" } | null;
  setSort: (field: string) => void;
  filters: F;
  setFilter: <K extends keyof F>(key: K, value: F[K]) => void;
  resetFilters: () => void;
  selected: Set<string>;
  toggleSelect: (id: string) => void;
  selectAll: () => void;
  clearSelection: () => void;
  isAllSelected: boolean;
  refetch: () => void;
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

const STORAGE_KEY_PREFIX = "table-loader:pageSize:";

function readStoredPageSize(key: string, fallback: number): number {
  if (typeof window === "undefined") return fallback;
  try {
    const raw = localStorage.getItem(key);
    if (raw === null) return fallback;
    const parsed = Number(raw);
    return Number.isFinite(parsed) && parsed > 0 ? parsed : fallback;
  } catch {
    return fallback;
  }
}

// ---------------------------------------------------------------------------
// Hook
// ---------------------------------------------------------------------------

/**
 * Reusable data-loading hook for table views. Combines TanStack React Query
 * with debounced search, pagination, tri-state sort (asc -> desc -> null),
 * typed filters, and row selection.
 *
 * Page size is persisted to localStorage keyed by the first entry of
 * `queryKey`. Selection is automatically cleared whenever the data parameters
 * (search, sort, filters, page) change so stale IDs are never retained.
 */
export function useTableLoader<T, F = Record<string, unknown>>(
  options: UseTableLoaderOptions<T, F>,
): UseTableLoaderReturn<T, F> {
  const {
    queryKey,
    queryFn,
    defaultPageSize = 20,
    defaultSort = null,
    filterDefaults,
    staleTime,
  } = options;

  // Derive a stable localStorage key from the first query key segment.
  const storageKey = `${STORAGE_KEY_PREFIX}${String(queryKey[0] ?? "default")}`;

  // --- state ---------------------------------------------------------------

  const [page, setPage] = useState(1);
  const [pageSize, setPageSizeState] = useState<number>(() =>
    readStoredPageSize(storageKey, defaultPageSize),
  );
  const [searchInput, setSearchInputState] = useState("");
  const debouncedSearch = useDebouncedValue(searchInput.trim(), 300);
  const [sort, setSortState] = useState<{ field: string; order: "asc" | "desc" } | null>(
    defaultSort ?? null,
  );
  const [filters, setFilters] = useState<F>(() => (filterDefaults ?? {}) as F);
  const [selected, setSelected] = useState<Set<string>>(new Set());

  // --- derived query params ------------------------------------------------

  const queryParams = useMemo<TableQueryParams<F>>(
    () => ({
      page,
      pageSize,
      search: debouncedSearch,
      sort,
      filters,
    }),
    [page, pageSize, debouncedSearch, sort, filters],
  );

  // --- data fetching -------------------------------------------------------

  const query = useQuery<T[], Error>({
    queryKey: [...queryKey, queryParams],
    queryFn: () => queryFn(queryParams),
    placeholderData: keepPreviousData,
    staleTime: staleTime ?? 30_000,
  });

  // --- page reset on param changes -----------------------------------------

  // Track whether this is the initial mount so we skip the first reset.
  const isInitialMount = useRef(true);

  useEffect(() => {
    if (isInitialMount.current) {
      isInitialMount.current = false;
      return;
    }
    setPage(1);
  }, [debouncedSearch, sort, filters]);

  // --- clear selection on data-param changes -------------------------------

  const prevParamsRef = useRef({ debouncedSearch, sort, filters, page });

  useEffect(() => {
    const prev = prevParamsRef.current;
    if (
      prev.debouncedSearch !== debouncedSearch ||
      prev.sort !== sort ||
      prev.filters !== filters ||
      prev.page !== page
    ) {
      setSelected(new Set());
    }
    prevParamsRef.current = { debouncedSearch, sort, filters, page };
  }, [debouncedSearch, sort, filters, page]);

  // --- callbacks -----------------------------------------------------------

  const setSearch = useCallback((value: string) => {
    setSearchInputState(value);
  }, []);

  const setPageSize = useCallback(
    (size: number) => {
      setPageSizeState(size);
      setPage(1);
      try {
        localStorage.setItem(storageKey, String(size));
      } catch {
        // localStorage may be unavailable (SSR, private browsing quota, etc.)
      }
    },
    [storageKey],
  );

  /** Tri-state sort toggle: asc -> desc -> null. */
  const setSort = useCallback((field: string) => {
    setSortState((prev) => {
      if (prev?.field !== field) return { field, order: "asc" };
      if (prev.order === "asc") return { field, order: "desc" };
      return null;
    });
  }, []);

  const setFilter = useCallback(<K extends keyof F>(key: K, value: F[K]) => {
    setFilters((prev) => ({ ...prev, [key]: value }));
  }, []);

  const resetFilters = useCallback(() => {
    setFilters((filterDefaults ?? {}) as F);
  }, [filterDefaults]);

  const toggleSelect = useCallback((id: string) => {
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }, []);

  const selectAll = useCallback(() => {
    if (!query.data) return;
    setSelected(new Set((query.data as Array<T & { id?: string }>).map((row) => row.id ?? "")));
  }, [query.data]);

  const clearSelection = useCallback(() => setSelected(new Set()), []);

  const data = query.data ?? [];
  const isAllSelected = data.length > 0 && selected.size === data.length;

  // --- return --------------------------------------------------------------

  return useMemo(
    () => ({
      data,
      isLoading: query.isLoading,
      error: query.error,
      page,
      pageSize,
      setPage,
      setPageSize,
      search: searchInput,
      setSearch,
      debouncedSearch,
      sort,
      setSort,
      filters,
      setFilter,
      resetFilters,
      selected,
      toggleSelect,
      selectAll,
      clearSelection,
      isAllSelected,
      refetch: query.refetch,
    }),
    [
      data,
      query.isLoading,
      query.error,
      page,
      pageSize,
      setPageSize,
      searchInput,
      setSearch,
      debouncedSearch,
      sort,
      setSort,
      filters,
      setFilter,
      resetFilters,
      selected,
      toggleSelect,
      selectAll,
      clearSelection,
      isAllSelected,
      query.refetch,
    ],
  );
}

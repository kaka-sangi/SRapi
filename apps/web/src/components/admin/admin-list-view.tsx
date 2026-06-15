"use client";

import type { UseQueryResult } from "@tanstack/react-query";
import type { LucideIcon } from "lucide-react";
import { ChevronDown, ChevronUp, ChevronsUpDown, SearchX } from "lucide-react";
import { useLanguage } from "@/context/LanguageContext";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { PageQueryState } from "@/components/layout/page-query-state";
import { EmptyState } from "@/components/ui/empty-state";
import { Skeleton } from "@/components/ui/skeleton";
import { Checkbox } from "@/components/ui/checkbox";
import { Pagination } from "@/components/ui/pagination";
import {
  Table,
  TableScroll,
  TableHeader,
  TableBody,
  TableRow,
  TableHead,
  TableCell,
} from "@/components/ui/table";
import { cn } from "@/lib/cn";
import type { AdminListResult } from "@/lib/admin-api";
import type { SortState } from "@/hooks/use-admin-list";
import type { ColumnVisibility } from "@/hooks/use-column-visibility";

export interface Column<T> {
  key: string;
  header: string;
  render: (row: T) => React.ReactNode;
  align?: "left" | "right";
  /** hide this column below the sm breakpoint to keep mobile tables readable */
  hideOnMobile?: boolean;
  className?: string;
  /** Provide a comparable value to enable click-to-sort on this column. */
  sortValue?: (row: T) => string | number | null | undefined;
  /** If true, column is always visible and cannot be hidden by the user. */
  pinned?: boolean;
}

export interface ListSelection {
  selected: Set<string>;
  onToggle: (id: string) => void;
  onTogglePage: (ids: string[], checked: boolean) => void;
  /** Rendered above the table while at least one row is selected. */
  bulkActions?: React.ReactNode;
}

export interface ListPagination {
  page: number;
  pageSize: number;
  total: number;
  onPageChange: (page: number) => void;
}

/**
 * Empty / no-results / loading states fill a comfortable slice of the viewport
 * and center their content, so a fresh or sparse list reads as intentional
 * rather than a short card stranded at the top of an otherwise empty page.
 */
const EMPTY_FILL = "min-h-[55vh] justify-center";

/**
 * Generic admin resource table. Define columns once per resource; this handles
 * loading / error / empty states, the mobile horizontal-scroll guard, and a
 * consistent row rhythm. Optional props add a toolbar slot (search / filters),
 * click-to-sort headers, a pagination footer, and bulk row-selection — so a
 * single component lifts every admin list page at once.
 */
export function AdminListView<T>({
  query,
  columns,
  getRowId,
  emptyIcon,
  emptyTitle,
  emptyBody,
  emptyAction,
  minWidth = 640,
  rowActions,
  dimRow,
  toolbar,
  sort,
  onSort,
  selection,
  pagination,
  isFiltered = false,
  noResultsTitle,
  noResultsBody,
  onClearFilters,
  columnVisibility,
}: {
  query: UseQueryResult<AdminListResult<T>>;
  columns: Column<T>[];
  getRowId: (row: T) => string;
  emptyIcon?: LucideIcon;
  emptyTitle: string;
  emptyBody?: string;
  emptyAction?: React.ReactNode;
  minWidth?: number;
  rowActions?: (row: T) => React.ReactNode;
  dimRow?: (row: T) => boolean;
  toolbar?: React.ReactNode;
  sort?: SortState;
  onSort?: (key: string) => void;
  selection?: ListSelection;
  pagination?: ListPagination;
  isFiltered?: boolean;
  noResultsTitle?: string;
  noResultsBody?: string;
  onClearFilters?: () => void;
  columnVisibility?: ColumnVisibility;
}) {
  const { t } = useLanguage();

  const visibleColumns = columnVisibility
    ? columns.filter((c) => c.pinned || columnVisibility.isVisible(c.key))
    : columns;

  return (
    <Card className="anim-rise-sm overflow-hidden">
      {toolbar}
      {selection && selection.selected.size > 0 ? (
        <BulkBar
          count={selection.selected.size}
          onClear={() => selection.onTogglePage([...selection.selected], false)}
        >
          {selection.bulkActions}
        </BulkBar>
      ) : null}
      <PageQueryState query={query} isEmpty={(d) => d.data.length === 0} skeleton={<ListSkeleton />}>
        {(data) =>
          data.data.length === 0 ? (
            isFiltered ? (
              <EmptyState
                className={EMPTY_FILL}
                icon={SearchX}
                title={noResultsTitle ?? t("adminCommon.noResults")}
                description={noResultsBody ?? t("adminCommon.noResultsBody")}
                action={
                  onClearFilters ? (
                    <Button variant="outline" size="sm" onClick={onClearFilters}>
                      {t("adminCommon.clearFilters")}
                    </Button>
                  ) : undefined
                }
              />
            ) : (
              <EmptyState
                className={EMPTY_FILL}
                icon={emptyIcon}
                title={emptyTitle}
                description={emptyBody}
                action={emptyAction}
              />
            )
          ) : (
            <ListTable
              rows={sortRows(data.data, visibleColumns, sort)}
              columns={visibleColumns}
              getRowId={getRowId}
              minWidth={minWidth}
              rowActions={rowActions}
              dimRow={dimRow}
              sort={sort}
              onSort={onSort}
              selection={selection}
            />
          )
        }
      </PageQueryState>
      {pagination && pagination.total > pagination.pageSize ? (
        <div className="border-t border-srapi-border">
          <Pagination
            page={pagination.page}
            pageSize={pagination.pageSize}
            total={pagination.total}
            onPageChange={pagination.onPageChange}
            labelFor={(from, to, total) => t("adminCommon.pageLabel", { from, to, total })}
            labelPrev={t("adminCommon.previousPage")}
            labelNext={t("adminCommon.nextPage")}
          />
        </div>
      ) : null}
    </Card>
  );
}

function ListTable<T>({
  rows,
  columns,
  getRowId,
  minWidth,
  rowActions,
  dimRow,
  sort,
  onSort,
  selection,
}: {
  rows: T[];
  columns: Column<T>[];
  getRowId: (row: T) => string;
  minWidth: number;
  rowActions?: (row: T) => React.ReactNode;
  dimRow?: (row: T) => boolean;
  sort?: SortState;
  onSort?: (key: string) => void;
  selection?: ListSelection;
}) {
  const pageIds = rows.map(getRowId);
  const allOnPage = pageIds.length > 0 && pageIds.every((id) => selection?.selected.has(id));
  const someOnPage = pageIds.some((id) => selection?.selected.has(id));

  return (
    <TableScroll minWidth={minWidth}>
      <Table>
        <TableHeader>
          <tr>
            {selection ? (
              <TableHead className="w-10">
                <Checkbox
                  aria-label="select all"
                  checked={allOnPage}
                  indeterminate={!allOnPage && someOnPage}
                  onChange={(e) => selection.onTogglePage(pageIds, e.target.checked)}
                />
              </TableHead>
            ) : null}
            {columns.map((c) => {
              const sortable = Boolean(c.sortValue && onSort);
              const active = sort?.key === c.key;
              return (
                <TableHead
                  key={c.key}
                  className={cn(
                    c.align === "right" && "text-right",
                    c.hideOnMobile && "hidden sm:table-cell",
                  )}
                >
                  {sortable ? (
                    <button
                      type="button"
                      onClick={() => onSort?.(c.key)}
                      className={cn(
                        "inline-flex items-center gap-1 transition-colors hover:text-srapi-text-primary",
                        c.align === "right" && "flex-row-reverse",
                        active && "text-srapi-text-primary",
                      )}
                    >
                      {c.header}
                      {active ? (
                        sort?.dir === "asc" ? (
                          <ChevronUp className="size-3" />
                        ) : (
                          <ChevronDown className="size-3" />
                        )
                      ) : (
                        <ChevronsUpDown className="size-3 opacity-40" />
                      )}
                    </button>
                  ) : (
                    c.header
                  )}
                </TableHead>
              );
            })}
            {rowActions && <TableHead aria-label="actions" className="w-px" />}
          </tr>
        </TableHeader>
        <TableBody>
          {rows.map((row) => {
            const id = getRowId(row);
            const isSelected = selection?.selected.has(id) ?? false;
            return (
              <TableRow
                key={id}
                className={cn(dimRow?.(row) && "opacity-50", isSelected && "bg-srapi-card-muted")}
              >
                {selection ? (
                  <TableCell className="w-10">
                    <Checkbox
                      aria-label="select row"
                      checked={isSelected}
                      onChange={() => selection.onToggle(id)}
                    />
                  </TableCell>
                ) : null}
                {columns.map((c) => (
                  <TableCell
                    key={c.key}
                    className={cn(
                      c.align === "right" && "text-right",
                      c.hideOnMobile && "hidden sm:table-cell",
                      c.className,
                    )}
                  >
                    {c.render(row)}
                  </TableCell>
                ))}
                {rowActions && (
                  <TableCell className="w-px whitespace-nowrap text-right">{rowActions(row)}</TableCell>
                )}
              </TableRow>
            );
          })}
        </TableBody>
      </Table>
    </TableScroll>
  );
}

/** Sort the current page's rows by the active column's `sortValue` (client-side). */
function sortRows<T>(rows: T[], columns: Column<T>[], sort?: SortState): T[] {
  if (!sort) return rows;
  const col = columns.find((c) => c.key === sort.key);
  if (!col?.sortValue) return rows;
  const dir = sort.dir === "asc" ? 1 : -1;
  return [...rows].sort((a, b) => {
    const av = col.sortValue!(a);
    const bv = col.sortValue!(b);
    if (av == null && bv == null) return 0;
    if (av == null) return 1;
    if (bv == null) return -1;
    if (typeof av === "number" && typeof bv === "number") return (av - bv) * dir;
    return String(av).localeCompare(String(bv)) * dir;
  });
}

function BulkBar({
  count,
  onClear,
  children,
}: {
  count: number;
  onClear: () => void;
  children?: React.ReactNode;
}) {
  const { t } = useLanguage();
  return (
    <div className="anim-rise-sm flex flex-wrap items-center gap-3 border-b border-srapi-border bg-srapi-card-muted px-4 py-2.5">
      <span className="font-mono text-2xs text-srapi-text-secondary">
        {t("adminCommon.selectedCount", { count })}
      </span>
      <button
        type="button"
        onClick={onClear}
        className="text-2xs text-srapi-text-tertiary underline-offset-2 hover:text-srapi-text-primary hover:underline"
      >
        {t("adminCommon.clearSelection")}
      </button>
      <div className="ml-auto flex flex-wrap items-center gap-2">{children}</div>
    </div>
  );
}

function ListSkeleton() {
  return (
    <div className="min-h-[55vh] p-0">
      {/* header row */}
      <div className="flex gap-4 border-b border-srapi-border px-4 py-3">
        <Skeleton className="h-3.5 w-28" />
        <Skeleton className="hidden h-3.5 w-24 sm:block" />
        <Skeleton className="hidden h-3.5 w-20 sm:block" />
        <Skeleton className="ml-auto h-3.5 w-14" />
      </div>
      {/* data rows */}
      {["w-36", "w-28", "w-32", "w-24", "w-30", "w-20"].map((w, i) => (
        <div key={i} className="flex items-center gap-4 border-b border-srapi-border/50 px-4 py-3.5">
          <Skeleton className={`h-4 ${w}`} />
          <Skeleton className="hidden h-4 w-20 sm:block" />
          <Skeleton className="hidden h-4 w-16 sm:block" />
          <Skeleton className="ml-auto h-4 w-10" />
        </div>
      ))}
    </div>
  );
}

/** Shared row-count caption for admin list headers. */
export function ListCount({ total }: { total: number }) {
  const { t } = useLanguage();
  return (
    <span className="font-mono text-2xs text-srapi-text-tertiary tabular">
      {t("adminCommon.total", { count: total })}
    </span>
  );
}

export type { AdminListResult };

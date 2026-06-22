"use client";

import * as React from "react";
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
import { ExpandableRow } from "@/components/ui/expandable-row";
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
import { useListKeyboardNav } from "@/hooks/use-list-keyboard-nav";

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
const EMPTY_FILL = "justify-center";

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
  rowClassName,
  rowSeverity,
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
  expandRow,
  density = "regular",
  enableKeyboardNav = false,
  emptyContent,
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
  /** Extra className applied per row (mainly for log panels' severity left stripe). */
  rowClassName?: (row: T) => string | undefined;
  /** Higher-level: maps to data-sev="info|success|warning|error|critical" used by .log-row */
  rowSeverity?: (row: T) => "info" | "success" | "warning" | "error" | "critical" | undefined;
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
  /**
   * When set, clicking a row toggles an inline expanded detail row rendered
   * inside an <ExpandableRow> spanning all columns.
   */
  expandRow?: (row: T) => React.ReactNode;
  /** Row vertical padding density. "compact" tightens py for log-dense lists. */
  density?: "compact" | "regular";
  /** Wire j/k/Enter/Esc/Home/End keyboard nav to the table scroll wrapper. */
  enableKeyboardNav?: boolean;
  /**
   * Optional override for the "no data, not filtered" empty slot — e.g. an
   * <IllustratedEmptyState>. When provided, replaces the bare <EmptyState>.
   * The no-results state (filtered → empty) keeps the standard SearchX EmptyState.
   */
  emptyContent?: React.ReactNode;
}) {
  const { t } = useLanguage();

  const visibleColumns = columnVisibility
    ? columns.filter((c) => c.pinned || columnVisibility.isVisible(c.key))
    : columns;

  return (
    <Card className="overflow-hidden">
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
            ) : emptyContent ? (
              <div className={cn("flex items-center justify-center px-4 py-12", EMPTY_FILL)}>
                {emptyContent}
              </div>
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
              rowClassName={rowClassName}
              rowSeverity={rowSeverity}
              sort={sort}
              onSort={onSort}
              selection={selection}
              expandRow={expandRow}
              density={density}
              enableKeyboardNav={enableKeyboardNav}
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
  rowClassName,
  rowSeverity,
  sort,
  onSort,
  selection,
  expandRow,
  density,
  enableKeyboardNav,
}: {
  rows: T[];
  columns: Column<T>[];
  getRowId: (row: T) => string;
  minWidth: number;
  rowActions?: (row: T) => React.ReactNode;
  dimRow?: (row: T) => boolean;
  rowClassName?: (row: T) => string | undefined;
  rowSeverity?: (row: T) => "info" | "success" | "warning" | "error" | "critical" | undefined;
  sort?: SortState;
  onSort?: (key: string) => void;
  selection?: ListSelection;
  expandRow?: (row: T) => React.ReactNode;
  density: "compact" | "regular";
  enableKeyboardNav: boolean;
}) {
  const pageIds = rows.map(getRowId);
  const allOnPage = pageIds.length > 0 && pageIds.every((id) => selection?.selected.has(id));
  const someOnPage = pageIds.some((id) => selection?.selected.has(id));

  const [expandedRowId, setExpandedRowId] = React.useState<string | null>(null);
  const toggleExpanded = React.useCallback((id: string) => {
    setExpandedRowId((prev) => (prev === id ? null : id));
  }, []);

  // Keyboard nav: ArrowDown/j move selection, Enter activates (which toggles
  // expansion when expandRow is provided).
  const { active, bindRoot } = useListKeyboardNav({
    rowIds: pageIds,
    enabled: enableKeyboardNav,
    onActivate: (id) => {
      if (expandRow) toggleExpanded(id);
    },
  });

  const densityCellClass = density === "compact" ? "py-1.5 px-3" : "py-3 px-4";

  // Total column count for the inline expansion <td colSpan>.
  const colSpanTotal = columns.length + (rowActions ? 1 : 0) + (selection ? 1 : 0);

  const table = (
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
              const isActive = sort?.key === c.key;
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
                        isActive && "text-srapi-text-primary",
                      )}
                    >
                      {c.header}
                      {isActive ? (
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
          {rows.map((row, idx) => {
            const id = getRowId(row);
            const isSelected = selection?.selected.has(id) ?? false;
            // Cap stagger at 12 so a 100-row page doesn't waterfall for seconds.
            const stagger = Math.min(idx, 12);
            const sev = rowSeverity?.(row);
            const isExpanded = expandRow != null && expandedRowId === id;
            const isKbActive = enableKeyboardNav && active === id;
            return (
              <React.Fragment key={id}>
                <TableRow
                  data-sev={sev}
                  data-row-id={id}
                  aria-expanded={expandRow ? isExpanded : undefined}
                  onClick={
                    expandRow
                      ? (e) => {
                          // Don't toggle when interactive descendants were clicked.
                          const target = e.target as HTMLElement | null;
                          if (
                            target?.closest(
                              'button,a,input,textarea,select,[role="button"],[role="checkbox"],[role="menuitem"]',
                            )
                          ) {
                            return;
                          }
                          toggleExpanded(id);
                        }
                      : undefined
                  }
                  className={cn(
                    "transition-colors",
                    sev ? "log-row" : "hover:bg-srapi-card-muted/60",
                    dimRow?.(row) && "opacity-50",
                    isSelected && "bg-srapi-accent-soft",
                    expandRow && "cursor-pointer",
                    isKbActive && "bg-srapi-card-muted/70 ring-1 ring-inset ring-srapi-primary/30",
                    rowClassName?.(row),
                  )}
                  style={{ "--stagger-index": stagger } as React.CSSProperties}
                >
                  {selection ? (
                    <TableCell className={cn("w-10", densityCellClass)}>
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
                        densityCellClass,
                        c.align === "right" && "text-right",
                        c.hideOnMobile && "hidden sm:table-cell",
                        c.className,
                      )}
                    >
                      {c.render(row)}
                    </TableCell>
                  ))}
                  {rowActions && (
                    <TableCell
                      className={cn("w-px whitespace-nowrap text-right", densityCellClass)}
                    >
                      {rowActions(row)}
                    </TableCell>
                  )}
                </TableRow>
                {expandRow && isExpanded ? (
                  <tr data-expand-for={id}>
                    <TableCell colSpan={colSpanTotal} className="p-0">
                      <ExpandableRow expanded>{expandRow(row)}</ExpandableRow>
                    </TableCell>
                  </tr>
                ) : null}
              </React.Fragment>
            );
          })}
        </TableBody>
      </Table>
    </TableScroll>
  );

  if (!enableKeyboardNav) return table;

  return (
    <div
      role="grid"
      aria-label="list"
      tabIndex={bindRoot.tabIndex}
      onKeyDown={bindRoot.onKeyDown}
      className="outline-none focus-visible:ring-2 focus-visible:ring-srapi-primary/30"
    >
      {table}
    </div>
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
    <div className="flex flex-wrap items-center gap-3 border-b border-srapi-border bg-srapi-accent-soft px-4 py-2.5">
      <span className="text-xs font-medium text-srapi-primary">
        {t("adminCommon.selectedCount", { count })}
      </span>
      <button
        type="button"
        onClick={onClear}
        className="text-xs text-srapi-text-tertiary underline-offset-2 hover:text-srapi-text-primary hover:underline"
      >
        {t("adminCommon.clearSelection")}
      </button>
      <div className="ml-auto flex flex-wrap items-center gap-2">{children}</div>
    </div>
  );
}

/**
 * Content-shaped loading placeholder mirroring the Table's row structure: a
 * header strip + 5 body rows. Cell widths are varied per row so the shimmer
 * reads as «list of distinct values» rather than a uniform block. The leading
 * cell is widest (typical id/name column), the trailing cell narrows toward
 * an actions slot — matches the columns most admin lists actually render.
 */
function ListSkeleton() {
  // Widths per visual «column slot». Index 0 = primary identity, 4 = actions.
  // Each row picks slightly different widths so the rows don't visually line up.
  const ROW_WIDTHS: ReadonlyArray<readonly [string, string, string, string, string]> = [
    ["w-44", "w-24", "w-20", "w-16", "w-8"],
    ["w-36", "w-28", "w-24", "w-14", "w-8"],
    ["w-48", "w-20", "w-28", "w-16", "w-8"],
    ["w-32", "w-32", "w-20", "w-12", "w-8"],
    ["w-40", "w-24", "w-24", "w-16", "w-8"],
  ];
  return (
    <div className="p-0">
      {/* header row — slightly smaller heights, matches TableHead */}
      <div className="flex items-center gap-4 border-b border-srapi-border px-4 py-3">
        <Skeleton className="h-3 w-28" />
        <Skeleton className="hidden h-3 w-20 sm:block" />
        <Skeleton className="hidden h-3 w-16 sm:block" />
        <Skeleton className="hidden h-3 w-12 md:block" />
        <Skeleton className="ml-auto h-3 w-8" />
      </div>
      {/* body rows — content-shaped, mirrors visible columns under sm/md */}
      {ROW_WIDTHS.map(([a, b, c, d, e], i) => (
        <div
          key={i}
          className="flex items-center gap-4 border-b border-srapi-border/50 px-4 py-3.5"
          style={{ "--stagger-index": i } as React.CSSProperties}
        >
          <Skeleton className={`h-4 ${a}`} />
          <Skeleton className={`hidden h-4 ${b} sm:block`} />
          <Skeleton className={`hidden h-4 ${c} sm:block`} />
          <Skeleton className={`hidden h-4 ${d} md:block`} />
          <Skeleton className={`ml-auto h-4 ${e}`} />
        </div>
      ))}
    </div>
  );
}

/** Shared row-count caption for admin list headers. */
export function ListCount({ total }: { total: number }) {
  const { t } = useLanguage();
  return (
    <span className="text-xs font-medium tabular text-srapi-text-tertiary">
      {t("adminCommon.total", { count: total })}
    </span>
  );
}

export type { AdminListResult };

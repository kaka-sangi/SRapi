"use client";

import type { ReactNode } from "react";
import { AlertTriangle, Database, FileSearch, Loader2, RefreshCw } from "lucide-react";
import { Badge, Button, Card, Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui";
import { adminErrorMessage } from "@/lib/admin-api";
import { clampPercent, statusBadgeVariant } from "@/lib/admin-format";
import { cn } from "@/lib/cn";
import type { Pagination } from "../../../../../packages/sdk/typescript/src/types.gen";

export function AdminPageHeader({
  title,
  description,
  actions,
}: {
  title: string;
  description?: string;
  actions?: ReactNode;
}) {
  return (
    <div className="flex flex-col justify-between gap-4 rounded-2xl border border-srapi-border bg-srapi-card p-6 sm:flex-row sm:items-start">
      <div className="space-y-1">
        <h1 className="font-serif text-2xl font-medium tracking-tight text-srapi-text-primary">
          {title}
        </h1>
        {description ? (
          <p className="max-w-3xl text-xs leading-relaxed text-srapi-text-secondary">
            {description}
          </p>
        ) : null}
      </div>
      {actions ? <div className="flex shrink-0 flex-wrap items-center gap-2">{actions}</div> : null}
    </div>
  );
}

export function AdminSection({
  title,
  description,
  children,
  actions,
  className,
}: {
  title: string;
  description?: string;
  children: ReactNode;
  actions?: ReactNode;
  className?: string;
}) {
  return (
    <section className={cn("rounded-2xl border border-srapi-border bg-srapi-card p-6", className)}>
      <div className="mb-5 flex flex-col justify-between gap-3 sm:flex-row sm:items-start">
        <div className="space-y-1">
          <h2 className="font-serif text-lg font-medium tracking-tight text-srapi-text-primary">
            {title}
          </h2>
          {description ? (
            <p className="text-xs leading-relaxed text-srapi-text-secondary">{description}</p>
          ) : null}
        </div>
        {actions ? <div className="flex shrink-0 flex-wrap items-center gap-2">{actions}</div> : null}
      </div>
      {children}
    </section>
  );
}

export function AdminStatCard({
  label,
  value,
  detail,
  icon,
  tone = "neutral",
}: {
  label: string;
  value: ReactNode;
  detail?: ReactNode;
  icon?: ReactNode;
  tone?: "neutral" | "success" | "warning" | "danger" | "accent";
}) {
  const toneClass = {
    neutral: "text-srapi-text-primary",
    success: "text-srapi-success",
    warning: "text-yellow-700 dark:text-yellow-500",
    danger: "text-srapi-error",
    accent: "text-srapi-primary",
  }[tone];

  return (
    <div className="min-w-0 rounded-2xl border border-srapi-border bg-srapi-card-muted/20 p-5">
      <div className="mb-3 flex items-center justify-between gap-3 text-srapi-text-secondary">
        <span className="truncate font-mono text-[10px] font-bold uppercase tracking-wider">
          {label}
        </span>
        {icon ? <span className="shrink-0 text-srapi-primary">{icon}</span> : null}
      </div>
      <div className={cn("truncate font-serif text-2xl font-semibold tracking-tight", toneClass)}>
        {value}
      </div>
      {detail ? <div className="mt-2 text-[11px] text-srapi-text-secondary">{detail}</div> : null}
    </div>
  );
}

export function AdminLoadingState({ label }: { label: string }) {
  return (
    <Card className="flex items-center gap-3 bg-srapi-card p-6 text-xs text-srapi-text-secondary">
      <Loader2 className="h-4 w-4 animate-spin text-srapi-primary" />
      {label}
    </Card>
  );
}

export function AdminErrorState({
  error,
  onRetry,
  title = "Admin API request failed",
}: {
  error: unknown;
  onRetry?: () => void;
  title?: string;
}) {
  return (
    <Card className="space-y-4 border-srapi-error/30 bg-srapi-error/5 p-6">
      <div className="flex items-start gap-3">
        <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-srapi-error" />
        <div className="space-y-1">
          <div className="font-mono text-xs font-bold uppercase tracking-wider text-srapi-error">
            {title}
          </div>
          <p className="text-xs leading-relaxed text-srapi-text-secondary">
            {adminErrorMessage(error)}
          </p>
        </div>
      </div>
      {onRetry ? (
        <Button type="button" variant="outline" size="sm" onClick={onRetry}>
          <RefreshCw size={12} />
          Retry
        </Button>
      ) : null}
    </Card>
  );
}

export function AdminEmptyState({
  title,
  description,
}: {
  title: string;
  description?: string;
}) {
  return (
    <div className="rounded-2xl border border-dashed border-srapi-border bg-srapi-card-muted/20 p-8 text-center">
      <Database className="mx-auto mb-3 h-5 w-5 text-srapi-text-secondary" />
      <div className="font-mono text-xs font-bold uppercase tracking-wider text-srapi-text-primary">
        {title}
      </div>
      {description ? (
        <p className="mx-auto mt-2 max-w-xl text-xs leading-relaxed text-srapi-text-secondary">
          {description}
        </p>
      ) : null}
    </div>
  );
}

export function AdminContractGap({
  title,
  description,
  paths,
}: {
  title: string;
  description: string;
  paths?: string[];
}) {
  return (
    <AdminSection title={title} description={description}>
      <div className="rounded-2xl border border-yellow-500/20 bg-yellow-500/5 p-5">
        <div className="flex items-start gap-3">
          <FileSearch className="mt-0.5 h-4 w-4 shrink-0 text-yellow-700 dark:text-yellow-500" />
          <div className="space-y-3">
            <p className="text-xs leading-relaxed text-srapi-text-secondary">
              This page intentionally does not render synthetic records. Add or extend the
              OpenAPI admin contract first, then bind the UI to the generated SDK.
            </p>
            {paths?.length ? (
              <div className="flex flex-wrap gap-2">
                {paths.map((path) => (
                  <Badge key={path} variant="warning">
                    {path}
                  </Badge>
                ))}
              </div>
            ) : null}
          </div>
        </div>
      </div>
    </AdminSection>
  );
}

export function AdminStatusBadge({ status }: { status?: string | null }) {
  return <Badge variant={statusBadgeVariant(status)}>{status || "unknown"}</Badge>;
}

export function AdminTable({
  columns,
  rows,
  getRowKey,
  empty,
}: {
  columns: Array<{ key: string; header: ReactNode; className?: string }>;
  rows: Array<Record<string, ReactNode>>;
  getRowKey: (row: Record<string, ReactNode>, index: number) => string;
  empty: ReactNode;
}) {
  if (rows.length === 0) {
    return <>{empty}</>;
  }

  return (
    <Table>
      <TableHeader>
        <TableRow>
          {columns.map((column) => (
            <TableHead key={column.key} className={column.className}>
              {column.header}
            </TableHead>
          ))}
        </TableRow>
      </TableHeader>
      <TableBody>
        {rows.map((row, index) => (
          <TableRow key={getRowKey(row, index)}>
            {columns.map((column) => (
              <TableCell key={column.key} className={column.className}>
                {row[column.key]}
              </TableCell>
            ))}
          </TableRow>
        ))}
      </TableBody>
    </Table>
  );
}

export function AdminPaginationSummary({ pagination }: { pagination?: Pagination }) {
  if (!pagination) {
    return null;
  }
  const pageSize = pagination.page_size > 0 ? pagination.page_size : Math.max(pagination.total, 25);

  return (
    <div className="pt-3 font-mono text-[11px] text-srapi-text-secondary">
      Page {pagination.page} / page size {pageSize} / total {pagination.total}
    </div>
  );
}

export function AdminBarList({
  items,
  emptyLabel,
}: {
  items: Array<{ label: string; value: number; detail?: ReactNode }>;
  emptyLabel: string;
}) {
  if (items.length === 0) {
    return <AdminEmptyState title={emptyLabel} />;
  }

  const max = Math.max(...items.map((item) => item.value), 1);

  return (
    <div className="space-y-3">
      {items.map((item, index) => (
        <div key={`${item.label}-${index}`} className="space-y-1.5">
          <div className="flex items-center justify-between gap-3 font-mono text-[11px]">
            <span className="truncate text-srapi-text-primary">{item.label}</span>
            <span className="shrink-0 text-srapi-text-secondary">{item.detail ?? item.value}</span>
          </div>
          <div className="h-2 overflow-hidden rounded-full bg-srapi-border">
            <div
              className="h-full rounded-full bg-srapi-primary"
              style={{ width: `${clampPercent((item.value / max) * 100)}%` }}
            />
          </div>
        </div>
      ))}
    </div>
  );
}

export function AdminTrendBars({
  points,
  emptyLabel,
}: {
  points: Array<{ label: string; value: number }>;
  emptyLabel: string;
}) {
  if (points.length === 0) {
    return <AdminEmptyState title={emptyLabel} />;
  }

  const max = Math.max(...points.map((point) => point.value), 1);

  return (
    <div className="flex h-44 items-end gap-2 rounded-2xl border border-srapi-border bg-srapi-bg/50 p-4">
      {points.map((point, index) => (
        <div key={`${point.label}-${index}`} className="flex min-w-0 flex-1 flex-col items-center gap-2">
          <div
            className="w-full rounded-t-lg bg-srapi-primary/80"
            style={{ height: `${Math.max(6, clampPercent((point.value / max) * 100))}%` }}
            title={`${point.label}: ${point.value}`}
          />
          <span className="max-w-full truncate font-mono text-[9px] text-srapi-text-secondary">
            {point.label}
          </span>
        </div>
      ))}
    </div>
  );
}

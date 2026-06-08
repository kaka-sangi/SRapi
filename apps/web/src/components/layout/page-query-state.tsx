"use client";

import type { UseQueryResult } from "@tanstack/react-query";
import { AlertTriangle } from "lucide-react";
import { Skeleton } from "@/components/ui/skeleton";
import { Button } from "@/components/ui/button";
import { EmptyState } from "@/components/ui/empty-state";
import { useLanguage } from "@/context/LanguageContext";

/**
 * Standard loading / error / empty wrapper so pages never hand-roll
 * `isLoading ? ... : error ? ...`. Renders children with the resolved data.
 */
export function PageQueryState<T>({
  query,
  isEmpty,
  emptyTitle,
  emptyDescription,
  skeleton,
  children,
}: {
  query: UseQueryResult<T>;
  isEmpty?: (data: T) => boolean;
  emptyTitle?: string;
  emptyDescription?: string;
  skeleton?: React.ReactNode;
  children: (data: T) => React.ReactNode;
}) {
  const { t } = useLanguage();

  if (query.isLoading) {
    return (
      <>
        {skeleton ?? (
          <div className="space-y-3">
            <Skeleton className="h-3.5 w-28" />
            <Skeleton className="h-9 w-full" />
            <Skeleton className="h-9 w-full" />
            <Skeleton className="h-9 w-2/3" />
          </div>
        )}
      </>
    );
  }

  if (query.isError) {
    return (
      <EmptyState
        icon={AlertTriangle}
        title={t("common.error")}
        description={t("common.errorBody")}
        action={
          <Button variant="outline" size="sm" onClick={() => query.refetch()}>
            {t("common.retry")}
          </Button>
        }
      />
    );
  }

  // No data and not an error: either still pending or the query is disabled
  // (`enabled: false`, e.g. an on-demand detail fetch). Show the skeleton rather
  // than a misleading error state — `isLoading` is false for a paused query, so
  // a bare `data === undefined` check used to render the error EmptyState here.
  if (query.data === undefined) {
    return <>{skeleton ?? null}</>;
  }

  // Only short-circuit to the wrapper's own empty state when the caller supplied
  // copy for it. With `isEmpty` but no `emptyTitle`, fall through so children can
  // render their own (usually richer: icon, filtered-vs-empty, CTA) empty state —
  // passing `isEmpty` alone must not override that with a bare default.
  if (isEmpty?.(query.data) && emptyTitle) {
    return <EmptyState title={emptyTitle} description={emptyDescription} />;
  }

  return <>{children(query.data)}</>;
}

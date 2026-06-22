"use client";

import { ChevronLeft, ChevronRight } from "lucide-react";
import { Button } from "./button";

export function Pagination({
  page,
  pageSize,
  total,
  onPageChange,
  labelFor,
  labelPrev = "Previous page",
  labelNext = "Next page",
}: {
  page: number;
  pageSize: number;
  total: number;
  onPageChange: (page: number) => void;
  /** localized "{from}–{to} of {total}" label builder */
  labelFor?: (from: number, to: number, total: number) => string;
  /** localized accessible labels for the prev/next buttons */
  labelPrev?: string;
  labelNext?: string;
}) {
  const from = total === 0 ? 0 : (page - 1) * pageSize + 1;
  const to = Math.min(page * pageSize, total);
  const hasPrev = page > 1;
  const hasNext = to < total;
  const label = labelFor ? labelFor(from, to, total) : `${from}–${to} / ${total}`;

  return (
    <div className="flex items-center justify-between gap-3 border-t border-srapi-border bg-srapi-card-muted/40 px-5 py-3">
      <span className="font-mono text-2xs uppercase tracking-wider text-srapi-text-tertiary">
        {label}
      </span>
      <div className="flex items-center gap-1.5">
        <Button
          variant="outline"
          size="icon"
          disabled={!hasPrev}
          onClick={() => onPageChange(page - 1)}
          aria-label={labelPrev}
        >
          <ChevronLeft className="size-4" />
        </Button>
        <Button
          variant="outline"
          size="icon"
          disabled={!hasNext}
          onClick={() => onPageChange(page + 1)}
          aria-label={labelNext}
        >
          <ChevronRight className="size-4" />
        </Button>
      </div>
    </div>
  );
}

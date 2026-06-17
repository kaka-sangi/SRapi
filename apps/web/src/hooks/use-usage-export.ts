"use client";

import { useCallback, useRef, useState } from "react";
import { getCurrentUserUsage } from "../../../../packages/sdk/typescript/src/index";
import { configureSDKClient, parseMoneyValue } from "@/lib/api/_shared";
import { escapeCsv } from "@/lib/csv";
import type { LiveUsageLog } from "@/lib/api/types";

/**
 * Client-side CSV export for the current user's usage logs.
 *
 * The /me/usage endpoint is paginated (page / page_size). This hook walks
 * every page, surfaces progress (current / total / percent), and can be
 * cancelled mid-flight via an AbortController. Nothing here touches React
 * Query — an export is a one-shot user action, not cached server state, so it
 * lives in plain local state and a ref-held controller.
 */

const PAGE_SIZE = 100;

export type UsageExportPhase = "idle" | "running" | "done" | "cancelled" | "error";

export interface UsageExportProgress {
  phase: UsageExportPhase;
  /** Rows fetched so far. */
  current: number;
  /** Total rows reported by the first page's pagination (0 until known). */
  total: number;
  /** 0-100, clamped. */
  percent: number;
  /** Populated when phase === "error". */
  error?: string;
}

const INITIAL: UsageExportProgress = {
  phase: "idle",
  current: 0,
  total: 0,
  percent: 0,
};

/** Columns emitted to the CSV, in order. Keep header + accessor in lockstep. */
const COLUMNS: { header: string; value: (log: LiveUsageLog) => string | number }[] = [
  { header: "created_at", value: (l) => l.created_at },
  { header: "request_id", value: (l) => l.request_id },
  { header: "model", value: (l) => l.model },
  { header: "requested_model", value: (l) => l.requested_model ?? "" },
  { header: "upstream_model", value: (l) => l.upstream_model ?? "" },
  { header: "source_endpoint", value: (l) => l.source_endpoint },
  { header: "billing_mode", value: (l) => l.billing_mode ?? "" },
  { header: "success", value: (l) => (l.success ? "true" : "false") },
  { header: "total_tokens", value: (l) => l.total_tokens ?? 0 },
  { header: "cost", value: (l) => parseMoneyValue(l.cost) },
  { header: "input_cost", value: (l) => parseMoneyValue(l.input_cost) },
  { header: "output_cost", value: (l) => parseMoneyValue(l.output_cost) },
  { header: "cache_read_cost", value: (l) => parseMoneyValue(l.cache_read_cost) },
  { header: "cache_write_cost", value: (l) => parseMoneyValue(l.cache_write_cost) },
  { header: "currency", value: (l) => l.currency ?? "USD" },
];


function rowsToCsv(rows: LiveUsageLog[]): string {
  const header = COLUMNS.map((c) => escapeCsv(c.header)).join(",");
  const body = rows.map((row) => COLUMNS.map((c) => escapeCsv(c.value(row))).join(","));
  // Leading BOM so Excel opens UTF-8 content without mangling non-ASCII.
  return `﻿${[header, ...body].join("\r\n")}\r\n`;
}

function triggerDownload(csv: string, filename: string) {
  const blob = new Blob([csv], { type: "text/csv;charset=utf-8;" });
  const url = URL.createObjectURL(blob);
  const link = document.createElement("a");
  link.href = url;
  link.download = filename;
  document.body.appendChild(link);
  link.click();
  link.remove();
  URL.revokeObjectURL(url);
}

export interface UseUsageExportResult {
  progress: UsageExportProgress;
  isExporting: boolean;
  /** Begin paging + download. No-op if already running. Resolves to rows written (0 if cancelled/empty). */
  start: () => Promise<number>;
  /** Abort an in-flight export. */
  cancel: () => void;
  /** Reset back to idle (e.g. when the dialog closes). */
  reset: () => void;
}

export function useUsageExport(): UseUsageExportResult {
  const [progress, setProgress] = useState<UsageExportProgress>(INITIAL);
  const controllerRef = useRef<AbortController | null>(null);

  const cancel = useCallback(() => {
    controllerRef.current?.abort();
  }, []);

  const reset = useCallback(() => {
    controllerRef.current?.abort();
    controllerRef.current = null;
    setProgress(INITIAL);
  }, []);

  const start = useCallback(async (): Promise<number> => {
    if (controllerRef.current) return 0;

    configureSDKClient();
    const controller = new AbortController();
    controllerRef.current = controller;
    setProgress({ phase: "running", current: 0, total: 0, percent: 0 });

    const collected: LiveUsageLog[] = [];
    let page = 1;
    let total = 0;
    let hasMore = true;

    try {
      while (hasMore && !controller.signal.aborted) {
        const response = await getCurrentUserUsage({
          query: { page, page_size: PAGE_SIZE },
          signal: controller.signal,
          throwOnError: true,
        });

        const rows = (response.data?.data ?? []) as LiveUsageLog[];
        const pagination = response.data?.pagination;
        if (page === 1) {
          total = pagination?.total ?? rows.length;
        }
        collected.push(...rows);

        const current = collected.length;
        const denominator = total > 0 ? total : current;
        setProgress({
          phase: "running",
          current,
          total,
          percent: denominator > 0 ? Math.min(100, Math.round((current / denominator) * 100)) : 0,
        });

        const hasNext = pagination?.has_next ?? rows.length >= PAGE_SIZE;
        const reachedTotal = total > 0 && current >= total;
        hasMore = hasNext && rows.length > 0 && !reachedTotal;
        page += 1;
      }

      if (controller.signal.aborted) {
        setProgress((p) => ({ ...p, phase: "cancelled" }));
        return 0;
      }

      if (collected.length > 0) {
        const stamp = new Date().toISOString().slice(0, 10);
        triggerDownload(rowsToCsv(collected), `usage_${stamp}.csv`);
      }

      setProgress({
        phase: "done",
        current: collected.length,
        total: total || collected.length,
        percent: 100,
      });
      return collected.length;
    } catch (err) {
      // An abort surfaces as a rejected fetch; treat it as a clean cancel.
      if (controller.signal.aborted) {
        setProgress((p) => ({ ...p, phase: "cancelled" }));
        return 0;
      }
      setProgress((p) => ({
        ...p,
        phase: "error",
        error: err instanceof Error ? err.message : String(err),
      }));
      return 0;
    } finally {
      if (controllerRef.current === controller) {
        controllerRef.current = null;
      }
    }
  }, []);

  return {
    progress,
    isExporting: progress.phase === "running",
    start,
    cancel,
    reset,
  };
}

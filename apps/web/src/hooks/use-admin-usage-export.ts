"use client";

import { useCallback, useRef, useState } from "react";
import { exportAdminUsage } from "../../../../packages/sdk/typescript/src/index";
import { configureSDKClient, parseMoneyValue } from "@/lib/api/_shared";
import type { UsageLog } from "@/lib/sdk-types";

// Admin-side CSV export. The backend `GET /admin/usage/export?start=&end=` is a
// single-shot JSON payload (no paging — unlike the per-user export hook). This
// hook fires it, builds a CSV from the `logs` array, and triggers a download.
//
// Kept deliberately small: no progress bar, no abort midway (the backend either
// returns the whole snapshot or fails). If admin usage volume grows so large
// that this becomes painful, the right move is a streaming/paged export
// endpoint, not making the client work harder.

export type AdminUsageExportPhase = "idle" | "running" | "done" | "error";

export interface AdminUsageExportState {
  phase: AdminUsageExportPhase;
  rows: number;
  error?: string;
}

const INITIAL: AdminUsageExportState = { phase: "idle", rows: 0 };

const COLUMNS: { header: string; value: (log: UsageLog) => string | number }[] = [
  { header: "created_at", value: (l) => l.created_at },
  { header: "request_id", value: (l) => l.request_id },
  { header: "user_id", value: (l) => l.user_id },
  { header: "api_key_id", value: (l) => l.api_key_id },
  { header: "provider_id", value: (l) => l.provider_id ?? "" },
  { header: "account_id", value: (l) => l.account_id ?? "" },
  { header: "model", value: (l) => l.model },
  { header: "requested_model", value: (l) => l.requested_model ?? "" },
  { header: "upstream_model", value: (l) => l.upstream_model ?? "" },
  { header: "source_endpoint", value: (l) => l.source_endpoint },
  { header: "billing_mode", value: (l) => l.billing_mode ?? "" },
  { header: "success", value: (l) => (l.success ? "true" : "false") },
  { header: "error_class", value: (l) => l.error_class ?? "" },
  { header: "input_tokens", value: (l) => l.input_tokens },
  { header: "output_tokens", value: (l) => l.output_tokens },
  { header: "cached_tokens", value: (l) => l.cached_tokens },
  { header: "total_tokens", value: (l) => l.total_tokens },
  { header: "latency_ms", value: (l) => l.latency_ms },
  { header: "cost", value: (l) => parseMoneyValue(l.cost) },
  { header: "input_cost", value: (l) => parseMoneyValue(l.input_cost) },
  { header: "output_cost", value: (l) => parseMoneyValue(l.output_cost) },
  { header: "cache_read_cost", value: (l) => parseMoneyValue(l.cache_read_cost) },
  { header: "cache_write_cost", value: (l) => parseMoneyValue(l.cache_write_cost) },
  { header: "currency", value: (l) => l.currency },
];

// Wrap in quotes when needed; neutralise spreadsheet formula injection
// (=, +, -, @, leading control chars) by prefixing a single quote. Same
// approach as use-usage-export.ts — keep both in sync if you tweak it.
function escapeCsv(value: string | number): string {
  const str = String(value);
  const needsFormulaGuard = /^[=+\-@\t\r]/.test(str);
  const guarded = needsFormulaGuard ? `'${str}` : str;
  if (/[",\n\r]/.test(guarded)) {
    return `"${guarded.replace(/"/g, '""')}"`;
  }
  return guarded;
}

function rowsToCsv(rows: UsageLog[]): string {
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

export interface UseAdminUsageExportResult {
  state: AdminUsageExportState;
  isExporting: boolean;
  start: (filenameStamp?: string) => Promise<number>;
  reset: () => void;
}

export function useAdminUsageExport(): UseAdminUsageExportResult {
  const [state, setState] = useState<AdminUsageExportState>(INITIAL);
  const runningRef = useRef(false);

  const reset = useCallback(() => setState(INITIAL), []);

  const start = useCallback(async (filenameStamp?: string): Promise<number> => {
    if (runningRef.current) return 0;
    runningRef.current = true;
    configureSDKClient();
    setState({ phase: "running", rows: 0 });
    try {
      const response = await exportAdminUsage({ throwOnError: true });
      const logs = (response.data?.data?.logs ?? []) as UsageLog[];
      if (logs.length > 0) {
        const stamp = filenameStamp ?? new Date().toISOString().slice(0, 10);
        triggerDownload(rowsToCsv(logs), `admin_usage_${stamp}.csv`);
      }
      setState({ phase: "done", rows: logs.length });
      return logs.length;
    } catch (err) {
      setState({
        phase: "error",
        rows: 0,
        error: err instanceof Error ? err.message : String(err),
      });
      return 0;
    } finally {
      runningRef.current = false;
    }
  }, []);

  return {
    state,
    isExporting: state.phase === "running",
    start,
    reset,
  };
}

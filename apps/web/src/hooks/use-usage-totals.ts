"use client";

import { useMemo } from "react";
import type { UsageLogSummary } from "@/lib/srapi-types";

export interface UsageTotals {
  requests: number;
  successCount: number;
  successRate: number;
  totalTokens: number;
  totalCost: number;
  currency: string;
}

export function useUsageTotals(logs: UsageLogSummary[]): UsageTotals {
  return useMemo(() => {
    const requests = logs.length;
    const successCount = logs.filter((l) => l.success).length;
    const totalTokens = logs.reduce((sum, l) => sum + l.total_tokens, 0);
    const totalCost = logs.reduce((sum, l) => sum + l.cost, 0);
    return {
      requests,
      successCount,
      successRate: requests === 0 ? 0 : (successCount / requests) * 100,
      totalTokens,
      totalCost,
      currency: logs.find((l) => l.currency)?.currency ?? "USD",
    };
  }, [logs]);
}

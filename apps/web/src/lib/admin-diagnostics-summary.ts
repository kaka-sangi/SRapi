import type { CacheStatsEntry, CircuitBreakerEntry } from "@/lib/admin-api";

type DiagnosticsTone = "active" | "limited" | "disabled" | "error";

export interface DiagnosticSummary {
  tone: DiagnosticsTone;
  labelKey: string;
  detail: string;
}

export function circuitBreakerStateLabelKey(state: CircuitBreakerEntry["state"]): string {
  switch (state) {
    case "closed":
      return "diagnostics.breakerClosed";
    case "open":
      return "diagnostics.breakerOpen";
    case "half-open":
      return "diagnostics.breakerHalfOpen";
    default:
      return "diagnostics.breakerClosed";
  }
}

export function circuitBreakerSummary(entry: CircuitBreakerEntry): DiagnosticSummary {
  if (entry.state === "open") {
    return {
      tone: "error",
      labelKey: "diagnostics.openBreaker",
      detail: failureDetail(entry),
    };
  }
  if (entry.state === "half-open") {
    return {
      tone: "limited",
      labelKey: "diagnostics.recoveringBreaker",
      detail: failureDetail(entry),
    };
  }
  if (entry.success_rate < 0.8 || entry.consecutive_failures >= 3) {
    return {
      tone: "limited",
      labelKey: "diagnostics.degradedBreaker",
      detail: failureDetail(entry),
    };
  }
  return {
    tone: "active",
    labelKey: "diagnostics.healthyBreaker",
    detail: failureDetail(entry),
  };
}

export function cacheHealthSummary(cache: CacheStatsEntry): DiagnosticSummary {
  const hitRate = parseHitRate(cache.hit_rate);
  if (cache.evictions > 0 && hitRate < 50) {
    return {
      tone: "limited",
      labelKey: "diagnostics.cacheChurn",
      detail: cacheDetail(cache),
    };
  }
  if (cache.misses > cache.hits && hitRate < 50) {
    return {
      tone: "limited",
      labelKey: "diagnostics.cacheCold",
      detail: cacheDetail(cache),
    };
  }
  return {
    tone: "active",
    labelKey: "diagnostics.cacheHealthy",
    detail: cacheDetail(cache),
  };
}

function failureDetail(entry: CircuitBreakerEntry): string {
  return `req:${entry.requests} ok:${entry.total_successes} fail:${entry.total_failures} streak:+${entry.consecutive_successes}/-${entry.consecutive_failures}`;
}

function cacheDetail(cache: CacheStatsEntry): string {
  return `size:${cache.size} hits:${cache.hits} misses:${cache.misses} evictions:${cache.evictions}`;
}

function parseHitRate(value: string): number {
  const parsed = Number.parseFloat(value.replace("%", ""));
  return Number.isFinite(parsed) ? parsed : 0;
}

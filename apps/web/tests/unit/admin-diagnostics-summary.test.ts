import { describe, expect, it } from "vitest";
import {
  cacheHealthSummary,
  circuitBreakerStateLabelKey,
  circuitBreakerSummary,
} from "@/lib/admin-diagnostics-summary";
import type { CacheStatsEntry, CircuitBreakerEntry } from "@/lib/admin-api";

describe("admin diagnostics summaries", () => {
  it("maps circuit breaker states to stable translation keys", () => {
    expect(circuitBreakerStateLabelKey("closed")).toBe("diagnostics.breakerClosed");
    expect(circuitBreakerStateLabelKey("open")).toBe("diagnostics.breakerOpen");
    expect(circuitBreakerStateLabelKey("half-open")).toBe("diagnostics.breakerHalfOpen");
  });

  it("classifies circuit breakers by risk", () => {
    const healthy: CircuitBreakerEntry = {
      account_id: 1,
      state: "closed",
      requests: 100,
      total_successes: 98,
      total_failures: 2,
      consecutive_successes: 4,
      consecutive_failures: 0,
      success_rate: 0.98,
    };
    const degraded: CircuitBreakerEntry = {
      ...healthy,
      success_rate: 0.76,
      consecutive_failures: 4,
    };
    const open: CircuitBreakerEntry = { ...healthy, state: "open", success_rate: 0.2 };

    expect(circuitBreakerSummary(healthy)).toMatchObject({
      tone: "active",
      labelKey: "diagnostics.healthyBreaker",
    });
    expect(circuitBreakerSummary(degraded)).toMatchObject({
      tone: "limited",
      labelKey: "diagnostics.degradedBreaker",
    });
    expect(circuitBreakerSummary(open)).toMatchObject({
      tone: "error",
      labelKey: "diagnostics.openBreaker",
    });
  });

  it("classifies cache health by hit rate and churn", () => {
    const healthy: CacheStatsEntry = {
      name: "models",
      hits: 90,
      misses: 10,
      evictions: 0,
      size: 12,
      hit_rate: "90%",
    };
    const cold: CacheStatsEntry = {
      ...healthy,
      hits: 10,
      misses: 30,
      hit_rate: "25%",
    };
    const churn: CacheStatsEntry = {
      ...healthy,
      hits: 10,
      misses: 60,
      evictions: 8,
      hit_rate: "14%",
    };

    expect(cacheHealthSummary(healthy)).toMatchObject({
      tone: "active",
      labelKey: "diagnostics.cacheHealthy",
    });
    expect(cacheHealthSummary(cold)).toMatchObject({
      tone: "limited",
      labelKey: "diagnostics.cacheCold",
    });
    expect(cacheHealthSummary(churn)).toMatchObject({
      tone: "limited",
      labelKey: "diagnostics.cacheChurn",
    });
  });
});

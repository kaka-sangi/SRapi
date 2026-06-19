import { describe, expect, it } from "vitest";
import { opsEvidenceChainStatus } from "@/app/admin/ops/evidence-chain-health";
import type { OpsSystemLogHealth } from "@/lib/sdk-types";

const healthy = {
  storage_mode: "durable",
  writable: true,
  degraded: false,
  stale: false,
  total_count: 10,
  level_counts: { debug: 0, info: 8, warn: 1, error: 1 },
  last_log_at: "2026-06-19T01:00:00Z",
  last_error_at: "2026-06-19T00:00:00Z",
  last_error_source: "gateway",
  last_error_message: "upstream failed",
  error_evidence_recorder: {
    enabled: true,
    started: true,
    draining: false,
    degraded: false,
    queue_depth: 0,
    queue_capacity: 256,
    enqueued_count: 20,
    processed_count: 20,
    recorded_count: 20,
    dropped_count: 0,
    write_failed_count: 0,
  },
  checked_at: "2026-06-19T01:00:00Z",
} satisfies OpsSystemLogHealth;

describe("opsEvidenceChainStatus", () => {
  it("reports healthy when store and recorder are both healthy", () => {
    expect(opsEvidenceChainStatus(healthy)).toEqual({ status: "healthy", tone: "active" });
  });

  it("reports degraded when evidence writes are dropping", () => {
    expect(
      opsEvidenceChainStatus({
        ...healthy,
        error_evidence_recorder: {
          ...healthy.error_evidence_recorder,
          degraded: true,
          dropped_count: 2,
        },
      }),
    ).toEqual({ status: "degraded", tone: "error" });
  });

  it("reports degraded when system logs are stale", () => {
    expect(opsEvidenceChainStatus({ ...healthy, stale: true })).toEqual({
      status: "degraded",
      tone: "error",
    });
  });
});

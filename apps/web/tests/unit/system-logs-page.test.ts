import { describe, expect, it } from "vitest";
import {
  formatSystemLogEvidenceValue,
  opsSystemLogEvidenceItems,
} from "@/app/admin/ops/system-logs/system-log-evidence";
import type { OpsSystemLog } from "@/lib/sdk-types";

describe("system logs page evidence summary", () => {
  it("keeps only low-sensitive evidence in the summary", () => {
    const log: OpsSystemLog = {
      id: "1",
      level: "error",
      message: "gateway failed",
      source: "gateway.auth",
      request_id: "req-123",
      trace_id: "trace-456",
      metadata: {
        api_key_prefix: "sk_111111111111",
        attempted_key_prefix: "sk_aaaaaaaaaaaa",
        deleted_key_id: 24,
        deleted_key_name: "deleted-gateway",
        usage_log_id: 91,
        scheduler_decision_id: 33,
        attempt_no: 2,
        provider_id: 9,
        account_id: 42,
        model: "gpt-4o-mini",
        upstream_model: "upstream-live",
        source_endpoint: "/v1/chat/completions",
        target_protocol: "anthropic-compatible",
        status_code: 502,
        error_class: "server_bad",
        error_phase: "upstream",
        error_owner: "provider",
        reason: "deleted_key",
        prompt: "secret prompt",
        headers: { Authorization: "Bearer secret" },
      },
      created_at: "2026-06-18T10:00:00Z",
    };

    const items = opsSystemLogEvidenceItems(log);
    expect(items).toEqual([
      { key: "req", value: "req-123" },
      { key: "trace", value: "trace-456" },
      { key: "api_key_prefix", value: "sk_111111111111" },
      { key: "attempted_key_prefix", value: "sk_aaaaaaaaaaaa" },
      { key: "deleted_key_id", value: "24" },
      { key: "deleted_key_name", value: "deleted-gateway" },
      { key: "usage_log_id", value: "91" },
      { key: "scheduler_decision_id", value: "33" },
    ]);
  });

  it("prioritizes effect and stage for gateway effect failures", () => {
    const log: OpsSystemLog = {
      id: "2",
      level: "error",
      message: "failed to refresh gateway account snapshot",
      source: "gateway.usage",
      request_id: "req-ops",
      trace_id: "",
      metadata: {
        effect: "account_snapshot_refresh",
        stage: "usage_list",
        event_type: "GatewayAccountSnapshotRefreshRequested",
        account_id: 42,
        provider_id: 9,
        attempt_no: 3,
        error_class: "account_snapshot_refresh_failed",
      },
      created_at: "2026-06-18T10:00:00Z",
    };

    expect(opsSystemLogEvidenceItems(log)).toEqual([
      { key: "req", value: "req-ops" },
      { key: "effect", value: "account_snapshot_refresh" },
      { key: "stage", value: "usage_list" },
      { key: "event_type", value: "GatewayAccountSnapshotRefreshRequested" },
      { key: "attempt_no", value: "3" },
      { key: "provider_id", value: "9" },
      { key: "account_id", value: "42" },
      { key: "error_class", value: "account_snapshot_refresh_failed" },
    ]);
  });

  it("formats primitive evidence values and ignores unsupported ones", () => {
    expect(formatSystemLogEvidenceValue("  trimmed  ")).toBe("trimmed");
    expect(formatSystemLogEvidenceValue(12)).toBe("12");
    expect(formatSystemLogEvidenceValue(true)).toBe("true");
    expect(formatSystemLogEvidenceValue({})).toBe("");
  });
});

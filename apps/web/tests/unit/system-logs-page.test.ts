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
      { key: "provider_id", value: "9" },
      { key: "account_id", value: "42" },
    ]);
  });

  it("formats primitive evidence values and ignores unsupported ones", () => {
    expect(formatSystemLogEvidenceValue("  trimmed  ")).toBe("trimmed");
    expect(formatSystemLogEvidenceValue(12)).toBe("12");
    expect(formatSystemLogEvidenceValue(true)).toBe("true");
    expect(formatSystemLogEvidenceValue({})).toBe("");
  });
});

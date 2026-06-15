import { describe, expect, it } from "vitest";
import { gatewayErrorHintKey } from "@/lib/gateway-error-hint";

describe("gatewayErrorHintKey", () => {
  it("returns null for empty/blank/unknown input", () => {
    expect(gatewayErrorHintKey(null)).toBeNull();
    expect(gatewayErrorHintKey(undefined)).toBeNull();
    expect(gatewayErrorHintKey("")).toBeNull();
    expect(gatewayErrorHintKey("something totally unrelated")).toBeNull();
  });

  it("prefers the specific embedded reason over the generic no-account fallback", () => {
    // The 503 body embeds both 'no available account' and the real reason.
    expect(
      gatewayErrorHintKey(
        "no available account: 3 candidate(s) rejected [capability_mismatch(2), cooldown_active(1)]",
      ),
    ).toBe("capabilityMismatch");
  });

  it("falls back to noAvailableAccount when only the generic reason is present", () => {
    expect(gatewayErrorHintKey("no available account")).toBe("noAvailableAccount");
    expect(
      gatewayErrorHintKey("no available account: 1 candidate(s) rejected [group_excluded(1)]"),
    ).toBe("noAvailableAccount");
  });

  it("maps individual reject-reason codes to their hint keys", () => {
    const cases: Record<string, string> = {
      credential_invalid: "credentialInvalid",
      needs_reauth: "needsReauth",
      auth_error: "needsReauth",
      session_invalid: "needsReauth",
      content_safety_blocked: "contentSafety",
      hard_sticky_missing: "stickySession",
      hard_sticky_mismatch: "stickySession",
      sticky_account_not_found: "stickySession",
      cooldown_active: "cooldown",
      circuit_open: "cooldown",
      quota_exhausted: "quotaExhausted",
      cost_window_exceeded: "costLimited",
      daily_cost_limit_exceeded: "costLimited",
      provider_disabled: "disabled",
      account_disabled: "disabled",
      model_not_supported: "modelNotSupported",
      model_not_found: "modelNotFound",
      model_not_allowed: "modelNotAllowed",
      insufficient_balance: "insufficientBalance",
      ip_not_allowed: "ipNotAllowed",
      risk_control_blocked: "riskBlocked",
      concurrency_limit_exceeded: "rateLimited",
    };
    for (const [message, key] of Object.entries(cases)) {
      expect(gatewayErrorHintKey(message)).toBe(key);
    }
  });

  it("is case-insensitive", () => {
    expect(gatewayErrorHintKey("CAPABILITY_MISMATCH")).toBe("capabilityMismatch");
  });
});

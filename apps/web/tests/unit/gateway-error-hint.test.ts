import { describe, expect, it } from "vitest";
import { gatewayErrorHintKey, extractMissingCapabilityKeys } from "@/lib/gateway-error-hint";

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

// extractMissingCapabilityKeys: backend-side remediation (commit e7345d0b)
// started emitting `capability_mismatch:<key>` to identify WHICH capability
// the scheduler couldn't satisfy. Operators previously saw a generic
// "20 candidate(s) rejected [capability_mismatch(20)]" and had to dig into
// the scheduler decisions UI to discover the missing key. This helper makes
// the gateway-error-hint rendering site able to surface the key inline.
describe("extractMissingCapabilityKeys", () => {
  it("returns [] for nullish / non-matching input", () => {
    expect(extractMissingCapabilityKeys(null)).toEqual([]);
    expect(extractMissingCapabilityKeys(undefined)).toEqual([]);
    expect(extractMissingCapabilityKeys("")).toEqual([]);
    expect(extractMissingCapabilityKeys("no available account")).toEqual([]);
    // Bare capability_mismatch with no colon-suffix yields no specific key.
    expect(
      extractMissingCapabilityKeys(
        "no available account: 5 candidate(s) rejected [capability_mismatch(5)]",
      ),
    ).toEqual([]);
  });

  it("extracts the missing key from a colon-suffix reject reason", () => {
    expect(
      extractMissingCapabilityKeys(
        "no available account: 20 candidate(s) rejected [capability_mismatch:responses(20)]",
      ),
    ).toEqual(["responses"]);
  });

  it("dedupes repeated keys while preserving first-seen order", () => {
    expect(
      extractMissingCapabilityKeys(
        "[capability_mismatch:responses(5), capability_mismatch:embeddings(3), capability_mismatch:responses(2)]",
      ),
    ).toEqual(["responses", "embeddings"]);
  });

  it("is case-insensitive on the prefix but lowercases the returned key", () => {
    expect(
      extractMissingCapabilityKeys(
        "[CAPABILITY_MISMATCH:Responses(1), capability_mismatch:VISION_INPUT(1)]",
      ),
    ).toEqual(["responses", "vision_input"]);
  });

  it("only matches the canonical [a-z][a-z0-9_]* key shape after the colon", () => {
    // Future audit-tag suffixes (e.g. capability_mismatch:: or with
    // free-text) must not be silently swallowed as keys.
    expect(
      extractMissingCapabilityKeys("[capability_mismatch:(1)]"),
    ).toEqual([]);
    // Leading underscore is rejected (not a canonical key).
    expect(
      extractMissingCapabilityKeys("[capability_mismatch:_secret(1)]"),
    ).toEqual([]);
  });
});

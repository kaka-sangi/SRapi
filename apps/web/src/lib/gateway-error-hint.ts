/**
 * Maps a gateway/scheduler failure message to a plain-language hint key.
 *
 * The gateway surfaces terse machine codes to callers — e.g.
 *   `no available account: 3 candidate(s) rejected [capability_mismatch(2), cooldown_active(1)]`
 * which is precise but opaque to a non-expert operator. This helper scans such a
 * message for known reject-reason codes and returns the i18n key of a short
 * "what to do" hint (under the `gatewayHints` namespace), or null when nothing
 * recognizable matches.
 *
 * Rules are ordered most-specific first: a `no available account` message often
 * embeds the *real* per-candidate reason (capability_mismatch, cooldown, …), so
 * those are matched before the generic no-account fallback.
 *
 * Pure and dependency-free so it can be unit-tested in isolation; the component
 * resolves the returned key through its translator.
 */
const RULES: ReadonlyArray<readonly [RegExp, string]> = [
  [/capability_mismatch/i, "capabilityMismatch"],
  [/credential_invalid|credential_error|credential unavailable|decrypt/i, "credentialInvalid"],
  [/needs_reauth|auth_failed|auth_error|session_invalid|re-?auth/i, "needsReauth"],
  [/content_safety_blocked/i, "contentSafety"],
  [/hard_sticky_|sticky_account_not_found|sticky_broken/i, "stickySession"],
  [/cooldown_active|circuit_open|circuit breaker/i, "cooldown"],
  [/cost_window_exceeded|daily_cost_limit_exceeded|monthly_cost_quota_exceeded|cost_limit_exceeded/i, "costLimited"],
  [/quota_exhausted|quota_protected|quota_exceeded/i, "quotaExhausted"],
  [/provider_disabled|account_disabled/i, "disabled"],
  [/model_not_supported/i, "modelNotSupported"],
  [/model_not_found/i, "modelNotFound"],
  [/model_not_allowed/i, "modelNotAllowed"],
  [/insufficient_balance/i, "insufficientBalance"],
  [/ip_not_allowed/i, "ipNotAllowed"],
  [/risk_control_blocked/i, "riskBlocked"],
  [/rate.?limit|concurrency_(?:limit|full)|\brpm\b|\btpm\b|too many requests|\b429\b/i, "rateLimited"],
  [/group_excluded|no_available_account|no available account/i, "noAvailableAccount"],
];

/**
 * Every hint key the helper can emit (deduped, derived from the rules above).
 * A test asserts each one has en + zh text under the `gatewayHints` namespace,
 * so a new rule can never ship a key without a translation.
 */
export const GATEWAY_HINT_KEYS: readonly string[] = [...new Set(RULES.map(([, key]) => key))];

/**
 * Returns the unqualified hint key (e.g. "capabilityMismatch") for a failure
 * message, or null when nothing matches. Callers resolve it as
 * `t(\`gatewayHints.\${key}\`)`.
 */
export function gatewayErrorHintKey(message: string | null | undefined): string | null {
  if (!message) return null;
  for (const [pattern, key] of RULES) {
    if (pattern.test(message)) return key;
  }
  return null;
}

/**
 * Pulls the specific missing capability key out of a `capability_mismatch:<key>`
 * reject reason (the format the backend started emitting at commit e7345d0b so
 * operators can tell whether it was "responses", "embeddings", "vision_input"
 * etc.). Multiple occurrences (e.g. several candidates failing on different
 * keys) are returned as a deduped array preserving first-seen order. Returns
 * an empty array when the message uses the bare "capability_mismatch" or none
 * is present.
 *
 * Pure regex match — no allocation when nothing is present (the early bail
 * makes this safe to call on every error message).
 */
export function extractMissingCapabilityKeys(
  message: string | null | undefined,
): string[] {
  if (!message || !/capability_mismatch:/.test(message)) return [];
  // The backend renders `capability_mismatch:<key>` where <key> is a
  // canonical capability identifier (lowercase + underscore only — see
  // capabilitiescontract.Key* constants). Match conservatively so we don't
  // pick up unrelated colon-suffixes that may share the prefix in future
  // log lines.
  const pattern = /capability_mismatch:([a-z][a-z0-9_]*)/gi;
  const seen = new Set<string>();
  const out: string[] = [];
  let match: RegExpExecArray | null;
  while ((match = pattern.exec(message)) !== null) {
    const key = match[1].toLowerCase();
    if (!seen.has(key)) {
      seen.add(key);
      out.push(key);
    }
  }
  return out;
}

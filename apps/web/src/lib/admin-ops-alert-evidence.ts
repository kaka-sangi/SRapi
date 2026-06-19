import {
  adminAccountsHealthHref,
  adminErrorInvestigationHref,
  adminRequestEvidenceHref,
  adminSchedulerDecisionsHref,
} from "@/lib/admin-log-links";
import type { JsonObject } from "@/lib/sdk-types";

export interface OpsAlertEvidenceLinks {
  errorLogs: string | null;
  requestEvidence: string | null;
  schedulerDecision: string | null;
  accountHealth: string | null;
}

export type OpsAlertRunbookStep =
  | "openErrorLogs"
  | "inspectRequestEvidence"
  | "inspectSchedulerDecision"
  | "checkAccountHealth"
  | "checkQuotaOrRateLimit"
  | "checkCredentials"
  | "checkProviderNetwork"
  | "checkPolicyOrAccountState"
  | "reviewSloBurnRate"
  | "reviewAlertRule"
  | "validateRoutingScope";

const QUOTA_ERROR_CLASSES = new Set(["quota_exhausted", "quota_exceeded", "rate_limit"]);
const CREDENTIAL_ERROR_CLASSES = new Set([
  "auth_failed",
  "permission_denied",
  "invalid_api_key",
  "api_key_disabled",
  "credential_invalid",
  "needs_reauth",
  "session_invalid",
  "account_locked",
  "account_banned",
]);
const PROVIDER_NETWORK_ERROR_CLASSES = new Set([
  "timeout",
  "network_error",
  "invalid_response",
  "upstream_error",
  "provider_5xx",
  "overloaded",
  "stream_interrupted",
  "empty_completion",
]);
const POLICY_ERROR_CLASSES = new Set(["policy_error", "content_policy", "abuse_detected"]);
const SCHEDULER_ERROR_CLASSES = new Set([
  "no_available_account",
  "lease_failed",
  "concurrency_full",
  "cooldown_active",
  "capability_mismatch",
]);

export function buildOpsAlertEvidenceLinks(details?: JsonObject): OpsAlertEvidenceLinks {
  const requestID = detailString(details, "request_id", "requestId");
  const accountID = detailString(details, "account_id", "accountId");
  const providerID = detailString(details, "provider_id", "providerId");
  const errorClass = detailString(details, "error_class", "errorClass");
  const sourceEndpoint = detailString(details, "source_endpoint", "sourceEndpoint");
  const model = detailString(details, "model", "canonical_model", "model_alias");
  const start = detailString(details, "window_start", "windowStart");
  const end = detailString(details, "window_end", "windowEnd");

  return {
    errorLogs: adminErrorInvestigationHref({
      account_id: accountID,
      provider_id: providerID,
      error_class: errorClass,
      source_endpoint: sourceEndpoint,
      model,
    }),
    requestEvidence: adminRequestEvidenceHref({
      request_id: requestID,
      account_id: accountID,
      provider_id: providerID,
      error_class: errorClass,
      source_endpoint: sourceEndpoint,
      model,
      start,
      end,
    }),
    schedulerDecision: adminSchedulerDecisionsHref({
      request_id: requestID,
      account_id: accountID,
      provider_id: providerID,
      model,
      source_endpoint: sourceEndpoint,
      start,
      end,
    }),
    accountHealth:
      accountID || providerID
        ? adminAccountsHealthHref({ account_id: accountID, provider_id: providerID })
        : null,
  };
}

export function buildOpsAlertRunbookSteps(details?: JsonObject): OpsAlertRunbookStep[] {
  const requestID = detailString(details, "request_id", "requestId");
  const accountID = detailString(details, "account_id", "accountId");
  const providerID = detailString(details, "provider_id", "providerId");
  const sourceEndpoint = detailString(details, "source_endpoint", "sourceEndpoint");
  const model = detailString(details, "model", "canonical_model", "model_alias");
  const errorClass = canonicalAlertErrorClass(detailString(details, "error_class", "errorClass"));
  const ruleID = detailString(details, "rule_id", "ruleId");
  const sloID = detailString(details, "slo_id", "sloId");
  const metricType = detailString(details, "metric_type", "metricType");
  const hasBurnRateSignal = Boolean(
    detailString(details, "burn_rate_threshold", "long_burn_rate", "short_burn_rate"),
  );
  const hasRuleSignal = Boolean(
    ruleID || metricType || detailString(details, "rule_name", "ruleName", "observed_value"),
  );
  const hasRequestEvidenceScope = Boolean(
    requestID || accountID || providerID || sourceEndpoint || model || errorClass,
  );
  const hasSchedulerEvidenceScope = Boolean(
    requestID ||
      accountID ||
      providerID ||
      sourceEndpoint ||
      model ||
      (errorClass && SCHEDULER_ERROR_CLASSES.has(errorClass)),
  );

  const steps: OpsAlertRunbookStep[] = [];
  const add = (step: OpsAlertRunbookStep) => {
    if (!steps.includes(step)) steps.push(step);
  };

  if (errorClass || accountID || providerID || sourceEndpoint || model) add("openErrorLogs");
  if (hasRequestEvidenceScope) add("inspectRequestEvidence");
  if (hasSchedulerEvidenceScope) add("inspectSchedulerDecision");
  if (accountID || providerID) add("checkAccountHealth");
  if (errorClass && QUOTA_ERROR_CLASSES.has(errorClass)) add("checkQuotaOrRateLimit");
  if (errorClass && CREDENTIAL_ERROR_CLASSES.has(errorClass)) add("checkCredentials");
  if (errorClass && PROVIDER_NETWORK_ERROR_CLASSES.has(errorClass)) add("checkProviderNetwork");
  if (errorClass && POLICY_ERROR_CLASSES.has(errorClass)) add("checkPolicyOrAccountState");
  if (sloID || ruleID?.startsWith("slo.") || hasBurnRateSignal) add("reviewSloBurnRate");
  if (hasRuleSignal) add("reviewAlertRule");
  if (sourceEndpoint || model || providerID || (errorClass && SCHEDULER_ERROR_CLASSES.has(errorClass))) {
    add("validateRoutingScope");
  }
  if (steps.length === 0) add("reviewAlertRule");

  return steps;
}

function canonicalAlertErrorClass(value: string | null): string | null {
  const errorClass = value?.trim().toLowerCase();
  if (!errorClass) return null;

  switch (errorClass) {
    case "rate_limited":
    case "provider_rate_limited":
    case "rate_limit_error":
    case "rate_limit_exceeded":
    case "too_many_requests":
    case "rpm_limit_exceeded":
    case "tpm_limit_exceeded":
      return "rate_limit";
    case "auth_error":
    case "authentication_error":
    case "credential_error":
      return "auth_failed";
    case "stream_idle_timeout":
    case "stream_timeout":
    case "ws_idle_timeout":
    case "acquire_timeout":
    case "request_timeout":
    case "timeout_error":
      return "timeout";
    case "permission_error":
    case "forbidden":
      return "permission_denied";
    case "transport_error":
      return "network_error";
    case "bad_gateway":
    case "server_error":
    case "upstream_5xx":
      return "provider_5xx";
    default:
      return errorClass;
  }
}

function detailString(details: JsonObject | undefined, ...keys: string[]): string | null {
  for (const key of keys) {
    const value = details?.[key];
    if (typeof value === "string" && value.trim()) return value.trim();
    if (typeof value === "number" && Number.isFinite(value)) return String(value);
  }
  return null;
}

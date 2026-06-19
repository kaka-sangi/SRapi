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
  | "reviewSloBurnRate"
  | "reviewAlertRule"
  | "validateRoutingScope";

const QUOTA_ERROR_CLASSES = new Set(["quota_exceeded", "rate_limited", "provider_rate_limited"]);
const CREDENTIAL_ERROR_CLASSES = new Set([
  "auth_error",
  "authentication_error",
  "credential_invalid",
  "session_invalid",
  "account_locked",
  "account_banned",
]);
const PROVIDER_NETWORK_ERROR_CLASSES = new Set([
  "timeout",
  "network_error",
  "invalid_response",
  "upstream_error",
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
  const errorClass = detailString(details, "error_class", "errorClass")?.toLowerCase();
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
    requestID || accountID || providerID || sourceEndpoint || model,
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
  if (sloID || ruleID?.startsWith("slo.") || hasBurnRateSignal) add("reviewSloBurnRate");
  if (hasRuleSignal) add("reviewAlertRule");
  if (sourceEndpoint || model || providerID) add("validateRoutingScope");
  if (steps.length === 0) add("reviewAlertRule");

  return steps;
}

function detailString(details: JsonObject | undefined, ...keys: string[]): string | null {
  for (const key of keys) {
    const value = details?.[key];
    if (typeof value === "string" && value.trim()) return value.trim();
    if (typeof value === "number" && Number.isFinite(value)) return String(value);
  }
  return null;
}

import {
  adminAccountsHealthHref,
  adminRequestDumpsHref,
  adminRequestEvidenceHref,
  adminSchedulerDecisionsHref,
  adminSystemLogsHref,
} from "@/lib/admin-log-links";
import type { OpsAlertRunbookStep } from "@/lib/admin-ops-alert-evidence";
import type { OpsErrorLog } from "@/lib/sdk-types";

const ERROR_EVIDENCE_WINDOW_MS = 5 * 60 * 1000;

export type ErrorLogTriageLinkKind =
  | "systemLogs"
  | "requestEvidence"
  | "requestDumps"
  | "schedulerDecision"
  | "accountHealth";

interface ErrorLogTriageLink {
  kind: ErrorLogTriageLinkKind;
  href: string;
}

export interface ErrorLogTriage {
  steps: OpsAlertRunbookStep[];
  links: ErrorLogTriageLink[];
}

const ROUTING_ERROR_CLASSES = new Set([
  "no_available_account",
  "scheduler_rejected",
  "lease_failed",
  "concurrency_full",
]);

const QUOTA_ERROR_CLASSES = new Set([
  "quota_exceeded",
  "quota_exhausted",
  "rate_limited",
  "rate_limit",
  "provider_rate_limited",
]);

const CREDENTIAL_ERROR_CLASSES = new Set([
  "auth_error",
  "auth_failed",
  "authentication_error",
  "credential_error",
  "credential_invalid",
  "session_invalid",
  "account_locked",
  "account_banned",
  "challenge_required",
  "device_unrecognized",
]);

const PROVIDER_NETWORK_ERROR_CLASSES = new Set([
  "timeout",
  "network_error",
  "invalid_response",
  "upstream_error",
  "provider_5xx",
  "server_bad",
  "stream_interrupted",
]);

/** Build operator-facing triage steps and evidence links for one failed request. */
export function buildErrorLogTriage(detail: OpsErrorLog): ErrorLogTriage {
  const requestID = clean(detail.request_id);
  const accountID = clean(detail.account_id);
  const providerID = clean(detail.provider_id);
  const sourceEndpoint = clean(detail.source_endpoint);
  const model = clean(detail.model);
  const errorClass = clean(detail.error_class).toLowerCase();
  const phase = clean(detail.error_phase).toLowerCase();
  const owner = clean(detail.error_owner).toLowerCase();
  const statusCode = detail.status_code;
  const routingIssue = isRoutingIssue(errorClass, phase, owner);
  const quotaIssue = isQuotaIssue(errorClass, statusCode);
  const credentialIssue = isCredentialIssue(errorClass, statusCode, owner);
  const steps: OpsAlertRunbookStep[] = [];

  const addStep = (step: OpsAlertRunbookStep) => {
    if (!steps.includes(step)) steps.push(step);
  };

  if (requestID) addStep("inspectRequestEvidence");
  if (routingIssue) addStep("inspectSchedulerDecision");
  if (accountID || providerID) addStep("checkAccountHealth");
  if (quotaIssue) addStep("checkQuotaOrRateLimit");
  if (credentialIssue) addStep("checkCredentials");
  if (isProviderNetworkIssue(errorClass, phase, owner, statusCode, routingIssue, quotaIssue, credentialIssue)) {
    addStep("checkProviderNetwork");
  }
  if (sourceEndpoint || model || providerID) addStep("validateRoutingScope");
  if (steps.length === 0 && (errorClass || statusCode != null)) addStep("inspectRequestEvidence");

  return {
    steps,
    links: buildLinks(detail, routingIssue),
  };
}

function buildLinks(detail: OpsErrorLog, routingIssue: boolean): ErrorLogTriageLink[] {
  const requestID = clean(detail.request_id);
  const traceID = clean(detail.trace_id);
  const accountID = clean(detail.account_id);
  const providerID = clean(detail.provider_id);
  const errorClass = clean(detail.error_class);
  const sourceEndpoint = clean(detail.source_endpoint);
  const model = clean(detail.model);
  const window = errorEvidenceWindow(detail.occurred_at);
  const links: ErrorLogTriageLink[] = [];
  const addLink = (kind: ErrorLogTriageLinkKind, href: string | null) => {
    if (href && !links.some((link) => link.href === href)) links.push({ kind, href });
  };

  addLink("systemLogs", adminSystemLogsHref(requestID ? { request_id: requestID } : { trace_id: traceID }));
  addLink(
    "requestEvidence",
    adminRequestEvidenceHref({
      request_id: requestID,
      account_id: accountID,
      provider_id: providerID,
      error_class: errorClass,
      source_endpoint: sourceEndpoint,
      model,
      start: window?.start,
      end: window?.end,
    }),
  );
  if (routingIssue || requestID) {
    addLink(
      "schedulerDecision",
      adminSchedulerDecisionsHref({
        request_id: requestID,
        account_id: accountID,
        provider_id: providerID,
        model,
        source_endpoint: sourceEndpoint,
        start: window?.start,
        end: window?.end,
      }),
    );
  }
  if (accountID || providerID) {
    addLink("accountHealth", adminAccountsHealthHref({ account_id: accountID, provider_id: providerID }));
  }
  addLink("requestDumps", adminRequestDumpsHref({ request_id: requestID }));

  return links;
}

function errorEvidenceWindow(value?: string | null): { start: string; end: string } | null {
  const raw = clean(value);
  if (!raw) return null;
  const occurredAt = new Date(raw);
  if (Number.isNaN(occurredAt.getTime())) return null;
  return {
    start: new Date(occurredAt.getTime() - ERROR_EVIDENCE_WINDOW_MS).toISOString(),
    end: new Date(occurredAt.getTime() + ERROR_EVIDENCE_WINDOW_MS).toISOString(),
  };
}

function isRoutingIssue(errorClass: string, phase: string, owner: string): boolean {
  return phase === "routing" || owner === "scheduler" || ROUTING_ERROR_CLASSES.has(errorClass);
}

function isQuotaIssue(errorClass: string, statusCode?: number): boolean {
  return QUOTA_ERROR_CLASSES.has(errorClass) || statusCode === 429;
}

function isCredentialIssue(errorClass: string, statusCode: number | undefined, owner: string): boolean {
  if (CREDENTIAL_ERROR_CLASSES.has(errorClass)) return true;
  return (statusCode === 401 || statusCode === 403) && owner !== "client";
}

function isProviderNetworkIssue(
  errorClass: string,
  phase: string,
  owner: string,
  statusCode: number | undefined,
  routingIssue: boolean,
  quotaIssue: boolean,
  credentialIssue: boolean,
): boolean {
  if (routingIssue || quotaIssue || credentialIssue) return false;
  if (PROVIDER_NETWORK_ERROR_CLASSES.has(errorClass)) return true;
  if (phase === "upstream" || phase === "network") return true;
  if (owner === "provider") return true;
  return statusCode === 408 || statusCode === 504;
}

function clean(value?: string | number | null): string {
  if (value === null || value === undefined) return "";
  return String(value).trim();
}

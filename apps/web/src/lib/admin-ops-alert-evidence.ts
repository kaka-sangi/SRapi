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

export function buildOpsAlertEvidenceLinks(details?: JsonObject): OpsAlertEvidenceLinks {
  const requestID = detailString(details, "request_id", "requestId");
  const accountID = detailString(details, "account_id", "accountId");
  const providerID = detailString(details, "provider_id", "providerId");

  return {
    errorLogs: adminErrorInvestigationHref({
      account_id: accountID,
      provider_id: providerID,
      error_class: detailString(details, "error_class", "errorClass"),
      source_endpoint: detailString(details, "source_endpoint", "sourceEndpoint"),
      model: detailString(details, "model", "canonical_model", "model_alias"),
    }),
    requestEvidence: adminRequestEvidenceHref({ request_id: requestID }),
    schedulerDecision: adminSchedulerDecisionsHref({ request_id: requestID }),
    accountHealth:
      accountID || providerID
        ? adminAccountsHealthHref({ account_id: accountID, provider_id: providerID })
        : null,
  };
}

function detailString(details: JsonObject | undefined, ...keys: string[]): string | null {
  for (const key of keys) {
    const value = details?.[key];
    if (typeof value === "string" && value.trim()) return value.trim();
    if (typeof value === "number" && Number.isFinite(value)) return String(value);
  }
  return null;
}

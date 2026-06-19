import {
  adminErrorInvestigationHref,
  adminRequestEvidenceHref,
  adminSchedulerDecisionsHref,
} from "@/lib/admin-log-links";
import type { AccountHealthSnapshot } from "@/lib/sdk-types";

export interface AccountHealthInvestigationLinks {
  errorLogs: string | null;
  requestEvidence: string | null;
  schedulerDecisions: string | null;
}

/** Return whether an account-health snapshot is abnormal enough to warrant error-log investigation. */
export function accountHealthNeedsInvestigation(health?: AccountHealthSnapshot | null): boolean {
  if (!health) return false;
  return Boolean(
    health.error_class ||
      health.circuit_state !== "closed" ||
      health.success_rate < 0.95 ||
      health.error_rate > 0 ||
      health.timeout_count > 0 ||
      health.rate_limit_count > 0,
  );
}

/** Build the cross-plane evidence links for an abnormal account-health snapshot. */
export function adminAccountHealthInvestigationLinks(
  health?: AccountHealthSnapshot | null,
): AccountHealthInvestigationLinks | null {
  if (!health || !accountHealthNeedsInvestigation(health)) return null;
  const common = {
    account_id: health.account_id,
    provider_id: health.provider_id,
  };
  return {
    errorLogs: adminErrorInvestigationHref({
      ...common,
      error_class: health.error_class,
    }),
    requestEvidence: adminRequestEvidenceHref({
      ...common,
      error_class: health.error_class,
    }),
    schedulerDecisions: adminSchedulerDecisionsHref({ account_id: health.account_id }),
  };
}

/** Build a precise Error logs link for an abnormal account-health snapshot. */
export function adminAccountHealthInvestigationHref(
  health?: AccountHealthSnapshot | null,
): string | null {
  return adminAccountHealthInvestigationLinks(health)?.errorLogs ?? null;
}

import { adminErrorInvestigationHref } from "@/lib/admin-log-links";
import type { AccountHealthSnapshot } from "@/lib/sdk-types";

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

/** Build a precise Error logs link for an abnormal account-health snapshot. */
export function adminAccountHealthInvestigationHref(
  health?: AccountHealthSnapshot | null,
): string | null {
  if (!health || !accountHealthNeedsInvestigation(health)) return null;
  return adminErrorInvestigationHref({
    account_id: health.account_id,
    provider_id: health.provider_id,
    error_class: health.error_class,
  });
}

import {
  adminErrorInvestigationHref,
  adminRequestEvidenceHref,
} from "@/lib/admin-log-links";
import type { AccountHealthSnapshot } from "@/lib/sdk-types";
import {
  accountHealthNeedsInvestigation,
  adminAccountHealthInvestigationHref,
} from "@/lib/admin-account-health-investigation";

export type AccountHealthOpsGroupKey = "circuit" | "quota" | "rate_limit" | "timeout" | "degraded";
export type AccountHealthMaintenanceAction = "recover" | "clear_error" | "refresh_quota";

export interface AccountHealthOpsGroup {
  key: AccountHealthOpsGroupKey;
  accountIds: string[];
  count: number;
  errorClass: string | null;
  investigationHref: string | null;
  requestEvidenceHref: string | null;
}

export interface AccountHealthOpsSummary {
  total: number;
  healthy: number;
  attention: number;
  groups: AccountHealthOpsGroup[];
}

const GROUP_ORDER: AccountHealthOpsGroupKey[] = [
  "circuit",
  "quota",
  "rate_limit",
  "timeout",
  "degraded",
];

/** Group account-health snapshots into operator-actionable buckets. */
export function buildAccountHealthOpsSummary(
  snapshots: AccountHealthSnapshot[],
): AccountHealthOpsSummary {
  const buckets = new Map<AccountHealthOpsGroupKey, AccountHealthSnapshot[]>();
  for (const snapshot of snapshots) {
    if (!accountHealthNeedsInvestigation(snapshot)) continue;
    const key = primaryHealthIssue(snapshot);
    const bucket = buckets.get(key) ?? [];
    bucket.push(snapshot);
    buckets.set(key, bucket);
  }

  const groups = GROUP_ORDER.flatMap((key) => {
    const members = buckets.get(key) ?? [];
    if (members.length === 0) return [];
    const errorClass = dominantErrorClass(members);
    return [{
      key,
      accountIds: members.map((h) => h.account_id),
      count: members.length,
      errorClass,
      investigationHref: errorClass
        ? adminErrorInvestigationHref({ error_class: errorClass })
        : adminAccountHealthInvestigationHref(members[0]),
      requestEvidenceHref: errorClass
        ? adminRequestEvidenceHref({ error_class: errorClass })
        : null,
    }];
  });

  const attention = groups.reduce((sum, group) => sum + group.count, 0);
  return {
    total: snapshots.length,
    healthy: snapshots.length - attention,
    attention,
    groups,
  };
}

/** Return the maintenance actions that best match a grouped health issue. */
export function accountHealthGroupMaintenanceActions(
  group: AccountHealthOpsGroup,
): AccountHealthMaintenanceAction[] {
  switch (group.key) {
    case "circuit":
      return ["recover", "clear_error"];
    case "quota":
      return ["refresh_quota", "clear_error"];
    case "rate_limit":
      return ["clear_error"];
    case "timeout":
      return ["recover", "clear_error"];
    case "degraded":
      return ["recover", "clear_error"];
  }
}

function primaryHealthIssue(snapshot: AccountHealthSnapshot): AccountHealthOpsGroupKey {
  if (snapshot.circuit_state !== "closed") return "circuit";
  if (snapshot.quota_exhausted) return "quota";
  if (snapshot.rate_limit_count > 0 || isRateLimitClass(snapshot.error_class)) return "rate_limit";
  if (snapshot.timeout_count > 0 || snapshot.error_class === "timeout") return "timeout";
  return "degraded";
}

function dominantErrorClass(snapshots: AccountHealthSnapshot[]): string | null {
  const counts = new Map<string, number>();
  for (const snapshot of snapshots) {
    const errorClass = snapshot.error_class?.trim();
    if (!errorClass) continue;
    counts.set(errorClass, (counts.get(errorClass) ?? 0) + 1);
  }
  let best: string | null = null;
  let bestCount = 0;
  for (const [errorClass, count] of counts) {
    if (count <= bestCount) continue;
    best = errorClass;
    bestCount = count;
  }
  return best;
}

function isRateLimitClass(errorClass?: string | null): boolean {
  const value = errorClass?.trim();
  return value === "rate_limited" || value === "rate_limit" || value === "quota_exceeded";
}

import type {
  CreateScheduledTestPlanRequest,
  ScheduledTestPlan,
} from "../../../../packages/sdk/typescript/src/types.gen";

export type ScheduledTestScope = ScheduledTestPlan["scope_type"];

export const SCHEDULED_TEST_SCOPES: ScheduledTestScope[] = ["all", "account", "group"];

export interface ScheduledTestFormState {
  name: string;
  scopeType: ScheduledTestScope;
  scopeId: string;
  intervalSeconds: string;
  cronExpression: string;
  maxResults: string;
  autoRecover: boolean;
  enabled: boolean;
}

export function emptyScheduledTestForm(): ScheduledTestFormState {
  return {
    name: "",
    scopeType: "all",
    scopeId: "",
    intervalSeconds: "3600",
    cronExpression: "",
    maxResults: "0",
    autoRecover: false,
    enabled: true,
  };
}

export function scheduledTestFormFromPlan(plan: ScheduledTestPlan): ScheduledTestFormState {
  return {
    name: plan.name,
    scopeType: plan.scope_type,
    scopeId: plan.scope_id != null ? String(plan.scope_id) : "",
    intervalSeconds: String(plan.interval_seconds ?? 3600),
    cronExpression: plan.cron_expression ?? "",
    maxResults: String(plan.max_results ?? 0),
    autoRecover: plan.auto_recover,
    enabled: plan.enabled,
  };
}

export function buildScheduledTestBody(
  form: ScheduledTestFormState,
): CreateScheduledTestPlanRequest {
  const name = form.name.trim();
  if (!name) {
    throw new Error("Name is required.");
  }
  const body: CreateScheduledTestPlanRequest = {
    name,
    enabled: form.enabled,
    scope_type: form.scopeType,
    interval_seconds: parsePositiveInt(form.intervalSeconds, 3600),
    cron_expression: form.cronExpression.trim(),
    max_results: parsePositiveInt(form.maxResults, 0),
    auto_recover: form.autoRecover,
  };
  if (form.scopeType === "all") {
    body.scope_id = null;
  } else {
    const scopeId = Number(form.scopeId.trim());
    if (!Number.isInteger(scopeId) || scopeId <= 0) {
      throw new Error("A valid account or group ID is required for this scope.");
    }
    body.scope_id = scopeId;
  }
  return body;
}

function parsePositiveInt(raw: string, fallback: number): number {
  const value = Number(raw.trim());
  if (!Number.isFinite(value) || value < 0) {
    return fallback;
  }
  return Math.trunc(value);
}

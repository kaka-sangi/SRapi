import type {
  CreateOpsSloRequest,
  OpsAlertSeverity,
  OpsBurnRateThreshold,
  OpsSliType,
  OpsSloDefinition,
  OpsSloStatus,
  UpdateOpsSloRequest,
} from "../../../../packages/sdk/typescript/src/types.gen";

type ErrorOwner = "client" | "business" | "scheduler" | "reverse_proxy" | "provider" | "internal";

export const OPS_SLI_TYPES: OpsSliType[] = ["availability", "latency", "freshness", "quality"];
export const OPS_SLO_STATUSES: OpsSloStatus[] = ["active", "disabled"];
export const OPS_ERROR_OWNERS: ErrorOwner[] = [
  "client",
  "business",
  "scheduler",
  "reverse_proxy",
  "provider",
  "internal",
];

export interface OpsSloFormState {
  name: string;
  sliType: OpsSliType;
  objective: string;
  windowDays: string;
  status: OpsSloStatus;
  sourceEndpoint: string;
  model: string;
  providerId: string;
  errorOwnerExclude: ErrorOwner[];
  policyName: string;
  thresholdsJson: string;
}

const defaultThresholds: OpsBurnRateThreshold[] = [
  {
    severity: "critical",
    short_window_seconds: 300,
    long_window_seconds: 3600,
    burn_rate: 14,
    min_request_count: 20,
  },
  {
    severity: "warning",
    short_window_seconds: 1800,
    long_window_seconds: 21600,
    burn_rate: 6,
    min_request_count: 50,
  },
  {
    severity: "ticket",
    short_window_seconds: 7200,
    long_window_seconds: 86400,
    burn_rate: 2,
    min_request_count: 100,
  },
];

export function emptyOpsSloForm(): OpsSloFormState {
  return {
    name: "",
    sliType: "availability",
    objective: "99.5",
    windowDays: "28",
    status: "active",
    sourceEndpoint: "/v1/chat/completions",
    model: "",
    providerId: "",
    errorOwnerExclude: ["client", "business"],
    policyName: "multi_window_burn_rate",
    thresholdsJson: JSON.stringify(defaultThresholds, null, 2),
  };
}

export function opsSloFormFromDefinition(definition: OpsSloDefinition): OpsSloFormState {
  return {
    name: definition.name,
    sliType: definition.sli_type,
    objective: String(definition.objective * 100),
    windowDays: String(definition.window_days),
    status: definition.status,
    sourceEndpoint: definition.filter.source_endpoint,
    model: definition.filter.model,
    providerId: definition.filter.provider_id ?? "",
    errorOwnerExclude: definition.filter.error_owner_exclude,
    policyName: definition.alert_policy.name,
    thresholdsJson: JSON.stringify(definition.alert_policy.thresholds, null, 2),
  };
}

export function buildCreateOpsSloBody(form: OpsSloFormState): CreateOpsSloRequest {
  return {
    name: form.name.trim(),
    sli_type: form.sliType,
    objective: parseObjectivePercent(form.objective),
    window_days: parsePositiveInteger(form.windowDays, "Window days"),
    status: form.status,
    filter: {
      source_endpoint: form.sourceEndpoint.trim(),
      model: form.model.trim(),
      ...(form.providerId.trim() ? { provider_id: form.providerId.trim() } : {}),
      error_owner_exclude: form.errorOwnerExclude,
    },
    alert_policy: {
      name: form.policyName.trim() || "multi_window_burn_rate",
      thresholds: parseThresholds(form.thresholdsJson),
    },
  };
}

export function buildUpdateOpsSloBody(form: OpsSloFormState): UpdateOpsSloRequest {
  const body = buildCreateOpsSloBody(form);
  return {
    name: body.name,
    objective: body.objective,
    window_days: body.window_days,
    status: body.status,
    filter: body.filter,
    alert_policy: body.alert_policy,
  };
}

export function toggleErrorOwner(values: ErrorOwner[], owner: ErrorOwner, checked: boolean): ErrorOwner[] {
  if (checked) {
    return values.includes(owner) ? values : [...values, owner];
  }
  return values.filter((value) => value !== owner);
}

function parseObjectivePercent(value: string): number {
  const parsed = Number(value);
  if (!Number.isFinite(parsed) || parsed <= 0 || parsed > 100) {
    throw new Error("Objective must be greater than 0 and no more than 100.");
  }
  return parsed;
}

function parsePositiveInteger(value: string, fieldName: string): number {
  const parsed = Number(value);
  if (!Number.isInteger(parsed) || parsed < 1) {
    throw new Error(`${fieldName} must be a positive integer.`);
  }
  return parsed;
}

function parseThresholds(value: string): OpsBurnRateThreshold[] {
  let parsed: unknown;
  try {
    parsed = JSON.parse(value || "[]") as unknown;
  } catch {
    throw new Error("Burn-rate thresholds must be valid JSON.");
  }
  if (!Array.isArray(parsed)) {
    throw new Error("Burn-rate thresholds must be a JSON array.");
  }
  return parsed.map((item, index) => parseThreshold(item, index));
}

function parseThreshold(item: unknown, index: number): OpsBurnRateThreshold {
  if (!item || Array.isArray(item) || typeof item !== "object") {
    throw new Error(`Burn-rate threshold ${index + 1} must be an object.`);
  }
  const threshold = item as Partial<OpsBurnRateThreshold>;
  if (!isSeverity(threshold.severity)) {
    throw new Error(`Burn-rate threshold ${index + 1} has an invalid severity.`);
  }
  const shortWindow = Number(threshold.short_window_seconds);
  const longWindow = Number(threshold.long_window_seconds);
  const burnRate = Number(threshold.burn_rate);
  const minRequestCount = Number(threshold.min_request_count);
  if (
    !Number.isInteger(shortWindow) ||
    !Number.isInteger(longWindow) ||
    shortWindow < 1 ||
    longWindow < 1 ||
    burnRate <= 0 ||
    !Number.isFinite(burnRate) ||
    !Number.isInteger(minRequestCount) ||
    minRequestCount < 0
  ) {
    throw new Error(`Burn-rate threshold ${index + 1} contains invalid numeric values.`);
  }
  return {
    severity: threshold.severity,
    short_window_seconds: shortWindow,
    long_window_seconds: longWindow,
    burn_rate: burnRate,
    min_request_count: minRequestCount,
  };
}

function isSeverity(value: unknown): value is OpsAlertSeverity {
  return value === "critical" || value === "warning" || value === "ticket";
}

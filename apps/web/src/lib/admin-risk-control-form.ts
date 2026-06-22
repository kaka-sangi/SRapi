import type {
  RiskControlConfig,
  RiskControlMode,
} from "../../../../packages/sdk/typescript/src/types.gen";

type RiskControlTab = "basic" | "limits" | "scope";

const RISK_CONTROL_TABS: Array<{ id: RiskControlTab; label: string }> = [
  { id: "basic", label: "Basic" },
  { id: "limits", label: "Limits" },
  { id: "scope", label: "Scope" },
];

export interface RiskControlFormState {
  enabled: boolean;
  mode: RiskControlMode;
  maxFailedRequestsPerMinute: string;
  maxCostPerDay: string;
  cooldownSeconds: string;
  blockedCountries: string[];
  blockedIps: string[];
}

interface RiskControlSaveConfirmationState {
  phrase: string;
  confirmation: string;
}

const RISK_CONTROL_SAVE_CONFIRMATION_PHRASE = "SAVE RISK CONTROL CONFIG";

export function createRiskControlForm(config: RiskControlConfig): RiskControlFormState {
  return {
    enabled: config.enabled,
    mode: config.mode,
    maxFailedRequestsPerMinute: String(config.max_failed_requests_per_minute),
    maxCostPerDay: config.max_cost_per_day,
    cooldownSeconds: String(config.cooldown_seconds),
    blockedCountries: normalizeCountries(config.blocked_countries ?? []),
    blockedIps: [...(config.blocked_ips ?? [])],
  };
}

export function buildRiskControlConfig(form: RiskControlFormState): RiskControlConfig {
  return {
    enabled: form.enabled,
    mode: form.mode,
    max_failed_requests_per_minute: parseNonNegativeInteger(
      form.maxFailedRequestsPerMinute,
      "Max failed requests per minute",
    ),
    max_cost_per_day: parseDecimalString(form.maxCostPerDay, "Max cost per day"),
    cooldown_seconds: parseNonNegativeInteger(form.cooldownSeconds, "Cooldown seconds"),
    blocked_countries: normalizeCountries(form.blockedCountries),
    blocked_ips: cleanList(form.blockedIps),
  };
}

function updateRiskControlForm(
  form: RiskControlFormState,
  updater: (form: RiskControlFormState) => RiskControlFormState,
): RiskControlFormState {
  return updater(form);
}

function createRiskControlSaveConfirmation(): RiskControlSaveConfirmationState {
  return {
    phrase: RISK_CONTROL_SAVE_CONFIRMATION_PHRASE,
    confirmation: "",
  };
}

function canConfirmRiskControlSave(
  state: RiskControlSaveConfirmationState | null,
): boolean {
  return Boolean(state && state.confirmation.trim() === state.phrase);
}

function cleanList(items: string[]): string[] {
  const out: string[] = [];
  for (const item of items) {
    const trimmed = item.trim();
    if (trimmed && !out.includes(trimmed)) out.push(trimmed);
  }
  return out;
}

function normalizeCountries(items: string[]): string[] {
  return cleanList(items.map((item) => item.toUpperCase()));
}

function parseNonNegativeInteger(value: string, fieldName: string): number {
  const parsed = Number(value);
  if (!Number.isInteger(parsed) || parsed < 0) {
    throw new Error(`${fieldName} must be a non-negative integer.`);
  }
  return parsed;
}

function parseDecimalString(value: string, fieldName: string): string {
  const normalized = value.trim();
  if (!/^[0-9]+(\.[0-9]+)?$/.test(normalized)) {
    throw new Error(`${fieldName} must be a decimal string.`);
  }
  return normalized;
}

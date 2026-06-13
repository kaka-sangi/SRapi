import type { OpsSettings } from "@/lib/sdk-types";

export interface OpsSettingsFormState {
  autoRefreshEnabled: boolean;
  refreshIntervalSeconds: string;
}

// There is no GET for ops settings, so the form opens on documented defaults
// rather than the live values; submitting replaces the whole object (PUT).
export function defaultOpsSettingsForm(): OpsSettingsFormState {
  return {
    autoRefreshEnabled: true,
    refreshIntervalSeconds: "30",
  };
}

export function buildOpsSettingsBody(form: OpsSettingsFormState): OpsSettings {
  return {
    auto_refresh_enabled: form.autoRefreshEnabled,
    refresh_interval_seconds: requireInt(form.refreshIntervalSeconds, "Refresh interval", 1),
  };
}

function requireInt(value: string, name: string, min: number): number {
  const n = Number(value.trim());
  if (!Number.isInteger(n) || n < min) {
    throw new Error(`${name} must be a whole number ≥ ${min}.`);
  }
  return n;
}
